# Issue 71 Summary

## What Was Implemented
Removed the `go mod tidy` before-hook from `.goreleaser.yaml` to prevent the release build from embedding `vcs.modified=true` in the binary metadata, which caused version strings to show `+dirty` suffix.

## Changes Made
- `.goreleaser.yaml`: Removed the `before.hooks` section containing `go mod tidy`

## Key Decisions
- Remove hook entirely vs. move to pre-tag workflow: Chose removal because the CI test workflow already enforces `go mod tidy` produces no changes before code can be merged

## Trade-offs Accepted
- Build may fail if `go.sum` is out of date: Acceptable because CI enforces this before merge, making it impossible for stale `go.sum` to reach main branch

## Test Coverage
- New tests added: 0 (config-only change)
- Coverage change: N/A

## Known Limitations
None

## Future Improvements
None needed - the fix is complete
