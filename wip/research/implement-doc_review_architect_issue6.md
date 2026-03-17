---
focus: architect
issue: 6
blocking_count: 0
advisory_count: 2
---

# Architect Review: Issue 6 -- DistributedProvider

## Summary

The implementation fits the existing architecture well. `DistributedProvider` implements the `RecipeProvider` and `RefreshableProvider` interfaces from `internal/recipe/provider.go`, exactly as designed in Issue 1. The dependency direction is correct (`distributed` imports `recipe`, never the reverse). The Loader routes qualified names through the provider chain without special-casing distributed providers by type -- it matches on `Source()` value, which is the intended dispatch mechanism. The wiring in `main.go` follows the same pattern as other providers: build instances, append to the slice, pass to `NewLoader`.

No blocking findings.

## Findings

### ADVISORY-1: `ForceListRecipes` is a public method with a single caller

`internal/distributed/client.go:147` -- `ForceListRecipes` duplicates most of `ListRecipes` with the cache freshness check removed. It exists solely for `DistributedProvider.Refresh()`. This is structurally fine since both methods live in the same package and the duplication is small (~25 lines). If more cache-bypass patterns emerge, consider a `listRecipesInternal(ctx, owner, repo, skipFreshCheck bool)` to reduce the surface. Not compounding -- contained to one package.

### ADVISORY-2: Loader `getFromDistributed` does a linear scan over all providers

`internal/recipe/loader.go:110-112` -- `getFromDistributed` iterates `l.providers` to find one matching `RecipeSource(qualifier)`. With many distributed registries this is O(n) per qualified-name lookup. The existing `ProviderBySource` method does the same linear scan, so this is consistent with current patterns. If the number of distributed providers grows large enough for this to matter, a `map[RecipeSource]RecipeProvider` index would be the fix, but that's a future optimization, not a structural problem today.

## Structural Observations (no action needed)

**Interface compliance**: The compile-time interface check in `provider_test.go:376-380` (`var _ recipe.RecipeProvider = p`) is the right approach for Go -- catches drift early.

**Dynamic `Source()` vs `SourceDistributed` constant**: The plan's AC says `Source()` returns `SourceDistributed`, but the implementation returns `RecipeSource("owner/repo")` -- a dynamic per-instance value. This is architecturally necessary: `GetFromSource` and `getFromDistributed` both match providers by comparing `Source()` to a stored `"owner/repo"` string. A single `SourceDistributed` constant would make it impossible to route to the correct provider when multiple distributed sources are registered. The implementation is correct; the plan AC is imprecise.

**Dependency direction**: `distributed` imports `recipe` (for `RecipeProvider`, `RecipeSource`, `RecipeInfo`). `recipe` does not import `distributed`. `cmd/tsuku/main.go` imports both to wire them together. This matches the existing layering where `cmd/` is the composition root.

**Cache isolation**: `CacheManager` in `distributed` is independent from the central registry's `internal/registry` cache. Separate TTLs, separate size limits, separate directory (`$TSUKU_HOME/cache/distributed/`). No shared state or cross-package coupling.

**Shared client across providers**: `main.go:88-93` creates one `GitHubClient` and one `CacheManager`, then shares them across all `DistributedProvider` instances. This is correct -- the client is stateless (HTTP clients + cache dir), and the cache naturally partitions by `owner/repo` subdirectories.
