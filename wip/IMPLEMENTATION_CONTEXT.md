## Goal

Fix race condition in eval_deps_test.go where parallel tests manipulate the global TSUKU_HOME environment variable.

## Context

During PR #1125, `TestCheckEvalDeps_AllMissing` and `TestCheckEvalDeps_SomeInstalled` failed in CI but passed consistently when run locally (verified with `go test -count=10`). The tests eventually passed on CI re-run.

## Problem

Three tests in `internal/actions/eval_deps_test.go` run with `t.Parallel()` while each modifying the process-global `TSUKU_HOME` environment variable:

- `TestCheckEvalDeps_AllMissing` (line 39)
- `TestCheckEvalDeps_SomeInstalled` (line 62)
- `TestCheckEvalDeps_AllInstalled` (line 89)

Each test:
1. Saves original `TSUKU_HOME`
2. Sets `TSUKU_HOME` to its own temp directory
3. Calls `CheckEvalDeps()` which reads `TSUKU_HOME` via `GetToolsDir()`
4. Restores original value in defer

When running in parallel, test A may set TSUKU_HOME, then test B overwrites it before test A calls `CheckEvalDeps()`, causing test A to look in the wrong directory.

## Acceptance Criteria

- [ ] Tests no longer race on TSUKU_HOME
- [ ] Tests still verify the same behavior
- [ ] `go test -race ./internal/actions/...` passes

## Suggested Fix

Option A: Remove `t.Parallel()` from these three tests (simplest)

Option B: Refactor `CheckEvalDeps` to accept toolsDir as parameter, or add a test-only variant that doesn't read from env

## Dependencies

None
