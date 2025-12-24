# Issue 576 Implementation Plan

## Summary
Add checksum validation for URL-based patches by adding a `Checksum` field to the `Patch` struct and implementing validation that requires checksums for patches loaded from external URLs.

## Approach
This approach adds security validation at the recipe parsing level, ensuring patch integrity before execution. The `apply_patch` action already supports checksum validation at runtime (via the `sha256` parameter), but recipes currently cannot declare checksums in the `[[patches]]` section. This change enables recipe authors to specify checksums and enforces their presence for URL-based patches.

### Alternatives Considered
- **Add validation in apply_patch action only**: Would catch missing checksums at execution time rather than parse time, providing worse error messages and delaying detection until runtime.
- **Make checksum optional with warnings**: Would not provide the security guarantee needed for MITM protection. Patches from external URLs are fixed content that should always be verified.

## Files to Modify
- `internal/recipe/types.go` - Add `Checksum` field to `Patch` struct
- `internal/recipe/validator.go` - Add `validatePatches()` function to enforce checksum requirement for URL-based patches

## Files to Create
None

## Implementation Steps
- [x] Add `Checksum string` field to `Patch` struct in types.go
- [x] Add `validatePatches()` function in validator.go that:
  - Iterates through all patches
  - For patches with `url` field set, requires non-empty `checksum`
  - For patches with `data` field (inline), allows empty checksum
  - Returns clear error messages indicating which patch index is missing checksum
- [x] Call `validatePatches()` from `ValidateBytes()` function
- [x] Update ToTOML() method in types.go to serialize checksum field
- [ ] Update test recipes in testdata/ that use URL patches to include checksums:
  - bash-source.toml (9 patches)
  - readline-source.toml (if applicable)
  - python-source.toml (if applicable)
- [ ] Add unit tests for patch validation
- [ ] Run full test suite to ensure no regressions

## Testing Strategy
- Unit tests in `internal/recipe/validator_test.go`:
  - Test URL patch without checksum → validation error
  - Test URL patch with checksum → validation passes
  - Test inline patch without checksum → validation passes (no error)
  - Test inline patch with checksum → validation passes
  - Test empty patches array → validation passes
  - Test error message includes patch index
- Integration: Existing recipes with patches must be updated with valid checksums
- Manual verification: Run validator against updated test recipes

## Risks and Mitigations
- **Risk**: Existing test recipes will fail validation until checksums are added
  - **Mitigation**: Download each patch file, compute SHA256, update recipes in same commit
- **Risk**: Checksum format validation (should be hex string)
  - **Mitigation**: Validate checksum format (64 hex characters) in validatePatches()

## Success Criteria
- [ ] Patch struct has Checksum field
- [ ] validatePatches() function exists and is called during validation
- [ ] URL patches without checksums produce clear validation errors
- [ ] Inline patches don't require checksums
- [ ] All test recipes pass validation
- [ ] All existing tests pass
- [ ] go vet passes
- [ ] go build succeeds

## Open Questions
None - approach is straightforward
