# Issue #19 Baseline

## Issue
feat(cli): define distinct exit codes for different error types

## Branch
feature/19-exit-codes (from main at 4b96e32)

## Test Results
All 16 packages pass.

## Exit Code Requirements
Per issue, define specific exit codes:

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid arguments / usage error |
| 3 | Recipe not found |
| 4 | Version not found |
| 5 | Network error |
| 6 | Installation failed |
| 7 | Verification failed |
| 8 | Dependency resolution failed |

## Current State
All errors currently use `os.Exit(1)`.
