# Issue 1126 Implementation Plan

## Summary
Extract an unexported `checkEvalDepsInDir` helper function that accepts `toolsDir` as a parameter, allowing tests to call it directly without modifying the global TSUKU_HOME env var.

## Approach
Refactor to separate the "get config" from "do work" concerns. Tests call the internal function directly with their temp directory, keeping parallel execution.

### Alternatives Considered
- **Option A (remove t.Parallel)**: Simple but reduces test parallelism unnecessarily
- **Option B chosen (extract helper)**: Clean separation, keeps tests parallel, no production API change

## Files to Modify
- `internal/actions/eval_deps.go` - Extract `checkEvalDepsInDir` helper
- `internal/actions/eval_deps_test.go` - Use helper instead of setting env var

## Implementation Steps
- [x] Extract `checkEvalDepsInDir(deps, toolsDir)` function
- [x] Have `CheckEvalDeps` call the helper with `GetToolsDir()`
- [x] Update tests to call `checkEvalDepsInDir` directly
- [x] Remove env var manipulation from tests
- [x] Run tests with race detector to verify fix

## Testing Strategy
- Run `go test -race ./internal/actions/...` to verify no race conditions
- Run tests multiple times to verify consistency

## Success Criteria
- [x] Tests no longer race on TSUKU_HOME
- [x] Tests still verify the same behavior
- [x] `go test -race ./internal/actions/...` passes
