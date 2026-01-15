# Issue 914 Baseline

## Environment
- Date: 2026-01-15
- Branch: docs/version-sorting (reused from design PR #917)
- Base commit: a0bcb99 (origin/main)

## Test Results
- Total: ~50 test packages
- Passed: Most packages pass
- Failed: internal/actions (pre-existing, see below)

## Build Status
Pass - `go build ./cmd/tsuku` succeeds

## Pre-existing Issues
The following tests fail on main (not related to this work):
- TestDownloadCache_* - multiple download cache tests
- TestContainsSymlink - symlink detection tests
- TestCreateSymlink - symlink creation test

These failures exist on origin/main (commit a0bcb99) and are unrelated to version sorting work.

## Relevant Baseline: internal/version
The package we'll be modifying passes all tests:
```
ok  github.com/tsukumogami/tsuku/internal/version  11.470s
```
