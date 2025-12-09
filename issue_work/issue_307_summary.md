# Issue 307 Summary

## What Was Implemented

Implemented startup cleanup functionality that removes orphaned validation artifacts (containers and temp directories) from interrupted tsuku validation runs.

## Changes Made

- `internal/validate/cleanup.go`: New Cleaner struct with:
  - Container cleanup: Lists and removes containers with `tsuku-validate` label that are in exited/dead state
  - Temp directory cleanup: Removes `tsuku-validate-*` directories older than 1 hour
  - Lock-aware: Skips containers that are currently locked by another process
  - Best-effort: Never blocks startup on cleanup failures, logs all actions at debug level

- `internal/validate/cleanup_test.go`: Comprehensive unit tests:
  - Temp directory cleanup with old/new directories
  - Container cleanup with mocked runtime
  - Lock handling (skips locked containers)
  - Error handling (graceful degradation)
  - Options configuration

## Key Decisions

- **Separate Cleaner struct**: Kept cleanup separate from LockManager and RuntimeDetector to maintain single responsibility. The Cleaner orchestrates both components.
- **Best-effort approach**: Cleanup errors are logged but never propagate to callers, ensuring tsuku startup is never blocked.
- **Functional-style options**: Used CleanerOption pattern for configuration (WithLogger, WithTempDir, WithMaxTempDirAge).

## Trade-offs Accepted

- **Container removal via exec**: Rather than extending the Runtime interface, the Cleaner executes `podman/docker rm` directly. This avoids interface changes but means slightly different code paths for running vs removing containers.
- **Time-based temp cleanup**: Uses 1 hour threshold which may leave some directories longer than necessary, but avoids false positives from active validations.

## Test Coverage

- New tests added: 11 test functions
- All tests pass including race detector scenarios

## Known Limitations

- Container cleanup requires the runtime to be available; if no runtime is detected, only temp directories are cleaned.
- Lock check uses file-based locking which is only effective within the same filesystem.

## Future Improvements

- Could add metrics/telemetry for cleanup operations
- Could make the container label prefix configurable
