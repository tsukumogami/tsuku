# Issue 1843 Implementation Plan

## Summary

Add an `animateDone` channel to the Spinner that `animate()` closes on exit. `Stop()` and `StopWithMessage()` wait on this channel before writing to `s.output`, eliminating the concurrent write race.

## Approach

The issue's suggested fix is the right approach: signal-and-wait rather than mutex-around-writes. This is cleaner because it avoids holding a mutex during I/O and naturally serializes all writes.

### Alternatives Considered
- **Mutex around all `s.output` writes**: Works but holds mutex during I/O, and `animate()` already holds the mutex for `s.message` reads, so the locking pattern gets awkward.
- **sync.Once for stop logic**: Doesn't solve the race — the problem is concurrent writes, not double-stop.

## Files to Modify
- `internal/progress/spinner.go` - Add `animateDone` channel, wait in Stop/StopWithMessage, close in animate

## Implementation Steps
- [x] Add `animateDone` channel to Spinner struct and initialize in NewSpinner
- [x] Close `animateDone` at the end of `animate()`
- [x] Wait on `animateDone` in `Stop()` after closing `done`
- [x] Wait on `animateDone` in `StopWithMessage()` after closing `done`
- [x] Verify with `go test -race`

## Success Criteria
- [ ] `go test -race ./internal/progress/...` passes
- [ ] All existing spinner tests pass
- [ ] Full test suite passes
