# Issue 1413 Baseline

## Environment
- Date: 2026-02-05
- Branch: fix/1413-discovery-builder-type-changes
- Base commit: da88e5537fa9231d7057d853b336fcc978eeaffb

## Test Results
- Total: 35 test packages
- Passed: 24 packages
- Failed: 11 packages

### Pre-existing Failures
The following test failures existed before this work began:

1. **TestGovulncheck** - Known vulnerability in crypto/tls@go1.25.6 (needs Go 1.25.7 upgrade)
2. **TestResolveCargo_NoRustInstalled** - Expected failure when Rust not available
3. **TestSandboxIntegration_SystemDependencies** - Sandbox integration test
4. **TestSandboxIntegration_WithRepository** - Sandbox integration test
5. **TestSandboxIntegration_ContainerCaching** - Sandbox integration test
6. **TestSandboxIntegration** - Sandbox integration test suite
7. **TestEvalPlanCacheFlow** - Validation test failure
8. Other test failures in internal/actions, internal/discover, internal/queue packages

## Build Status
Build succeeds without errors or warnings.

## Pre-existing Issues
- Multiple test packages have failing tests that are not related to discovery metadata enrichment
- Tests appear to be environment-dependent (e.g., Rust not installed, sandbox container issues)
