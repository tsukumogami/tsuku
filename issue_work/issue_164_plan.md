# Issue 164 Implementation Plan

## Summary

Add an optional `module` parameter to cpan_install action that allows specifying the module name directly when it differs from the distribution name.

## Approach

When `module` parameter is provided, use it directly for cpanm instead of converting from distribution name. This handles non-standard naming like `ack` distribution containing `App::Ack` module.

### Why this approach
- Backward compatible - existing recipes without `module` continue to work
- Explicit - recipe authors can override when needed
- Simple - just one additional parameter check

### Alternatives Considered
- **Put module in version section**: Would require changes to version provider and is less intuitive since module is an installation concern, not a version concern
- **Auto-detect via MetaCPAN API**: Adds network dependency and complexity; explicit is better

## Files to Modify
- `internal/actions/cpan_install.go` - Add optional `module` parameter handling and validation

## Implementation Steps
- [x] Add `isValidModuleName()` validation function
- [x] Add logic to use `module` parameter if provided, otherwise convert from distribution
- [x] Update function docs to document the new parameter
- [x] Add unit tests for module parameter handling
- [x] Verify all tests pass

Mark each step [x] after it is implemented and committed.

## Testing Strategy
- Unit tests: Test `isValidModuleName()` with valid/invalid inputs
- Unit tests: Test Execute() uses module when provided vs converts distribution
- Existing tests should continue to pass

## Risks and Mitigations
- **Validation bypass**: Module names use `::` which distribution validation rejects - add separate module validation

## Success Criteria
- [ ] cpan_install accepts optional `module` parameter
- [ ] When `module` is provided, it's used directly for cpanm
- [ ] When `module` is not provided, distribution name is converted (existing behavior)
- [ ] Module names are validated to prevent injection
- [ ] All existing tests pass
- [ ] New module-related tests pass
