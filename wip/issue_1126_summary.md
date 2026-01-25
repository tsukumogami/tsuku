# Issue 1126 Summary

## What Was Implemented
Extracted `checkEvalDepsInDir` helper function that accepts `toolsDir` as a parameter, allowing tests to call it directly without modifying the global TSUKU_HOME env var.

## Changes Made
- `internal/actions/eval_deps.go`: Added `checkEvalDepsInDir(deps, toolsDir)` helper; `CheckEvalDeps` now calls it
- `internal/actions/eval_deps_test.go`: Tests call helper directly with temp directory; removed env var manipulation

## Key Decisions
- Used Option B (refactor) instead of Option A (remove parallel): Keeps tests parallel while eliminating the race

## Trade-offs Accepted
- Added one small unexported function: Minimal complexity, better test isolation

## Test Coverage
- No new tests added (refactored existing tests)
- Coverage unchanged

## Known Limitations
None
