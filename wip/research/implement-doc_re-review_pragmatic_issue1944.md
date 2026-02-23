# Re-review: Issue #1944 -- Pragmatic Reviewer

**0 blocking, 0 advisory.**

## Previous Findings -- Status

### 1. Custom contains/searchSubstring reimplementing strings.Contains (was BLOCKING)

**Fixed.** The test file now uses `strings.Contains` directly (lines 150, 175, 235, 265 of `install_sandbox_test.go`). No custom helper remains.

### 2. Double JSON output on sandbox failure (was BLOCKING, from architect/maintainer)

**Fixed.** `emitSandboxJSON` (line 144 of `install_sandbox.go`) now returns `nil` unconditionally and calls `exitWithCode(ExitInstallFailed)` directly for non-passing states (line 151). This prevents control from returning to the caller at `install.go:84`, so `handleInstallError` never fires a second JSON object. The comment at lines 140-143 documents the design choice clearly.

## New Scan

No new findings. The extracted `buildSandboxJSONOutput` pure function is a clean separation that enables testing without stdout capture -- justified, not over-engineered. `strPtr` helper in tests is idiomatic Go for `*string` literals.
