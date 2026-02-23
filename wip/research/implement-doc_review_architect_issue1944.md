# Architect Review: #1944 feat(sandbox): add --json flag for structured sandbox output

**1 blocking, 2 advisory**

## Blocking

### 1. Double JSON output on sandbox failure breaks the "single JSON object" contract

**`cmd/tsuku/install_sandbox.go:143-157` and `cmd/tsuku/install.go:83-84`**

When `--sandbox --json` is set and the sandbox test fails (either `result.Error != nil` or `!result.Passed`), two JSON objects are written to stdout:

1. `emitSandboxJSON()` calls `printJSON(out)` at line 145, writing a `sandboxJSONOutput` object.
2. `emitSandboxJSON()` returns a non-nil error (lines 152 or 155).
3. `runSandboxInstall()` returns that error to `install.go:83`.
4. `handleInstallError(err)` at `install.go:84` checks `installJSON` (true) and calls `printJSON(resp)` at line 380, writing an `installError` object.

Result: stdout contains two JSON objects with different schemas. CI workflows parsing with `jq` will break because `jq` expects a single JSON value by default.

The comment at `install_sandbox.go:32-36` explicitly states: "stdout contains exactly one JSON object." The implementation violates its own contract.

The fix is to prevent `handleInstallError` from emitting JSON when the sandbox JSON path has already written output. Options:
- Have `emitSandboxJSON` call `exitWithCode` directly on failure instead of returning an error (matching how `emitSandboxHumanReadable` propagates errors but only prints text, which is harmless to duplicate in human mode).
- Have `emitSandboxJSON` return a sentinel error type that `handleInstallError` recognizes and skips JSON output for.
- Handle the sandbox error exit inline in `install.go:83-84` rather than delegating to `handleInstallError`.

Note: plan generation errors (lines 48-76 of `install_sandbox.go`) correctly return errors *before* any JSON is written, so they produce exactly one JSON object via `handleInstallError`. The problem is only when the sandbox runs successfully enough to produce a `SandboxResult` that indicates failure.

**Blocking** because downstream CI workflows (#1945, #1946, #1947) will parse this output with `jq` and get broken results. The bug compounds: every workflow migration will need to work around double JSON until this is fixed.

## Advisory

### 2. `--json` flag description is inconsistent with the new sandbox behavior

**`cmd/tsuku/install.go:219`**

The flag is described as "Emit structured JSON error output on failure" but in sandbox mode it emits JSON on *all* outcomes (success, failure, skipped). Every other command's `--json` flag uses "Output in JSON format" (e.g., `validate.go:234`, `list.go:183`, `versions.go:106`). The description is now misleading for the sandbox path and inconsistent with the codebase convention.

Consider changing to "Output in JSON format" or "Emit structured JSON output" to match the codebase pattern and accurately describe sandbox behavior.

**Advisory** because the flag works correctly; only the help text is misleading. No downstream code reads the description string.

### 3. Test helper `contains` and `searchSubstring` reimplement `strings.Contains`

**`cmd/tsuku/install_sandbox_test.go:378-389`**

The test file defines custom `contains()` and `searchSubstring()` functions, with a comment explaining this avoids "importing strings in a test file that already uses the main package." But this isn't a real constraint -- test files in `package main` can import `strings` without issue. The codebase already imports `strings` in `install_sandbox.go` (same package). Multiple other test files in `cmd/tsuku/` import `strings`.

This creates a parallel pattern (custom substring search) where the standard library function suffices. It won't compound since it's contained in test code, but it's unnecessary complexity.

**Advisory** because it's test-only code with no callers outside this file.
