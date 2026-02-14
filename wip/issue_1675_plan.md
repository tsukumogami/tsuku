# Issue 1675 Implementation Plan

## Summary

Fix SIGTERM handling by adding explicit `std::process::exit(0)` after cleanup and ensuring the signal handler properly intercepts SIGTERM before the default OS handler terminates the process.

## Approach

The current code uses `tokio::signal::unix::signal()` to listen for SIGTERM, but this does **not** override the default signal behavior. When SIGTERM arrives, both the tokio handler AND the OS's default handler receive it. The OS handler terminates the process (exit status "signal: terminated") before cleanup completes.

The fix involves:
1. Keep using `tokio::signal::unix::signal()` (it does install a handler that intercepts the signal)
2. Add explicit `std::process::exit(0)` after cleanup to ensure clean exit code
3. Handle repeated SIGTERMs during shutdown gracefully (idempotent shutdown flag)

**Why this works:** Tokio's signal handler does prevent the default behavior on first signal. The issue is that our cleanup code runs but the function returns `Ok(())` which exits naturally, and by that point subsequent signals or race conditions can cause unclean exit. Calling `std::process::exit(0)` explicitly ensures we exit with code 0 before any race can occur.

### Alternatives Considered

- **Use `signal_hook` crate**: Would add another dependency when tokio's signal handling is sufficient. The real issue is our exit path, not signal interception.

- **Use `ctrlc` crate**: Only handles SIGINT (Ctrl+C), not SIGTERM. Not applicable.

- **Spawn cleanup in separate thread**: Adds complexity and doesn't address the fundamental issue of ensuring exit code 0.

## Files to Modify

- `tsuku-llm/src/main.rs` - Add explicit `std::process::exit(0)` after cleanup, add second SIGTERM handling during shutdown
- `internal/llm/lifecycle_integration_test.go` - Remove skip directives from the two fixed tests

## Files to Create

None.

## Implementation Steps

- [ ] **Step 1: Add explicit process exit after cleanup**
  - In `main.rs`, after `cleanup_files(&socket, &lock)` and the final log message, add `std::process::exit(0)`
  - This ensures the process exits with code 0 before returning from main() where race conditions could occur

- [ ] **Step 2: Handle SIGTERM during shutdown grace period**
  - Modify the `wait_for_in_flight` function or the shutdown path to also listen for SIGTERM
  - If a second SIGTERM arrives during the grace period, immediately proceed to cleanup (don't wait further)
  - This ensures multiple SIGTERMs don't cause issues

- [ ] **Step 3: Ensure cleanup runs even if grace period is interrupted**
  - Use tokio::select! in the in-flight wait loop to also watch for SIGTERM
  - On second SIGTERM, break out of wait loop and proceed to cleanup

- [ ] **Step 4: Remove skip directives from integration tests**
  - In `internal/llm/lifecycle_integration_test.go`, remove:
    - Line 315: `t.Skip("Skipped: socket cleanup after SIGTERM needs fix (see issue #1675)")`
    - Line 353: `t.Skip("Skipped: multiple SIGTERM handling needs fix (see issue #1675)")`

- [ ] **Step 5: Run tests to verify fix**
  - Build tsuku-llm: `cd tsuku-llm && cargo build`
  - Run Rust unit tests: `cargo test`
  - Run Go integration tests: `go test -tags=integration -v ./internal/llm/... -run SIGTERM`

## Testing Strategy

**Unit tests:**
- Existing Rust tests in `main.rs` (duration parsing, etc.) should continue to pass
- No new unit tests needed for signal handling (hard to unit test signal behavior)

**Integration tests:**
- `TestIntegration_SIGTERMTriggersGracefulShutdown`: Verifies socket and lock file cleanup after SIGTERM, and clean exit status
- `TestIntegration_MultipleSIGTERMIsSafe`: Verifies daemon handles multiple rapid SIGTERMs gracefully with exit status 0

**Manual verification:**
1. Build and start daemon: `./tsuku-llm serve`
2. Send SIGTERM: `kill -TERM <pid>`
3. Verify socket file removed and process exits with code 0

## Risks and Mitigations

- **Risk: Race between cleanup and second SIGTERM**
  - Mitigation: Use atomic flag to prevent re-entry into cleanup, exit immediately after cleanup

- **Risk: In-flight requests interrupted mid-cleanup**
  - Mitigation: Cleanup runs after grace period; if second SIGTERM arrives, we prioritize clean exit over request completion (documented behavior)

- **Risk: Exit before logging completes**
  - Mitigation: Use blocking flush before `std::process::exit(0)` if needed (tracing subscriber typically flushes synchronously)

## Success Criteria

- [ ] `TestIntegration_SIGTERMTriggersGracefulShutdown` passes:
  - Socket file is removed after SIGTERM
  - Lock file is removed after SIGTERM
  - Process exits with status 0

- [ ] `TestIntegration_MultipleSIGTERMIsSafe` passes:
  - Daemon handles multiple rapid SIGTERMs without crash
  - Process exits with status 0 (not "signal: terminated")

- [ ] All other tests remain passing:
  - 48 Rust tests pass
  - All Go tests pass

- [ ] No regressions in other shutdown paths:
  - Idle timeout still cleans up correctly
  - gRPC Shutdown still cleans up correctly

## Open Questions

None. The approach is straightforward and aligned with the design document requirements.
