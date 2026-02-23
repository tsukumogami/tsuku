# Re-review: Issue #1944 -- --json flag for sandbox output

**0 blocking, 0 advisory.**

## Previous findings and resolution status

### 1. BLOCKING (resolved): Double JSON on stdout when sandbox fails

**Previous finding**: `emitSandboxJSON` returned an error for non-passing states, which propagated back to `install.go:84` where `handleInstallError` would call `printJSON` a second time -- two JSON objects on stdout, breaking any `jq` consumer.

**Resolution**: `emitSandboxJSON` (`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/cmd/tsuku/install_sandbox.go:144-153`) now handles all terminal states itself. For failures, it calls `exitWithCode(ExitInstallFailed)` directly at line 151, so control never returns to the caller. For success, it returns `nil`, so the `handleInstallError` guard at `install.go:84` is never entered. The godoc at lines 140-143 explains this contract clearly:

```go
// The JSON output is the terminal action -- this function never returns an error
// to the caller, which prevents handleInstallError from emitting a second JSON
// object. For non-passing states, it calls exitWithCode directly to set the
// appropriate process exit code.
```

The separation of `buildSandboxJSONOutput` (pure, testable) from `emitSandboxJSON` (side-effecting, calls `exitWithCode`) is clean. Tests exercise `buildSandboxJSONOutput` directly, which is the right call since `emitSandboxJSON` calls `os.Exit`. **Fixed.**

### 2. Advisory (resolved): Divergent substring helpers in test files

**Previous finding**: `install_sandbox_test.go` used `strings.Contains` from the standard library while `create_test.go` defined custom `containsString`/`containsSubstring` helpers, creating two idioms for the same operation.

**Resolution**: `install_sandbox_test.go` continues to use `strings.Contains` (lines 150, 175, 235, 265). However, on re-examination this is the correct call. The custom helpers in `create_test.go:476-487` are hand-rolled reimplementations of `strings.Contains` with extra length guards -- they don't add any behavior that the stdlib function doesn't already provide. The sandbox tests are using the simpler, more idiomatic approach. If anything, the custom helpers in `create_test.go` are the ones that should eventually be replaced with `strings.Contains`. Not a maintainability concern for this change. **Resolved (no action needed).**

### 3. Advisory (accepted): Flag help text says "on failure"

**Previous finding**: `install.go:219` defines the `--json` flag as `"Emit structured JSON error output on failure"`, but sandbox mode emits JSON on success too.

**Resolution**: The flag help text at `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/cmd/tsuku/install.go:219` still reads:

```go
installCmd.Flags().BoolVar(&installJSON, "json", false, "Emit structured JSON error output on failure")
```

This remains technically inaccurate for sandbox mode where JSON is emitted for all outcomes (passed, failed, skipped, error). That said, this is a one-line help string, not API documentation, and the behavior difference is clearly documented in the `runSandboxInstall` godoc (lines 34-36 of `install_sandbox.go`). The risk of a next developer being confused by a flag help string is low -- they'll see the behavior in the code before they read the flag text. **Accepted as-is.**

## New findings

None. The code is clear. The `buildSandboxJSONOutput` / `emitSandboxJSON` split is well-motivated and well-documented. Test coverage hits all result states (passed, failed, skipped, error, passed-but-verify-failed, round-trip). The `resolveEnvFlags` godoc accurately describes its behavior.
