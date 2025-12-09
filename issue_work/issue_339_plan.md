# Issue 339 Implementation Plan

## Summary

Extend `generate-registry.py` to extract `dependencies` and `runtime_dependencies` arrays from TOML metadata and include them in `recipes.json`.

## Approach

Modify the existing `parse_recipe` and `validate_metadata` functions to:
1. Extract dependency arrays from TOML metadata
2. Validate dependency names match `NAME_PATTERN` (`^[a-z0-9-]+$`)
3. Validate each dependency references an existing recipe
4. Bump schema version to "1.1.0"

### Alternatives Considered

- **Separate dependencies.json file**: Rejected per design doc - adds complexity, requires two fetches, and keeping files in sync.
- **Lazy validation (no cross-recipe check)**: Rejected - broken dependency links would cause confusing errors in the UI.

## Files to Modify

- `scripts/generate-registry.py` - Add dependency extraction, validation, and schema bump
- `internal/recipe/recipes/l/libyaml.toml` - Fix pre-existing missing homepage field

## Files to Create

None

## Implementation Steps

- [x] Fix pre-existing libyaml.toml missing homepage
- [ ] Add dependency array validation in `validate_metadata()`
- [ ] Add cross-recipe dependency validation in `main()`
- [ ] Extract dependency fields in `parse_recipe()`
- [ ] Bump `SCHEMA_VERSION` to "1.1.0"
- [ ] Add unit tests for dependency validation

## Testing Strategy

- Manual verification: Run script and check output JSON includes dependency arrays
- Edge cases to verify:
  - Recipe with no dependencies (should have empty arrays)
  - Recipe with `dependencies` only
  - Recipe with `runtime_dependencies` only
  - Recipe with both types
  - Invalid dependency name (should fail validation)
  - Non-existent dependency reference (should fail validation)

## Risks and Mitigations

- **Risk**: Dependencies reference non-existent recipes
  - **Mitigation**: Cross-recipe validation checks all referenced dependencies exist

- **Risk**: Schema version change breaks existing consumers
  - **Mitigation**: New fields are additive and optional; existing consumers can ignore them

## Success Criteria

- [ ] `generate-registry.py` extracts `dependencies` from TOML metadata
- [ ] `generate-registry.py` extracts `runtime_dependencies` from TOML metadata
- [ ] Dependency names validated against `NAME_PATTERN`
- [ ] Each dependency references an existing recipe (validation error if not)
- [ ] `schema_version` bumped to "1.1.0"
- [ ] Fields default to empty array if not present in TOML

## Open Questions

None - requirements are clear from the issue and design document.
