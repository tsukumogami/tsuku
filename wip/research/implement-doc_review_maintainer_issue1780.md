# Maintainer Review: Issue #1780 (GPU Variant Benchmark Tooling)

**Reviewer focus**: Maintainability -- can the next developer run this script and modify it with confidence?

**Overall**: The script is well-structured with good header documentation, clear argument parsing, and thorough hardware detection. The results template is useful as a placeholder. Two issues will trip up the next developer who tries to use or modify the benchmark logic.

---

## Blocking

### 1. `run_single_benchmark` has two return paths with incompatible contracts

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/scripts/benchmark-llm-variants.sh`, lines 185-227

The function has two code paths:

**Path A** (line 193-195): If `tsuku llm bench --format json` succeeds, its JSON stdout flows directly to the caller, then `return`. The caller (`run_benchmark_suite`, line 252) expects the format `"prefill_tps decode_tps"` (two space-separated numbers parsed by `awk '{print $1}'` and `awk '{print $2}'`). JSON output won't parse correctly -- `awk '{print $1}'` on a JSON blob will extract a brace or key, not a number.

**Path B** (line 197-227): The fallback path computes tokens/second and echoes `"N/A $tps"`, which does match the caller's contract.

The next developer will see the `bench --format json` path, think "the bench subcommand is preferred," and not realize the output contract is broken. When `tsuku llm bench` eventually ships, this script will silently produce garbage numbers in the results file (bc will fail or produce 0, and the error is swallowed by `2>/dev/null` on line 252).

**Fix**: Either parse the JSON output in Path A to extract prefill/decode tps and echo them in the expected format, or document that Path A is intentionally stubbed out pending the bench subcommand's API finalization (and add a TODO with the expected format).

### 2. Magic threshold `1.5` for verdict with no explanation

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/scripts/benchmark-llm-variants.sh`, line 474

```bash
if (( $(echo "$ratio > 1.5" | bc -l 2>/dev/null || echo "0") )); then
```

The verdict ("PASS" vs "INCONCLUSIVE") hinges on whether any model achieves 1.5x speedup. There's no comment explaining why 1.5x is the threshold. The results doc (line 481) says "meaningful speedup" without defining the bar. The template doc says (line 136): "GPU acceleration must provide clear, measurable benefit" -- also undefined.

The next developer who runs this on hardware and gets 1.4x will see INCONCLUSIVE and not know whether to adjust the threshold or flag a real problem. Conversely, someone who gets 1.51x on the smallest model but 0.9x on the largest will see PASS, which may be misleading.

**Fix**: Add a comment at the threshold explaining the rationale (e.g., "1.5x accounts for measurement noise; below this, the GPU variant's added complexity may not be worth it"). Consider whether the threshold should apply per-model or across all models.

---

## Advisory

### 3. `local_result` used outside function scope

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/scripts/benchmark-llm-variants.sh`, lines 416, 431

```bash
local_result="${GPU_RESULTS[$label]}"
```

This variable is named with the `local_` prefix convention, suggesting it's a local variable, but it's used in the main body of the script (inside a `{ ... } > "$OUTPUT"` block, not inside a function). `local` keyword only works inside functions. The `local_` prefix is misleading -- the next developer might add `local` before it and get a bash error, or might think this is inside a function when it isn't.

Rename to `result` or `entry` to avoid the false signal.

### 4. GPU_RUNTIME is set twice for NVIDIA, second value silently overwrites first

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/scripts/benchmark-llm-variants.sh`, lines 103-108

```bash
GPU_RUNTIME="CUDA $(nvidia-smi --query-gpu=compute_cap ...)"   # line 103
...
if [[ "$cuda_version" != "unknown" ]]; then
    GPU_RUNTIME="CUDA $cuda_version"                            # line 107
fi
```

Line 103 sets `GPU_RUNTIME` to CUDA compute capability (e.g., "CUDA 8.9"). Line 107 overwrites it with the CUDA toolkit version (e.g., "CUDA 12.4"). These are different things (compute capability vs runtime version). If the `nvidia-smi` grep on line 105 fails, the hardware report will show compute capability labeled as the "GPU Runtime," which is wrong.

The intent seems to be: try to get CUDA version, fall back to compute cap. But the flow reads as "set it, then immediately overwrite it." A comment or restructuring (try cuda_version first, fall back to compute_cap) would make the precedence clearer.

### 5. Script overwrites the template doc with single-rig results

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/scripts/benchmark-llm-variants.sh`, line 502

The template at `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/docs/designs/benchmarks/gpu-variant-performance.md` has sections for both NVIDIA and AMD results. The script writes a single-rig result (one GPU vendor) and overwrites the entire file. Running on NVIDIA first, then AMD, will destroy the NVIDIA results.

The template's "How to Reproduce" section (lines 58-67) instructs running on NVIDIA, then AMD -- but following these instructions literally will lose the first set of results.

This isn't a script bug exactly -- it's a mismatch between the template's expectations and the script's behavior. The next developer who follows the template instructions will lose data. Either the script should merge results into the existing file, or the template should note that each run produces a separate file (using `--output` to avoid clobbering).

---

## What reads well

The header comment (lines 1-32) is complete: it explains what the script does, usage, options, prerequisites, and exit codes. The hardware detection in `detect_hardware()` handles three vendors plus a sysfs fallback clearly. The argument parser handles `--help` by extracting the header comment, which is a nice self-documenting pattern.
