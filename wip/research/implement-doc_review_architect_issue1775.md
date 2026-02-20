# Architecture Review: #1775 refactor(executor): thread GPU through plan generation

## Review Scope

Files changed (per state file):
- `internal/executor/plan_generator.go`
- `internal/executor/plan_generator_test.go`
- `internal/executor/executor.go`
- `internal/executor/filter_test.go`

## Findings

### Finding 1: GPU auto-detection follows the established LinuxFamily pattern (Positive)

**File**: `internal/executor/plan_generator.go:101-104`

The GPU auto-detection in `GeneratePlan()` follows the same pattern as LinuxFamily detection at lines 87-99: check if the value was provided by the caller, and if not, detect from the host system. The auto-detection by mutating `cfg.GPU` before it flows into `generateDependencyPlans()` and `depCfg` is a practical approach -- it ensures the detected value propagates everywhere without requiring each downstream function to re-detect.

**Severity**: N/A (positive observation)

### Finding 2: depCfg GPU propagation completes a prior gap (Positive)

**File**: `internal/executor/plan_generator.go:757`

The `depCfg` construction at line 757 (`GPU: cfg.GPU`) and the LinuxFamily propagation at line 756 (`LinuxFamily: cfg.LinuxFamily`) close the gap identified in the #1773 review. The dependency plan `target` construction at line 677 (`platform.NewTarget(targetOS+"/"+targetArch, cfg.LinuxFamily, libc, cfg.GPU)`) now receives the GPU value, ensuring GPU-filtered steps in dependency recipes are filtered correctly.

**Severity**: N/A (positive observation)

### Finding 3: shouldExecute() runtime path calls DetectGPU() directly -- correct for its context (Advisory)

**File**: `internal/executor/executor.go:133`

```go
target := recipe.NewMatchTarget(runtime.GOOS, runtime.GOARCH, "", "", platform.DetectGPU())
```

The `shouldExecute()` method is the runtime execution path (used by `DryRun()`), not the plan generation path. It doesn't have access to `PlanConfig`, so calling `platform.DetectGPU()` directly is appropriate here. The plan generation path (`GeneratePlan`) does the right thing by using `cfg.GPU` with auto-detection fallback.

One structural note: this means `DryRun()` and `GeneratePlan()` detect GPU independently. If a caller sets `PlanConfig.GPU = "none"` (the CPU override path planned for #1777), `DryRun()` wouldn't respect that override. This is consistent with how LinuxFamily is handled in `shouldExecute()` (it passes empty string, not the detected family). `DryRun()` appears to be a lightweight display path, not a critical filtering path, so the gap is contained.

**Severity**: Advisory

### Finding 4: Production callers omit GPU field -- correct due to auto-detection (Advisory)

**Files**:
- `internal/builders/orchestrator.go:297-303`
- `cmd/tsuku/install_lib.go:111-117`
- `cmd/tsuku/eval.go:281-300`

These callers construct `PlanConfig` without a `GPU` field. This is correct because `GeneratePlan()` auto-detects when `GPU` is empty. The `eval.go` callsite has explicit `--os`, `--arch`, and `--linux-family` flags for cross-platform plan generation but no `--gpu` flag. This means cross-platform eval can't override the GPU value.

For the eval command specifically, this is a gap that will matter when CI generates plans for different GPU configurations. However, this is future work (not in this issue's scope), and the pattern established here (add a `PlanConfig` field, let callers pass it when needed) makes it trivial to add later by adding an `evalGPU` flag similar to the existing `evalLinuxFamily` flag.

**Severity**: Advisory (contained, easy to extend later)

### Finding 5: Test coverage validates the critical propagation path (Positive)

**File**: `internal/executor/plan_generator_test.go:1901-2204`

The tests cover three important scenarios:
1. `TestGeneratePlan_GPUFiltering` -- mutually exclusive GPU steps are filtered correctly
2. `TestGeneratePlan_GPUAutoDetection` -- empty GPU triggers auto-detection
3. `TestGeneratePlan_GPUPropagationThroughDependencies` -- GPU value flows through to dependency plan generation

The third test is architecturally important: it validates that the `depCfg.GPU` propagation actually works end-to-end, which was the gap identified in prior reviews.

**Severity**: N/A (positive observation)

### Finding 6: filter_test.go adds GPU-aware step filtering tests (Positive)

**File**: `internal/executor/filter_test.go:267-343`

`TestFilterStepsByTarget_GPU` validates that `FilterStepsByTarget()` correctly handles GPU conditions in When clauses, including the edge case where an empty GPU string excludes all GPU-filtered steps. This is consistent with how the function handles other dimensions.

**Severity**: N/A (positive observation)

### Finding 7: No parallel patterns introduced (Positive)

The change threads GPU through the existing plan generation pipeline without introducing a second path. The `PlanConfig.GPU` field follows the exact same pattern as `LinuxFamily` (caller provides or auto-detect). The `generateDependencyPlans` and `depCfg` propagation follow the same structure used for all other config fields. No new abstractions, interfaces, or dispatch mechanisms were added.

**Severity**: N/A (positive observation)

## Summary

No blocking findings. The implementation follows the established patterns exactly: `PlanConfig` field with auto-detection fallback, propagation through `depCfg`, target construction with the GPU value, and two-stage step filtering via `WhenClause.Matches()`. The depCfg gap identified in the #1773 review is now closed. The two advisory notes (shouldExecute runtime path, missing eval --gpu flag) are both contained and consistent with how other dimensions are handled in those same paths.
