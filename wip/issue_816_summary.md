# Issue 816 Summary

## What Was Implemented

Fixed macOS test failures caused by symlink resolution in temporary directories. The issue was that `/var` on macOS is a symlink to `/private/var`, causing path comparison to fail when binaries were resolved via `EvalSymlinks`.

## Changes Made

- `internal/install/checksum.go`: Added `EvalSymlinks(toolDir)` before the binary loop in `ComputeBinaryChecksums` to canonicalize the tool directory path, ensuring both paths use the same prefix for comparison.

## Key Decisions

- **Fix in production code, not tests**: A user with `$TSUKU_HOME` in a symlinked directory would hit the same issue. The production fix benefits real-world usage, not just test environments.
- **Canonicalize at call site**: Kept `isWithinDir` as a pure path comparison utility. Path resolution is the caller's responsibility.

## Trade-offs Accepted

- **One additional syscall**: `EvalSymlinks(toolDir)` adds a syscall, but it's called once per invocation (outside the loop), so performance impact is negligible.

## Test Coverage

- New tests added: 0 (existing tests now pass)
- The 4 failing tests now pass on macOS

## Known Limitations

None. The fix correctly handles symlinked paths across platforms.

## Future Improvements

None needed. This was a targeted fix for a specific platform behavior.
