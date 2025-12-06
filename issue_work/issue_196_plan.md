# Issue 196 Implementation Plan

## Summary

Add `Mode`, `VersionFormat`, and `Reason` fields to `VerifySection` with corresponding constants and TOML parsing support.

## Approach

Direct extension of existing types. The fields are simple strings with TOML struct tags, following the same pattern as existing `VerifySection` fields.

### Alternatives Considered

- Using an enum type: Not chosen because Go doesn't have native enums and string constants are idiomatic for TOML-parsed values
- Adding a separate VerifyMode struct: Overcomplicated for three simple fields

## Files to Modify

- `internal/recipe/types.go` - Add new fields to `VerifySection` and define constants
- `internal/recipe/types_test.go` - Add tests for parsing new fields

## Files to Create

None

## Implementation Steps

- [x] Add Mode, VersionFormat, Reason fields to VerifySection struct
- [x] Add constants for valid modes (version, output)
- [x] Add constants for valid formats (raw, semver, semver_full, strip_v)
- [x] Add unit tests for parsing recipes with new verify fields

## Testing Strategy

- Unit tests: Parse recipes with each new field, verify defaults, test combinations
- Manual verification: Not needed - TOML parsing is well-established

## Risks and Mitigations

- Risk: Breaking existing recipe parsing
  - Mitigation: Fields are optional with empty string defaults, no breaking change

## Success Criteria

- [ ] `VerifySection` has Mode, VersionFormat, Reason fields with correct TOML tags
- [ ] Constants defined for all valid modes and formats
- [ ] Unit tests pass for parsing recipes with new fields
- [ ] Existing tests continue to pass

## Open Questions

None
