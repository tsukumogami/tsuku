# Architect Review: Issue #1756

## Findings

### 1. TestSequentialInference bypasses LocalProvider, creating a parallel gRPC usage pattern - ADVISORY

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/stability_test.go:57-74`

`TestSequentialInference` manually constructs gRPC connections via `grpcDial()` and uses the raw `inferenceClient` test wrapper (proto-level), while `TestCrashRecovery` in the same file uses `provider.Complete()` through `LocalProvider`. This mirrors an existing split in `lifecycle_integration_test.go` where `TestIntegration_gRPCComplete` uses raw proto and `TestIntegration_ShortTimeoutTriggersShutdown` uses `LocalProvider`. The two tests in the new file are testing different things (server stability vs. provider recovery), so the abstraction level difference is intentional, not accidental drift.

However, `TestSequentialInference` is described as testing "sustained workloads without degradation" -- which is a server-level concern. Using raw gRPC is the right level for that. No divergence risk since the raw gRPC helpers (`grpcDial`, `inferenceClient`, `testMessage`) already exist in `lifecycle_integration_test.go` and are reused, not duplicated.

**Advisory** rather than blocking because both abstraction levels are already established in the existing test file, and this test reuses the existing helpers rather than creating new ones.

### 2. TestCrashRecovery correctly exercises the LocalProvider abstraction - NO FINDING

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/stability_test.go:86-172`

`TestCrashRecovery` uses `NewLocalProvider()` and calls `provider.Complete()` and `provider.GetStatus()`. This is the correct abstraction level for testing the crash recovery path: it exercises `EnsureRunning`, `invalidateConnection`, and `ensureConnection` -- all of which live inside `LocalProvider`. Testing at the raw gRPC level would miss the recovery logic entirely.

The test pattern (start daemon, baseline call, SIGKILL, expect failure, expect recovery) matches how `lifecycle_integration_test.go` structures its daemon lifecycle tests: `startDaemon` + `isDaemonReady` + `isDaemonRunning` helpers, `require.Eventually` for async readiness, `t.Setenv` for isolation.

### 3. Inconsistent TSUKU_HOME env setup between existing tests and new file - ADVISORY

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/stability_test.go:23,89` vs `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/lifecycle_integration_test.go:285,398,433,486`

The new file uses `t.Setenv("TSUKU_HOME", tsukuHome)` (the safe, auto-cleanup form). The existing `lifecycle_integration_test.go` uses `os.Setenv` / `os.Unsetenv` (the manual form) in `TestIntegration_ShortTimeoutTriggersShutdown` (line 285-286), `TestIntegration_gRPCGetStatus` (line 398-399), `TestIntegration_gRPCComplete` (line 433-434), and `TestIntegration_gRPCShutdown` (line 486-487). Some existing tests like `TestIntegration_LockFilePreventsduplicates` don't set `TSUKU_HOME` at all.

The new file's approach (`t.Setenv`) is strictly better -- it restores the original value automatically and fails if called from a non-test goroutine. This is not a structural violation; it's actually an improvement over the existing pattern. The inconsistency is contained in test code and doesn't affect production layering.

**Advisory** because the inconsistency is in test infrastructure only. Ideally the existing tests would migrate to `t.Setenv` too, but that's cleanup, not a structural concern.

### 4. No dependency or layering violations - NO FINDING

The new file imports only:
- Standard library (`context`, `fmt`, `path/filepath`, `syscall`, `testing`, `time`)
- `testify/require` (test dependency, already used throughout)

No new production dependencies. No imports from higher-level packages. No circular dependency risks. The file uses only types and helpers already defined in the same package (`llm`): `NewLocalProvider`, `CompletionRequest`, `CompletionResponse`, `Message`, `RoleUser`, plus test helpers from `lifecycle_integration_test.go` (`skipIfModelCDNUnavailable`, `startDaemon`, `isDaemonReady`, `isDaemonRunning`).

### 5. Build tag and package placement fit existing structure - NO FINDING

The file uses `//go:build integration` matching `lifecycle_integration_test.go`. It's in `package llm` (internal test, not `llm_test`), consistent with the other integration test files that need access to unexported types. File naming (`stability_test.go`) is descriptive and doesn't collide with existing test files.

## Summary

Blocking: 0, Advisory: 2

The new test file fits the existing architecture well. It reuses established test helpers from `lifecycle_integration_test.go` rather than creating parallel infrastructure. The two tests use different abstraction levels (raw gRPC vs. LocalProvider) appropriate to what each is testing. The `t.Setenv` usage is actually an improvement over the `os.Setenv` pattern in existing tests. No dependency direction violations, no new production code, no layering concerns.
