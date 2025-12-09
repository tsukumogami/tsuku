# Issue 295 Implementation Summary

## Overview

Added file locking to `StateManager` to prevent `state.json` corruption from concurrent tsuku processes.

## Changes Made

### New Files

1. **`internal/install/filelock.go`**
   - `FileLock` type with `LockShared()`, `LockExclusive()`, and `Unlock()` methods
   - Creates lock file on demand, closes on unlock

2. **`internal/install/filelock_unix.go`**
   - Unix implementation using `flock(2)` via `syscall.Flock`
   - Build tag: `//go:build !windows`

3. **`internal/install/filelock_windows.go`**
   - Windows implementation using `LockFileEx` via `golang.org/x/sys/windows`
   - Build tag: `//go:build windows`

### Modified Files

1. **`internal/install/state.go`**
   - Added `lockPath()` method returning `$TSUKU_HOME/state.json.lock`
   - Modified `Load()` to acquire shared file lock for reading
   - Modified `Save()` to acquire exclusive file lock and use atomic write pattern
   - Modified `UpdateTool()`, `RemoveTool()`, `UpdateLibrary()`, `RemoveLibraryVersion()` to hold exclusive lock for entire read-modify-write cycle
   - Added `loadWithoutLock()` and `saveWithoutLock()` helpers

2. **`internal/install/state_test.go`**
   - Added `TestStateManager_ConcurrentUpdates` - concurrent goroutines modifying different tools
   - Added `TestStateManager_ConcurrentReadWrite` - concurrent readers and writers
   - Added `TestStateManager_lockPath` - verifies lock file path
   - Added `TestStateManager_AtomicWrite_NoPartialState` - verifies temp file cleanup

## Architecture

```
StateManager
├── Load()                    # Shared lock -> read -> unlock
├── Save()                    # Exclusive lock -> atomic write -> unlock
├── UpdateTool()              # Exclusive lock -> read -> modify -> write -> unlock
├── RemoveTool()              # Exclusive lock -> read -> delete -> write -> unlock
├── UpdateLibrary()           # Exclusive lock -> read -> modify -> write -> unlock
└── RemoveLibraryVersion()    # Exclusive lock -> read -> delete -> write -> unlock

FileLock ($TSUKU_HOME/state.json.lock)
├── LockShared()              # Multiple readers allowed
├── LockExclusive()           # Single writer, blocks readers
└── Unlock()                  # Releases lock and closes file
```

## Atomic Write Pattern

1. Marshal state to JSON
2. Acquire exclusive file lock
3. Write to `state.json.tmp`
4. Rename `state.json.tmp` -> `state.json` (atomic on POSIX)
5. Release lock

## Testing

All new tests pass:
- `TestStateManager_ConcurrentUpdates` - 10 goroutines, 10 updates each
- `TestStateManager_ConcurrentReadWrite` - 5 readers, 3 writers, 20 ops each
- `TestStateManager_lockPath`
- `TestStateManager_AtomicWrite_NoPartialState`

## Known Issues

Pre-existing test failures (documented in baseline):
- `TestGolangCILint`
- `TestGoModTidy`
- `TestGovulncheck`

These are unrelated to file locking changes.
