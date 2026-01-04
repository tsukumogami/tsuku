# Implementation Plan: Issue #805

## Problem Summary

Sandbox mode fails for recipes requiring implicit action dependencies (cmake, zig, make, pkg-config) because these dependencies are installed on the host before plan generation but aren't included in the plan, making them unavailable during sandbox execution.

**Root cause**: The `install` command passes `RecipeLoader: nil` to plan generation, disabling the dependency embedding infrastructure that already exists in format v3.

## Design Reference

This implementation follows the design in `docs/DESIGN-sandbox-implicit-dependencies.md` (Option 2: Include Implicit Dependencies in Plan).

## Approach

Enable the existing format v3 dependency embedding by providing `RecipeLoader` during plan generation. This unifies the code path for `tsuku eval`, `tsuku install`, and `tsuku install --plan`, making plans truly self-contained.

## Files to Modify

### 1. `cmd/tsuku/install_deps.go`
**Changes**:
- Add `RecipeLoader` field to `planRetrievalConfig` struct
- Pass `loader` when constructing `planCfg`
- Thread `RecipeLoader` through to `executor.PlanConfig`
- Delete `ensurePackageManagersForRecipe()` call and function definition
- Delete `findDependencyBinPath()` helper function
- Delete `SetResolvedDeps()` workaround

**Impact**: ~109 lines deleted, 3-5 lines added/changed

### 2. `internal/executor/plan.go`
**Changes**:
- Add `LinuxFamily string` field to existing `Platform` struct with `json:"linux_family,omitempty"` tag

**Impact**: 1 line added

### 3. `internal/executor/plan_generator.go`
**Changes**:
- Add `detectPlatform()` function to populate `Platform` field
- Add `recipeHasFamilySpecificSteps()` helper
- Call `detectPlatform()` at start of `GeneratePlan()`
- Populate `plan.Platform` field

**Impact**: ~50 lines added

### 4. `internal/executor/executor.go`
**Changes**:
- Add `validatePlatform()` function
- Add `validateResourceLimits()` function
- Call validations at start of `ExecutePlan()`

**Impact**: ~80 lines added

## Implementation Steps

### Step 1: Add Platform struct extension
- Add `LinuxFamily` field to `Platform` struct in `plan.go`
- Verify JSON serialization works correctly

### Step 2: Add platform detection and validation
- Implement `detectPlatform()` in `plan_generator.go`
- Implement `recipeHasFamilySpecificSteps()` helper
- Implement `validatePlatform()` in `executor.go`
- Implement `validateResourceLimits()` in `executor.go`

### Step 3: Enable RecipeLoader and remove workarounds (ATOMIC)
- Add `RecipeLoader` field to `planRetrievalConfig` struct
- Thread field through to `executor.PlanConfig`
- Pass `loader` when constructing config
- Delete `ensurePackageManagersForRecipe()` call (line 392)
- Delete `ensurePackageManagersForRecipe()` function (lines 143-195)
- Delete `findDependencyBinPath()` helper (lines 197-223)
- Delete `SetResolvedDeps()` workaround (lines 451-475)

**Note**: Steps must be done atomically in single commit to avoid broken intermediate state.

### Step 4: Update plan generation to use platform detection
- Call `detectPlatform()` at start of `GeneratePlan()`
- Populate `plan.Platform` field
- Update dependencies to inherit platform

### Step 5: Update plan execution to validate platform
- Call `validatePlatform()` at start of `ExecutePlan()`
- Call `validateResourceLimits()` at start of `ExecutePlan()`

## Testing Strategy

### Unit Tests
- Test `detectPlatform()` with and without family-specific steps
- Test `recipeHasFamilySpecificSteps()` detection logic
- Test `validatePlatform()` with matching/mismatching platforms
- Test `validateResourceLimits()` with valid/invalid trees

### Integration Tests
- Test `tsuku eval ninja` produces plan with dependencies
- Test `tsuku install ninja` uses same plan structure
- Test `tsuku install --plan plan.json` works in sandbox
- Test platform mismatch is detected and fails fast

### Acceptance Criteria Validation
- [ ] `tsuku install ninja --sandbox` completes successfully
- [ ] Tools using cmake_build, configure_make, meson_build work in sandbox mode
- [ ] Behavior matches normal install mode (implicit dependencies available)
- [ ] Multi-family sandbox tests can run for tools with build actions

## Risks and Mitigations

**Risk**: Breaking existing plans without dependencies
**Mitigation**: Plans without dependencies will execute with warning (pre-GA, acceptable breakage)

**Risk**: Platform detection failures
**Mitigation**: Detection failures emit warning but continue (graceful degradation)

**Risk**: Performance impact from dependency resolution
**Mitigation**: Plan caching already exists, redundant state checks are cheap (~10ms)

## Estimated Complexity

- Code changes: Medium (~120 lines added, ~109 lines deleted)
- Testing: Medium (new validation functions need coverage)
- Risk: Low (reusing existing format v3 infrastructure)
- Time estimate: 3-5 hours

## Success Criteria

- All tests pass (no regressions from baseline)
- Build succeeds without warnings
- Sandbox tests work for recipes with build actions
- Platform validation catches mismatches
- Code follows project conventions (gofmt, golangci-lint)
