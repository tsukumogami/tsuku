# Issue 304 Implementation Plan

## Summary

Implement a `LockManager` that uses flock-based file locking to prevent interference between parallel tsuku validation processes, storing locks in `$TSUKU_HOME/validate/locks`.

## Approach

Use `syscall.Flock` for POSIX advisory file locks - matching the existing pattern in `internal/actions/nix_portable.go`. Each container gets its own lock file named by container ID. Lock files contain metadata (container ID, PID, timestamp) for debugging.

### Alternatives Considered
- **Database-based locking**: Overkill for simple coordination between processes
- **PID file locking**: Less robust than flock; race conditions possible

## Files to Modify

None

## Files to Create

- `internal/validate/lock.go` - LockManager implementation with flock-based locking
- `internal/validate/lock_test.go` - Unit tests including concurrent access tests

## Implementation Steps

- [ ] Create LockManager struct with configurable lock directory
- [ ] Implement Acquire() to create lock file and acquire exclusive flock
- [ ] Implement Lock struct with container metadata and Release() method
- [ ] Add ListOrphaned() to find stale locks (for cleanup integration)
- [ ] Write unit tests for basic lock/unlock operations
- [ ] Write concurrent access tests using goroutines

## Testing Strategy

- Unit tests:
  - Basic acquire/release cycle
  - Double acquire returns error (non-blocking mode)
  - Release cleans up lock file
  - ListOrphaned finds stale locks
- Concurrent tests:
  - Multiple goroutines competing for same lock
  - Different locks can be held simultaneously

## Risks and Mitigations

- **flock not available on all filesystems**: Use standard Linux temp directories; validate package already targets Linux environments
- **Stale locks from crashed processes**: flock automatically releases on process exit; ListOrphaned for manual cleanup

## Success Criteria

- [ ] LockManager creates lock files in `$TSUKU_HOME/validate/locks`
- [ ] Uses flock for atomic lock acquisition
- [ ] Lock file contains container ID for debugging
- [ ] Lock released properly (both explicit and via Close)
- [ ] Unit tests for concurrent access pass

## Open Questions

None - design is clear from DESIGN-container-validation-slice-2.md
