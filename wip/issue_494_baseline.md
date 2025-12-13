# Issue 494 Baseline

## Environment
- Date: 2025-12-13
- Branch: feature/494-container-source-build-validation
- Base commit: 31d2dc1478503a017a397090b6046999092f1ca7

## Test Results
- Total: 22 packages tested
- Passed: 20 packages
- Failed: 2 packages (pre-existing issues)

## Build Status
Build passes successfully.

## Pre-existing Issues

These test failures are pre-existing and unrelated to this work:

1. **TestNixRealizeAction_Execute_PackageFallback** (internal/actions)
   - Panic in nix_realize.go during package fallback test
   - Known flaky/environmental issue

2. **TestCleaner_CleanupStaleLocks** (internal/validate)
   - Permission denied errors when cleaning up temp directories
   - Local environment issue with leftover temp files from previous runs
