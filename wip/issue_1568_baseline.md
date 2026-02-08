# Issue 1568 Baseline

## Environment
- Date: 2026-02-08T21:00:00Z
- Branch: fix/1568-restore-blocked-tracking
- Base commit: 8f0cf7bd6646d3c998d6074107643289eda20cbc

## Test Results
- Total: ~200 tests (short mode)
- Passed: Most tests pass
- Failed: 3 pre-existing failures (unrelated to this issue):
  - `TestSandboxIntegration/simple_binary_install`: Plan validation error
  - `TestSandboxIntegration/multi_family_filtering`: Package manager incompatibility
  - `TestEvalPlanCacheFlow`: 404 on GitHub download

## Build Status
- `go build ./cmd/tsuku`: Pass
- `go build ./cmd/batch-generate`: Pass

## Pre-existing Issues
- Sandbox integration tests failing due to plan validation issues
- Eval plan cache test failing due to external resource 404
- These are infrastructure/test fixture issues, not code bugs

## Relevant Files for This Issue
- `.github/workflows/batch-generate.yml` - CI failure recording (lines 722-731)
- `internal/batch/results.go` - FailureRecord struct
- `internal/batch/orchestrator.go` - blocked_by handling
- `internal/dashboard/dashboard.go` - dashboard generation
- `data/failures/` - failure data files
