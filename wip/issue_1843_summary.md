# Issue 1843 Summary

## What Was Implemented

Added an `animateDone` channel to the Spinner that serializes writes to `s.output`. The `animate()` goroutine closes this channel on exit, and `Stop()`/`StopWithMessage()` wait on it before writing, eliminating the concurrent write race.

## Changes Made
- `internal/progress/spinner.go`: Added `animateDone` channel field, initialized in `NewSpinner`, closed by `animate()` on exit (and by `Start()` in non-TTY path), waited on by `Stop()` and `StopWithMessage()`

## Key Decisions
- Used signal-and-wait (channel) rather than mutex-around-writes: cleaner, avoids holding mutex during I/O, naturally serializes all writes
- Closed `animateDone` in the non-TTY `Start()` path to prevent `Stop()` from blocking when no goroutine was launched

## Test Coverage
- No new tests added (existing tests already cover all paths; the race detector is the validation)
- `go test -race ./internal/progress/...` now passes

## Requirements Mapping

| AC | Status | Evidence / Reason |
|----|--------|-------------------|
| No data race in Stop() | Implemented | `<-s.animateDone` at spinner.go:82 waits for animate to exit |
| No data race in StopWithMessage() | Implemented | `<-s.animateDone` at spinner.go:101 waits for animate to exit |
| Non-TTY path doesn't deadlock | Implemented | `close(s.animateDone)` at spinner.go:56 before returning |
| All existing tests pass | Verified | 37/37 packages pass, 0 failures with -race |
