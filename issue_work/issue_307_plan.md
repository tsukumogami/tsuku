# Issue 307 Implementation Plan

## Summary
Implement startup cleanup to remove orphaned containers and temp directories from interrupted validation runs.

## Approach
Create a `Cleaner` struct in `internal/validate/cleanup.go` that:
1. Uses `RuntimeDetector` to find available container runtime
2. Lists containers with `tsuku-validate-` label prefix
3. Removes exited/dead containers (respecting locks)
4. Lists temp directories matching `tsuku-validate-*` in the temp dir
5. Removes directories older than 1 hour
6. Logs all actions at debug level
7. Never blocks startup on cleanup failures (best-effort)

### Alternatives Considered
- **Integrate cleanup into LockManager**: Rejected because container cleanup requires the runtime interface, which is a different concern than lock management.
- **Cleanup at exit instead of startup**: Rejected because if tsuku is killed (SIGKILL), cleanup won't run. Startup cleanup catches orphans from any termination.

## Files to Modify
None - all new code

## Files to Create
- `internal/validate/cleanup.go` - Cleaner implementation
- `internal/validate/cleanup_test.go` - Unit tests

## Implementation Steps
- [ ] Create `Cleaner` struct with dependencies (RuntimeDetector, LockManager)
- [ ] Implement container cleanup that lists and removes orphaned containers
- [ ] Implement temp directory cleanup that removes old `tsuku-validate-*` directories
- [ ] Add `Cleanup()` method that coordinates both cleanups
- [ ] Write unit tests with mocked runtime and filesystem
- [ ] Run linting and tests

## Testing Strategy
- Unit tests: Mock RuntimeDetector and filesystem operations
- Test cases:
  - No containers to clean
  - Container cleanup with locked containers (should skip)
  - Container cleanup with orphaned containers (should remove)
  - Temp directory older than 1 hour (should remove)
  - Temp directory younger than 1 hour (should keep)
  - Runtime unavailable (should skip container cleanup gracefully)
  - Errors don't propagate (best-effort cleanup)

## Risks and Mitigations
- **Risk**: Container removal command varies by runtime
  - **Mitigation**: Use existing Runtime interface or add Remove method
- **Risk**: Listing containers requires runtime-specific commands
  - **Mitigation**: Execute `podman/docker ps -a --filter "label=tsuku-validate"` via exec

## Success Criteria
- [ ] Lists containers with `tsuku-validate` label
- [ ] Removes exited/dead containers respecting locks
- [ ] Removes temp directories older than 1 hour
- [ ] Logs cleanup actions at debug level
- [ ] Does not block startup on cleanup failures

## Open Questions
None - design is clear from the design document
