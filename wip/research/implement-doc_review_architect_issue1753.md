# Architecture Review: Issue #1753 - Fix Dead gRPC Connections on Server Crash

**Issue**: #1753
**Focus**: Architect
**Review Date**: 2026-02-18

## Summary

The implementation correctly addresses the architectural issue identified in the design: dead gRPC connections cached after a server crash. The fix is properly scoped, fits the existing provider architecture, and includes comprehensive tests. No blocking findings.

---

## Detailed Findings

### 1. Error Recovery Pattern (Lines 78-94, `local.go`)

**Finding**: The `sendRequest()` method invalidates the cached connection on gRPC error.

**What changed:**
```go
// Lines 81-91 in local.go
pbResp, err := p.client.Complete(ctx, pbReq)
if err != nil {
    // Invalidate the cached connection so subsequent calls trigger
    // reconnection via ensureConnection instead of reusing a dead connection.
    p.client = nil
    if p.conn != nil {
        _ = p.conn.Close()
        p.conn = nil
    }
    return nil, fmt.Errorf("local LLM completion failed: %w", err)
}
```

**Architecture alignment**: This pattern fits the provider contract. The `Provider` interface defines `Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)`. When `Complete()` fails due to a server crash, the connection state is now reset so the next call to `Complete()` will trigger `ensureConnection()` to establish a fresh connection. The error is propagated to the caller (the builder layer), which makes the retry decision.

**Why this matters**: Before this fix, a server crash would leave `p.conn` and `p.client` pointing to a dead connection. All subsequent calls would fail immediately without triggering reconnection. This is a state management bug in the provider layer, not a higher-level concern.

**Status**: Correct. No further action needed.

---

### 2. Separation of Concerns (Provider vs Lifecycle vs AddonManager)

**Finding**: The fix respects the layering between three components.

**Architecture**:
- **AddonManager**: Downloads/verifies the addon binary
- **ServerLifecycle**: Manages the daemon process (start, health check, restart on stale socket)
- **LocalProvider**: Client-side connection management

**How they interact in `Complete()` (lines 56-72)**:
1. Addon verification (AddonManager concern)
2. Server running check (ServerLifecycle concern)
3. Connection establishment (Provider concern)
4. gRPC call + error handling (Provider concern)

The fix keeps error recovery in the provider layer where it belongs. The provider doesn't restart the server or re-verify the addon -- it just invalidates the cached connection and returns the error. The next `Complete()` call will re-enter the same flow, and `ServerLifecycle.EnsureRunning()` will detect that the server has crashed and restart it (if configured to do so).

**Status**: Correct. The layering prevents provider code from reaching into daemon management.

---

### 3. Connection Lifecycle (Nil Checks)

**Finding**: The fix uses nil checks to detect whether a connection exists, which is the idiomatic Go pattern for optional fields.

**Lines 98-100** (`ensureConnection()`):
```go
if p.client != nil {
    return nil
}
```

**Line 119** (`Close()`):
```go
if p.conn != nil {
    err := p.conn.Close()
    p.conn = nil
    p.client = nil
    return err
}
```

**Line 130** (`Shutdown()`):
```go
if p.client == nil {
    return nil
}
```

**Question**: Is it safe to assume both `conn` and `client` are in sync?

**Answer**: Yes, because:
- They are set together in `ensureConnection()` (lines 112-113)
- They are cleared together on error in `sendRequest()` (lines 85-89)
- They are cleared together in `Close()` (lines 121-122)

**Risk**: There's no guarantee that a concurrent caller could observe inconsistent state, but this is mitigated by the fact that `LocalProvider` is typically created once per application and cached by the factory. The provider is not designed as a concurrent-safe object with mutex protection -- callers must not use the same provider from multiple goroutines without external synchronization. This is consistent with the provider pattern used by Claude and Gemini providers, which also don't protect internal state.

**Status**: Acceptable. The nil-check pattern is idiomatic and matches the rest of the codebase.

---

### 4. Test Coverage

**Finding**: The fix includes three targeted test functions that validate the error recovery behavior.

**Test 1: `TestSendRequestInvalidatesConnectionOnError` (lines 237-294)**
- Injects a mock server that returns an error from `Complete()`
- Verifies that `provider.conn` and `provider.client` are nil'd out after the error
- Directly tests the fix's core behavior

**Test 2: `TestSendRequestSucceedsOnValidResponse` (lines 296-344)**
- Verifies that `sendRequest()` does NOT invalidate the connection on success
- Ensures the fix doesn't over-invalidate (only on error)

**Test 3: `TestLocalProviderWithMockServer` (lines 163-231)**
- Integration-style test with a full mock gRPC server
- Validates the happy path (successful `Complete()`, `GetStatus()`, `Shutdown()`)

**Observation**: Tests 1 and 2 directly test `sendRequest()` rather than going through the public `Complete()` method. This is intentional -- the tests inject a mock gRPC connection to avoid the complexity of the addon manager and lifecycle management. The test comment explains this tradeoff (lines 269-270).

**Status**: Good. Tests cover both the happy path and error recovery. The direct testing of `sendRequest()` is justified.

---

### 5. Design Doc Alignment

**From DESIGN-llm-testing-strategy.md:**
- Section: "Prerequisite": "The crash-recovery stability test depends on a fix to `LocalProvider` that invalidates `p.conn` and `p.client` on gRPC errors."
- Section: "Solution Architecture": "Connection recovery fix for `LocalProvider`" is listed as a prerequisite dependency for `TestCrashRecovery` stability test.

**Implementation alignment**: The fix exactly matches the design requirement:
- Invalidates `p.conn` and `p.client` on gRPC error ✓
- Allows subsequent `Complete()` calls to trigger reconnection via `ensureConnection()` ✓
- Returns the error to the caller without retrying internally ✓

**Status**: Aligned with design.

---

### 6. No New Interfaces or Dependencies Introduced

**Finding**: The fix modifies internal state management within `LocalProvider` only.

**Public API**: No changes to the `Provider` interface (still just `Name()` and `Complete()`).

**Imports**: No new imports in `local.go`. Uses existing packages: `context`, `encoding/json`, `fmt`, `net`, `os`, `path/filepath`, `time`, `google.golang.org/grpc` (existing).

**Dependency direction**: `LocalProvider` depends on:
- `internal/llm/addon` (addon manager)
- `internal/llm/proto` (protobuf types for gRPC)
- No upward dependency to builder layer or CLI

**Status**: No new structural dependencies introduced.

---

### 7. Error Propagation

**Finding**: The error from the gRPC call is wrapped and propagated to the caller.

**Line 90**:
```go
return nil, fmt.Errorf("local LLM completion failed: %w", err)
```

This follows Go's error wrapping convention and preserves the original error chain for debugging. The message context ("local LLM completion failed") makes the error's origin clear without requiring stack traces.

**Question**: Should the provider retry internally on transient gRPC errors?

**Answer**: No. The design doc explicitly states that the provider layer is stateless. Retry logic should live at a higher level (the builder layer), where the conversation context and semantic retry conditions are understood. A gRPC transport error is the provider's signal to clean up its state and return the error.

**Status**: Correct error handling pattern.

---

## Advisory Findings

### Comment Clarity (Line 76-77)

**Finding**: The comment above `sendRequest()` is clear and explains the rationale.

```go
// sendRequest converts the request to proto format, sends it over gRPC,
// and invalidates the cached connection on error so subsequent calls
// trigger reconnection via ensureConnection.
```

This is helpful context for future maintainers. No action needed, but worth noting as good documentation practice.

---

### Test Initialization Pattern (Lines 271-274)

The test manually constructs a `LocalProvider` with only the `conn` and `client` fields set:
```go
provider := &LocalProvider{
    conn:   conn,
    client: pb.NewInferenceServiceClient(conn),
}
```

This leaves `addonManager` and `lifecycle` as nil pointers. The test explicitly calls `sendRequest()` directly to avoid these fields. This is a valid testing technique but worth noting: tests that call public methods like `Complete()` must set up the full provider state. The comment explains this (lines 269-270), so it's clear.

**Status**: Acceptable test pattern.

---

## Architecture Summary

| Aspect | Finding | Status |
|--------|---------|--------|
| Error recovery scope | Isolated to provider state mgmt, doesn't bypass layering | ✓ Correct |
| Connection lifecycle | Nil-check pattern idiomatic, state stays in sync | ✓ Correct |
| Test coverage | Core behavior + success path + integration | ✓ Adequate |
| Design alignment | Matches prerequisite requirements exactly | ✓ Aligned |
| Dependency direction | No new upward dependencies | ✓ Clean |
| Error handling | Wraps and propagates, no premature retry | ✓ Correct |
| Comments | Clear explanation of invalidation rationale | ✓ Good |

---

## Conclusion

The implementation correctly fixes the dead connection caching bug within the provider layer's architecture. The change respects the separation between addon management, server lifecycle, and client connection management. Tests validate both the error recovery behavior and the happy path. No blocking issues or pattern violations detected.

The fix is ready as a prerequisite for issue #1756 (`TestCrashRecovery` stability test), which depends on this connection invalidation behavior.
