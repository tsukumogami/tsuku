# Issue 551 Implementation Plan

## Summary
Create `setup_build_env` action that explicitly configures build environment variables by leveraging the enhanced `buildAutotoolsEnv()` function from issue #550.

## Approach
The action is essentially a wrapper that:
1. Calls `buildAutotoolsEnv(ctx)` to get the environment with all paths configured
2. Prints informative messages about what environment variables are being set
3. No parameters needed - uses ctx.Dependencies automatically

### Why This Approach
- **Reuses existing logic**: `buildAutotoolsEnv()` already does all the path construction
- **Explicit control**: Recipes can call this action when they need build env setup without running configure_make
- **Informative**: Shows users what environment is being configured
- **Simple**: Minimal code since the heavy lifting is done by buildAutotoolsEnv()

### Alternatives Considered
- **Duplicate buildAutotoolsEnv logic**: Would create maintenance burden
- **Make it modify ExecutionContext**: Actions should not mutate context
- **Accept parameters for selective setup**: Unnecessary complexity, always want full setup

## Files to Modify
- `internal/actions/action.go` - Register the new action in init()

## Files to Create
- `internal/actions/setup_build_env.go` - New action implementation
- `internal/actions/setup_build_env_test.go` - Unit tests

## Implementation Steps
- [ ] Create SetupBuildEnvAction struct and basic methods (Name, IsDeterministic)
- [ ] Implement Execute() method that calls buildAutotoolsEnv() and prints info
- [ ] Register action in init()
- [ ] Add unit tests verifying action executes successfully
- [ ] Verify all existing tests still pass

## Testing Strategy
- Unit tests: Verify action calls buildAutotoolsEnv() and completes without error
- Integration tests: Not needed - buildAutotoolsEnv() is already tested, this is just a wrapper
- Manual verification: Not needed - CI will test with real recipes (#553, #554)

## Risks and Mitigations
- **Risk**: Action has no observable side effects (doesn't modify files/state)
  - **Mitigation**: This is by design - env vars affect child processes, not detectable in tests
- **Risk**: Users might expect parameters to control behavior
  - **Mitigation**: Design explicitly states no parameters needed

## Success Criteria
- [ ] SetupBuildEnvAction registered in action registry
- [ ] Action calls buildAutotoolsEnv(ctx) correctly
- [ ] Action prints informative messages about configured environment
- [ ] Unit tests verify execution completes successfully
- [ ] All existing tests continue to pass

## Open Questions
None - implementation is straightforward wrapper around existing functionality.
