# Issue 1612 Baseline

## Environment
- Date: 2026-02-11
- Branch: feature/1612-fork-detection
- Base commit: d3f0d475 (main)

## Test Results
- Total: 27 packages
- Passed: 25 packages
- Failed: 2 packages (pre-existing)

## Pre-existing Failures
1. `internal/sandbox` - Container/Dockerfile issues (unrelated to discovery)
   - TestSandboxIntegration_SystemDependencies
   - TestSandboxIntegration_WithRepository
   - TestSandboxIntegration_ContainerCaching
   - TestSandboxIntegration

2. `internal/validate` - External dependency issue
   - TestEvalPlanCacheFlow - GitHub file download returns 404

## Build Status
Pass - `go build ./cmd/tsuku` succeeds

## Relevant Tests
The following tests in internal/discover will be affected:
- TestVerifyGitHubRepo - needs fork detection
- TestLLMDiscovery_Integration - should handle forks

## Files to Modify
- internal/discover/llm_discovery.go - add fork detection logic
- internal/discover/resolver.go - add fork fields to Metadata
- internal/discover/llm_discovery_test.go - add fork tests
