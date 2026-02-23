# Maintainer Review: Issue #1944 -- feat(sandbox): add --json flag for structured sandbox output

**1 blocking, 2 advisory**

---

## BLOCKING

### 1. Double JSON output on sandbox failure

**`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/cmd/tsuku/install_sandbox.go:143-157`** and **`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/cmd/tsuku/install.go:83-85`**

When `--sandbox --json` is used and the sandbox test fails, two JSON objects are printed to stdout:

1. `emitSandboxJSON` (install_sandbox.go:145) calls `printJSON(out)` -- emitting the `sandboxJSONOutput` struct.
2. `emitSandboxJSON` returns a non-nil error (`"sandbox test failed with exit code %d"`).
3. `runSandboxInstall` propagates that error to the caller.
4. The `Run` handler (install.go:83-85) passes it to `handleInstallError`.
5. `handleInstallError` (install.go:371-380) sees `installJSON` is true and calls `printJSON(resp)` -- emitting a second `installError` struct.

A CI workflow parsing with `jq` will receive two concatenated JSON objects on stdout. The `emitSandboxJSON` godoc even promises "writes a single JSON object to stdout" (line 138), but the error propagation path breaks that promise.

The same double-emit occurs for `result.Error != nil` (line 151-152) and `!result.Passed` (line 154-155).

**Fix:** Either have `emitSandboxJSON` call `exitWithCode` directly on failure (bypassing `handleInstallError`), or return a sentinel error type that `handleInstallError` recognizes and skips JSON output for. The simplest fix is to check `installJSON` and `installSandbox` in `handleInstallError` and skip JSON emission when both are set, since `emitSandboxJSON` already handled it.

**Trace for clarity:**

```
install.go:83    err := runSandboxInstall(...)    // returns non-nil error on failure
install.go:84    handleInstallError(err)           // checks installJSON -> printJSON(resp)
                                                   // second JSON object on stdout
```

```
install_sandbox.go:145  printJSON(out)             // first JSON object on stdout
install_sandbox.go:155  return fmt.Errorf(...)     // propagates to handleInstallError
```

---

## ADVISORY

### 2. Flag description says "on failure" but sandbox emits JSON on success too

**`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/cmd/tsuku/install.go:219`**

```go
installCmd.Flags().BoolVar(&installJSON, "json", false, "Emit structured JSON error output on failure")
```

Without `--sandbox`, this is accurate: `installJSON` is only checked inside `handleInstallError`. But with `--sandbox --json`, the flag also emits a full result JSON on success (install_sandbox.go:130-131). The next developer reading the flag help will think `--json` is error-only and won't expect stdout output on a passing sandbox run.

Consider updating the description to something like `"Emit structured JSON output (success and failure with --sandbox, errors only otherwise)"`, or at minimum adding a note in the `--sandbox` flag help that `--json` changes its output mode.

### 3. Divergent substring helpers in the same package

**`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/cmd/tsuku/install_sandbox_test.go:376-389`** and **`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/cmd/tsuku/create_test.go:476-485`**

Two nearly identical substring search helpers exist in `package main` test files:

- `contains` / `searchSubstring` in `install_sandbox_test.go`
- `containsString` / `containsSubstring` in `create_test.go`

Both carry a comment about avoiding importing `strings`, but `package main` test files can import `strings` without issue. The real risk is that the next developer adds a third variant or modifies one but not the other. Since both are in the same Go package, they'll get a compile error if they accidentally define both with the same name, so this is low-risk. But consolidating into a shared `testutil_test.go` helper (or just using `strings.Contains`) would remove the trap.
