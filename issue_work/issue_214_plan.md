# Issue 214 Implementation Plan

## Summary

Add a `Type` field to `MetadataSection` with valid values "tool" (default) and "library", including validation that rejects unknown types.

## Approach

Add the field to the struct with a TOML tag, define constants for valid values, and add validation in `validateMetadata()`. This follows existing patterns for enum validation (like `validSources`) and constants (like `VerifyModeVersion`).

### Alternatives Considered

- **Custom UnmarshalTOML for MetadataSection**: Rejected - overkill for a simple string field; existing struct TOML parsing works fine
- **No constants, just string literals**: Rejected - constants provide documentation and reusability for future code that checks type

## Files to Modify

- `internal/recipe/types.go` - Add `Type` field to `MetadataSection` and constants for valid types
- `internal/recipe/validator.go` - Add type validation in `validateMetadata()` function
- `internal/recipe/types_test.go` - Add parsing test for type field
- `internal/recipe/validator_test.go` - Add validation tests for type field

## Files to Create

None

## Implementation Steps

- [x] Add `RecipeTypeTool` and `RecipeTypeLibrary` constants to types.go
- [x] Add `Type` field to `MetadataSection` struct
- [x] Add type validation to `validateMetadata()` in validator.go
- [x] Add parsing test for type field in types_test.go
- [x] Add validation tests for valid and invalid type values in validator_test.go

## Testing Strategy

- **Unit tests (parsing)**: Verify TOML unmarshaling correctly populates Type field for "tool", "library", and empty (default)
- **Unit tests (validation)**: Verify validation accepts "tool", "library", and empty; rejects unknown values like "invalid"

## Risks and Mitigations

- **Breaking existing recipes**: Mitigated - empty/missing type defaults to "tool" behavior, so existing recipes continue to work

## Success Criteria

- [x] `type` field added to metadata schema (valid values: "tool", "library"; default: "tool")
- [x] Recipe validation accepts `type = "library"`
- [x] Unit tests for schema validation
- [x] All existing tests pass

## Open Questions

None
