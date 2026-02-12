# Issue 1648 Baseline

## Environment
- Date: 2026-02-11
- Branch: docs/disambiguation
- Base commit: bb517594 (chore: fix I1322 diagram class to needsDesign)

## Test Results
- Total packages: 28
- Passed: 26
- Failed: 2 (pre-existing)

### Pre-existing Failures

1. **internal/sandbox** (4 tests failed)
   - TestSandboxIntegration_SystemDependencies
   - TestSandboxIntegration_WithRepository
   - TestSandboxIntegration_ContainerCaching
   - TestSandboxIntegration
   - Cause: Docker container build issues (Debian PPA configuration)

2. **internal/validate** (1 test failed)
   - TestEvalPlanCacheFlow
   - Cause: External dependency (GitHub file download 404)

## Build Status
Pass - `go build ./cmd/tsuku` succeeds

## Pre-existing Issues
These failures are infrastructure-related (Docker/network) and unrelated to disambiguation work:
- Sandbox tests require Docker and encounter Debian PPA issues
- Validate test depends on external GitHub file availability
