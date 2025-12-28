# Issue 705 Baseline

## Environment
- Date: 2025-12-27
- Branch: docs/metadata-command (reusing existing design branch)
- Base commit: 9419284 (docs: add design for info command enhancements)

## Test Results

Ran: `go test -test.short ./...`

**Pre-existing Failures** (not related to this issue):
1. `TestSandboxIntegration/simple_binary_install` - Docker exec format error (architecture mismatch)
2. `TestEvalPlanCacheFlow` - Network failure (404 on GitHub download)

These are environmental/flaky test failures, not related to the info command enhancement.

**All other tests**: PASS

## Build Status

`go build -o tsuku ./cmd/tsuku`: SUCCESS

## Coverage

Not tracking baseline coverage for this issue (will verify tests added for new functionality).

## Pre-existing Issues

- Sandbox integration tests fail in rootless Docker environments
- Some integration tests have network dependencies that can fail intermittently

## Notes

- Reusing existing branch that has the approved design document
- Will implement the info command enhancements as specified in `docs/DESIGN-info-enhancements.md`
- Design adds two flags to `cmd/tsuku/info.go`: `--recipe` and `--metadata-only`
