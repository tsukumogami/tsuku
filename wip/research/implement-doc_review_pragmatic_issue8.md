# Pragmatic Review: Issue 8

**Issue:** #8 feat(cli): add source-directed loading to update, outdated, and verify
**Focus:** pragmatic (over-engineering, dead code, simplest correct approach)
**Files reviewed:** cmd/tsuku/update.go, cmd/tsuku/outdated.go, cmd/tsuku/verify.go, cmd/tsuku/source_directed_test.go, internal/recipe/loader.go

## Findings

### 1. ParseAndCache takes an unused context.Context parameter

**File:** `internal/recipe/loader.go:445`
**Severity:** Advisory

`ParseAndCache(_ context.Context, name string, data []byte)` ignores the context parameter. Both callers pass `context.Background()`. The method does CPU-only TOML parsing with no I/O or cancellation points. This is speculative generality -- the parameter serves no current purpose.

However, this follows Go convention for methods that may gain I/O in the future, and the cost is one extra argument at two call sites. Not worth blocking over.

### 2. loadRecipeForTool is well-placed, not over-abstracted

**File:** `cmd/tsuku/outdated.go:147-175`
**Severity:** n/a (positive)

`loadRecipeForTool` is called from three distinct commands (update, outdated, verify). The shared helper eliminates what would be tripled logic for source-directed loading. This is the right level of abstraction.

### 3. isDistributedSource is minimal and correctly scoped

**File:** `cmd/tsuku/update.go:106-108`
**Severity:** n/a (positive)

Single-line helper, called from `loadRecipeForTool` and tested. Uses `strings.Contains(source, "/")` which correctly distinguishes "owner/repo" from "central", "local", "embedded", and "". No over-engineering.

### 4. update.go silently discards state Load error

**File:** `cmd/tsuku/update.go:67`
**Severity:** Advisory

`state, _ := mgr.GetState().Load()` discards the error. If state fails to load, `state` is nil, and `loadRecipeForTool` handles nil state by falling through to the normal chain (line 149 of outdated.go). So this is safe -- the tool still updates, just without source-directed loading. The fallback is appropriate for a best-effort optimization.

The `outdated` command by contrast does `if stateErr != nil { exitWithCode(ExitGeneral) }` (line 49-52). The difference is justified: outdated needs state to enumerate tools, while update already verified installation via `mgr.List()`.

## Summary

No blocking findings. The implementation is the simplest correct approach for the problem. The shared `loadRecipeForTool` helper avoids duplication across three commands. The scrutiny fix correctly addressed the original `ensureSourceProvider` divergence by switching update.go to use `loadRecipeForTool` + `CacheRecipe`. No dead code remains from the fix.

**Blocking:** 0
**Advisory:** 2
