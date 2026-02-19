# Pragmatic Review: Issue #1753 - Fix Dead gRPC Connections on Server Crash

**Reviewer**: Pragmatic (simplicity, YAGNI, KISS)
**Issue**: #1753
**Design Doc**: docs/designs/DESIGN-llm-testing-strategy.md
**Files Changed**: `internal/llm/local.go`, `internal/llm/local_test.go`

---

## Summary

The implementation adds connection invalidation to `LocalProvider.sendRequest()` so that gRPC errors (including those from server crashes) trigger reconnection on the next `Complete()` call. The fix is minimal, correct, and well-tested. **No blocking issues.**

---

## Detailed Findings

### Finding 1: Dead connection invalidation is correct
**File**: `internal/llm/local.go`, lines 82-89
**Severity**: None (good implementation)
**Detail**:

When `p.client.Complete(ctx, pbReq)` returns an error, the code:
1. Nils `p.client` so `ensureConnection` will check `if p.client != nil` and reconnect
2. Closes and nils `p.conn` to release the stale socket
3. Wraps the error with context about the failure

This is exactly the right approach. The invariant "if `p.client == nil`, reconnect" is maintained. No leaked connections or dangling references.

**Test coverage**:
- `TestSendRequestInvalidatesConnectionOnError` (lines 237-294) verifies both fields are nil after error
- `TestSendRequestSucceedsOnValidResponse` (lines 296-344) verifies fields stay non-nil on success
- Both use a mock gRPC server in bufconn, avoiding external dependencies

All paths are tested.

---

### Finding 2: No race condition in connection state
**File**: `internal/llm/local.go`, lines 97-114 (ensureConnection)
**Severity**: None (correct by design)
**Detail**:

The `LocalProvider` fields `conn` and `client` are modified only in:
1. `sendRequest` on error (lines 85-88)
2. `ensureConnection` on first connection (lines 112-113)
3. `Close` on explicit cleanup (lines 120-122)

All three are called from `Complete()` sequentially (not concurrently from the same `LocalProvider` instance). The design assumes single-threaded access per provider instance. This matches the pattern in `internal/llm/lifecycle.go` which also maintains non-atomic state.

**No blocking issue**, but this is implicit contract documentation. The code doesn't panic on concurrent access; it just has unpredictable behavior. This is acceptable for a constructor-provided singleton that's used in a single request sequence.

---

### Finding 3: Error message is clear and contextual
**File**: `internal/llm/local.go`, line 90
**Severity**: None (advisory observation)
**Detail**:

The error is wrapped with `fmt.Errorf("local LLM completion failed: %w", err)`. This message:
- Names which component failed (local LLM)
- Preserves the original gRPC error for debugging
- Is clear to callers what to do (reconnect will happen automatically on next call)

Good pattern. No changes needed.

---

### Finding 4: Nil check safety in GetStatus and Shutdown
**File**: `internal/llm/local.go`, lines 130-143
**Severity**: None (correct)
**Detail**:

Both `Shutdown` (line 130) and `GetStatus` (line 139) check `if p.client == nil` before using it. They don't reconnect -- they just return early or call `ensureConnection`. This is correct:

- `Shutdown` is for explicit cleanup, so a nil client means the daemon was already stopped
- `GetStatus` calls `ensureConnection` first (line 139), so it will reconnect if needed

Consistent with the invalidation pattern.

---

### Finding 5: Close() is idempotent and safe
**File**: `internal/llm/local.go`, lines 118-126
**Severity**: None
**Detail**:

The `Close()` method:
1. Checks `if p.conn != nil` before closing
2. Sets `p.client = nil` after closing
3. Returns nil if conn is already nil (lines 125)

Safe to call multiple times. Matches the invalidation pattern from `sendRequest`. Good.

---

## Testing Summary

**Test coverage**: 5 test cases added
- `TestSocketPath` (lines 19-36): Path resolution, OK
- `TestLockPath` (lines 39-46): Lock file path, OK
- `TestNewLocalProvider` (lines 58-65): Provider initialization, OK
- `TestSendRequestInvalidatesConnectionOnError` (lines 237-294): **Core fix test** ✓
- `TestSendRequestSucceedsOnValidResponse` (lines 296-344): **Regression test (happy path)** ✓

The two core tests are well-designed:
- Use bufconn for in-memory gRPC (no external dependencies)
- Directly inject a connection into `LocalProvider` to isolate the logic
- Verify both invariants: nil after error, non-nil after success
- Include error message assertions

**Gaps**: None identified. The integration test (`TestLocalProviderIntegration`, lines 88-121) provides end-to-end validation with real addon if running.

---

## Correctness Check Against Design Doc

From DESIGN-llm-testing-strategy.md, section "Chosen: Layered Stability Tests":

> Fix both issues: (1) an `actions/cache` step in the CI workflow caches the model directory... (2) a `sharedModelDir(t)` test helper provides a single model directory...

The design mentions this fix is a **prerequisite** for `TestCrashRecovery`:

> The crash-recovery stability test (`TestCrashRecovery`) depends on a fix to `LocalProvider` that invalidates `p.conn` and `p.client` on gRPC errors.

**Verdict**: Implementation matches the design's prerequisite requirement exactly. This issue is correctly scoped as a production code fix (not test infrastructure) and completes the dependency.

---

## Assessment

**Blocking issues**: 0
**Advisory findings**: 0

The implementation:
- ✓ Solves the problem (dead connections are invalidated)
- ✓ Is minimal and doesn't over-engineer
- ✓ Preserves existing error semantics
- ✓ Has comprehensive test coverage
- ✓ Follows established patterns in the codebase
- ✓ Matches the design doc's intent

**Ready to merge.**
