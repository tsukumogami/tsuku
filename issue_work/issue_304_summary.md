# Issue 304 Summary

## What Was Implemented

A `LockManager` using POSIX advisory file locks (`flock`) to prevent interference between parallel tsuku validation processes.

## Changes Made

- `internal/validate/lock.go`: New file with LockManager, Lock, and LockMetadata types
- `internal/validate/lock_test.go`: 9 unit tests including concurrent access with race detector

## Key Decisions

- **flock over fcntl**: Simpler API, automatically releases on process termination
- **JSON metadata in lock files**: Enables debugging by showing which process holds the lock
- **Non-blocking mode support**: Allows caller to decide whether to wait or fail fast

## Trade-offs Accepted

- **Linux-only flock**: Acceptable since validation containers only target Linux
- **Process-level locking**: Does not prevent multiple goroutines in same process from acquiring same lock (handled by caller)

## Test Coverage

- New tests added: 9
- All tests pass with race detector enabled

## Known Limitations

- Lock files remain if process crashes before cleanup (TryCleanupStale handles this)
- flock does not work across NFS (acceptable for local temp directories)

## Future Improvements

- Integration with issue #307 (startup cleanup for orphaned containers) will use TryCleanupStale()
