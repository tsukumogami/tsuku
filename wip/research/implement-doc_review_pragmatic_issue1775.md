# Review: #1775 refactor(executor): thread GPU through plan generation

**Focus**: pragmatic (simplicity, YAGNI, KISS)
**Reviewer**: pragmatic-reviewer

## Files Changed

- `internal/executor/plan_generator.go`
- `internal/executor/plan_generator_test.go`
- `internal/executor/executor.go`
- `internal/executor/filter_test.go`

## Findings

### Finding 1: `shouldExecute()` calls `DetectGPU()` on every invocation (Advisory)

**File**: `internal/executor/executor.go:133`
**Code**: `target := recipe.NewMatchTarget(runtime.GOOS, runtime.GOARCH, "", "", platform.DetectGPU())`

`shouldExecute()` is called once per step during `DryRun()`. `DetectGPU()` reads sysfs on Linux (filesystem I/O) on each call. For a recipe with 10 steps, that's 10 sysfs scans.

The issue summary notes "shouldExecute() runtime path now calls DetectGPU() directly since it lacks access to PlanConfig." This is pragmatically fine -- `DetectGPU()` is cheap (a handful of small file reads), and `shouldExecute()` is only used in the `DryRun()` path which is not performance-critical. Caching would add complexity for no user-visible benefit.

**Severity**: Advisory. Correct as-is. Not worth optimizing unless `shouldExecute()` gets called in a hot loop.

### Finding 2: cfg.GPU mutation propagates cleanly to depCfg (No Issue)

**File**: `internal/executor/plan_generator.go:101-104`
**Code**:
```go
if cfg.GPU == "" {
    cfg.GPU = platform.DetectGPU()
}
```

Mutating `cfg.GPU` in-place (same pattern as the existing LinuxFamily auto-detection at line 87-98) means `generateDependencyPlans()` at line 231 receives the already-populated value through `cfg`, and `depCfg` at line 757 copies it. Clean, minimal approach.

**Severity**: No issue. Correct and simple.

### Finding 3: LinuxFamily now propagated in depCfg (Scope expansion, but justified)

**File**: `internal/executor/plan_generator.go:756`
**Code**: `LinuxFamily: cfg.LinuxFamily, // Propagate for family-specific dependency filtering`

The issue description says "Adds a GPU field to PlanConfig and wires auto-detection into GeneratePlan()". Adding `LinuxFamily` to `depCfg` fixes a pre-existing bug flagged in the #1773 maintainer review as blocking (tracked for #1775). Since the reviewer explicitly deferred this fix to #1775, including it here is expected, not scope creep.

The test at `TestGeneratePlan_DepCfgLinuxFamilyPropagation` confirms this fix works.

**Severity**: No issue. Correctly addresses a deferred finding.

### Finding 4: Test coverage is thorough without gold-plating (No Issue)

11 new tests added:
- `TestGeneratePlan_GPUFiltering`: 4 GPU values, verifies mutual exclusion
- `TestGeneratePlan_GPUAutoDetection`: verifies auto-detect path
- `TestGeneratePlan_GPUPropagationThroughDependencies`: 3 GPU values through dep chain with mockRecipeLoader
- `TestGeneratePlan_DepCfgLinuxFamilyPropagation`: confirms the LinuxFamily fix
- `TestFilterStepsByTarget_GPU`: 4 cases including empty GPU edge case

Tests exercise the exact acceptance criteria. No speculative test infrastructure.

**Severity**: No issue.

### Finding 5: `filter_test.go` empty GPU edge case behavior (Advisory)

**File**: `internal/executor/filter_test.go:317-319`
**Code**:
```go
{
    name:     "empty GPU string excludes all GPU-filtered steps",
    target:   platform.NewTarget("linux/amd64", "debian", "glibc", ""),
    wantLen:  1,
    wantActs: []string{"install_binaries"},
},
```

When GPU is empty string, all GPU-filtered steps are excluded. This is correct behavior for `FilterStepsByTarget` (used by external callers like the CI validation pipeline), since `WhenClause.Matches()` fails when `target.GPU()` returns empty against a non-empty `w.GPU` list. In `GeneratePlan()`, the auto-detection at line 102-104 ensures GPU is never empty, so this edge case can't happen through normal plan generation. The test documents the behavior for direct callers of `FilterStepsByTarget`.

**Severity**: Advisory. The behavior is intentional and documented by the test. No action needed.

## Summary

No blocking findings. The implementation is minimal and correct. Three changes were made:

1. GPU auto-detection in `GeneratePlan()` via cfg mutation (3 lines)
2. GPU propagation to depCfg (1 line)
3. GPU detection in `shouldExecute()` for runtime/dry-run path (1 parameter added)

Plus the LinuxFamily depCfg fix (1 line) deferred from #1773. Total production code change is about 6 lines across 2 files. Tests are proportionate (11 tests for 6 lines is justified given the filtering combinatorics).

The approach of mutating `cfg.GPU` in-place at the top of `GeneratePlan()` is the simplest correct way to thread the value through -- it avoids adding a new parameter or restructuring the dependency plan generation. Matches the existing pattern for LinuxFamily auto-detection.
