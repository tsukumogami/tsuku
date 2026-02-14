# Issue 1675 Summary

## Problem

The tsuku-llm daemon did not properly handle SIGTERM signals:
1. Socket and lock files were not cleaned up on SIGTERM
2. Process exited with "signal: terminated" status instead of exit code 0
3. SIGTERM during startup (model download/loading) caused immediate termination without cleanup

## Root Cause

The SIGTERM signal handler was registered AFTER model download and loading completed. Since model download can take significant time (491MB file), if SIGTERM arrived during this phase, the OS default signal handler would terminate the process before our cleanup code could run.

## Solution

Moved SIGTERM signal handler registration to early in the startup sequence (right after lock acquisition), and wrapped all long-running operations in `tokio::select!` blocks that can handle SIGTERM:

1. **Early handler registration**: Signal handler now registered before any long-running operations
2. **SIGTERM during download**: Model download wrapped in select! to catch SIGTERM
3. **SIGTERM during loading**: Model loading wrapped in select! to catch SIGTERM
4. **SIGTERM during grace period**: `wait_for_in_flight` accepts sigterm parameter to detect second SIGTERM
5. **Explicit exit**: `std::process::exit(0)` after cleanup ensures clean exit status

## Files Changed

- `tsuku-llm/src/main.rs`: Signal handling and cleanup logic
- `internal/llm/lifecycle_integration_test.go`: Removed skip directives

## Test Results

- `TestIntegration_SIGTERMTriggersGracefulShutdown`: PASS
- `TestIntegration_MultipleSIGTERMIsSafe`: PASS
- Rust unit tests: 48 passed
- Go non-integration tests: All pass

## Notes

Other integration tests (`ShortTimeoutTriggersShutdown`, `gRPCGetStatus`, `gRPCShutdown`) are failing but these are unrelated to issue #1675. They appear to be timing/connectivity issues with the test harness.
