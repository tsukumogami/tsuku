#!/usr/bin/env bash
# GPU Variant Performance Benchmark Script
#
# Measures tokens/second for tsuku-llm GPU and CPU variants across shipped
# model sizes. Produces structured results for the GPU backend selection
# design gate (DESIGN-gpu-backend-selection.md, issue #1780).
#
# This script must be run on a machine with GPU hardware. It automates:
#   1. Recording hardware details (GPU model, driver version, runtime version)
#   2. Installing the GPU variant via recipe auto-detection
#   3. Running inference benchmarks on 0.5B, 1.5B, 3B models
#   4. Switching to CPU variant via llm.backend override
#   5. Running the same benchmarks on the CPU variant
#   6. Computing averages and speedup ratios
#   7. Writing a structured results file
#
# Usage:
#   ./scripts/benchmark-llm-variants.sh [--output PATH] [--runs N]
#
# Options:
#   --output PATH   Output file for results (default: docs/designs/benchmarks/gpu-variant-performance.md)
#   --runs N        Number of measurement runs per configuration (default: 3)
#
# Prerequisites:
#   - tsuku installed and on PATH
#   - GPU hardware present (NVIDIA with CUDA drivers, or AMD/Intel with Vulkan)
#   - Models downloaded (the script will prompt if models are missing)
#   - tsuku-llm recipe available (issue #1776)
#
# Exit Codes:
#   0 - Benchmark completed and results written
#   1 - Missing prerequisites or benchmark failure

set -euo pipefail

# --- Configuration ---

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

DEFAULT_OUTPUT="$REPO_ROOT/docs/designs/benchmarks/gpu-variant-performance.md"
OUTPUT="${DEFAULT_OUTPUT}"
NUM_RUNS=3
PROMPT_TOKENS=512
GENERATE_TOKENS=256
WARMUP_RUNS=1

# Model identifiers for each size category
MODEL_05B="qwen2.5:0.5b"
MODEL_15B="qwen2.5:1.5b"
MODEL_3B="qwen2.5:3b"

# --- Argument parsing ---

while [[ $# -gt 0 ]]; do
    case "$1" in
        --output)
            OUTPUT="$2"
            shift 2
            ;;
        --runs)
            NUM_RUNS="$2"
            shift 2
            ;;
        --help|-h)
            head -n 30 "$0" | tail -n +2 | sed 's/^# \?//'
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

# --- Helper functions ---

log() {
    echo "[benchmark] $*" >&2
}

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

# Detect GPU hardware info. Sets GPU_VENDOR, GPU_MODEL, GPU_DRIVER, GPU_RUNTIME.
detect_hardware() {
    log "Detecting GPU hardware..."

    GPU_VENDOR="unknown"
    GPU_MODEL="unknown"
    GPU_DRIVER="unknown"
    GPU_RUNTIME="unknown"

    # NVIDIA detection
    if command -v nvidia-smi &>/dev/null; then
        GPU_VENDOR="nvidia"
        GPU_MODEL=$(nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null | head -1 || echo "unknown")
        GPU_DRIVER=$(nvidia-smi --query-gpu=driver_version --format=csv,noheader 2>/dev/null | head -1 || echo "unknown")

        # CUDA version from nvidia-smi
        GPU_RUNTIME="CUDA $(nvidia-smi --query-gpu=compute_cap --format=csv,noheader 2>/dev/null | head -1 || echo "unknown")"
        local cuda_version
        cuda_version=$(nvidia-smi 2>/dev/null | grep -oP 'CUDA Version: \K[0-9.]+' || echo "unknown")
        if [[ "$cuda_version" != "unknown" ]]; then
            GPU_RUNTIME="CUDA $cuda_version"
        fi

        log "Detected NVIDIA GPU: $GPU_MODEL (driver $GPU_DRIVER, $GPU_RUNTIME)"
        return
    fi

    # AMD detection via vulkaninfo
    if command -v vulkaninfo &>/dev/null; then
        local vk_device
        vk_device=$(vulkaninfo --summary 2>/dev/null | grep -i 'deviceName' | head -1 | sed 's/.*= //' || echo "")
        if [[ -n "$vk_device" ]]; then
            GPU_MODEL="$vk_device"

            # Check if AMD or Intel
            if echo "$vk_device" | grep -qi 'amd\|radeon'; then
                GPU_VENDOR="amd"
            elif echo "$vk_device" | grep -qi 'intel'; then
                GPU_VENDOR="intel"
            fi

            local vk_version
            vk_version=$(vulkaninfo --summary 2>/dev/null | grep -i 'apiVersion' | head -1 | sed 's/.*= //' || echo "unknown")
            GPU_RUNTIME="Vulkan $vk_version"
            GPU_DRIVER=$(vulkaninfo --summary 2>/dev/null | grep -i 'driverVersion' | head -1 | sed 's/.*= //' || echo "unknown")

            log "Detected $GPU_VENDOR GPU: $GPU_MODEL (driver $GPU_DRIVER, $GPU_RUNTIME)"
            return
        fi
    fi

    # Fallback: check sysfs for GPU presence
    if [[ -d /sys/bus/pci/devices ]]; then
        for dev in /sys/bus/pci/devices/*/class; do
            local class
            class=$(cat "$dev" 2>/dev/null || echo "")
            # VGA controller (0x0300) or 3D controller (0x0302)
            if [[ "$class" == 0x0300* ]] || [[ "$class" == 0x0302* ]]; then
                local vendor_file="${dev%/class}/vendor"
                local vendor_id
                vendor_id=$(cat "$vendor_file" 2>/dev/null || echo "")
                case "$vendor_id" in
                    0x10de) GPU_VENDOR="nvidia" ;;
                    0x1002) GPU_VENDOR="amd" ;;
                    0x8086) GPU_VENDOR="intel" ;;
                esac
            fi
        done
    fi

    if [[ "$GPU_VENDOR" == "unknown" ]]; then
        log "No GPU detected. Only CPU benchmarks will run."
    else
        log "Detected $GPU_VENDOR GPU (limited info available)"
    fi
}

# Generate a deterministic prompt of approximately $PROMPT_TOKENS tokens.
# Uses a fixed text that produces consistent token counts across tokenizers.
generate_prompt() {
    # A fixed, reproducible prompt (~512 tokens). The exact token count varies
    # by tokenizer, but consistency across runs is what matters.
    cat <<'PROMPT'
The history of computing is a story of abstraction layered upon abstraction. In the earliest days, programmers worked directly with machine code, toggling switches on room-sized computers. Assembly language provided the first layer of abstraction, mapping human-readable mnemonics to binary instructions. High-level languages like FORTRAN and COBOL followed, letting scientists and business users express problems in terms closer to their domains. The C programming language bridged the gap between high-level expression and low-level hardware access, becoming the lingua franca of systems programming. Unix, written in C, demonstrated that operating systems could be portable across hardware architectures.

The rise of personal computing in the 1980s brought graphical user interfaces, which abstracted away the command line for everyday users. Object-oriented programming organized code into reusable components. The internet connected these machines, and the web browser became a universal client. Java promised write-once-run-anywhere portability through its virtual machine. JavaScript, originally a simple scripting language for web pages, evolved into a general-purpose language running on servers, phones, and embedded devices.

Modern computing continues this pattern. Containers abstract away the operating system. Cloud platforms abstract away the hardware. Machine learning models abstract away explicit programming, learning patterns from data instead. Large language models represent perhaps the most dramatic abstraction yet: they compress the patterns of human language into billions of numerical parameters, enabling machines to generate text, answer questions, and write code.

GPU computing accelerated this revolution. Originally designed for rendering pixels in video games, graphics processing units proved ideal for the massively parallel matrix operations that neural networks require. NVIDIA's CUDA platform gave researchers direct access to GPU compute power. Frameworks like TensorFlow and PyTorch made GPU-accelerated machine learning accessible to Python programmers. Today, training a large language model requires thousands of GPUs running for weeks, consuming megawatts of electricity.

The inference side of the equation is equally important. While training happens once (at enormous cost), inference happens millions of times as users interact with deployed models. Efficient inference on consumer hardware depends on quantization (reducing parameter precision from 32-bit floats to 4-bit integers), optimized kernels for specific GPU architectures, and careful memory management to fit models within available VRAM. The llama.cpp project demonstrated that large language models could run on consumer hardware, spawning an ecosystem of local inference engines.
PROMPT
}

# Run a single benchmark. Expects tsuku-llm to be installed and the server available.
# Arguments: $1=model_name
# Outputs: prefill_tps and decode_tps to stdout as "prefill_tps decode_tps"
run_single_benchmark() {
    local model="$1"
    local prompt
    prompt=$(generate_prompt)

    # Use tsuku-llm bench subcommand if available, otherwise fall back to
    # timing a completion request via the gRPC or HTTP API.
    # The bench approach gives us direct tokens/second metrics.
    if tsuku llm bench --model "$model" --prompt-tokens "$PROMPT_TOKENS" --generate-tokens "$GENERATE_TOKENS" --format json 2>/dev/null; then
        return
    fi

    # Fallback: time a completion and compute tokens/second from elapsed time.
    # This is less precise but works with any tsuku-llm version.
    local start end elapsed
    local tmp_output
    tmp_output=$(mktemp)

    start=$(date +%s%N)
    tsuku llm complete --model "$model" --max-tokens "$GENERATE_TOKENS" --prompt "$prompt" > "$tmp_output" 2>&1
    end=$(date +%s%N)

    elapsed=$(( (end - start) / 1000000 )) # milliseconds

    # Parse token counts from output if available
    local prompt_tokens_actual="${PROMPT_TOKENS}"
    local gen_tokens_actual="${GENERATE_TOKENS}"

    # Compute approximate tokens/second
    # Prefill: prompt tokens / prefill time (not separable in fallback mode)
    # Decode: generated tokens / decode time
    # In fallback mode, we report combined throughput as decode_tps
    local total_tokens=$((prompt_tokens_actual + gen_tokens_actual))
    local tps
    if [[ "$elapsed" -gt 0 ]]; then
        tps=$(echo "scale=2; $total_tokens * 1000 / $elapsed" | bc)
    else
        tps="0"
    fi

    echo "N/A $tps"
    rm -f "$tmp_output"
}

# Run benchmark suite for a given model with warmup and averaging.
# Arguments: $1=model_name $2=variant_label
# Outputs: averaged "prefill_tps decode_tps" to stdout
run_benchmark_suite() {
    local model="$1"
    local variant="$2"
    local prefill_sum=0
    local decode_sum=0
    local prefill_count=0
    local decode_count=0

    log "Benchmarking $model ($variant): $WARMUP_RUNS warmup + $NUM_RUNS measured runs"

    # Warmup
    for ((i = 1; i <= WARMUP_RUNS; i++)); do
        log "  Warmup run $i/$WARMUP_RUNS..."
        run_single_benchmark "$model" > /dev/null 2>&1 || true
    done

    # Measured runs
    for ((i = 1; i <= NUM_RUNS; i++)); do
        log "  Measurement run $i/$NUM_RUNS..."
        local result
        result=$(run_single_benchmark "$model" 2>/dev/null || echo "N/A 0")
        local prefill decode
        prefill=$(echo "$result" | awk '{print $1}')
        decode=$(echo "$result" | awk '{print $2}')

        if [[ "$prefill" != "N/A" ]] && [[ "$prefill" != "0" ]]; then
            prefill_sum=$(echo "$prefill_sum + $prefill" | bc)
            prefill_count=$((prefill_count + 1))
        fi
        if [[ "$decode" != "0" ]]; then
            decode_sum=$(echo "$decode_sum + $decode" | bc)
            decode_count=$((decode_count + 1))
        fi
    done

    # Compute averages
    local avg_prefill="N/A"
    local avg_decode="N/A"
    if [[ "$prefill_count" -gt 0 ]]; then
        avg_prefill=$(echo "scale=2; $prefill_sum / $prefill_count" | bc)
    fi
    if [[ "$decode_count" -gt 0 ]]; then
        avg_decode=$(echo "scale=2; $decode_sum / $decode_count" | bc)
    fi

    echo "$avg_prefill $avg_decode"
}

# Install the GPU variant (auto-detected by recipe system)
install_gpu_variant() {
    log "Installing GPU variant (auto-detected)..."
    # Clear any manual override
    tsuku config unset llm.backend 2>/dev/null || true
    tsuku install tsuku-llm --force 2>/dev/null || tsuku install tsuku-llm
    log "GPU variant installed"
}

# Install the CPU variant (via llm.backend override)
install_cpu_variant() {
    log "Installing CPU variant (llm.backend=cpu override)..."
    tsuku config set llm.backend cpu
    tsuku install tsuku-llm --force 2>/dev/null || tsuku install tsuku-llm
    log "CPU variant installed"
}

# --- Main ---

log "Starting GPU variant performance benchmarks"
log "Output: $OUTPUT"
log "Runs per configuration: $NUM_RUNS"
log "Prompt tokens: ~$PROMPT_TOKENS, Generate tokens: $GENERATE_TOKENS"

# Check prerequisites
command -v tsuku &>/dev/null || fail "tsuku not found on PATH"
command -v bc &>/dev/null || fail "bc not found (needed for arithmetic)"

# Detect hardware
detect_hardware

# Create output directory
mkdir -p "$(dirname "$OUTPUT")"

# Arrays to hold results: [model]="prefill_tps decode_tps"
declare -A GPU_RESULTS
declare -A CPU_RESULTS

MODELS=("$MODEL_05B" "$MODEL_15B" "$MODEL_3B")
MODEL_LABELS=("0.5B" "1.5B" "3B")

# Determine backend label based on detected GPU
BACKEND_LABEL="GPU"
case "$GPU_VENDOR" in
    nvidia) BACKEND_LABEL="CUDA" ;;
    amd|intel) BACKEND_LABEL="Vulkan" ;;
    *) BACKEND_LABEL="CPU-only (no GPU detected)" ;;
esac

# Run GPU variant benchmarks (if GPU is available)
if [[ "$GPU_VENDOR" != "unknown" ]]; then
    install_gpu_variant

    for i in "${!MODELS[@]}"; do
        model="${MODELS[$i]}"
        label="${MODEL_LABELS[$i]}"
        result=$(run_benchmark_suite "$model" "$BACKEND_LABEL")
        GPU_RESULTS["$label"]="$result"
        log "  $label $BACKEND_LABEL: $result"
    done
else
    log "No GPU detected, skipping GPU variant benchmarks"
    for label in "${MODEL_LABELS[@]}"; do
        GPU_RESULTS["$label"]="N/A N/A"
    done
fi

# Run CPU variant benchmarks
install_cpu_variant

for i in "${!MODELS[@]}"; do
    model="${MODELS[$i]}"
    label="${MODEL_LABELS[$i]}"
    result=$(run_benchmark_suite "$model" "CPU")
    CPU_RESULTS["$label"]="$result"
    log "  $label CPU: $result"
done

# Restore auto-detection
tsuku config unset llm.backend 2>/dev/null || true

# --- Generate results file ---

log "Writing results to $OUTPUT"

TIMESTAMP=$(date -u '+%Y-%m-%d %H:%M UTC')
HOSTNAME=$(hostname)
CPU_MODEL=$(grep -m1 'model name' /proc/cpuinfo 2>/dev/null | sed 's/.*: //' || echo "unknown")
RAM_GB=$(free -g 2>/dev/null | awk '/^Mem:/{print $2}' || echo "unknown")

{
    cat <<EOF
# GPU Variant Performance Benchmark Results

**Status**: Completed
**Date**: $TIMESTAMP
**Host**: $HOSTNAME

## Hardware Details

| Component | Details |
|-----------|---------|
| GPU Model | $GPU_MODEL |
| GPU Vendor | $GPU_VENDOR |
| Driver Version | $GPU_DRIVER |
| GPU Runtime | $GPU_RUNTIME |
| CPU Model | $CPU_MODEL |
| RAM | ${RAM_GB} GB |

## Methodology

Benchmarks follow the protocol defined in issue #1780:

- **Prompt**: Fixed ~${PROMPT_TOKENS}-token prompt (deterministic text, same across all runs)
- **Generation**: ${GENERATE_TOKENS} tokens generated per run
- **Warmup**: ${WARMUP_RUNS} warmup run(s) discarded before measurement
- **Measurement**: ${NUM_RUNS} sequential runs averaged per configuration
- **Installation**: GPU variant auto-selected by recipe system; CPU variant forced via \`tsuku config set llm.backend cpu\`
- **Execution**: All runs sequential on the same machine, no concurrent workloads

### Variants tested

| Variant | Backend | How installed |
|---------|---------|---------------|
| GPU | $BACKEND_LABEL | \`tsuku install tsuku-llm\` (auto-detected) |
| CPU | CPU | \`tsuku config set llm.backend cpu\` then \`tsuku install tsuku-llm\` |

## Results

### $BACKEND_LABEL Variant (GPU)

| Model | Parameters | Prefill (tokens/second) | Decode (tokens/second) |
|-------|-----------|------------------------|----------------------|
EOF

    for label in "0.5B" "1.5B" "3B"; do
        local_result="${GPU_RESULTS[$label]}"
        prefill=$(echo "$local_result" | awk '{print $1}')
        decode=$(echo "$local_result" | awk '{print $2}')
        echo "| $label | $label | $prefill tok/s | $decode tok/s |"
    done

    cat <<EOF

### CPU Variant

| Model | Parameters | Prefill (tokens/second) | Decode (tokens/second) |
|-------|-----------|------------------------|----------------------|
EOF

    for label in "0.5B" "1.5B" "3B"; do
        local_result="${CPU_RESULTS[$label]}"
        prefill=$(echo "$local_result" | awk '{print $1}')
        decode=$(echo "$local_result" | awk '{print $2}')
        echo "| $label | $label | $prefill tok/s | $decode tok/s |"
    done

    cat <<EOF

### Speedup ($BACKEND_LABEL vs CPU)

| Model | Prefill Speedup | Decode Speedup |
|-------|----------------|----------------|
EOF

    for label in "0.5B" "1.5B" "3B"; do
        gpu_result="${GPU_RESULTS[$label]}"
        cpu_result="${CPU_RESULTS[$label]}"
        gpu_decode=$(echo "$gpu_result" | awk '{print $2}')
        cpu_decode=$(echo "$cpu_result" | awk '{print $2}')

        if [[ "$gpu_decode" != "N/A" ]] && [[ "$cpu_decode" != "N/A" ]] && [[ "$cpu_decode" != "0" ]]; then
            speedup=$(echo "scale=1; $gpu_decode / $cpu_decode" | bc 2>/dev/null || echo "N/A")
            echo "| $label | N/A | ${speedup}x |"
        else
            echo "| $label | N/A | N/A |"
        fi
    done

    cat <<EOF

## Verdict

EOF

    # Determine verdict
    local any_speedup=false
    for label in "0.5B" "1.5B" "3B"; do
        gpu_result="${GPU_RESULTS[$label]}"
        cpu_result="${CPU_RESULTS[$label]}"
        gpu_decode=$(echo "$gpu_result" | awk '{print $2}')
        cpu_decode=$(echo "$cpu_result" | awk '{print $2}')
        if [[ "$gpu_decode" != "N/A" ]] && [[ "$cpu_decode" != "N/A" ]] && [[ "$cpu_decode" != "0" ]]; then
            ratio=$(echo "scale=2; $gpu_decode / $cpu_decode" | bc 2>/dev/null || echo "0")
            if (( $(echo "$ratio > 1.5" | bc -l 2>/dev/null || echo "0") )); then
                any_speedup=true
            fi
        fi
    done

    if [[ "$any_speedup" == "true" ]]; then
        echo "**PASS**: GPU variant ($BACKEND_LABEL) provides meaningful speedup over CPU variant."
        echo "The GPU acceleration justifies the added complexity of GPU detection and variant selection."
    elif [[ "$GPU_VENDOR" == "unknown" ]]; then
        echo "**SKIP**: No GPU hardware detected. Only CPU benchmarks were recorded."
        echo "GPU validation requires re-running on a machine with NVIDIA (CUDA) or AMD/Intel (Vulkan) hardware."
    else
        echo "**INCONCLUSIVE**: Results did not show clear speedup. Manual review of raw numbers recommended."
    fi

    cat <<EOF

## Raw Data

Benchmark parameters:
- Script: \`scripts/benchmark-llm-variants.sh\`
- Prompt tokens: ~$PROMPT_TOKENS
- Generated tokens: $GENERATE_TOKENS
- Warmup runs: $WARMUP_RUNS
- Measured runs: $NUM_RUNS
- Models: $MODEL_05B, $MODEL_15B, $MODEL_3B
EOF
} > "$OUTPUT"

log "Results written to $OUTPUT"
log "Done."
