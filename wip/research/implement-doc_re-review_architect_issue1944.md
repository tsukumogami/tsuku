# Architect Re-Review: #1944 feat(sandbox): add --json flag for structured sandbox output

**0 blocking, 1 advisory**

## Blocking finding from previous round: RESOLVED

### Double JSON output on sandbox failure

**Previous finding:** `emitSandboxJSON()` returned a non-nil error on failure, which propagated to `handleInstallError()` in `install.go:84`, causing a second JSON object to be written to stdout.

**Fix applied:** `emitSandboxJSON()` (`cmd/tsuku/install_sandbox.go:144-154`) now calls `exitWithCode(ExitInstallFailed)` directly on failure/error states (line 151) instead of returning an error. This matches the first remediation option from the previous review.

The control flow is now correct for all sandbox JSON paths:

- **Plan generation errors** (lines 48-76): `runSandboxInstall` returns error before any JSON is written. `handleInstallError` emits one `installError` JSON object. Correct.
- **Sandbox failure/error** (line 150-151): `emitSandboxJSON` writes one `sandboxJSONOutput` object, then calls `exitWithCode` directly. Process terminates. `handleInstallError` never reached. Correct.
- **Sandbox success** (line 153): `emitSandboxJSON` writes one `sandboxJSONOutput` object, returns `nil`. `handleInstallError` not called. Correct.

The "exactly one JSON object" contract documented at `install_sandbox.go:33` is now upheld in all cases.

## Previous advisory findings

### Advisory #2 (flag description): Still present, still advisory

`cmd/tsuku/install.go:219` -- The `--json` flag is described as "Emit structured JSON error output on failure" but in sandbox mode it emits JSON on all outcomes including success. Unchanged from previous round. Not blocking because the flag works correctly and no downstream code reads the description.

### Advisory #3 (test helper functions): Resolved

The custom `contains()` and `searchSubstring()` helpers are gone. The test file now imports `strings` directly (`cmd/tsuku/install_sandbox_test.go:7`).

## Architecture fit

The fix follows the established pattern in this codebase: functions that handle terminal output for a specific mode call `exitWithCode` directly rather than returning errors through a generic handler. This matches how `handleAmbiguousInstallError` (`install.go:407-429`) works -- it prints its specialized JSON and calls `exitWithCode` without returning. The sandbox JSON path now uses the same pattern.
