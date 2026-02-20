# Architecture Review: #1774 feat(recipe): add gpu field to WhenClause

## Review Scope

Reviewed the implementation of GPU field additions to `WhenClause`, `MatchTarget`, `Constraint`, and related methods in `internal/recipe/` and `internal/platform/`, plus the constructor cascade across all callsites.

## Design Alignment

The implementation follows the design doc's intent precisely. The GPU field is added to `WhenClause` following the libc pattern: same slice-based field type, same AND-semantics matching logic, same array unmarshaling with single-string-to-array coercion, same `ToMap()` serialization, same `IsEmpty()` inclusion, same validation against `platform.ValidGPUTypes`.

The separation between platform detection (#1773, prior issue) and recipe filtering (#1774, this issue) is clean.

## Pattern Consistency Assessment

### Follows existing patterns correctly

1. **WhenClause.GPU field** (`types.go:251`): `[]string` with `toml:"gpu,omitempty"` tag. Same shape as `Libc []string`. Correct.

2. **IsEmpty()** (`types.go:256-260`): Includes `len(w.GPU) == 0` in the conjunction. Follows the libc pattern.

3. **Matches()** (`types.go:325-337`): GPU matching block is structurally identical to the libc matching block (lines 310-322). The only intentional difference is that GPU matching is not gated on `os == "linux"`, which is correct -- GPU filtering applies on all platforms (macOS has "apple", Linux has nvidia/amd/intel/none).

4. **UnmarshalTOML()** (`types.go:469-482`): GPU parsing uses the same `[]interface{}` / `string` switch pattern as libc (lines 453-466) and OS (lines 422-435). Consistent.

5. **ToMap()** (`types.go:560-562`): Conditional serialization of GPU field, same as libc.

6. **Constraint.GPU field** (`types.go:594`): Added as a string field alongside OS, Arch, LinuxFamily. Clone() includes it. Correct.

7. **MergeWhenClause GPU propagation** (`types.go:684-686`): Only propagates GPU to constraint when `len(when.GPU) == 1`. Multi-value GPU (e.g., `["amd", "intel"]`) leaves constraint.GPU empty, matching the multi-OS behavior (lines 658-663). This is consistent.

8. **ValidateStepsAgainstPlatforms()** (`platform.go:453-463`): GPU validation mirrors libc validation pattern -- checks values against `platform.ValidGPUTypes`. Correct.

9. **Matchable interface** (`types.go:196`): `GPU() string` method added alongside OS, Arch, LinuxFamily, Libc. Both `MatchTarget` and `platform.Target` implement it.

10. **Constructor cascade**: `NewMatchTarget` gains a `gpu` parameter. All callsites pass `""` for GPU in contexts where GPU is irrelevant (test fixtures, existing runtime code). Production code that auto-detects uses `DetectTarget()` which now includes GPU. `SetGPU()` helper on Target provides a non-breaking way to override GPU in tests.

### Dependency direction

`internal/recipe/platform.go` imports `internal/platform` for `ValidGPUTypes` and `ValidLibcTypes`. This is the correct direction: recipe-level code depends on platform-level constants. No circular imports.

## Findings

### Advisory: executor.go:132 -- runtime `shouldExecute` omits GPU detection

`internal/executor/executor.go:132` constructs a `MatchTarget` with empty GPU:

```go
target := recipe.NewMatchTarget(runtime.GOOS, runtime.GOARCH, "", "", "")
```

This means at plan *execution* time, GPU-filtered steps are not re-evaluated against the actual GPU. The comment says this is for validation, and plan generation (which happens earlier via `GeneratePlan()`) already filters steps with the GPU-aware target. So execution-time filtering is a secondary guard, not the primary filter. Steps that didn't match the GPU were already excluded from the plan.

This is consistent with how linuxFamily and libc are also empty in this same call. The executor relies on the plan generator to have done the correct filtering. If this convention changes in the future, GPU should be added here, but for now it's consistent with the existing pattern. **Advisory -- no action needed unless execution-time re-filtering is introduced.**

### Advisory: `info.go:447` and `sysdeps.go:210,212` pass empty GPU

Both `info.go:447` and `sysdeps.go:210,212` construct `NewTarget()` with `""` for GPU. These are used for platform-specific display (e.g., `tsuku info`) and sysdeps resolution, neither of which currently needs GPU information. When #1775 (thread GPU through plan generation) is implemented, these may need to auto-detect GPU if they're used in plan-generation paths. For now, they're display-only and this is fine. **Advisory.**

## Summary

The implementation is architecturally clean. No blocking findings. The GPU field is added to `WhenClause` by following the established libc pattern exactly: same field shape, same matching semantics, same serialization, same validation, same constraint merging behavior. The `Matchable` interface extension is minimal and both implementations are updated. The constructor cascade is handled with empty-string defaults for callsites that don't need GPU, and `SetGPU()` provides test ergonomics.

Two advisory notes about callsites that pass empty GPU -- these are consistent with how libc and linuxFamily are handled in those same callsites, and will be addressed by #1775 when GPU is threaded into plan generation end-to-end.
