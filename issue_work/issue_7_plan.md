# Issue 7 Implementation Plan

## Summary
Replace hardcoded `go-version: '1.24'` with `go-version-file: 'go.mod'` in all `actions/setup-go` steps in the CI workflow.

## Approach
Use the `go-version-file` parameter supported by `actions/setup-go@v5` to read the Go version directly from `go.mod`. This is the recommended approach per the issue description and aligns with GitHub Actions best practices.

### Alternatives Considered
- **Keep hardcoded version**: Not chosen because it creates duplication and risk of version drift.
- **Use environment variable**: Not chosen because `go-version-file` is a first-class feature of the action and simpler to maintain.

## Files to Modify
- `.github/workflows/test.yml` - Replace `go-version: '1.24'` with `go-version-file: 'go.mod'` in all 3 setup-go steps

## Files to Create
None

## Implementation Steps
- [x] Update `unit-tests` job setup-go step (line 21)
- [x] Update `integration-linux` job setup-go step (line 71)
- [x] Update `integration-macos` job setup-go step (line 100)

Mark each step [x] after it is implemented and committed. This enables clear resume detection.

## Testing Strategy
- Unit tests: N/A - this is a CI configuration change
- Integration tests: N/A
- Manual verification: Push the branch and observe that CI picks up Go 1.24.0 from go.mod

## Risks and Mitigations
- **Risk**: `go-version-file` might not work as expected with the action version
  - **Mitigation**: `actions/setup-go@v5` fully supports `go-version-file` - this is a well-documented feature

## Success Criteria
- [ ] All 3 instances of hardcoded `go-version` replaced with `go-version-file`
- [ ] CI workflow passes on the PR
- [ ] Go version in CI matches go.mod specification

## Open Questions
None - the solution is straightforward and well-documented by GitHub Actions.
