# Issue 1016 Summary

## What Was Implemented

Added batch processing and timeout handling to the `InvokeDltest` function for dlopen verification. Paths are now split into batches of 50 libraries, each with a 5-second timeout, with automatic retry on crash using halved batch sizes.

## Changes Made

- `internal/verify/dltest.go`:
  - Added `DefaultBatchSize` (50) and `BatchTimeout` (5s) constants
  - Added `BatchError` type with `IsTimeout` flag and `Unwrap()` support
  - Added `splitIntoBatches()` helper for chunking paths
  - Added `invokeBatch()` with per-batch timeout handling
  - Added `invokeBatchWithRetry()` with recursive halving on crash
  - Modified `InvokeDltest()` to use batching infrastructure

- `internal/verify/dltest_test.go`:
  - Added tests for `splitIntoBatches()` (empty, single, multiple, edge cases)
  - Added tests for `BatchError` (timeout and crash error messages)
  - Added tests for `InvokeDltest` with mock helper scripts
  - Added timeout test (using `exec sleep` for proper signal handling)
  - Added retry-on-crash test with counter file

## Key Decisions

- **Batch size of 50**: Conservative limit well under ARG_MAX while providing good batching efficiency
- **5-second timeout**: Balances catching hangs vs allowing legitimate slow library initialization
- **Exit code differentiation**: Exit 0, 1, 2 are expected (all ok, some failed, usage error); others indicate crash
- **Use `exec` in shell scripts for timeout test**: Ensures SIGKILL reaches the right process

## Trade-offs Accepted

- **Recursive retry**: Uses recursion for halving; acceptable since max depth is log2(50) = ~6 levels
- **No timeout retry**: Timeouts don't retry (unlike crashes); assumes timeout indicates fundamental issue

## Test Coverage

- New tests added: 15 tests for batch processing
- All existing tests continue to pass
- Timeout test skipped with `-test.short` flag

## Known Limitations

- Timeout test requires ~5 seconds to run (skipped in short mode)
- Shell scripts that fork processes may not be killed properly by timeout
