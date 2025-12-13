# Issue #442: feat(actions): add deterministic flag to plan schema

## Summary

Added `deterministic` boolean field to plan steps and overall plan to explicitly mark when ecosystem primitives introduce non-determinism due to compiler versions, native extensions, or package manager variability.

## Changes

### Core Implementation

1. **internal/actions/decomposable.go**
   - Added `deterministicActions` map classifying all primitives
   - Added `IsDeterministic(action string) bool` function
   - Tier 1 primitives (download, extract, chmod, install_binaries, set_env, set_rpath, link_dependencies, install_libraries) marked deterministic
   - Tier 2 ecosystem primitives (go_build, cargo_build, npm_exec, pip_install, gem_exec) marked non-deterministic

2. **internal/executor/plan.go**
   - Added `Deterministic` field to `InstallationPlan` struct
   - Added `Deterministic` field to `ResolvedStep` struct

3. **internal/executor/plan_generator.go**
   - Updated `resolveStep()` to set step-level deterministic flag
   - Updated `GeneratePlan()` to compute plan-level deterministic (AND of all steps)

4. **internal/install/state.go**
   - Added `Deterministic` field to `Plan` struct
   - Added `Deterministic` field to `PlanStep` struct
   - Updated `NewPlanFromExecutor()` signature to accept deterministic parameter

### CLI Display

5. **cmd/tsuku/plan.go**
   - Updated `printPlanHuman()` to display plan-level determinism status
   - Updated `printStep()` to show "(non-deterministic)" indicator for affected steps

### Tests

6. **internal/actions/decomposable_test.go**
   - Fixed pre-existing test: updated expected primitive count from 12 to 13
   - Added `TestIsDeterministic` covering all action types

7. **internal/executor/plan_test.go**
   - Added `TestDeterministicFieldJSONRoundTrip` for serialization
   - Added `TestDeterministicFieldInJSON` verifying field presence in JSON output

8. **internal/install/state_test.go**
   - Updated `TestNewPlanFromExecutor` for new signature

## Pre-existing Issue Fixed

- `TestPrimitives` expected 12 primitives but there were 13 (gem_exec and pip_install were added in recent PRs)
- Fixed by updating expected count

## Test Results

All 21 packages pass including the new determinism tests.

## Example Output

```
Plan for example@1.0.0

Platform:      linux/amd64
Generated:     2025-12-12 10:00:00 UTC
Recipe:        registry (hash: abc123...)
Deterministic: no (contains ecosystem primitives with residual non-determinism)

Steps (3):
  1. [download]
     URL: https://example.com/file.tar.gz
     Checksum: sha256:...
  2. [extract]
     Params: format=tar.gz
  3. [go_build] (non-deterministic)
     Params: package=./cmd/tool
```

## Related

- Blocked by: #440 (completed)
- Design: docs/DESIGN-plan-evaluation.md
