# Testability Review

## Verdict: PASS

The acceptance criteria are largely testable and well-structured, with a few gaps in edge case coverage and some criteria that need tightening to be fully verifiable.

## Untestable Criteria

1. **"The primary command's output is not delayed by the update check"** (Automatic updates, 3rd item): "Not delayed" is subjective and hard to verify deterministically. Timing-based assertions are flaky in CI. -> Make it testable by specifying a measurable threshold (e.g., "adds less than 50ms to command execution time") or by verifying structurally that the check runs in a separate goroutine that doesn't block the main path (unit test on concurrency design).

2. **"Rollback is instant (no download)"** (Rollback, 2nd item): "Instant" is subjective. -> Reword to "rollback completes without making network requests when the previous version directory exists on disk." That's verifiable by asserting no HTTP calls during rollback.

3. **"Out-of-channel notifications appear at most once per week per tool"** (Notification, 5th item): Testable in principle but requires time manipulation or a clock abstraction. Not untestable per se, but worth noting it needs a seam. -> Ensure the implementation uses an injectable clock so tests can advance time without waiting a week.

## Missing Test Coverage

1. **Concurrent update safety (Known Limitation 2):** R21 requires atomic operations and the PRD acknowledges a race condition between two tsuku processes updating the same tool. No AC verifies that concurrent updates don't corrupt state. Add: "Two concurrent `tsuku update <tool>` invocations result in one success and one clean failure, with no corruption to state.json or the tool directory."

2. **Offline/network failure behavior (R20):** No AC covers graceful offline degradation. The criteria mention transient network failures only in the context of deferred notices (suppressing 1-2 consecutive failures). Missing: "When network is unavailable, update checks fail silently with no stderr output" and "Cached check results are used when the network is unavailable."

3. **State consistency after crash (Known Limitation 3):** R21 requires temp-file-then-rename atomicity. No AC verifies this. Add: "If tsuku is killed mid-update, no partial state is written to state.json" or "`tsuku doctor` detects orphaned staging directories." (The latter is partially covered by the doctor criterion in Failure handling, but only as a side mention, not a dedicated test case.)

4. **`TSUKU_AUTO_UPDATE=1` overriding CI detection (R16):** The AC covers suppression via `CI=true` but doesn't cover the explicit opt-in override. Add: "When both `CI=true` and `TSUKU_AUTO_UPDATE=1` are set, update checks run."

5. **Old version garbage collection (R18):** No AC covers the retention period or GC behavior. Add: "Previous version directories are retained for at least one auto-update cycle" and "Versions older than the retention period are removed by garbage collection."

6. **`.tsuku.toml` exact pin overriding auto-update (R17):** The configuration AC mentions precedence but doesn't have a concrete scenario. Add: "A tool with `node = "20.16.0"` in `.tsuku.toml` is never auto-updated, even if the global config has `updates.enabled = true`."

7. **`updates.auto_apply = false` config option (D1):** The decisions section mentions this config for notification-only mode, but no AC tests it. Add: "When `updates.auto_apply = false`, available updates are reported but not installed."

8. **Self-update checksum verification failure:** The self-update ACs cover success and generic failure, but don't specifically test checksum mismatch. Add: "Self-update aborts and preserves the current binary when checksum verification fails."

9. **Telemetry for update outcomes (R22):** No AC covers telemetry events being emitted for successful or failed auto-updates.

10. **Batch update (`tsuku update --all`) behavior (R14):** No AC verifies this. Add: "`tsuku update --all` updates all tools with available versions within their pin boundaries."

## Summary

The PRD's acceptance criteria cover the core happy paths well and are organized by feature area, making them easy to map to test cases. The main gaps are in edge cases (concurrency, offline, crash recovery), configuration interactions (`TSUKU_AUTO_UPDATE=1` override, `auto_apply = false`), and Phase 2 features that have requirements but no corresponding ACs (batch update, GC, telemetry). Tightening the two timing-related criteria ("not delayed," "instant") with measurable or structural assertions would make them reliably automatable.
