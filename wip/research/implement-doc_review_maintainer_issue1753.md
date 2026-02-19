# Maintainer Review: Issue #1753 - Invalidate Dead gRPC Connections on Server Crash

**Issue**: #1753 (fix(llm): invalidate dead gRPC connections on server crash)
**Design Doc**: docs/designs/DESIGN-llm-testing-strategy.md
**Review Focus**: Maintainability (clarity, readability, duplication)
**Date**: 2026-02-18

## Summary

The implementation correctly addresses the core issue: when a gRPC request fails due to a server crash, the LocalProvider now invalidates its cached connection and client, forcing reconnection on the next request. The fix includes well-designed tests that validate both the error path (connection invalidation) and the success path (connection preservation). However, there are two maintainability concerns that warrant attention before merge.

## Findings

### 1. Asymmetric Error Handling in Related Methods - BLOCKING

**Severity**: Blocking
**File**: `internal/llm/local.go`
**Lines**: 138-143 (GetStatus) and 129-135 (Shutdown)

**Issue**: The `sendRequest` method (lines 78-94) now invalidates the connection on error, but two other methods that call the gRPC client (`GetStatus` and `Shutdown`) do not implement the same invalidation logic. This creates an asymmetry where:

- `Complete()` → `sendRequest()` → gRPC error → connection invalidated
- `GetStatus()` → gRPC error → connection stays cached (dead)
- `Shutdown()` → gRPC error → connection stays cached (dead)

A caller that checks server status after a crash will get a gRPC error with a dead connection still cached. The next call to `Complete()` will succeed at reconnection, but intermediate status checks silently fail with stale connections.

**Current Code**:
```go
// GetStatus retrieves the addon's current status.
func (p *LocalProvider) GetStatus(ctx context.Context) (*pb.StatusResponse, error) {
	if err := p.ensureConnection(ctx); err != nil {
		return nil, err
	}
	return p.client.GetStatus(ctx, &pb.StatusRequest{})
}

// Shutdown sends a shutdown request to the addon.
func (p *LocalProvider) Shutdown(ctx context.Context, graceful bool) error {
	if p.client == nil {
		return nil
	}
	_, err := p.client.Shutdown(ctx, &pb.ShutdownRequest{Graceful: graceful})
	return err
}
```

**Next developer's mental model**: "If `sendRequest` invalidates on error, then all methods that hit the gRPC client must do the same, right?" No. The next developer will inherit a codebase where error handling is inconsistent, and adding a new gRPC call method will require remembering to add invalidation logic. This is a name-behavior mismatch: `sendRequest` is the *only* place that invalidates, but a reader will expect all gRPC calls to behave the same way.

**How to fix**:
1. **Extract a helper method** (recommended): Create a `callGRPCWithRecovery` or `callGRPC` helper that wraps any gRPC call, handles errors consistently, and invalidates on failure. This consolidates the invalidation logic and makes it obvious that all methods use the same pattern.

```go
// callGRPC executes a gRPC call and invalidates the connection on error.
func (p *LocalProvider) callGRPC(ctx context.Context, fn func() error) error {
	err := fn()
	if err != nil {
		p.client = nil
		if p.conn != nil {
			_ = p.conn.Close()
			p.conn = nil
		}
	}
	return err
}
```

Then update the methods:
```go
func (p *LocalProvider) GetStatus(ctx context.Context) (*pb.StatusResponse, error) {
	if err := p.ensureConnection(ctx); err != nil {
		return nil, err
	}
	var resp *pb.StatusResponse
	err := p.callGRPC(ctx, func() error {
		var err error
		resp, err = p.client.GetStatus(ctx, &pb.StatusRequest{})
		return err
	})
	return resp, err
}
```

2. **Alternative**: Duplicate the invalidation logic in `GetStatus` and `Shutdown`, but add a comment explaining why it's needed and how it mirrors `sendRequest`. Include a TODO to consolidate. This is less ideal but acceptable if extracting a helper is deemed over-engineering.

**Why this is blocking**: A caller who monitors server health (via `GetStatus`) after a crash will encounter silently stale connections. The code appears to handle crashes (because `sendRequest` does), but the contract is invisible. The next developer will either duplicate this bug or waste time wondering why status checks don't trigger reconnection.

---

### 2. Incomplete Invalidation in Shutdown - ADVISORY

**Severity**: Advisory
**File**: `internal/llm/local.go`
**Lines**: 129-135

**Issue**: The `Shutdown` method returns early if `p.client == nil`, but it doesn't account for the case where `p.conn` exists but `p.client` is nil (e.g., after `sendRequest` invalidates on error). This means a caller who tries to gracefully shutdown after a crash will fail silently, never sending the shutdown RPC.

**Current Code**:
```go
func (p *LocalProvider) Shutdown(ctx context.Context, graceful bool) error {
	if p.client == nil {
		return nil
	}
	_, err := p.client.Shutdown(ctx, &pb.ShutdownRequest{Graceful: graceful})
	return err
}
```

**Scenario**:
1. `Complete()` fails, `sendRequest` nils both `p.client` and `p.conn`
2. Caller invokes `Shutdown()`
3. Check `if p.client == nil` passes, returns nil early
4. Shutdown RPC is never sent (even though the server is still running)

**How to fix**:
Check both `p.client` and `p.conn` before returning early, or better yet, implement invalidation consistently so this case never arises (see Finding 1).

```go
func (p *LocalProvider) Shutdown(ctx context.Context, graceful bool) error {
	if p.client == nil && p.conn == nil {
		return nil
	}
	if p.client == nil {
		// Reconnect if client is nil but conn might still exist
		if err := p.ensureConnection(ctx); err != nil {
			return err
		}
	}
	_, err := p.client.Shutdown(ctx, &pb.ShutdownRequest{Graceful: graceful})
	if err != nil {
		p.invalidateConnection() // Use the helper from Finding 1
	}
	return err
}
```

**Why this is advisory**: The `Shutdown` method is called in cleanup/defer blocks, and failing silently is not ideal but not catastrophic. The idle timeout will eventually shut down the server. However, fixing this alongside Finding 1 ensures consistent behavior.

---

### 3. Test Coverage is Excellent - POSITIVE

**Severity**: N/A (positive finding)
**File**: `internal/llm/local_test.go`
**Lines**: 233-344

**Strengths**:
- `TestSendRequestInvalidatesConnectionOnError` (lines 233-294) directly tests the error path and verifies that both `p.client` and `p.conn` are niled.
- `TestSendRequestSucceedsOnValidResponse` (lines 296-344) verifies that successful calls do NOT invalidate the connection.
- Both tests use bufconn with mock servers, avoiding integration test complexity.
- Assertions are clear: they check that the connection state changes appropriately after each call.

**Why this is important**: The test design makes the expected behavior explicit. The next developer reading these tests will immediately understand the intent: "gRPC errors kill the cached connection; successes preserve it." This is good documentation.

**Suggestion**: Consider adding a test for the asymmetry in Finding 1. For example, `TestGetStatusInvalidatesConnectionOnError` would make it obvious that GetStatus should behave like sendRequest, and when you write that test, you'll immediately see it fail with the current code.

---

### 4. Comment Clarity is Good - POSITIVE

**Severity**: N/A (positive finding)
**File**: `internal/llm/local.go`
**Lines**: 75-77, 83-84

The comments are clear:
```go
// sendRequest converts the request to proto format, sends it over gRPC,
// and invalidates the cached connection on error so subsequent calls
// trigger reconnection via ensureConnection.
```

and

```go
// Invalidate the cached connection so subsequent calls trigger
// reconnection via ensureConnection instead of reusing a dead connection.
```

These comments explain *why* the invalidation happens and what the expected behavior is on subsequent calls. The next developer can use these as a template for similar logic elsewhere.

---

## Overall Assessment

**Blocking Issues**: 1 (asymmetric error handling across gRPC methods)
**Advisory Issues**: 1 (incomplete early return in Shutdown)
**Positive Findings**: 2 (test coverage, comment clarity)

The core fix (invalidate dead gRPC connections on error in `sendRequest`) is correct and well-tested. However, the implementation is incomplete: `GetStatus` and `Shutdown` do not implement the same error handling. This creates a maintainability trap where the solution appears to be generalized but only applies to one method.

**Recommendation**: Before merge, implement one of the following:
1. **Preferred**: Extract a helper method that all gRPC-calling methods use, consolidating the invalidation logic.
2. **Acceptable**: Duplicate the invalidation logic in `GetStatus` and `Shutdown` with comments explaining the pattern and why it's replicated.

Either approach fixes the asymmetry and makes the contract explicit for the next developer. The fix itself is sound; it just needs to be applied consistently.
