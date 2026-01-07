# Issue 822 Summary

## What Was Implemented

Added platform constraint types (`Constraint`, `StepAnalysis`) and interpolation detection (`detectInterpolatedVars`) as the foundation for Linux family-aware golden file support. This is Phase 1 of Milestone 32 (Family-Aware Recipe Analysis).

## Changes Made

- `internal/recipe/types.go`:
  - Added `Constraint` struct with OS, Arch, LinuxFamily fields
  - Added `Clone()` method (nil-safe, returns empty constraint for nil receiver)
  - Added `Validate()` method (rejects LinuxFamily when OS is not "linux" or empty)
  - Added `StepAnalysis` struct combining Constraint with FamilyVarying flag
  - Added `knownVars` slice listing platform-varying interpolation variables
  - Added `interpolationPattern` regex (built dynamically from knownVars)
  - Added `detectInterpolatedVars()` function with recursive scanning
  - Added `detectInterpolatedVarsInto()` helper for recursive map/slice traversal

- `internal/recipe/types_test.go`:
  - Added 14 unit tests covering all acceptance criteria

## Key Decisions

- **Nil-safe methods**: `Clone()` returns `&Constraint{}` for nil receivers and `Validate()` returns nil for nil receivers, following existing patterns in `WhenClause`.
- **Dynamic regex from knownVars**: The interpolation pattern is built from `knownVars` using `strings.Join()` to ensure consistency. Adding a new variable only requires modifying `knownVars`.
- **Pointer semantics for Constraint**: Using `*Constraint` allows nil to clearly represent "unconstrained" (runs anywhere), which is the common case.

## Trade-offs Accepted

- **Regex compilation at init time**: The regex is compiled once at package init rather than lazily. This is acceptable because the pattern is static and small.
- **No helper for checking knownVars membership**: The function `detectInterpolatedVars` uses regex directly rather than checking against the slice. This is simpler and the regex already encodes the same information.

## Test Coverage

- New tests added: 14
- Coverage: All specified test cases from issue implemented

## Known Limitations

- `detectInterpolatedVars` only handles `string`, `map[string]interface{}`, and `[]interface{}` types. Other types (structs, typed slices) are ignored. This is intentional as step parameters use these interface types.

## Future Improvements

- Issue #823 will use `Constraint` type for WhenClause merge semantics
- Issue #824 will use `StepAnalysis`, `Constraint`, and `detectInterpolatedVars` for step analysis computation
