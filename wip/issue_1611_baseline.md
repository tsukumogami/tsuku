# Issue 1611 Baseline

## Environment
- Date: 2026-02-11
- Branch: feature/1611-html-stripping-url-validation
- Base commit: b6f05cb4b22ecc7c4744a6424ba568fd912f04ab

## Test Results
- Packages: 33 total
- Passed: 28
- Failed: 5 (pre-existing, unrelated to this issue)

## Build Status
Pass - no warnings

## Pre-existing Issues

### internal/sandbox (4 failing tests)
Docker/PPA integration issues with debian:bookworm-slim container:
- TestSandboxIntegration_SystemDependencies
- TestSandboxIntegration_WithRepository
- TestSandboxIntegration_ContainerCaching
- TestSandboxIntegration

Root cause: `add-apt-repository` fails in container due to Launchpad API issues.

### internal/validate (1 failing test)
- TestEvalPlanCacheFlow: Network dependency issue (404 from GitHub file download)

These failures are unrelated to HTML stripping and URL validation work.
