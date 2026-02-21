# Pragmatic Review: Issue #1644

**Issue**: #1644 test(llm): add end-to-end integration test without cloud keys
**Focus**: pragmatic (simplicity, YAGNI, KISS)
**File reviewed**: `internal/llm/local_e2e_test.go`

## Findings

### Finding 1: `findAddonBinary` go.mod walk is more general than needed (Advisory)

**File**: `internal/llm/local_e2e_test.go:169-183`
**Severity**: Advisory

`findAddonBinary` walks up the directory tree looking for `go.mod` to find the workspace root, then probes `tsuku-llm/target/{release,debug}/tsuku-llm`. This only executes after `TSUKU_LLM_BINARY` is unset, and the function is test-only code behind an `e2e` build tag, so the generality is bounded. The walk loop is 6 lines and clearly intentional -- it handles running the test from any subdirectory. Not worth inlining.

No action needed.

### Finding 2: `recipeGenerationSystemPrompt` and `recipeGenerationUserPrompt` are test-local helpers (Advisory)

**File**: `internal/llm/local_e2e_test.go:243-276`
**Severity**: Advisory

These two helpers are called exactly once each (line 98 and line 101). They could be inlined into `TestE2E_CreateWithLocalProvider`. However, they're named descriptively and extracting them improves readability of the test body. The production system prompt (`buildSystemPrompt()` in `client.go:291`) is intentionally NOT reused -- the E2E test uses a simplified prompt to control for prompt-induced flakiness, which is a reasonable test design choice.

No action needed.

### Finding 3: Cleanup path ignores errors silently (Advisory)

**File**: `internal/llm/local_e2e_test.go:150-153`

```go
if localProvider, ok := provider.(*LocalProvider); ok {
    _ = localProvider.Shutdown(ctx, true)
    _ = localProvider.Close()
}
```

Both `Shutdown` and `Close` errors are discarded. For test cleanup this is standard practice -- the test has already passed or failed by this point, and leaking a short-lived test server with a 30s idle timeout is harmless. The type assertion guard handles the impossible case where `provider` isn't `*LocalProvider`, which is belt-and-suspenders but costs nothing.

No action needed.

### Finding 4: `validateExtractPattern` is conditional and may never execute (Advisory)

**File**: `internal/llm/local_e2e_test.go:143-147`

The test validates tool call structure only IF the model returns `extract_pattern` directly. With small models, the first response is more likely to be `fetch_file` or `inspect_archive`. This means `validateExtractPattern` and its platform coverage checks (lines 234-238) may never run in practice. The test comment at line 119-122 acknowledges this explicitly.

This is a reasonable design choice for an E2E test against a nondeterministic model. The primary assertion (line 123: at least one tool call with a valid name) is the real gate. The `validateExtractPattern` path is bonus validation when the model cooperates.

No action needed.

## Summary

| Level | Count |
|-------|-------|
| Blocking | 0 |
| Advisory | 4 |

The test file is 277 lines, adds two tests, and introduces no new abstractions, packages, or production code. The helpers are test-scoped and appropriately sized. The `e2e` build tag correctly gates hardware-dependent tests. No over-engineering detected.
