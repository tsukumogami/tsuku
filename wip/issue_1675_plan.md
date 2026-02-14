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

- [x] **Step 1: Register SIGTERM handler early (before model download)**
  - Moved signal handler registration to right after lock acquisition
  - This ensures SIGTERM is caught during model download/loading phases

- [x] **Step 2: Handle SIGTERM during model download**
  - Wrapped model download in `tokio::select!` that also watches for SIGTERM
  - If SIGTERM arrives during download, cleanup and exit with status 0

- [x] **Step 3: Handle SIGTERM during model loading**
  - Wrapped model loading (spawn_blocking) in `tokio::select!`
  - If SIGTERM arrives during loading, cleanup and exit with status 0

- [x] **Step 4: Handle SIGTERM during shutdown grace period**
  - Modified `wait_for_in_flight` function to accept sigterm parameter
  - If second SIGTERM arrives during grace period, return immediately to proceed to cleanup

- [x] **Step 5: Add explicit process exit after cleanup**
  - Added `std::process::exit(0)` after cleanup to ensure clean exit status

- [x] **Step 6: Remove skip directives from integration tests**
  - Removed skip directives from `TestIntegration_SIGTERMTriggersGracefulShutdown`
  - Removed skip directives from `TestIntegration_MultipleSIGTERMIsSafe`

- [x] **Step 7: Verify fix with tests**
  - Rust unit tests: 48 passed
  - SIGTERM integration tests: Both pass

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

- [x] `TestIntegration_SIGTERMTriggersGracefulShutdown` passes:
  - Socket file is removed after SIGTERM
  - Lock file is removed after SIGTERM
  - Process exits with status 0

- [x] `TestIntegration_MultipleSIGTERMIsSafe` passes:
  - Daemon handles multiple rapid SIGTERMs without crash
  - Process exits with status 0 (not "signal: terminated")

- [x] All other tests remain passing:
  - 48 Rust tests pass
  - All Go non-integration tests pass

- [ ] No regressions in other shutdown paths:
  - Idle timeout still cleans up correctly (not tested - unrelated to this fix)
  - gRPC Shutdown still cleans up correctly (not tested - unrelated to this fix)

## Open Questions

None. The approach is straightforward and aligned with the design document requirements.
