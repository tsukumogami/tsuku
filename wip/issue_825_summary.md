# Issue 825 Implementation Summary

## Feature: Integrate Step Analysis into Recipe Loader

### Changes Made

#### 1. `internal/recipe/types.go`
- Added `SetAnalysis(*StepAnalysis)` method to `Step` struct
- Updated `Analysis()` doc comment to reflect that it can be non-nil after `SetAnalysis` is called

#### 2. `internal/recipe/loader.go`
- Added `constraintLookup` field to `Loader` struct
- Added `SetConstraintLookup(ConstraintLookup)` method to configure step analysis
- Modified `parseBytes()` to compute step analysis when constraint lookup is configured
- Modified `ParseFile()` to accept optional `ConstraintLookup` variadic parameter
- Added `computeStepAnalysis()` helper function that iterates steps and calls `ComputeAnalysis`

#### 3. `cmd/tsuku/main.go`
- Added import for `internal/actions`
- Added call to `loader.SetConstraintLookup(defaultConstraintLookup())` after loader creation
- Added `defaultConstraintLookup()` function that:
  - Uses `actions.Get()` to look up actions by name
  - Returns `(nil, false)` for unknown actions
  - For `SystemAction` types, extracts `ImplicitConstraint()` and converts to `recipe.Constraint`
  - Returns `(nil, true)` for known actions without implicit constraints

#### 4. `internal/recipe/loader_test.go`
Added comprehensive tests:
- `TestLoader_SetConstraintLookup` - Verifies setter works correctly
- `TestLoader_StepAnalysisComputation` - Verifies analysis is computed with constraint lookup
- `TestLoader_StepAnalysisSkippedWithoutLookup` - Verifies backward compatibility (nil when no lookup)
- `TestLoader_StepAnalysisWithFamilyVarying` - Verifies {{linux_family}} detection
- `TestLoader_StepAnalysisConflictError` - Verifies constraint conflict errors

### Architecture Notes

The import cycle between `recipe` and `actions` packages was avoided by:
1. Keeping `ConstraintLookup` type definition in `recipe` package (as an interface type)
2. Moving the default implementation to `cmd/tsuku/main.go` (wiring layer)
3. Using dependency injection via `SetConstraintLookup()` method

This approach maintains clean package boundaries while enabling the loader to compute step analysis at load time.

### Test Results
- All existing tests pass
- All new tests pass
- Build succeeds
- go vet passes

### Files Modified
- `cmd/tsuku/main.go` (+31 lines)
- `internal/recipe/loader.go` (+43 lines, -5 lines)
- `internal/recipe/loader_test.go` (+248 lines)
- `internal/recipe/types.go` (+35 lines, renamed field comment)
