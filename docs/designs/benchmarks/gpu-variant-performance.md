# GPU Variant Performance Benchmark Results

**Status**: Pending -- requires GPU hardware for execution
**Date**: Pending
**Automation**: `scripts/benchmark-llm-variants.sh`

## Hardware Details

Results will be collected on machines with the following GPU configurations.
Hardware details for each test run (GPU Model, Driver Version, runtime version)
are recorded by the benchmark script automatically.

### NVIDIA / CUDA Test Rig

| Component | Details |
|-----------|---------|
| GPU Model | Pending -- run benchmark on NVIDIA hardware |
| GPU Vendor | nvidia |
| Driver Version | Pending |
| CUDA Version | Pending |
| CPU Model | Pending |
| RAM | Pending |

### AMD / Vulkan Test Rig

| Component | Details |
|-----------|---------|
| GPU Model | Pending -- run benchmark on AMD hardware |
| GPU Vendor | amd |
| Driver Version | Pending |
| Vulkan Version | Pending |
| CPU Model | Pending |
| RAM | Pending |

## Methodology

Benchmarks follow the protocol defined in issue #1780 and the performance
validation section of DESIGN-gpu-backend-selection.md.

- **Prompt**: Fixed ~512-token prompt (deterministic text, identical across all runs)
- **Generation**: 256 tokens generated per run
- **Warmup**: 1 warmup run discarded before measurement (avoids cold-start effects)
- **Measurement**: 3 sequential runs averaged per configuration
- **Installation**: GPU variant auto-selected by recipe system; CPU variant forced via `tsuku config set llm.backend cpu`
- **Execution**: All runs sequential on the same machine, no concurrent workloads
- **Models**: 0.5B, 1.5B, 3B parameter models (the three sizes shipped with tsuku-llm)

### Variants Tested

| Variant | Backend | How Installed |
|---------|---------|---------------|
| NVIDIA GPU | CUDA | `tsuku install tsuku-llm` (auto-detected nvidia GPU) |
| AMD GPU | Vulkan | `tsuku install tsuku-llm` (auto-detected amd GPU) |
| CPU | CPU | `tsuku config set llm.backend cpu` then `tsuku install tsuku-llm` |

### How to Reproduce

```bash
# On an NVIDIA machine:
./scripts/benchmark-llm-variants.sh --runs 3

# On an AMD machine:
./scripts/benchmark-llm-variants.sh --runs 3

# The script auto-detects GPU hardware, runs both GPU and CPU variants,
# and writes results to this file.
```

## Results: NVIDIA / CUDA

### CUDA Variant (GPU)

| Model | Parameters | Prefill (tokens/second) | Decode (tokens/second) |
|-------|-----------|------------------------|----------------------|
| Qwen 2.5 | 0.5B | Pending tok/s | Pending tok/s |
| Qwen 2.5 | 1.5B | Pending tok/s | Pending tok/s |
| Qwen 2.5 | 3B | Pending tok/s | Pending tok/s |

### CPU Variant (on NVIDIA hardware)

| Model | Parameters | Prefill (tokens/second) | Decode (tokens/second) |
|-------|-----------|------------------------|----------------------|
| Qwen 2.5 | 0.5B | Pending tok/s | Pending tok/s |
| Qwen 2.5 | 1.5B | Pending tok/s | Pending tok/s |
| Qwen 2.5 | 3B | Pending tok/s | Pending tok/s |

### Speedup (CUDA vs CPU)

| Model | Prefill Speedup | Decode Speedup |
|-------|----------------|----------------|
| 0.5B | Pending | Pending |
| 1.5B | Pending | Pending |
| 3B | Pending | Pending |

## Results: AMD / Vulkan

### Vulkan Variant (GPU)

| Model | Parameters | Prefill (tokens/second) | Decode (tokens/second) |
|-------|-----------|------------------------|----------------------|
| Qwen 2.5 | 0.5B | Pending tok/s | Pending tok/s |
| Qwen 2.5 | 1.5B | Pending tok/s | Pending tok/s |
| Qwen 2.5 | 3B | Pending tok/s | Pending tok/s |

### CPU Variant (on AMD hardware)

| Model | Parameters | Prefill (tokens/second) | Decode (tokens/second) |
|-------|-----------|------------------------|----------------------|
| Qwen 2.5 | 0.5B | Pending tok/s | Pending tok/s |
| Qwen 2.5 | 1.5B | Pending tok/s | Pending tok/s |
| Qwen 2.5 | 3B | Pending tok/s | Pending tok/s |

### Speedup (Vulkan vs CPU)

| Model | Prefill Speedup | Decode Speedup |
|-------|----------------|----------------|
| 0.5B | Pending | Pending |
| 1.5B | Pending | Pending |
| 3B | Pending | Pending |

## Verdict

**Pending**: Actual measurements require GPU hardware.

This file was generated as a structured template by issue #1780. To complete the
validation gate:

1. Run `./scripts/benchmark-llm-variants.sh` on an NVIDIA machine (CUDA validation)
2. Run `./scripts/benchmark-llm-variants.sh` on an AMD machine (Vulkan validation)
3. The script overwrites this file with actual measurements and a computed verdict
4. If GPU variants show meaningful speedup over CPU, the gate passes
5. If speedup is not meaningful, the design should be reconsidered

The verdict determines whether the GPU backend selection design can move from
Planned to Current. GPU acceleration must provide clear, measurable benefit over
CPU-only inference to justify the added detection and variant selection complexity.

## Raw Data

Benchmark parameters (will be populated by the script):
- Script: `scripts/benchmark-llm-variants.sh`
- Prompt tokens: ~512
- Generated tokens: 256
- Warmup runs: 1
- Measured runs: 3
- Models: qwen2.5:0.5b, qwen2.5:1.5b, qwen2.5:3b
