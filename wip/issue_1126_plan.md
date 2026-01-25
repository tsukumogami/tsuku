# Issue 1126 Implementation Plan

## Summary
Remove `t.Parallel()` from three tests that modify the global `TSUKU_HOME` environment variable to eliminate the race condition.

## Approach
Option A from the issue: Remove `t.Parallel()` from the three affected tests. This is the simplest fix that ensures correct behavior.

### Alternatives Considered
- **Option B (refactor CheckEvalDeps)**: Add a test-only variant or parameter to avoid reading from env. Not chosen because it adds complexity to production code for test ergonomics, and the tests are fast enough to run sequentially.

## Files to Modify
- `internal/actions/eval_deps_test.go` - Remove `t.Parallel()` from three test functions

## Files to Create
None

## Implementation Steps
- [ ] Remove `t.Parallel()` from `TestCheckEvalDeps_AllMissing` (line 40)
- [ ] Remove `t.Parallel()` from `TestCheckEvalDeps_SomeInstalled` (line 63)
- [ ] Remove `t.Parallel()` from `TestCheckEvalDeps_AllInstalled` (line 90)
- [ ] Run tests with race detector to verify fix

## Testing Strategy
- Run `go test -race ./internal/actions/...` to verify no race conditions
- Run tests multiple times (`go test -count=10`) to verify consistency

## Risks and Mitigations
- **Risk**: Tests run slightly slower without parallelism
  - **Mitigation**: Impact is negligible (3 tests, <100ms each)

## Success Criteria
- [ ] Tests no longer race on TSUKU_HOME
- [ ] Tests still verify the same behavior
- [ ] `go test -race ./internal/actions/...` passes

## Open Questions
None
