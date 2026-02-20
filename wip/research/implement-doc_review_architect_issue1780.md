# Architect Review: Issue #1780 -- test(llm): validate GPU variant performance on shipped models

**Commit**: `82744432f180144c2a2a6a92f6aaad07795765d9`
**Reviewer focus**: Software architecture -- structural fit
**Blocking**: 0
**Advisory**: 2

## Summary

This commit adds two files: a benchmark automation script (`scripts/benchmark-llm-variants.sh`) and a results template (`docs/designs/benchmarks/gpu-variant-performance.md`). No Go code changed. The script uses `tsuku install` and `tsuku config set` to switch between GPU and CPU variants, runs inference, and writes structured results to the template file.

## Structural Assessment

**Fits the architecture.** The commit stays within established patterns:

1. **Script location**: `scripts/` is the existing home for operational automation (30+ scripts already live there). No parallel directory introduced.

2. **Results location**: `docs/designs/benchmarks/` is a new subdirectory under the existing `docs/designs/` tree. This is reasonable -- the results serve as an evidence artifact for the design gate described in `DESIGN-gpu-backend-selection.md`. The design doc at line 65 explicitly references issue #1780 as a gate condition.

3. **CLI surface usage**: The script invokes `tsuku install`, `tsuku config set`, `tsuku config unset`, and `tsuku llm bench`/`tsuku llm complete` -- all through the public CLI. No action dispatch bypass, no direct package calls. The `tsuku llm bench` call has a graceful fallback to `tsuku llm complete` (lines 193-196), which handles the case where the bench subcommand doesn't exist yet.

4. **No state contract impact**: No changes to Go structs, state files, or template variables.

5. **No dependency direction issues**: Shell script; no package imports.

## Advisory Findings

### A1: Script writes directly to the results template location

`scripts/benchmark-llm-variants.sh:41` -- `DEFAULT_OUTPUT` is hardcoded to `docs/designs/benchmarks/gpu-variant-performance.md`. When the script runs, it overwrites the template with actual results (line 502). This means the committed template is replaced in-tree by script output. The template file (currently showing "Pending" placeholders for NVIDIA and AMD rigs separately) has a richer structure than what the script generates -- the template has separate sections for NVIDIA/CUDA and AMD/Vulkan test rigs, while the script produces a single GPU section based on whatever hardware it detects.

This is a minor design tension, not a structural problem. The `--output` flag lets callers redirect elsewhere. But the template's two-rig structure won't survive a single-rig script run.

**Advisory** -- the template and the script's output format diverge. Consider either (a) making the script append per-rig sections instead of overwriting, or (b) simplifying the template to match the single-run output.

### A2: `tsuku llm bench` and `tsuku llm complete` are unverified CLI surface

`scripts/benchmark-llm-variants.sh:193-204` -- The script calls `tsuku llm bench` and `tsuku llm complete`, but neither subcommand is registered in `cmd/tsuku/` based on a search of the codebase. These are presumably subcommands of the `tsuku-llm` binary (accessed through `tsuku llm` delegation), or they're planned for a future issue. The script handles this gracefully via the fallback path, so it won't fail. But the fallback's timing approach (lines 199-226) can't separate prefill from decode throughput, which means prefill columns will always show "N/A" unless `tsuku llm bench` exists.

**Advisory** -- contained. The script documents this limitation implicitly through the "N/A" handling. No structural concern since the script doesn't introduce a parallel way to invoke LLM inference outside the CLI.

## No Blocking Findings

The commit adds standalone tooling (a shell script and a results template) that interacts with the rest of the system exclusively through the public CLI surface. It doesn't touch Go packages, doesn't modify the action registry, doesn't add state fields, and doesn't introduce parallel patterns. The `docs/designs/benchmarks/` subdirectory is a reasonable location for design-gate evidence artifacts.
