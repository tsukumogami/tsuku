# Pragmatic Review: Issue #1756

## Findings

### 1. Duplicated daemon-startup boilerplate - ADVISORY
`stability_test.go:22-42` and `stability_test.go:87-110` -- Both tests repeat the same 20-line sequence: TempDir, Setenv, startDaemon, Eventually(isDaemonReady), NewLocalProvider, Eventually(GetStatus.Ready). The existing `lifecycle_integration_test.go` tests have the same pattern (e.g., lines 288-305, 432-454). This is boilerplate that could be a `startAndWaitForDaemon(t) (*exec.Cmd, *LocalProvider, string)` helper, but since it's test code behind a build tag and the sequence isn't identical across all call sites (some tests intentionally vary timeout or skip the provider), this doesn't compound. Leave as-is or extract later.

### 2. `require.NotNil` after `require.Eventually` guarantees non-nil - ADVISORY
`stability_test.go:169` -- `require.NotNil(t, recoveryResp, ...)` is dead: the `require.Eventually` on line 153 only returns true when `callErr == nil` and `recoveryResp` is assigned. If Eventually times out, the test already fails. The nil check is inert but harmless.

## Summary
Blocking: 0, Advisory: 2

The new file correctly reuses all existing helpers (`skipIfModelCDNUnavailable`, `startDaemon`, `isDaemonReady`, `isDaemonRunning`, `grpcDial`, `newInferenceClient`, `testMessage`) from `lifecycle_integration_test.go`. No new abstractions, no duplicated helper functions, no scope creep. Tests are focused on what the issue title says: sequential inference stability and crash recovery. Clean PR.
