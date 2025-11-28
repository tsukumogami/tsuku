# Issues 6, 8, 9 Baseline

## Issues Summary

### Issue #6: Separate integration/e2e tests using build tags
- Unit tests should run fast with `go test ./...`
- Integration tests should run with `go test -tags=integration ./...`
- Use `//go:build integration` for tests that make network requests

### Issue #8: Enable race detection in CI tests
- Run tests with `-race` flag in CI
- **Status**: Already implemented in `.github/workflows/test.yml` (line 28)

### Issue #9: Add explicit golangci-lint configuration
- Create `.golangci.yaml` with v2 format
- Explicitly enable linters (no defaults)
- Document rationale

## Environment
- Date: 2025-11-28
- Branch: chore/6-8-9-ci-testing-improvements
- Base commit: e22def814425daa27267ceb01bf3579e70138f87

## Test Results
- Total test files: 20
- Test cases: 179
- Passed: 179
- Failed: 0
- Skipped: 1 (TestResponseSizeLimit - too slow)

## Build Status
- `go build ./...`: PASS (no warnings)
- `go vet ./...`: PASS

## Coverage
- Overall: 30.5%
- Command used: `go test -coverprofile=coverage.out ./...`

## Current CI Configuration
- File: `.github/workflows/test.yml`
- Race detection: Enabled (`-race` flag on line 28)
- golangci-lint: Runs via lint_test.go (downloads latest)
- No `.golangci.yaml` configuration file exists

## Current Test Structure
- All tests run with `go test ./...`
- Integration tests in CI use `test-matrix.json` to run tool installations
- lint_test.go uses `testing.Short()` to skip linting in short mode
- No build tags used for integration vs unit test separation

## Pre-existing Issues
- No `.golangci.yaml` file - using defaults
- Integration tests (tool installations) run via GitHub workflow, not Go build tags
- Issue #8 already resolved - race detection enabled in CI
