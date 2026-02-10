# Issue 1573 Baseline

## Environment
- Date: 2026-02-09
- Branch: feat/recipe-driven-ci-deps (reusing existing branch per user instruction)
- Base commit: 30b5c6cf7ad04b50ca677dcf158c4c890f74b1c4

## Test Results
- Total: 34 packages tested
- Passed: 32 packages
- Failed: 2 packages (pre-existing failures, unrelated to this issue)

### Pre-existing Test Failures

1. **internal/sandbox** - 4 failing tests:
   - `TestSandboxIntegration_SystemDependencies` - PPA repository failure in Docker build
   - `TestSandboxIntegration_WithRepository` - Same PPA issue
   - `TestSandboxIntegration_ContainerCaching` - Same PPA issue
   - `TestSandboxIntegration/simple_binary_install` - Plan validation error
   - `TestSandboxIntegration/multi_family_filtering` - Incompatible package managers

2. **internal/validate** - 1 failing test:
   - `TestEvalPlanCacheFlow` - GitHub 404 for test fixture

## Build Status
- Build: PASS (no errors or warnings)

## Coverage
Not tracked for baseline (will compare in Phase 5 if needed).

## Pre-existing Issues
The sandbox tests fail due to:
1. External dependency on Launchpad PPA that returns authentication errors
2. Docker container build issues unrelated to this issue's scope
3. GitHub API 404 for a test fixture file

These failures are infrastructure/environment issues, not code bugs. This issue only modifies `cmd/tsuku/info.go` and creates `internal/executor/system_deps.go` - neither affects sandbox or validate packages.
