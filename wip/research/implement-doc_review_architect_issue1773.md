# Architecture Review: #1773 - feat(platform): add GPU vendor detection via PCI sysfs

## Review Scope

Reviewed the implementation of GPU vendor detection in the platform package, including changes to `Target`, `Matchable`, `MatchTarget`, `PlanConfig`, and all constructor cascade updates.

## Files Changed

- `internal/platform/gpu.go` (new) - `ValidGPUTypes` constant
- `internal/platform/gpu_linux.go` (new) - `DetectGPU()` and `DetectGPUWithRoot()` via sysfs
- `internal/platform/gpu_darwin.go` (new) - returns "apple"
- `internal/platform/gpu_windows.go` (new) - returns "none"
- `internal/platform/gpu_test.go` (new) - 14 tests covering detection and Target GPU methods
- `internal/platform/target.go` (modified) - added `gpu` field to Target, `GPU()` method, `SetGPU()`, updated `NewTarget()` signature
- `internal/platform/family.go` (modified) - `DetectTarget()` calls `DetectGPU()` and threads result through `NewTarget()`
- `internal/recipe/types.go` (modified) - added `GPU() string` to `Matchable` interface, added `gpu` to `MatchTarget`, updated `NewMatchTarget()`
- `internal/executor/plan_generator.go` (modified) - added `GPU` field to `PlanConfig`, threaded through both plan generation paths
- `internal/platform/testdata/gpu/` (new) - mock sysfs directories for 6 test scenarios

Plus cascade updates to all `NewTarget()` and `NewMatchTarget()` callsites passing `""` for the new gpu parameter.

## Findings

### 1. Design alignment: follows the libc detection pattern exactly

**Severity: Positive observation**

The implementation mirrors `DetectLibc()` / `DetectLibcWithRoot()` faithfully:
- Platform-specific files via build tags (`gpu_linux.go`, `gpu_darwin.go`, `gpu_windows.go`) matching the pattern
- `DetectGPUWithRoot()` accepts a test root for mock sysfs, same as `DetectLibcWithRoot()`
- `ValidGPUTypes` parallels `ValidLibcTypes`
- `gpu` is an unexported field on `Target` with a public getter, same as `libc`
- `GPU() string` added to `Matchable` interface alongside `Libc() string`
- `NewMatchTarget()` gains `gpu` as a positional parameter, same treatment as `libc`

No parallel pattern introduced. GPU detection slots into the existing platform detection hierarchy without adding new abstractions.

### 2. `cmd/tsuku/info.go:447` and `cmd/tsuku/sysdeps.go:210,212` pass empty GPU to `NewTarget()`

**Severity: Advisory**

These callsites construct targets manually (for `--target-family` overrides and info display) and pass `""` for gpu. This is correct for now since GPU filtering doesn't happen in WhenClause yet (that's #1774). The `SetGPU()` helper exists for later use. No structural concern -- when #1775 threads GPU through these paths, the `""` values can be replaced with detected or overridden values. The code compiles and functions correctly today.

### 3. `executor.go:132` runtime `shouldExecute()` passes empty GPU to `NewMatchTarget()`

**Severity: Advisory**

The runtime execution path at `internal/executor/executor.go:132` constructs a `MatchTarget` without GPU. This is the execution-time filter (not plan-time), and since WhenClause doesn't have a GPU field yet, passing `""` is correct. When #1774 adds the GPU field to WhenClause, this callsite will need updating -- but the current code doesn't break any contract because `WhenClause.Matches()` has no GPU check to fail against.

### 4. `SetGPU()` uses value receiver -- good structural choice

**Severity: Positive observation**

`SetGPU()` at `internal/platform/target.go:56` uses a value receiver, returning a copy. This preserves the immutability of `Target` (all other methods are value receivers too). Tests confirm the original isn't mutated. This matches the design doc's suggestion for minimizing constructor cascade churn.

### 5. Detection logic is pure filesystem reads -- correct dependency direction

**Severity: Positive observation**

`gpu_linux.go` imports only `os`, `path/filepath`, and `strings`. No dependency on higher-level packages. No subprocess spawning. The `platform` package remains a leaf dependency, consistent with its position at the bottom of the import graph.

### 6. WhenClause does NOT have a GPU field yet -- correct scope boundary

**Severity: Positive observation**

The issue description specifies only GPU detection and `Matchable`/`Target` integration. WhenClause changes are issue #1774. The implementation correctly stops at the interface boundary without leaking ahead. `WhenClause.IsEmpty()` and `WhenClause.Matches()` are untouched.

## Overall Assessment

The implementation aligns with the design doc and follows established patterns. GPU detection mirrors the libc detection path structurally (platform-specific files, `WithRoot` test helper, `Valid*Types` constant, unexported field with public getter, `Matchable` interface method). The constructor cascade was handled cleanly by passing `""` at callsites that don't yet need GPU, with `SetGPU()` available as an escape hatch for tests. No structural violations, no parallel patterns, no dependency direction issues.

No blocking findings.
