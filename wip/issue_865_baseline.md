# Issue 865 Baseline

## Environment
- Date: 2026-01-14
- Branch: feat/865-cask-symlinks
- Base commit: 9eca6b871a995efa12aa82455eee3e510f74f6b6

## Test Results
- Packages: 29
- Passed: 26
- Failed: 3 (pre-existing)

### Pre-existing Failures

1. **internal/actions** - Symlink-related tests fail on macOS due to `/private/var` vs `/var` path resolution differences (TestCreateSymlink, TestContainsSymlink, TestDownloadCache_*)

2. **internal/sandbox** - Container integration tests fail due to Docker architecture mismatch and external PPA availability (TestSandboxIntegration)

3. **internal/validate** - Integration test fails due to external URL returning 404 (TestEvalPlanCacheFlow)

## Build Status
- `go build -o tsuku ./cmd/tsuku`: PASS

## Coverage
Not tracked in baseline (will compare if needed).

## Notes
The pre-existing failures are environment-specific (macOS symlink resolution, Docker on macOS, external URL availability) and not related to issue #865 work.
