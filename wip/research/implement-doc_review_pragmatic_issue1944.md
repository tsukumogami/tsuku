# Pragmatic Review: Issue #1944 (--json flag for sandbox output)

**1 blocking, 1 advisory**

---

## BLOCKING

**1. Hand-rolled `contains` / `searchSubstring` reimplements `strings.Contains`**
`cmd/tsuku/install_sandbox_test.go:376-389` -- `contains()` and `searchSubstring()` manually implement substring search "to avoid importing strings in a test file that already uses the main package." The comment's rationale is wrong: this is `package main`, and other test files in the same package (`install_test.go`, `create_test.go`, `plan_utils_test.go`) already import `"strings"`. This is 12 lines of dead-weight code duplicating a stdlib one-liner. Replace all `contains(` calls with `strings.Contains(` and delete both functions.

---

## ADVISORY

**2. `emitSandboxJSON` is a single-caller wrapper around `buildSandboxJSONOutput` + `printJSON`**
`cmd/tsuku/install_sandbox.go:143-158` -- Called from exactly one site (line 131). The function does three things: call `buildSandboxJSONOutput`, call `printJSON`, then map result states to errors. The `buildSandboxJSONOutput` extraction is justified (testable pure function), but `emitSandboxJSON` itself could be inlined into `runSandboxInstall`. Not blocking because the function is small and its name adds clarity to the call site.
