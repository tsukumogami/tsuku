# Issue 139 Implementation Plan

## Summary

Add defense-in-depth input validation to `ResolveNpm()` by validating the package name before URL construction, matching the pattern already used in `ListNpmVersions()`.

## Approach

The security review identified that `ResolveNpm()` constructs a URL without internal validation. While callers may validate, defense-in-depth requires that each function validate its inputs independently.

The fix is minimal: add `isValidNpmPackageName()` check at the start of `ResolveNpm()`.

### Alternatives Considered
- **Create dedicated buildNpmRegistryURL function**: The issue mentions this, but the current codebase doesn't have such a function. Creating one would be over-engineering for a single use case.
- **Do nothing**: Rejected - violates defense-in-depth principle identified in security audit.

## Files to Modify

- `internal/version/resolver.go` - Add validation to `ResolveNpm()`

## Files to Create

None

## Implementation Steps

- [x] Analyze current validation state (completed in baseline)
- [x] Add `isValidNpmPackageName()` check to `ResolveNpm()`
- [x] Add test for invalid package name in `ResolveNpm()`
- [x] Run all tests and verify

## Testing Strategy

- **Unit tests**: Add test case in npm_test.go to verify `ResolveNpm` rejects invalid package names
- **Existing tests**: All existing tests must pass (regression testing)

## Risks and Mitigations

- **Risk**: Double validation overhead
  - **Mitigation**: Validation is a cheap string check, defense-in-depth is worth the minimal overhead

## Success Criteria

- [x] `isValidSourceName()` already validates custom source names (verified in baseline)
- [x] `ResolveNpm()` validates package name before URL construction
- [x] All tests pass
- [x] Build succeeds

## Open Questions

None - the fix is well-defined from the security review.
