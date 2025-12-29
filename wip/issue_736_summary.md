# Issue 736 Summary

## What Was Implemented

Fixed sandbox mode to create required cache directories before attempting to mount them as container volumes. This prevents failures in fresh environments where `$TSUKU_HOME/cache/downloads` doesn't exist yet.

## Changes Made

- `cmd/tsuku/install_sandbox.go`: Added `cfg.EnsureDirectories()` call before creating the sandbox executor, ensuring all required directories exist before attempting to mount them as container volumes.

## Key Decisions

- Reused existing pattern from `create.go`: The same `EnsureDirectories()` call is already used in `create.go` for the same purpose, making this a consistent and minimal fix.
- Error handling returns wrapped error: Used `fmt.Errorf("failed to create directories: %w", err)` to provide clear context while preserving the underlying error.

## Trade-offs Accepted

- None: This is a straightforward fix with no trade-offs. The directory creation is idempotent and adds negligible overhead.

## Test Coverage

- No new tests added: The fix is trivial (4 lines) and follows an existing pattern. The change is exercised by existing sandbox tests.
- Coverage change: No measurable impact - the added code path is only exercised during sandbox execution.

## Known Limitations

- Manual testing was skipped (no container runtime available in this environment). CI will verify the fix works with actual container runtimes.

## Future Improvements

None identified - this fix is complete.
