# Issue #984 Summary

## Changes Made

### `internal/recipe/types.go`
- Added `SystemActionChecker` interface (line 834-838)
  - Mirrors the `IsExternallyManaged() bool` method from `SystemAction`
  - Defined locally to avoid circular import with `actions` package

- Added `IsExternallyManagedFor(target Matchable, actionLookup func(string) interface{}) bool` method (line 840-881)
  - Iterates recipe steps, filtering by `When` clause for target platform
  - Uses provided `actionLookup` function to retrieve action implementations
  - Returns `true` if all applicable steps use actions that implement `SystemActionChecker` with `IsExternallyManaged() == true`
  - Returns `false` conservatively for unknown actions or non-SystemAction types

### `internal/recipe/types_test.go`
- Added mock types for testing (lines 2272-2283):
  - `mockExternalAction` - returns `true` from `IsExternallyManaged()`
  - `mockNonExternalAction` - returns `false` from `IsExternallyManaged()`
  - `mockNonSystemAction` - doesn't implement `SystemActionChecker`

- Added 8 comprehensive test cases (lines 2285-2471):
  1. `TestRecipe_IsExternallyManagedFor_AllExternallyManaged` - all external actions return true
  2. `TestRecipe_IsExternallyManagedFor_MixedActions` - mixed actions return false
  3. `TestRecipe_IsExternallyManagedFor_NoSteps` - empty recipe returns true
  4. `TestRecipe_IsExternallyManagedFor_WhenClauseFiltering` - platform filtering works
  5. `TestRecipe_IsExternallyManagedFor_WhenClauseFiltersOut` - non-matching steps filtered
  6. `TestRecipe_IsExternallyManagedFor_UnknownAction` - unknown actions return false
  7. `TestRecipe_IsExternallyManagedFor_NonExternalSystemAction` - non-external SystemAction returns false
  8. `TestRecipe_IsExternallyManagedFor_AllStepsFilteredOut` - all filtered returns true

## Design Decisions

1. **Action lookup function pattern**: Uses `func(string) interface{}` parameter instead of importing `actions.Get` directly to avoid circular dependency between `recipe` and `actions` packages.

2. **Conservative fallback**: Returns `false` for unknown actions (not found in lookup) to ensure we don't skip dependency validation for unrecognized actions.

3. **Empty recipe handling**: Returns `true` for recipes with no steps (or all steps filtered out by `When` clause) because there's nothing to recurse into.

4. **Interface mirroring**: Defined local `SystemActionChecker` interface with just `IsExternallyManaged() bool` rather than the full `SystemAction` interface to minimize coupling.

## Test Results

All tests pass:
```
=== RUN   TestRecipe_IsExternallyManagedFor_AllExternallyManaged
--- PASS
=== RUN   TestRecipe_IsExternallyManagedFor_MixedActions
--- PASS
=== RUN   TestRecipe_IsExternallyManagedFor_NoSteps
--- PASS
=== RUN   TestRecipe_IsExternallyManagedFor_WhenClauseFiltering
--- PASS
=== RUN   TestRecipe_IsExternallyManagedFor_WhenClauseFiltersOut
--- PASS
=== RUN   TestRecipe_IsExternallyManagedFor_UnknownAction
--- PASS
=== RUN   TestRecipe_IsExternallyManagedFor_NonExternalSystemAction
--- PASS
=== RUN   TestRecipe_IsExternallyManagedFor_AllStepsFilteredOut
--- PASS
```

## Integration Point

This method will be called from `internal/verify/deps.go` (Issue #989) like:

```go
func actionLookup(name string) interface{} {
    return actions.Get(name)
}

if recipe.IsExternallyManagedFor(target, actionLookup) {
    // Validate but don't recurse into dependencies
} else {
    // Recursively validate dependencies
}
```
