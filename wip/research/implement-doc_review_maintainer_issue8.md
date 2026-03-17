# Maintainer Review: Issue 8

**Issue:** #8 feat(cli): add source-directed loading to update, outdated, and verify
**Focus:** maintainer (readability, naming, next-developer clarity)
**Files reviewed:** cmd/tsuku/update.go, cmd/tsuku/outdated.go, cmd/tsuku/verify.go, cmd/tsuku/source_directed_test.go, internal/recipe/loader.go

## Finding 1: Implicit contract between CacheRecipe and installWithDependencies

**File:** `cmd/tsuku/update.go:67-70`
**Severity:** Advisory

```go
state, _ := mgr.GetState().Load()
if r, err := loadRecipeForTool(context.Background(), toolName, state, cfg); err == nil && r != nil {
    loader.CacheRecipe(toolName, r)
}
```

The comment on lines 62-66 does a good job explaining WHY this caching is needed. But the coupling is fragile: `loadRecipeForTool` fetches and parses the recipe, then `CacheRecipe` injects it into the global loader so that `runInstallWithTelemetry` -> `installWithDependencies` -> `loader.Get` hits the cache instead of walking the provider chain.

The next developer looking at `installWithDependencies` (in `install_deps.go:213`) will see `loader.Get(toolName, recipe.LoaderOptions{})` and not know it relies on a pre-populated cache entry from the caller. If they refactor `installWithDependencies` to clear the cache or skip it, the update command silently loses source-directed loading.

This isn't blocking because the comment in update.go is clear enough, and `CacheRecipe`'s godoc explains its purpose. But the dependency runs through a global mutable cache with no compile-time enforcement.

## Finding 2: loadRecipeForTool lives in outdated.go

**File:** `cmd/tsuku/outdated.go:141-175`
**Severity:** Advisory

`loadRecipeForTool` is a shared helper called by `update.go`, `outdated.go`, and `verify.go`, but it's defined in `outdated.go`. The next developer looking for where recipe loading is customized will grep for `loadRecipeForTool` and find it in `outdated.go`, which is misleading -- it's not specific to the outdated command.

Similarly, `isDistributedSource` is defined in `update.go` but used by `loadRecipeForTool` in `outdated.go`. These shared helpers would be more discoverable in a file that signals shared purpose (like `helpers.go` or a new `source_directed.go`).

This doesn't cause bugs but creates a "why is it here?" moment for the next person reading `outdated.go`. The test file is already named `source_directed_test.go`, suggesting the author recognized these are cross-cutting concerns -- the production code should match.

## Finding 3: Test coverage for update command's CacheRecipe pattern

**File:** `cmd/tsuku/source_directed_test.go`
**Severity:** Advisory

The tests thoroughly cover `loadRecipeForTool` (7 test cases) and `ParseAndCache` (2 test cases), but none test the update command's specific pattern: call `loadRecipeForTool`, then `CacheRecipe`, then verify that a subsequent `loader.Get` returns the cached recipe rather than walking the chain.

This matters because the update command's correctness depends on the cache-then-get coupling (Finding 1). If `ParseAndCache` or `CacheRecipe` ever changes to use a different cache key, the update flow breaks silently. A test like `TestUpdateCachesDistributedRecipeForInstallFlow` that sets up a distributed provider, calls `loadRecipeForTool` + `CacheRecipe`, and then verifies `loader.Get` returns the same recipe would document this contract.

## Finding 4: ParseAndCache takes unused context parameter

**File:** `internal/recipe/loader.go:445`
**Severity:** Advisory

```go
func (l *Loader) ParseAndCache(_ context.Context, name string, data []byte) (*Recipe, error) {
```

The context parameter is accepted but explicitly discarded (underscore). This is a minor naming/signature issue -- callers pass `context.Background()` for no reason. However, keeping the context parameter anticipates future use (e.g., if parsing ever needs cancellation). The underscore makes the unused-ness visible, which is the idiomatic Go way to handle this. No action needed unless the codebase prefers removing unused parameters.

## Finding 5: Good patterns worth noting

- `loadRecipeForTool` is well-structured: clear branching (empty/non-distributed -> chain, distributed -> GetFromSource with fallback), consistent warning format for unreachable sources, nil-safe state handling.
- The test file uses `mockProvider` and `unreachableProvider` test doubles effectively, and the test names accurately describe what they validate.
- The comment block in `update.go:62-66` explains the anti-shadowing motivation clearly.
- `TestLoadRecipeForTool_EmbeddedSource` was added to close the scrutiny gap -- good follow-through.

## Summary

| # | Finding | Severity | Location |
|---|---------|----------|----------|
| 1 | Implicit cache coupling between update.go and install_deps.go | Advisory | update.go:67-70 |
| 2 | Shared helper loadRecipeForTool defined in outdated.go, not a shared file | Advisory | outdated.go:141 |
| 3 | No test for update's cache-then-get pattern | Advisory | source_directed_test.go |
| 4 | ParseAndCache accepts unused context | Advisory | loader.go:445 |

**Blocking findings:** 0
**Advisory findings:** 4

The implementation is clear, well-commented, and follows the existing codebase patterns. The scrutiny fix (replacing `ensureSourceProvider` with `loadRecipeForTool` + `CacheRecipe`) resolved the design intent violation. The remaining findings are minor organizational suggestions that don't create misread risk.
