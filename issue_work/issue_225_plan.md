# Issue 225 Implementation Plan

## Summary
Add a check in `installWithDependencies` to reject direct library installation with a helpful error message explaining that libraries are installed automatically as dependencies.

## Approach
When a user runs `tsuku install libyaml` directly (isExplicit=true, parent=""), detect that the recipe is a library type and return an error with guidance. The check is placed after recipe loading but before proceeding with library installation.

### Alternatives Considered
- Check in the cobra command handler before calling install: Rejected because the recipe needs to be loaded first to know if it's a library
- Add a flag to bypass the check for testing: Not needed, the existing code path for dependency installation already handles this correctly

## Files to Modify
- `cmd/tsuku/install.go` - Add check for direct library installation

## Files to Create
None

## Implementation Steps
- [ ] Add check after IsLibrary() detection to reject direct installs
- [ ] Add unit test for the error behavior

## Testing Strategy
- Unit tests: Test that direct library install returns appropriate error
- Manual verification: `tsuku install libyaml` shows helpful error

## Risks and Mitigations
- Risk: Breaking existing dependency installation flow
- Mitigation: Only check when isExplicit=true AND parent="" (direct user install)

## Success Criteria
- [ ] `tsuku install libyaml` returns error with message explaining libraries are auto-installed
- [ ] Dependency installation still works (installing ruby still installs libyaml)
- [ ] All tests pass

## Open Questions
None
