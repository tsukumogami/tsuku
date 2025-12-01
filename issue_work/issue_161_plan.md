# Issue 161 Implementation Plan

## Summary

Convert distribution names (Perl-Critic) to module names (Perl::Critic) before calling cpanm, since cpanm only understands module name format.

## Approach

Add a helper function `distributionToModule()` that converts distribution format (hyphens) to module format (double colons). Apply this conversion when building the cpanm install target.

### Why this approach
- Simple and direct - just string replacement
- Works for standard CPAN naming convention (most distributions)
- No network calls needed (unlike fetching tarball URLs from MetaCPAN)

### Alternatives Considered
- **Fetch tarball URL from MetaCPAN**: More accurate but adds network dependency and complexity
- **Accept module names in recipes**: Would require changing recipe format and documentation

## Files to Modify
- `internal/actions/cpan_install.go` - Add conversion when building target

## Implementation Steps
- [ ] Add distributionToModule() helper function
- [ ] Apply conversion to target before calling cpanm
- [ ] Add unit tests for distributionToModule()
- [ ] Test with actual cpanm (manual verification)
- [ ] Verify all tests pass

Mark each step [x] after it is implemented and committed.

## Testing Strategy
- Unit tests: Test distributionToModule() with various inputs
- Existing tests should continue to pass

## Risks and Mitigations
- **Non-standard naming**: Some distributions don't follow convention - documented limitation

## Success Criteria
- [ ] cpanm is called with module names (Perl::Critic@version)
- [ ] All existing tests pass
- [ ] New distributionToModule() tests pass
