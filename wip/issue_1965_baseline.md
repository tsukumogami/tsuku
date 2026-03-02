# Issue 1965 Baseline

## Environment
- Date: 2026-03-01
- Branch: fix/1965-macos-homebrew-rpath
- Base commit: 46e416b728a68216f04bf3e4dc49b5fccffeaa0c

## Test Results
- Passed packages: 34
- Failed packages: 5 (pre-existing, not related to this issue)

## Pre-existing Failures
- `internal/verify`: macOS `/private/var` vs `/var` symlink resolution in rpath tests
- `internal/platform`: Build failure — `isDisplayController` undefined in gpu_test.go
- `internal/actions`: Pre-existing failures
- Root package and `cmd/tsuku`: Pre-existing failures

## Build Status
- `go build -o tsuku ./cmd/tsuku`: Pass (no warnings)
