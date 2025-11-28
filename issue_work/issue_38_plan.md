# Issue 38 Implementation Plan

## Summary

Add unit tests for `internal/actions` and `internal/executor` packages to achieve 50% overall coverage, then update codecov.yml to enforce the new threshold.

## Approach

Focus on testing pure functions and utility code that doesn't require network calls or complex external dependencies. The strategy prioritizes:
1. Utility functions in `internal/actions/util.go` (high impact, easy to test)
2. Security-critical functions in `internal/actions/extract.go` (path validation)
3. Executor utility methods and edge cases
4. Update codecov.yml threshold last (ensures tests pass first)

### Alternatives Considered
- **Test with mocks for HTTP calls**: More comprehensive but increases complexity significantly. Not needed to hit 50%.
- **Add integration tests**: Already have integration tests via test-all-recipes.sh; unit tests provide better value for coverage metrics.

## Files to Modify
- `internal/actions/util_test.go` - Add tests for utility functions
- `internal/actions/extract_test.go` - Add tests for extraction security functions (new file)
- `internal/executor/executor_test.go` - Expand existing tests
- `codecov.yml` - Update target from 30% to 50%

## Files to Create
- `internal/actions/extract_test.go` - Tests for extract.go security functions

## Implementation Steps
- [ ] Add tests for `internal/actions/util.go` utility functions
  - ExpandVars, GetStandardVars
  - MapOS, MapArch, ApplyMapping
  - GetInt, GetBool, GetString, GetStringSlice, GetMapStringString
  - VerifyChecksum, ReadChecksumFile
- [ ] Add tests for `internal/actions/extract.go` security functions
  - isPathWithinDirectory
  - validateSymlinkTarget
  - detectFormat
- [ ] Add tests for `internal/executor/executor.go`
  - NewWithVersion
  - shouldExecute (conditional execution)
  - expandVars
  - Version(), WorkDir(), SetExecPaths()
- [ ] Run tests and verify coverage reaches 50%+
- [ ] Update codecov.yml target to 50%

## Testing Strategy
- Unit tests: Test pure functions with various inputs and edge cases
- Security tests: Verify path traversal and symlink attack prevention
- Manual verification: Run `go test -cover ./...` to confirm 50%+ overall

## Risks and Mitigations
- **Risk**: Tests might not reach 50% threshold
  - **Mitigation**: Focus on high-impact functions first, add more tests if needed
- **Risk**: Checksum tests need actual files
  - **Mitigation**: Use t.TempDir() for file-based tests

## Success Criteria
- [ ] Overall test coverage >= 50%
- [ ] `internal/actions` coverage improved from 25.8%
- [ ] `internal/executor` coverage improved from 31.4%
- [ ] codecov.yml target set to 50%
- [ ] All CI checks pass

## Open Questions
None - plan is straightforward.
