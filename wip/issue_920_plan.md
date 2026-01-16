# Issue 920 Implementation Plan

## Problem Summary
When generating installation plans on Linux (CI), the planner incorrectly includes Linux-specific action dependencies (like `patchelf`) in plans targeting Darwin. This causes golden file mismatches between local generation (macOS) and CI validation (Linux).

## Root Cause Analysis

After investigating the codebase, the bug has **two locations** where `runtime.GOOS` is used instead of the target OS:

### Location 1: `generateDependencyPlans` (plan_generator.go:632)
```go
func generateDependencyPlans(...) ([]DependencyPlan, error) {
    // Resolve direct dependencies from recipe
    deps := actions.ResolveDependencies(r)  // BUG: Uses runtime.GOOS
    ...
}
```

This function calls `ResolveDependencies()` which internally uses `runtime.GOOS` via:
```go
func ResolveDependencies(r *recipe.Recipe) ResolvedDeps {
    return ResolveDependenciesForPlatform(r, runtime.GOOS)  // Line 57
}
```

### Location 2: `resolveTransitiveSet` (resolver.go:370)
```go
func resolveTransitiveSet(...) error {
    ...
    // Resolve the dependency's own dependencies
    depDeps := ResolveDependencies(depRecipe)  // BUG: Uses runtime.GOOS
    ...
}
```

When resolving transitive dependencies, the code calls `ResolveDependencies()` instead of `ResolveDependenciesForPlatform()`, causing nested dependencies to be resolved for the host OS instead of the target OS.

### Why This Matters
- `HomebrewRelocateAction.Dependencies()` returns `LinuxInstallTime: []string{"patchelf"}`
- On Darwin, this should be filtered out by `getPlatformInstallDeps(deps, "darwin")`
- But when running on Linux CI, `ResolveDependencies()` uses `runtime.GOOS = "linux"`, so `patchelf` is included
- This propagates through nested dependencies (e.g., libcurl -> brotli -> homebrew -> homebrew_relocate -> patchelf)

## Solution Design

The fix requires passing the target OS through the dependency resolution chain:

1. **Create a new `ResolveTransitiveForPlatform` function** that accepts target OS
2. **Update `ResolveTransitive`** to call the new function with `runtime.GOOS` for backward compatibility
3. **Update `resolveTransitiveSet`** to use `ResolveDependenciesForPlatform` instead of `ResolveDependencies`
4. **Update `generateDependencyPlans`** to call `ResolveDependenciesForPlatform` with the target OS from config

## Files to Modify

### 1. `internal/actions/resolver.go`

**Line 270-318 (ResolveTransitive function):**
- Add new `ResolveTransitiveForPlatform` function that takes `targetOS string` parameter
- Update `ResolveTransitive` to call `ResolveTransitiveForPlatform(ctx, loader, deps, rootName, runtime.GOOS)`

**Line 325-412 (resolveTransitiveSet function):**
- Add `targetOS string` parameter to function signature
- Line 370: Change `ResolveDependencies(depRecipe)` to `ResolveDependenciesForPlatform(depRecipe, targetOS)`
- Line 405: Pass `targetOS` to recursive call

### 2. `internal/executor/plan_generator.go`

**Line 622-664 (generateDependencyPlans function):**
- Line 632: Change `actions.ResolveDependencies(r)` to `actions.ResolveDependenciesForPlatform(r, cfg.OS)`
- Need to handle case where `cfg.OS` is empty (default to `runtime.GOOS`)

**Line 668-731 (generateSingleDependencyPlan function):**
- Line 683: Ensure recursive call passes `cfg` which contains target OS

## Implementation Steps

1. [x] **Add `ResolveTransitiveForPlatform` function in `resolver.go`:**
   - Copy signature from `ResolveTransitive`
   - Add `targetOS string` parameter
   - Update both calls to `resolveTransitiveSet` to pass `targetOS`

2. [x] **Update `ResolveTransitive` for backward compatibility:**
   - Make it a thin wrapper that calls `ResolveTransitiveForPlatform` with `runtime.GOOS`

3. [x] **Update `resolveTransitiveSet` function signature:**
   - Add `targetOS string` parameter after `forRuntime bool`
   - Update line 370: `ResolveDependencies(depRecipe)` -> `ResolveDependenciesForPlatform(depRecipe, targetOS)`
   - Update line 405: Add `targetOS` argument to recursive call
   - Also fixed: merge recursive results back into main deps map

4. [x] **Update `generateDependencyPlans` in `plan_generator.go`:**
   - Determine effective target OS: `targetOS := cfg.OS; if targetOS == "" { targetOS = runtime.GOOS }`
   - Change line 632: `deps := actions.ResolveDependenciesForPlatform(r, targetOS)`

5. [x] **Write tests:**
   - Add test for `ResolveTransitiveForPlatform` with cross-platform scenario
   - Test that darwin plan generated on linux excludes `patchelf`
   - Test that linux plan generated on darwin includes `patchelf`
   - Added 4 new tests covering platform filtering in transitive deps

6. [x] **Update golden files:**
   - Remove workaround exclusions for libcurl and ncurses from CI
   - Regenerate affected golden files

## Testing Strategy

### Unit Tests
1. **Test `ResolveTransitiveForPlatform`:**
   ```go
   func TestResolveTransitiveForPlatform_CrossPlatform(t *testing.T) {
       // Create mock loader with homebrew_relocate dependency
       // Verify darwin target excludes patchelf
       // Verify linux target includes patchelf
   }
   ```

2. **Test `generateDependencyPlans` with explicit OS:**
   - Mock a recipe with homebrew action
   - Generate plan with `cfg.OS = "darwin"` on a Linux machine
   - Verify nested dependencies don't include `patchelf`

### Integration Tests
1. Run `tsuku plan libcurl --os darwin` on Linux CI
2. Verify output matches macOS-generated golden file
3. Run full golden file validation for previously excluded recipes

### Manual Verification
1. Build tsuku
2. Generate plan for libcurl targeting darwin: `./tsuku plan libcurl -t darwin-arm64`
3. Verify output contains no `patchelf` dependency

## Risks and Considerations

### Risk 1: Other callers of `ResolveDependencies`
- `cmd/tsuku/info.go:90` - Uses runtime.GOOS (acceptable, user runs info on their platform)
- `cmd/tsuku/install_deps.go:471,558` - Uses runtime.GOOS (acceptable, installing on current platform)
- `cmd/tsuku/check_deps.go:77` - Uses runtime.GOOS (acceptable, checking current platform)

These callers are correct as they operate on the current host platform, not a target platform.

### Risk 2: Breaking backward compatibility
- `ResolveTransitive` is exported and might be used by external code
- Solution: Keep `ResolveTransitive` with original signature as wrapper

### Risk 3: Empty `cfg.OS` in `generateDependencyPlans`
- Must handle gracefully with fallback to `runtime.GOOS`
- Already done in `GeneratePlan` at line 57-59, but `generateDependencyPlans` receives the original config
- Solution: Normalize OS before calling or handle in function

### Risk 4: Test file changes
- `resolver_test.go` uses `mockLoader` which doesn't set platform deps
- Need to add tests with `testPlatformAction` or similar mock action
