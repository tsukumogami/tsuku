# Issue 919 Implementation Plan

## Summary

The metacpan version provider's `ResolveVersion` method will be updated to normalize both the API-returned versions and user-provided versions by stripping the `v` prefix before comparison, matching the pattern used by other providers in the codebase.

## Approach

The root cause is a version format mismatch: MetaCPAN's API returns versions with a `v` prefix (e.g., `v1.0.35`) while users typically pass versions without the prefix (e.g., `1.0.35`). The fix normalizes versions using the existing `normalizeVersion()` utility function before comparison, ensuring consistent matching regardless of prefix usage.

### Alternatives Considered

1. **Normalize versions in ListMetaCPANVersions instead**: This would strip the `v` prefix when listing versions, making the list show `1.0.35` instead of `v1.0.35`. However, this changes the output of `tsuku versions` which users may already be relying on, and it inconsistently normalizes at the listing layer rather than the resolution layer where the mismatch actually occurs.

2. **Add a `v` prefix to user input if missing**: This would transform `1.0.35` to `v1.0.35` before comparison. However, this is fragile because not all CPAN distributions use the `v` prefix consistently, and it doesn't handle the general normalization case that `normalizeVersion()` already handles.

3. **Check both with and without prefix**: This would try matching both `version` and `v` + `version`. However, this is a special case approach that doesn't leverage the existing normalization infrastructure and could miss other format variations.

## Files to Modify

- `internal/version/provider_metacpan.go` - Update `ResolveVersion` to normalize versions before comparison using `normalizeVersion()`
- `internal/version/metacpan_test.go` - Add test case for version resolution with/without `v` prefix

## Files to Create

None.

## Implementation Steps

- [x] Update `ResolveVersion` in `provider_metacpan.go` to normalize both API versions and user-provided version using `normalizeVersion()` before comparison
- [x] Update the `VersionInfo` return value to use the original API version as `Tag` and normalized version as `Version` (matching GitHub provider pattern)
- [x] Add test case to `metacpan_test.go` for resolving version without `v` prefix when API returns with prefix
- [x] Add test case for resolving version with `v` prefix when API returns with prefix (should still work)
- [x] Run tests to verify fix: `go test ./internal/version/... -run MetaCPAN`
- [x] Manual verification: `./tsuku eval --recipe internal/recipe/recipes/c/carton.toml --os linux --arch amd64 --version 1.0.35`

## Testing Strategy

- **Unit tests**: Add test cases in `metacpan_test.go`:
  - Test `ResolveVersion` with version lacking `v` prefix when API returns versions with `v` prefix
  - Test `ResolveVersion` with version having `v` prefix (backward compatibility)
  - Test fuzzy matching still works (e.g., `1.0` matches `v1.0.35`)

- **Integration tests**: Verify with actual `tsuku eval` command:
  ```bash
  ./tsuku eval --recipe internal/recipe/recipes/c/carton.toml --os linux --arch amd64 --version 1.0.35
  ```
  Should NOT produce the warning "version resolution failed: version X not found for distribution Y, using 'dev'"

- **Manual verification**: Run `tsuku versions carton` and then `tsuku eval` with one of the listed versions (without the `v` prefix) to confirm the fix works end-to-end.

## Risks and Mitigations

- **Risk**: Breaking existing version resolution where users already include the `v` prefix
  - **Mitigation**: The `normalizeVersion()` function handles both cases - it strips the `v` prefix if present, so `v1.0.35` normalized equals `1.0.35` normalized. Tests will verify both cases.

- **Risk**: Other CPAN distributions may not use the `v` prefix consistently
  - **Mitigation**: By normalizing both sides of the comparison, we handle both prefixed and unprefixed versions from the API correctly. The fix is format-agnostic.

## Success Criteria

- [ ] `tsuku eval --recipe internal/recipe/recipes/c/carton.toml --version 1.0.35` resolves to version `1.0.35` (not `dev`)
- [ ] `tsuku versions carton` continues to show versions as returned by API (with `v` prefix)
- [ ] Unit tests pass for version resolution with and without `v` prefix
- [ ] No regression in fuzzy version matching
- [ ] Golden file generation for carton recipe works correctly

## Open Questions

None - the approach is straightforward and follows existing patterns in the codebase.
