# Pragmatic Review: #1776 tsuku-llm recipe with GPU-filtered variant selection

## Scope

Single file: `recipes/t/tsuku-llm.toml` (new, 109 lines)

## Findings

No blocking findings. No advisory findings.

## Analysis

The recipe is the simplest correct representation of the design doc's recipe sketch. It is nearly a 1:1 transcription of the design doc section "Decision 3" (lines 296-372), with the addition of `binary = "tsuku-llm"` on each step and the metadata fields (`homepage`, `supported_os`, `supported_arch`, `supported_libc`).

### Correctness check

- **9 `github_file` steps** match the 10 CI pipeline variants minus the macOS Metal distinction (macOS steps don't suffix `-metal` because the CI pipeline names them by platform alone). 2 macOS + 6 Linux GPU-filtered + 1 Windows CPU = 9 steps for 9 distinct target combinations.
- **GPU when clauses** are mutually exclusive on Linux: `gpu = ["nvidia"]`, `gpu = ["amd", "intel"]`, `gpu = ["none"]` cover all possible `DetectGPU()` return values on Linux (`apple` only appears on macOS, where there's no `gpu` filter).
- **macOS steps** omit `gpu` filter, correct since all Macs get Metal and `DetectGPU()` returns `"apple"` unconditionally on Darwin.
- **Windows step** omits `gpu` filter, correct since `DetectGPU()` returns `"none"` on Windows and there's only one variant.
- **Step-level `dependencies`**: `cuda-runtime` on NVIDIA steps, `vulkan-loader` on AMD/Intel steps, none on CPU/Metal/Windows. Matches the design doc's dependency chain. The `Step.Dependencies` field is supported in the codebase (`internal/recipe/types.go:350`).
- **`supported_libc = ["glibc"]`**: Correctly excludes musl/Alpine, matching the design doc's deferred items.
- **`asset_pattern` format**: Uses `v{version}` prefix (e.g., `tsuku-llm-v{version}-linux-amd64-cuda`), consistent with the #1791 release artifact alignment issue.
- **Windows `.exe` suffix**: The Windows step's asset pattern includes `.exe`, other platforms don't. Correct.

### Simplicity check

- No speculative generality. No unused parameters or fields.
- No abstractions -- it's a flat TOML file.
- No scope creep beyond the issue's requirements.
- Comments are informative without being redundant (one per step explaining variant rationale).

### Potential concern (not a finding)

The design doc "What's deferred" section (line 145) says "Windows support: The pipeline builds a Windows CPU variant but the recipe doesn't include it yet." But the design doc's own recipe sketch (line 362-367) includes Windows, and the issue's key decisions explicitly state "1 Windows CPU." The implementation follows the recipe sketch and key decisions, not the deferred list. This is an inconsistency in the design doc, not a recipe bug.
