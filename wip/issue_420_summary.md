# Issue 420 Implementation Summary

## Goal

Migrate the `internal/validate` package to use the unified `log.Logger` interface from `internal/log`.

## Changes Made

### cleanup.go
- Added import for `github.com/tsukumogami/tsuku/internal/log`
- Removed `CleanupLogger` interface definition (lines 26-29)
- Removed `noopLogger` struct and methods (lines 31-33)
- Changed `Cleaner.logger` field type from `CleanupLogger` to `log.Logger`
- Updated `WithLogger` option to accept `log.Logger` instead of `CleanupLogger`
- Updated `NewCleaner` default logger from `noopLogger{}` to `log.NewNoop()`

### executor.go
- Added import for `github.com/tsukumogami/tsuku/internal/log`
- Removed `ExecutorLogger` interface definition (lines 27-31)
- Removed `noopExecutorLogger` struct and methods (lines 33-37)
- Changed `Executor.logger` field type from `ExecutorLogger` to `log.Logger`
- Updated `WithExecutorLogger` option to accept `log.Logger` instead of `ExecutorLogger`
- Updated `NewExecutor` default logger from `noopExecutorLogger{}` to `log.NewNoop()`

### cleanup_test.go
- Added import for `github.com/tsukumogami/tsuku/internal/log`
- Updated `mockLogger` to implement full `log.Logger` interface:
  - Added `Info`, `Warn`, `Error` methods
  - Added `With` method returning `log.Logger`

### executor_test.go
- Added import for `github.com/tsukumogami/tsuku/internal/log`
- Updated `testLogger` to implement full `log.Logger` interface:
  - Added `infos`, `errors` fields
  - Added `Info`, `Error` methods
  - Added `With` method returning `log.Logger`

## Testing

- Build: PASS
- `internal/log` tests: All pass
- `internal/validate` tests: 1 pre-existing failure (unrelated)
  - `TestCleaner_CleanupStaleLocks` fails due to orphaned temp directories with permission issues on local system

## Acceptance Criteria Status

- [x] `CleanupLogger` interface replaced with `log.Logger` or removed
- [x] `ExecutorLogger` interface replaced with `log.Logger` or removed
- [x] Functional options updated to use `WithLogger(log.Logger)`
- [x] Existing tests updated and passing (pre-existing failure documented)
