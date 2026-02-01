# Issue 1314 Summary

## What Changed
- Added a guard in `cmd/tsuku/create.go` after builder selection (line ~250)
- When `--deterministic-only` is set and the builder requires LLM, exits immediately with exit code 9 and an actionable error message
- Updated design doc diagram: I1314 → done

## Files Modified
- `cmd/tsuku/create.go` — added 10-line guard block
- `docs/designs/DESIGN-discovery-resolver.md` — diagram status update

## Decisions
- No unit test added: the guard is in `runCreate()` which calls `os.Exit`, making it untestable without process-level tests. The logic is two boolean checks.
- Error message includes tool name, builder name, and source for context.
