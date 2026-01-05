# Issue 816 Baseline

## Environment
- Date: 2026-01-05
- Branch: fix/816-macos-symlink-checksum-tests
- Base commit: 0aa84177c7463f78e2c5b29934ba14ffdfcfdc33

## Test Results
- Total: ~140 tests in internal/install (all passing except 4 related to issue)
- Failed (issue #816):
  - TestComputeBinaryChecksums
  - TestComputeBinaryChecksums_WithSymlink
  - TestVerifyBinaryChecksums_AllMatch
  - TestVerifyBinaryChecksums_Mismatch

## Pre-existing Failures (unrelated to this issue)
- internal/actions: 11 tests failing (symlink/cache tests - likely same macOS issue)
- internal/sandbox: 4 tests failing (container tests - not on macOS)
- internal/validate: 1 test failing (network test - external URL 404)

## Build Status
Pass - no warnings

## Notes
Tests pass in CI (Linux). The issue is specific to macOS where `/var` is a symlink to `/private/var`. The path comparison logic needs to canonicalize both paths before comparing.
