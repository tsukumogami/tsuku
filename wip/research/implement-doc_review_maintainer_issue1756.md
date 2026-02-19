# Maintainer Review: Issue #1756

## Files Reviewed

- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/stability_test.go` (new)
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/lifecycle_integration_test.go` (context)
- `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/local.go` (context)

## Findings

### 1. Two API styles exercised without explaining why - ADVISORY

`stability_test.go:58-75` -- `TestSequentialInference` uses the low-level `grpcDial` + `inferenceClient` helper to make requests, while `TestCrashRecovery` (line 114) uses `provider.Complete()`. Both tests start with the same daemon setup and readiness polling. The next developer will wonder whether the difference is intentional (testing two different code paths) or accidental (one test was written later and the author forgot to use the higher-level API).

Looking at the existing tests in `lifecycle_integration_test.go`, `TestIntegration_gRPCComplete` explicitly documents why it goes low-level: "This test uses the proto client directly since the daemon is already running (bypasses the addon download/verification that would happen in production)." `TestSequentialInference` should have a similar note explaining that it deliberately bypasses `LocalProvider` to test the gRPC server's request-handling stability in isolation, separate from the provider's reconnect logic.

Without that note, the next developer who needs to add a stability test won't know which pattern to follow.

### 2. Crash recovery test comment claims wrong mechanism - BLOCKING

`stability_test.go:81-85` -- The comment says: "the first Complete call should fail (stale connection), and the second call should succeed because invalidateConnection clears the dead connection and EnsureRunning restarts the server."

Looking at `local.go:89-95`, `sendRequest` calls `invalidateConnection()` on error, which is correct. But the comment says `EnsureRunning` restarts the server -- actually, the restart path is: (1) `invalidateConnection` nils the connection, (2) the *next* `Complete` call re-enters `p.addonManager.EnsureAddon()` then `p.lifecycle.EnsureRunning()` then `p.ensureConnection()`. The comment conflates the error-handling call with the recovery call.

More importantly, the test at line 153-167 uses `require.Eventually` with a 5-minute outer timeout and 5-second polling interval to retry `provider.Complete`. This means the test doesn't actually verify the "first call fails, second call succeeds" contract described in the comment. It retries until *any* call succeeds. The "first call should fail" assertion at line 136-144 does verify the stale-connection failure, but the recovery phase might succeed on the 3rd, 5th, or 10th attempt -- the comment's "second call should succeed" is misleading.

The next developer reading this test will think the recovery is a clean two-step (fail once, succeed once), and might refactor the retry loop into a single call and be confused when it fails intermittently.

Suggested fix: Update the comment to say something like: "After the stale-connection error, invalidateConnection clears the dead connection. Subsequent Complete calls trigger EnsureRunning which restarts the server. Recovery may take several attempts while the daemon restarts and reloads the model."

### 3. `conn.Close()` error silently discarded in sequential test loop - ADVISORY

`stability_test.go:73` -- `_ = conn.Close()` discards the close error inside a loop that's specifically testing connection stability. If `conn.Close()` starts failing, that's signal the test should surface. The other test (`TestCrashRecovery`) doesn't have this issue because it uses `provider.Close()` via defer.

This is advisory because a close failure here wouldn't cause a misread of the test's intent, but it does contradict the test's purpose of detecting connection-level problems.

### 4. Daemon cleanup at end of TestSequentialInference is fire-and-forget but startDaemon already has a cleanup - ADVISORY

`stability_test.go:78` -- The test ends with `_ = daemon.Process.Signal(syscall.SIGTERM)`, but `startDaemon` (lifecycle_integration_test.go:178-183) already registers a `t.Cleanup` that calls `cmd.Process.Kill()`. So there are two shutdown paths: the explicit SIGTERM at the end, and the cleanup's SIGKILL.

This is consistent with the existing tests in `lifecycle_integration_test.go` (lines 258, 278, 423, 478 all do the same pattern), so it's an established convention in this file. The explicit SIGTERM is graceful shutdown; the cleanup is a safety net. Not confusing if you read `startDaemon`, but worth noting that the pattern is load-bearing: if someone removes the explicit SIGTERM thinking "cleanup handles it," they'll get SIGKILL instead of SIGTERM, which matters for tests that check socket cleanup.

### 5. Magic timeout values lack rationale - ADVISORY

Several timeout values appear without explaining what they accommodate:

- `stability_test.go:26` -- `10*time.Minute` idle timeout: presumably "long enough that the daemon won't idle-exit during the test," but 10 minutes is very generous for 5 sequential requests. The same value appears in the crash recovery test. The existing tests use `5*time.Minute` for most cases. The choice of 10 vs 5 isn't explained.
- `stability_test.go:136` -- `30*time.Second` timeout for the "expect failure" call. This is the stale-connection call that should fail fast. Why 30 seconds and not 5? If the daemon somehow restarts within 30 seconds, this assertion could pass when it shouldn't. A shorter timeout would make the test's intent clearer: "we expect this to fail immediately, not after a long wait."
- `stability_test.go:167` -- `5*time.Minute` outer timeout with `5*time.Second` polling. The comment at line 151 says "restart + model load can take a while on CPU" which is helpful, but the 5-second polling interval isn't motivated. Is it "long enough to not hammer the server" or "short enough to catch the recovery quickly"?

None of these will cause a misread that leads to a bug, but the 30-second timeout on the "expect failure" call (line 136) is the most concerning -- it creates a window where recovery could happen before the assertion, making the test less precise than it appears.

## Summary

Blocking: 1, Advisory: 4

The tests are well-structured and follow established patterns from `lifecycle_integration_test.go`. The main blocking issue is the comment on `TestCrashRecovery` that describes a simpler recovery model than what actually happens: the next developer will think "fail once, succeed once" when reality is "fail once, retry until success." The advisory items are about making the two-API-style choice explicit, tightening the stale-connection timeout, and minor cleanup hygiene.
