# Issue 1574 Baseline

## Environment
- Date: 2026-02-09
- Branch: feat/recipe-driven-ci-deps (shared with #1573)
- Base commit: ddf59d040a13044c45998855e52d882129bc795b

## Test Results
- Total: 26 packages
- Passed: 24 packages
- Failed: 2 packages (pre-existing)

## Build Status
Pass - no warnings

## Pre-existing Failures

These failures existed before starting work on #1574:

### internal/sandbox
- TestSandboxIntegration_SystemDependencies
- TestSandboxIntegration_WithRepository
- TestSandboxIntegration_ContainerCaching
- TestSandboxIntegration (and sub-tests)

Cause: Sandbox tests require container runtime and specific test fixtures.

### internal/validate
- TestEvalPlanCacheFlow

Cause: GitHub API rate limiting or network issues (404 on github_file download).

## Notes

This issue adds a shell script in `.github/scripts/`. No Go code changes expected, so test results should remain unchanged after implementation.
