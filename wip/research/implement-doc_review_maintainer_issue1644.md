# Maintainer Review: Issue #1644

**Issue**: #1644 test(llm): add end-to-end integration test without cloud keys
**Focus**: maintainer (clarity, readability, duplication)
**Reviewer**: maintainer-reviewer

## Files Reviewed

- `internal/llm/local_e2e_test.go` (new file, primary deliverable)
- `internal/llm/factory.go` (HasProvider/ProviderCount additions)
- `internal/llm/factory_test.go` (tests for HasProvider/ProviderCount)

## Findings

### Finding 1: Divergent prompt twins -- test uses a simplified prompt that will drift from production (Advisory)

**File**: `internal/llm/local_e2e_test.go:243-276`
**Severity**: Advisory

The test defines its own `recipeGenerationSystemPrompt()` and `recipeGenerationUserPrompt()` functions that are simplified versions of the production prompts in `internal/llm/client.go:291-313` (the `buildSystemPrompt()` function) and `internal/builders/github_release.go:1078-1112` (another `buildSystemPrompt()` with additional examples).

Key differences between the test prompt and the production prompts:
- Test says "You are a tool installation expert"; production says "You are an expert at analyzing GitHub releases to create installation recipes for tsuku, a package manager"
- Test omits common platform naming patterns (Rust-style targets, Go-style targets) present in both production prompts
- Test omits the k9s JSON example that exists in `github_release.go`'s version
- Test hardcodes asset names in the user prompt instead of using real release data

The next developer updating the production prompt (e.g., adding a new tool or changing instructions) won't know to update the E2E test's prompt. If the production prompt changes significantly, the E2E test might pass with the old simplified prompt while the real flow fails, or vice versa.

The test comment at line 244 says "This mirrors the prompt structure used by the real tsuku create command" -- but it doesn't mirror it; it's a simplified alternative. The comment will mislead the next developer into thinking this is a faithful copy.

**Suggestion**: Either (a) import and use the actual `buildSystemPrompt()` and construct a `GenerateRequest` to use `buildUserMessage()`, or (b) change the comment to "This is a simplified prompt for testing that the local provider can handle structured extraction. It intentionally differs from the production prompt in client.go." Option (b) is probably better since the test is validating the provider plumbing, not prompt quality.

### Finding 2: Test cleanup relies on type assertion to internal type (Advisory)

**File**: `internal/llm/local_e2e_test.go:149-153`

```go
if localProvider, ok := provider.(*LocalProvider); ok {
    _ = localProvider.Shutdown(ctx, true)
    _ = localProvider.Close()
}
```

The cleanup code uses a type assertion to `*LocalProvider` to call `Shutdown` and `Close`. Since this test lives in the `llm` package (not `llm_test`), it has access to unexported types, which is fine. But the errors from both `Shutdown` and `Close` are silently discarded with `_`. If shutdown fails and leaves a stale socket, subsequent test runs on the same machine may behave differently (connecting to an orphaned server vs starting a fresh one).

This isn't blocking because `t.TempDir()` isolates the `TSUKU_HOME`, so the socket lives in a temp directory that gets cleaned up. But the silent error discard could hide real problems during local debugging.

**Suggestion**: Use `t.Cleanup` to register the shutdown, and log (not fail) if errors occur:

```go
t.Cleanup(func() {
    if lp, ok := provider.(*LocalProvider); ok {
        if err := lp.Shutdown(context.Background(), true); err != nil {
            t.Logf("warning: addon shutdown error: %v", err)
        }
        if err := lp.Close(); err != nil {
            t.Logf("warning: connection close error: %v", err)
        }
    }
})
```

This also fixes the subtle issue that if the test fails between getting the provider and the cleanup block, the server doesn't get shut down at all.

### Finding 3: findAddonBinary returns empty string and caller checks for empty (Advisory)

**File**: `internal/llm/local_e2e_test.go:60-63, 157-196`

```go
addonPath := findAddonBinary(t)
if addonPath == "" {
    t.Skip(...)
}
```

`findAddonBinary` takes `*testing.T` as a parameter (calling `t.Helper()`) but never uses `t` for anything except the helper registration. The function could be a plain function returning `(string, bool)` or just `string`. This is a minor readability issue -- the `t.Helper()` call suggests the function might call `t.Fatal` or `t.Error`, but it doesn't. The next developer might add assertions inside it thinking that's the intended pattern.

**Suggestion**: Either remove the `*testing.T` parameter since it's unused beyond `Helper()`, or document that this is intentionally a test helper that only returns a value.

### Finding 4: validTools map rebuilt on every test run (Advisory)

**File**: `internal/llm/local_e2e_test.go:132-136`

```go
validTools := map[string]bool{
    ToolFetchFile:      true,
    ToolInspectArchive: true,
    ToolExtractPattern: true,
}
```

This is fine for a test. Uses the named constants from `tools.go` correctly, which avoids the magic string problem. No finding needed -- just noting it's done well.

### Finding 5: Test names are accurate and well-documented (No finding)

Both test functions have clear names that match their behavior:
- `TestE2E_FactoryFallbackToLocal` tests exactly factory fallback
- `TestE2E_CreateWithLocalProvider` tests the full creation flow

The doc comments explain preconditions, what's exercised, and how to run. The `scenario-13`/`scenario-14` references connect back to the test plan. This is good test documentation.

### Finding 6: HasProvider and ProviderCount are clear, well-tested (No finding)

`factory.go:267-276` adds two straightforward accessor methods. Names match behavior exactly. Tests cover positive and negative cases. No concerns.

## Summary

| Level | Count |
|-------|-------|
| Blocking | 0 |
| Advisory | 3 |

The test file is well-structured, clearly documented, and follows established patterns in the codebase. The three advisory findings are minor:

1. The test's simplified prompts have a misleading comment saying they "mirror" production prompts when they actually differ in meaningful ways. The comment should say the prompts are intentionally simplified.
2. Server cleanup could use `t.Cleanup` to be more robust against mid-test failures.
3. `findAddonBinary` accepts `*testing.T` but doesn't use it, which is mildly misleading.

None of these will cause the next developer to introduce a bug, but finding #1 has the most long-term risk: someone updating the production prompt might trust the E2E test as a safety net, not realizing it uses different prompts.
