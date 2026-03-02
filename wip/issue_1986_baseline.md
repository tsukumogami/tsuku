# Issue 1986 Baseline

## Environment
- Date: 2026-03-01
- Branch: fix/1986-cargo-extra-deps-env
- Base commit: ca649bc513248ef7a460faad1e13763f9427c5c4

## Test Results
- Total: 41 packages (39 passed, 2 failed)
- Passed: 39
- Failed: 2 (pre-existing, unrelated)
  - `internal/builders`: pre-existing failure
  - `internal/version`: fossil integration test hitting 503 from remote service

## Build Status
Pass (go build ./cmd/tsuku)

## Pre-existing Issues
- `internal/builders` test failure: unrelated to this issue
- `internal/version` fossil timeline provider: remote 503 error (transient infrastructure issue)
