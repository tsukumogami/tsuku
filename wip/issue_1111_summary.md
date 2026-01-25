# Issue 1111 Implementation Summary

## What Was Implemented

Added step-level dependency resolution that filters dependencies based on whether a step matches the target platform. This enables recipes to declare platform-specific dependencies that are only resolved when the corresponding step runs.

## Key Implementation Decisions

1. **New struct field vs Params**: Added explicit `Dependencies []string` field to Step struct rather than relying solely on `Params["dependencies"]`. This makes step dependencies a first-class concept and allows TOML to be round-tripped correctly.

2. **New function for target filtering**: Created `ResolveDependenciesForTarget()` that accepts a `Matchable` target parameter. The existing `ResolveDependenciesForPlatform()` calls this with `nil` target for backward compatibility (all steps contribute).

3. **Struct field takes precedence**: When both `Step.Dependencies` and `Params["dependencies"]` exist, the struct field wins. This ensures explicit dependencies from TOML parsing override any Params-based legacy approach.

4. **Nil target = backward compatible**: When target is nil, all steps contribute their dependencies regardless of When clauses. This preserves existing behavior for code that calls `ResolveDependencies()` or `ResolveDependenciesForPlatform()`.

## Files Modified

- `internal/recipe/types.go` - Added Dependencies field to Step, updated UnmarshalTOML and ToMap
- `internal/actions/resolver.go` - Added ResolveDependenciesForTarget with step filtering logic
- `internal/recipe/types_test.go` - Tests for Dependencies parsing and serialization
- `internal/actions/resolver_test.go` - Tests for target-filtered dependency resolution

## Test Coverage

- 4 test cases for TOML parsing of Dependencies field
- 2 test cases for ToMap serialization
- 7 test cases for resolver target filtering:
  - Matching When clause resolves deps
  - Non-matching When clause skips deps
  - Multiple steps with selective matching
  - Nil target processes all steps
  - Step with no When always matches
  - Struct field takes precedence over Params
  - Dependencies with version constraints

## Acceptance Criteria Status

All acceptance criteria from the issue are met:
- Dependencies []string field added to Step struct
- TOML parsing handles step-level dependencies
- Dependency resolver filters by When.Matches(target)
- Backward compatible (nil target = all steps)
- Struct field takes precedence over Params
- Comprehensive unit tests
