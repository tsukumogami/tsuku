# Issue 1095 Baseline

## Environment
- Date: 2026-01-25
- Branch: feature/1095-post-merge-golden-workflow
- Base commit: ca7f6ae4

## Test Results
- Total: 27 packages
- Passed: 23 packages
- Failed: 1 package (github.com/tsukumogami/tsuku - lint test)
- Skipped: 2 packages (no test files)
- No test files: 1 package

## Build Status
- Build: PASS

## Pre-existing Issues

The root package lint test fails due to staticcheck SA5011 warnings in `internal/actions/apt_actions_test.go`:
- Lines 21, 24, 164, 167, 328, 331: nil pointer dereference patterns

This is a pre-existing issue unrelated to this work. All other tests pass.

## Notes

This issue adds a new GitHub Actions workflow file (`.github/workflows/publish-golden-to-r2.yml`). No Go code changes are expected.
