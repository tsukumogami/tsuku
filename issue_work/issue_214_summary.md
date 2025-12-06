# Issue 214 Summary

## What Was Implemented

Added a `type` field to the recipe metadata schema that allows recipes to declare themselves as either tools or libraries. Libraries can be installed to a different location and excluded from user-facing commands.

## Changes Made

- `internal/recipe/types.go`: Added `RecipeTypeTool` and `RecipeTypeLibrary` constants, added `Type` field to `MetadataSection` struct
- `internal/recipe/validator.go`: Added validation in `validateMetadata()` to reject unknown type values
- `internal/recipe/types_test.go`: Added parsing tests for the type field
- `internal/recipe/validator_test.go`: Added validation tests for valid and invalid type values

## Key Decisions

- **Use constants for type values**: Provides documentation and allows type-safe references in future code
- **Empty type defaults to "tool"**: Ensures backward compatibility with existing recipes
- **Validation error for unknown types**: Catches typos early in recipe development

## Trade-offs Accepted

- None significant - this is a simple additive change

## Test Coverage

- New tests added: 2 test functions (8 subtests total)
- Coverage: Maintained (no drop)

## Known Limitations

- The type field only affects validation currently; runtime behavior (install location, CLI filtering) will be implemented in subsequent issues (#215, #225, #226)

## Future Improvements

- None required - implementation is complete for this issue's scope
