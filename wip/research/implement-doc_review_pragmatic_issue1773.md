# Pragmatic Review: Issue #1773 - GPU vendor detection via PCI sysfs

## Summary

Clean, minimal implementation that follows the libc detection pattern closely. No blocking findings.

## Findings

### Advisory: `ValidGPUTypes` exported but unused outside tests

**File**: `internal/platform/gpu.go:10`
**Severity**: Advisory

`ValidGPUTypes` is an exported package-level variable used only in `gpu_test.go`. No production code references it. Presumably intended for future recipe validation (#1792) or WhenClause parsing (#1774).

This is fine to keep -- it's small, well-named, and will be consumed by the next two issues in the dependency chain. If those issues don't materialize, it becomes dead weight.

### Advisory: `SetGPU` has no production callers

**File**: `internal/platform/target.go:56`
**Severity**: Advisory

`SetGPU` is defined on `Target` but only exercised in `gpu_test.go`. The design doc suggested it to minimize test churn ("consider adding a `SetGPU` setter so existing tests that don't care about GPU can pass `""` without changing constructor calls everywhere"), but the implementation went with adding the `gpu` parameter to `NewTarget()` directly (the 77-callsite cascade). Since no test uses `SetGPU` for its stated purpose, it's speculative. Small and inert, though.

### Advisory: `PlanConfig.GPU` field is pre-wired but unused

**File**: `internal/executor/plan_generator.go:27`
**Severity**: Advisory

The `GPU` field on `PlanConfig` is added and threaded into `NewTarget()` at lines 114 and 671, but zero callers of `PlanConfig{}` pass a GPU value. This is issue #1775's scope ("thread GPU through plan generation"). Including it here means the field exists with no functional effect -- every plan generates a target with `gpu=""`.

This is borderline scope creep from #1773 into #1775 territory, but it's harmless: the field defaults to `""` which means "no GPU filtering", matching the pre-GPU behavior exactly. The alternative (adding it in #1775) would require touching the same `NewTarget` call a second time. Pragmatically fine.

### Advisory: Impossible-case guard in gpu_linux.go:77-79

**File**: `internal/platform/gpu_linux.go:77-79`
**Severity**: Advisory

```go
priority, ok := gpuPriority[gpu]
if !ok {
    continue
}
```

At this point, `gpu` is a value from `pciVendorToGPU` (line 72), and every value in `pciVendorToGPU` (`"nvidia"`, `"amd"`, `"intel"`) has a corresponding entry in `gpuPriority`. The `!ok` branch is unreachable unless someone adds a vendor to `pciVendorToGPU` without a matching priority. Defensive coding, not a bug. Small enough to not matter.

## No Blocking Findings

The implementation:
- Correctly reads sysfs class and vendor files using only stdlib `os.ReadFile` and `filepath.Glob`
- Handles errors (unreadable files, missing sysfs) gracefully by returning `"none"`
- Tests cover all vendor types, multi-GPU priority, and edge cases (nonexistent root, non-GPU PCI devices)
- Follows the libc detection pattern exactly as the design doc specified
- The `Matchable` interface change is correctly propagated to all implementations (`platform.Target`, `recipe.MatchTarget`, `verify.platformTarget`)
- The 77-callsite constructor cascade passes `""` for GPU, preserving existing behavior
- Build tag files (`gpu_darwin.go`, `gpu_windows.go`) return constants, no unnecessary detection logic
- `WhenClause` correctly does NOT add a `gpu` field (that's #1774)
