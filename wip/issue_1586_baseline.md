# Issue 1586 Baseline

## Environment
- Date: 2026-02-09
- Branch: docs/plan-hash-removal (continuing from issue #1585)
- Base commit: c959f00b787f3a5c0f2d43c62d7baec67881d66e

## Test Results
- Total packages: 28
- Passed: 26
- Failed: 2

### Failing packages (pre-existing, infrastructure-related):

1. `internal/sandbox` - 4 failing tests
   - TestSandboxIntegration_SystemDependencies
   - TestSandboxIntegration_WithRepository
   - TestSandboxIntegration_ContainerCaching
   - TestSandboxIntegration
   - Cause: Container build failures (PPA issues in Dockerfile, unrelated to this work)

2. `internal/validate` - 1 failing test
   - TestEvalPlanCacheFlow
   - Cause: Network error (404 Not Found fetching github_file, transient infrastructure issue)

## Build Status
Pass - `go build ./...` succeeds without errors

## Pre-existing Issues
- Sandbox integration tests require container runtime and are failing due to Dockerfile PPA issues (deadsnakes PPA unavailable)
- Validate integration test failing due to transient GitHub API 404 error

## Notes
This baseline continues work from issue #1585 (RecipeHash removal). The branch already contains:
- RecipeHash field removed from PlanCacheKey, ResolvedStep, and install.PlanStep
- PlanFormatVersion bumped to 4
- validate-golden.sh updated to strip format_version and recipe_hash recursively
