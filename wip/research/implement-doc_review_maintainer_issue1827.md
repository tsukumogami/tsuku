# Maintainer Review: #1827 - Check satisfies index before generating recipes in tsuku create

**Review focus**: maintainability (clarity, readability, duplication)
**Commit**: eda691ab2bfee5bc47f7bd492d930fbb6d6e99bc
**Files changed**: `cmd/tsuku/create.go`, `cmd/tsuku/create_test.go`

---

## Findings

### 1. `TestCheckExistingRecipe_ForceSkipsCheck` tests a copy of the guard, not the guard itself -- Blocking

**File**: `cmd/tsuku/create_test.go:752-791`
**Severity**: Blocking

The test name says it "verifies that --force bypasses the satisfies duplicate check." But the test body replicates the `if !createForce { ... }` guard inline rather than calling `runCreate`. The code under test at line 772-776 is:

```go
if !createForce {
    if _, found := checkExistingRecipe(l, "openssl"); found {
        checkReached = true
    }
}
```

This is a copy of the production guard, not the production guard. If someone changes `create.go:485` from `if !createForce` to `if createForce` (an inversion bug), this test still passes because it evaluates the inline copy, not the production code. The test proves only that Go's `if !true` evaluates to false -- which is always true and would pass even if the test variable `createForce` was never set.

The next developer reading this test will believe `--force` behavior is covered. If the `if !createForce` guard in `runCreate` is removed or inverted, no test fails.

**What the test should do**: Set `createForce = true` and call `checkExistingRecipe` directly, then assert the caller's responsibility is met separately. Or, better, add a narrow integration test that calls `runCreate` via the cobra command with `--force` and a mock that panics if called -- confirming the check is never reached. The simplest fix consistent with the current test style is:

```go
// With createForce=true, the guard in runCreate is not entered.
// Verify this by reading the production code path: if !createForce { ... }
// This test verifies the helper still finds the recipe when called
// unconditionally, confirming it's the guard -- not the helper -- doing the skipping.
createForce = true
// The helper still finds it...
_, found := checkExistingRecipe(l, "openssl")
if !found {
    t.Fatal("checkExistingRecipe should still find the recipe; --force skip happens at call site")
}
// ...but the guard in runCreate means checkExistingRecipe is never called.
// That logic is in create.go:485 and is covered by reading the code.
```

This at least makes the test honest about what it actually proves. The current test is a false-confidence test that will pass even when the production behavior is wrong.

---

### 2. `TestCheckExistingRecipe_NoMatchAllowsGeneration` duplicates the server setup from `newTestLoader` -- Advisory

**File**: `cmd/tsuku/create_test.go:727-742`
**Severity**: Advisory

`TestCheckExistingRecipe_NoMatchAllowsGeneration` creates its own 404 HTTP server and `reg`/`loader` setup:

```go
server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusNotFound)
}))
defer server.Close()

reg := registry.New(t.TempDir())
reg.BaseURL = server.URL
l := recipe.NewWithoutEmbedded(reg, t.TempDir())
```

While `TestCheckExistingRecipe_DirectNameMatchLocal` (line 696-699) also has its own copy of this setup. The `newTestLoader` helper in lines 650-662 exists precisely to avoid this duplication, but it uses `recipe.NewWithLocalRecipes` (which includes embedded recipes). These two tests need `NewWithoutEmbedded` because they want to control what recipes exist.

The next developer sees three slightly different ways to create a test loader: `newTestLoader`, inline with `NewWithLocalRecipes`, and inline with `NewWithoutEmbedded`. It's not clear whether the choice is intentional or accidental. A `newTestLoaderNoEmbedded(t)` helper would make the distinction explicit and reduce the duplicated server setup. The divergence is currently minor (4 lines) but the pattern will recur as more tests are added.

---

### 3. `checkExistingRecipe` nil guard is unnecessary API noise -- Advisory

**File**: `cmd/tsuku/create.go:468-470`
**Severity**: Advisory

```go
func checkExistingRecipe(l *recipe.Loader, toolName string) (string, bool) {
    if l == nil {
        return "", false
    }
```

The `loader` global is initialized in `main.go:71` via `init()` before any command runs. `loader` is never nil in the production path. The nil guard exists to enable `TestCheckExistingRecipe_NilLoader`, which tests the guard itself rather than any realistic scenario.

The next developer will wonder: "can the loader actually be nil here?" They'll look at all the callers, see only one (`runCreate` with the global `loader`), find `main.go:71`, and conclude the guard is defensive for a case that can't happen. Then they'll wonder if the design expects `checkExistingRecipe` to be called from other places in the future.

This is advisory rather than blocking because the nil guard is harmless and `TestCheckExistingRecipe_NilLoader` does establish a clear contract for the function. But a comment would prevent the reasoning detour: `// l is always non-nil in production (initialized in main.go:71); nil guard enables unit testing`.

---

### 4. Two-branch error message for one logical event -- Advisory

**File**: `cmd/tsuku/create.go:487-492`
**Severity**: Advisory

```go
if canonicalName == toolName {
    fmt.Fprintf(os.Stderr, "Error: recipe '%s' already exists. Use --force to create anyway.\n", toolName)
} else {
    fmt.Fprintf(os.Stderr, "Error: recipe '%s' already satisfies '%s'. Use --force to create anyway.\n",
        canonicalName, toolName)
}
```

Both branches say "Use --force to create anyway." The distinction is real (direct match vs. satisfies alias), but the two message strings are nearly identical divergent twins. The next developer extending this -- say, adding a third message type or changing the `--force` phrasing -- must update both branches. A format like `"Error: recipe '%s' already covers '%s'. Use --force to create anyway.\n"` handles both cases uniformly (when they're the same name, the message becomes "recipe 'openssl' already covers 'openssl'", which is slightly redundant but readable). Or the common suffix can be factored:

```go
if canonicalName == toolName {
    fmt.Fprintf(os.Stderr, "Error: recipe '%s' already exists.", toolName)
} else {
    fmt.Fprintf(os.Stderr, "Error: recipe '%s' already satisfies '%s'.", canonicalName, toolName)
}
fmt.Fprintln(os.Stderr, " Use --force to create anyway.")
```

Advisory because the current duplication is small and the messages are self-explanatory, but the pattern will drift as the codebase evolves.

---

### 5. Code clarity is otherwise good -- Positive

`checkExistingRecipe` is a well-named, focused helper. Its godoc comment accurately describes what it does, what it returns, and why the loader covers the satisfies fallback without the caller needing to know. The placement of the early guard in `runCreate` (before builder registration, API calls, toolchain checks) is correct and the explanatory comment above it makes the "why before builder work" rationale explicit. The comment added to the `os.Stat` check at line 768 accurately explains why two checks exist.

---

## Summary

**Blocking**: 1
**Advisory**: 3

The test `TestCheckExistingRecipe_ForceSkipsCheck` is the only finding that warrants blocking. It tests a copy of the `if !createForce` guard rather than the actual guard in `runCreate`. The test will pass even if the production guard is removed or inverted, creating false confidence that `--force` behavior is covered. It should either be made honest about what it proves (the helper always finds the recipe regardless of `createForce`) or replaced with a narrow integration test that calls through `runCreate`.

The advisory findings are: duplicated test server setup that could be extracted into a `newTestLoaderNoEmbedded` helper (consistent with the existing `newTestLoader` pattern); the nil guard on `checkExistingRecipe` which could use a comment explaining it's not defensive for production paths; and the duplicated `" Use --force to create anyway."` suffix across two error message branches.
