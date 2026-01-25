# Issue 1126 Baseline

## Environment
- Date: 2026-01-25
- Branch: fix/1126-parallel-test-race
- Base commit: 3e1399731826dbc5329ccc3984e5525c6829bae3

## Test Results
- internal/actions package: PASS (8.148s)
- Race detection: PASS (9.505s)

Note: The race condition is intermittent. Tests pass locally but have failed sporadically in CI.

## Build Status
- Build: PASS

## Pre-existing Issues
- Race condition in `TestCheckEvalDeps_*` tests when running in parallel
- Each test modifies global `TSUKU_HOME` env var while using `t.Parallel()`
