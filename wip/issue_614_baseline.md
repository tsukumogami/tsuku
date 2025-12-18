# Issue 614 Baseline

## Environment
- Date: 2025-12-18
- Branch: fix/614-strip-dirs-sandbox
- Base commit: 4d1057b

## Test Results
- Total: All package tests
- Passed: Most tests passed
- Failed: 2 pre-existing failures (unrelated to strip_dirs issue)

### Pre-existing Failures
1. **TestSandboxIntegration/simple_binary_install** (internal/sandbox)
   - Error: `unknown flag: --plan`
   - This test is checking for a --plan flag that doesn't exist in install command

2. **TestEvalPlanCacheFlow** (internal/validate)
   - Error: `404 Not Found` when downloading file for checksum
   - Appears to be a flaky test or external resource issue

## Build Status
**PASS** - Build completes successfully

## Pre-existing Issues
- Two test failures documented above are not related to the strip_dirs sandbox issue
- The issue we're fixing affects T16 (golang), T19 (nodejs), T50 (perl) which are not in the regular test suite
- These are sandbox-specific tests run separately via test-matrix.json

## Issue Summary
The strip_dirs parameter is not being applied correctly during plan execution in sandbox mode for download_archive recipes. Files remain at their original archive paths instead of being stripped.

Example:
- Archive structure: `go/bin/go`
- Expected (with `strip_dirs=1`): `bin/go`
- Actual: files remain at `go/bin/go`

This causes chmod failures like: `chmod bin/go: no such file or directory`
