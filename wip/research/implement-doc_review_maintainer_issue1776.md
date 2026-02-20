# Maintainer Review: Issue #1776

**Issue**: feat(recipe): add tsuku-llm recipe with GPU-filtered variant selection
**File**: `recipes/t/tsuku-llm.toml` (new file, 108 lines)
**Reviewer focus**: Can the next developer understand and modify this with confidence?

## Summary

The recipe is well-structured and clear. The header comment (lines 1-16) establishes the variant selection model and documents all five platform/GPU combinations. Each step has a descriptive comment above it that explains why that variant exists. The `when` clauses and step-level `dependencies` follow the established patterns from `cuda-runtime.toml`, `vulkan-loader.toml`, and the WhenClause infrastructure in `internal/recipe/types.go`.

One advisory finding on a subtle gap in the `apple` GPU value handling. No blocking issues.

## Findings

### 1. macOS steps don't filter on `gpu = ["apple"]` -- intentional but undocumented divergence from the Linux pattern

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/recipes/t/tsuku-llm.toml`
**Lines**: 31-44
**Severity**: Advisory

The Linux steps use `gpu = ["nvidia"]`, `gpu = ["amd", "intel"]`, and `gpu = ["none"]` to create mutually exclusive variant selection. The macOS steps don't include any `gpu` filter -- they match on `os = ["darwin"]` and `arch` alone. This works because `DetectGPU()` on macOS always returns `"apple"` (see `internal/platform/gpu_darwin.go`), and there's only one macOS variant per architecture (Metal), so no disambiguation is needed.

But the next developer who adds a macOS variant (e.g., a future CPU-only macOS build) will look at the Linux steps, see the `gpu` filter pattern, and wonder why macOS doesn't use `gpu = ["apple"]`. They might add a new macOS step without a gpu filter and create an ambiguity (two steps matching the same target).

The header comment (lines 8-14) explains the variant mapping but doesn't call out why macOS omits the `gpu` field. A one-line comment above the macOS section would close this gap:

```toml
# macOS: Metal is the only variant per architecture, so no gpu filter is needed.
# (DetectGPU() returns "apple" on all Macs, but there's nothing to disambiguate.)
```

This is advisory because the current recipe is unambiguous and correct. The divergence only becomes a trap when someone adds a variant.

### 2. Windows step doesn't filter on `gpu = ["none"]` -- same divergence as macOS

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/recipes/t/tsuku-llm.toml`
**Lines**: 98-104
**Severity**: Advisory

The Windows step uses `when = { os = ["windows"], arch = "amd64" }` without a `gpu` filter, matching the macOS pattern. `DetectGPU()` on Windows returns `"none"` (see the design doc). This is correct for the same reason as macOS: only one variant exists per platform, so filtering isn't needed.

But the inconsistency with Linux creates the same future-developer trap. If a Windows CUDA variant is added later, the existing Windows step would need a gpu filter added to it (or it would match NVIDIA systems too). A brief comment, similar to the one suggested for macOS, would prevent that mistake.

No action strictly needed; the design doc's "What's deferred" section (line 145: "Windows support: The pipeline builds a Windows CPU variant but the recipe doesn't include it yet") already signals this is intentional, but that context isn't visible in the recipe file itself.

## What reads well

- The header comment (lines 1-16) is thorough. It names all five variant paths, explains the automatic selection mechanism, and documents the manual CPU override command. A developer encountering this recipe for the first time knows exactly what it does and how to troubleshoot.

- Step-level `dependencies` (lines 50, 59, 76, 85) are the right mechanism here. Only the matching step's dependencies get resolved, so an AMD GPU system never pulls in `cuda-runtime`. This follows the pattern established in `internal/recipe/types.go` (line 350: `Dependencies []string // Step-level dependencies, only resolved if this step matches target`).

- The `supported_libc = ["glibc"]` constraint (line 24) correctly prevents musl/Alpine users from attempting installation, matching the design doc's explicit deferral of musl support.

- The `binary = "tsuku-llm"` field on each step follows the `github_file` action's convention (matching `kind.toml` and other single-binary recipes), ensuring the downloaded file is installed as `tsuku-llm` regardless of the asset name.

- The recipe structure matches the design doc's example recipe almost exactly (lines 296-372 of `DESIGN-gpu-backend-selection.md`), with the addition of `binary`, `supported_os`, `supported_arch`, `supported_libc`, and `homepage` fields. These are recipe metadata improvements over the sketch, not deviations.
