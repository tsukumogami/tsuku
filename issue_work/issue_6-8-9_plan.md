# Issues 6, 8, 9 Implementation Plan

## Summary

Add explicit golangci-lint v2 configuration and clarify test separation. Issue #8 is already resolved.

## Approach

### Issue #6 Analysis: Integration Test Separation

After codebase analysis, the Go tests are already unit tests:
- All tests using `net/http` use `httptest.NewServer()` (mocked servers)
- No Go tests make actual network requests
- "Integration tests" (tool installations) run via GitHub workflow, not Go test

The `lint_test.go` already uses `testing.Short()` to skip slow linter tests.

**Decision**: No build tags needed. The current structure (unit tests + workflow-based integration tests) already meets the issue requirements. Will close issue #6 as already satisfied or add minimal documentation.

### Issue #8 Analysis: Race Detection

Already implemented in `.github/workflows/test.yml` line 28:
```yaml
run: go test -v -race -coverprofile=coverage.out ./...
```

**Decision**: Close issue #8 as already resolved.

### Issue #9: golangci-lint Configuration

Create `.golangci.yaml` with v2 format. The suggested linters from the issue are:
- staticcheck (comprehensive static analysis)
- govet (Go's built-in checks)
- ineffassign (detects ineffective assignments)
- misspell (catches common misspellings)
- unused (finds unused code)
- contextcheck (context usage patterns)

Additional useful linters to consider based on project nature:
- errcheck (error handling)
- gosec (security checks)
- bodyclose (HTTP body close)

### Alternatives Considered
- Use v1 configuration format: Not chosen - v2 is current standard and more maintainable
- Enable all linters: Not chosen - too noisy, better to start with focused set
- Build tags for integration tests: Not needed - tests already separated

## Files to Modify
- None required for issues #6 and #8

## Files to Create
- `.golangci.yaml` - golangci-lint v2 configuration

## Implementation Steps
- [x] Verify issue #8 is already resolved (race detection in CI)
- [x] Verify issue #6 test structure (no network-calling Go tests)
- [ ] Create `.golangci.yaml` with v2 format
- [ ] Run golangci-lint to verify configuration works
- [ ] Fix any lint issues if present

## Testing Strategy
- Run `go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run` to verify config
- Existing test suite should pass
- CI should pass with new configuration

## Risks and Mitigations
- New linters may report existing issues: Start with suggested linters, fix issues as needed
- golangci-lint version in CI may differ: Use v2 format which is current standard

## Success Criteria
- [ ] `.golangci.yaml` created with v2 format
- [ ] golangci-lint runs successfully with explicit configuration
- [ ] All enabled linters documented with rationale
- [ ] CI passes with new configuration

## Open Questions
None - requirements are clear from issues.

## References
- https://golangci-lint.run/docs/configuration/file/
- https://ldez.github.io/blog/2025/03/23/golangci-lint-v2/
