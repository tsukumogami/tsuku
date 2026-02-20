# Pragmatic Review: Issue #1780

**Commit**: `82744432f180144c2a2a6a92f6aaad07795765d9`
**Files**: `scripts/benchmark-llm-variants.sh`, `docs/designs/benchmarks/gpu-variant-performance.md`

## Blocking Findings

### B1. Fallback benchmark path calls nonexistent CLI commands

`scripts/benchmark-llm-variants.sh:193` -- `tsuku llm bench` does not exist in the codebase. Neither does `tsuku llm complete` (line 204). The script's primary path (`tsuku llm bench`) silently fails and falls through to the fallback, which also calls a nonexistent command. The entire benchmark loop will always fail, echo "N/A 0", and produce a results file full of N/A values. This is not "benchmark tooling" -- it is a template generator that pretends to benchmark.

If the commands don't exist yet, the script should `fail` with a clear prerequisite error rather than silently producing meaningless results. **Blocking.**

**Fix**: Add an upfront check (`tsuku llm bench --help || fail "tsuku llm bench not available"`) or document that this script is a placeholder pending tsuku-llm bench subcommand implementation. If it is a placeholder, remove the 500-line script and just ship the template.

### B2. Sysfs fallback GPU detection (lines 139-155) is dead code within this script

The sysfs PCI scan on lines 139-155 detects GPU vendor but sets `GPU_MODEL`, `GPU_DRIVER`, `GPU_RUNTIME` to "unknown". When `GPU_VENDOR != "unknown"` (line 330), the script proceeds to `install_gpu_variant` and run benchmarks. But with no driver tooling (no nvidia-smi, no vulkaninfo), the benchmarks will fail since the GPU runtime isn't usable. The sysfs path lets the script enter a guaranteed-failure benchmark loop instead of falling through to the "No GPU detected" branch.

**Fix**: If neither `nvidia-smi` nor `vulkaninfo` is available, treat GPU as absent regardless of sysfs. The sysfs detection is useful in `internal/platform/` for recipe filtering, not here where it gates actual GPU execution.

### B3. Scope creep: `generate_prompt()` embeds a 350-word essay (lines 166-179)

A fixed prompt for reproducibility needs ~1 line: a seed or a file path. Instead, there is a 350-word essay about computing history inlined in the script. This is not a test fixture file (which would be fine), it is prose hardcoded in bash. If the prompt needs to be deterministic, put it in `testdata/benchmark-prompt.txt` and `cat` it.

More importantly, since B1 means this prompt is never actually sent anywhere, the essay is dead code. **Blocking** (as scope creep compounding B1).

## Advisory Findings

### A1. Template doc duplicates script output

`docs/designs/benchmarks/gpu-variant-performance.md` is a hand-written template that the script will overwrite entirely (line 502 writes `> "$OUTPUT"`). The template exists only to pass CI checks before the script runs. Two files that must stay in sync for a feature that doesn't work yet. Consider shipping only the template (since the script can't run) or only the script (since it generates the output).

### A2. Associative arrays require bash 4+

`scripts/benchmark-llm-variants.sh:315-316` -- `declare -A` requires bash 4+. macOS ships bash 3. The shebang is `#!/usr/bin/env bash`. If anyone attempts to run this on macOS (where GPU benchmarking via Metal could be relevant per the tsuku-llm recipe's macOS Metal steps), it will fail with a cryptic syntax error.

### A3. `--runs` parameter (speculative generality, minor)

`NUM_RUNS` defaults to 3 and the issue title says "validate GPU variant performance." There will likely be exactly one invocation of this script per hardware class. The `--runs` flag adds 8 lines of arg parsing for a parameter that will probably never be overridden. Marginal -- the code is small.

## Summary

3 blocking, 3 advisory. The core issue: the script calls commands that don't exist (`tsuku llm bench`, `tsuku llm complete`), making it a ~500-line template generator. Either (a) document it as a placeholder awaiting the bench subcommand and strip the dead benchmark logic, or (b) defer this issue until the bench subcommand ships.
