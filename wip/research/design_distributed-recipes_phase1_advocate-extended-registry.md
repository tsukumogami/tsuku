# Advocate: Extended Registry

## Approach Description

The Extended Registry approach keeps the Loader's existing 4-tier resolution chain (in-memory cache, local filesystem, embedded, registry) largely intact, but extends the "registry" tier to consult an ordered list of Registry instances instead of a single one. A distributed source (e.g., a GitHub repository hosting recipes) becomes another `registry.Registry` instance configured with a GitHub-specific base URL and its own cache subdirectory. The central registry always comes first in the list, preserving unqualified-name priority.

Concretely:
1. The `Loader` struct's `registry *registry.Registry` field becomes `registries []*registry.Registry`, with the central registry at index 0.
2. `fetchFromRegistry()` iterates the list, trying each registry's cache then remote in sequence, stopping at the first hit.
3. Each Registry instance gets its own `CacheDir` subdirectory (e.g., `$TSUKU_HOME/registry/central/`, `$TSUKU_HOME/registry/github.com/someorg/somerepo/`), so cache metadata and TTL management remain per-registry.
4. `RecipeSource` gains a new value (e.g., `SourceDistributed`) and `RecipeInfo` carries a `RegistryID` or URL so callers can distinguish provenance.
5. `Plan.RecipeSource` shifts from free-form string to structured source info (registry URL + recipe name).

No new binary dependencies -- distributed registries are fetched over HTTP using the existing `raw.githubusercontent.com` URL pattern that the central registry already uses.

## Investigation

### Loader structure (internal/recipe/loader.go)

The Loader holds a single `*registry.Registry` and calls it through three touchpoints:
- `fetchFromRegistry()` (line 282): checks `GetCached()`, falls back to `FetchRecipe()`, then `CacheRecipe()`.
- `warnIfShadows()` (line 265): calls `GetCached()` to check if a local recipe shadows a registry recipe.
- `buildSatisfiesIndex()` (line 402): calls `GetCachedManifest()` to scan for satisfies entries.

These are all concrete method calls on the single registry pointer. Extending to a list means each of these three sites needs a loop. The loop in `fetchFromRegistry()` is straightforward -- try each registry in order. The satisfies index build would need to merge manifests from all registries (central first for priority). The shadow warning would check all registries.

The Loader also exposes `Registry()` (line 344) which returns the single registry pointer. Several commands use this accessor (update-registry, cache commands). This would need to become `Registries()` or keep `Registry()` for the primary/central one.

### Registry type (internal/registry/registry.go)

The `Registry` struct is already surprisingly flexible:
- `BaseURL` is configurable (default is `raw.githubusercontent.com/tsukumogami/tsuku/main`).
- `isLocal` flag already handles local filesystem registries.
- `recipeURL()` constructs `{BaseURL}/recipes/{letter}/{name}.toml` -- this pattern works for any GitHub repo with the same directory layout.
- `CacheDir` is passed at construction time and all cache operations are relative to it.

A distributed GitHub registry would just be another `Registry` instance with `BaseURL` set to `https://raw.githubusercontent.com/{owner}/{repo}/{branch}` and `CacheDir` set to `$TSUKU_HOME/registry/{owner}/{repo}/`. No new fetch strategy is needed -- the existing HTTP client and URL pattern work as-is, as long as the distributed registry follows the same directory layout (`recipes/{letter}/{name}.toml`).

### CachedRegistry (internal/registry/cached_registry.go)

`CachedRegistry` wraps a single `Registry` with TTL logic. It's only used by `update-registry` for bulk refresh operations. The main install path goes through `Loader.fetchFromRegistry()` which does its own cache-then-fetch logic (simpler, without TTL checks). This means `CachedRegistry` can remain a wrapper around individual registries without modification -- each distributed registry could optionally have its own `CachedRegistry` wrapper for `update-registry` support.

### CacheManager (internal/registry/cache_manager.go)

The `CacheManager` operates on a `cacheDir` and manages LRU eviction within it. With per-registry cache subdirectories, each registry would need its own `CacheManager` instance, or a single manager would need to walk multiple subdirectories. The simplest path: one `CacheManager` per registry, each bounded independently. This means the global cache size limit becomes per-registry, which is a minor semantic change but avoids coupling registries' eviction policies.

### State tracking (internal/install/state.go)

`Plan.RecipeSource` is already a `string` field (line 35). It's set during plan generation but never used for source-aware operations today. Extending it to include registry identity (e.g., `"central"`, `"github.com/someorg/somerepo"`) is backward compatible -- it's just a more specific string. `ToolState` doesn't track source at all, which remains a gap, but is orthogonal to this approach (all approaches need to solve this).

### Command surface (cmd/tsuku/)

17 files use `loader.` -- these are the blast radius. Most only call `loader.Get()` or `loader.GetWithContext()`, which would continue working transparently. The commands that need explicit changes:
- `recipes.go`: `ListAllWithSource()` needs to include distributed sources.
- `update_registry.go`: needs to refresh all registries, not just central.
- `search.go`: needs to search across all registries.
- `info.go`: should show which registry a recipe came from.
- `cache.go` commands: need to handle per-registry cache dirs.

### Manifest interaction

`buildSatisfiesIndex()` reads `GetCachedManifest()` from the single registry. For distributed registries, this either requires each registry to publish its own manifest (same schema, separate URL), or the satisfies index skips distributed registries (since they're lower priority and satisfies is primarily a central-registry concern). The simpler initial path is to only use the central registry's manifest for satisfies lookups, and add distributed manifest support later.

## Strengths

- **Minimal conceptual change**: The Loader's resolution chain stays the same. Distributed registries slot into the existing "registry" tier as additional entries. Developers familiar with the codebase don't need to learn a new abstraction hierarchy.

- **Registry type already fits**: The `Registry` struct's `BaseURL` + `CacheDir` + HTTP client design works for any GitHub repository that follows the same directory layout. No new "fetch strategy" types, no strategy pattern -- just another instance with different constructor arguments.

- **Per-registry cache isolation**: Each registry gets its own cache subdirectory with its own metadata, TTL, and eviction. This prevents a distributed registry's recipes from competing with central registry recipes in LRU eviction, and allows independent cache management.

- **Central priority is structural**: Because `registries[0]` is always the central registry, and `fetchFromRegistry()` iterates in order, unqualified names always resolve to central first. This invariant is enforced by construction, not by conditional logic.

- **Backward compatible wire format**: Distributed registries use the same `recipes/{letter}/{name}.toml` layout and the same HTTP fetch mechanism. No new protocols, no git dependencies, no new authentication flows.

- **Incremental delivery**: Phase 1 can ship with just the multi-registry Loader and config parsing. Phase 2 adds `tsuku registry add/remove` commands. Phase 3 adds per-registry manifest support. Each phase is independently useful.

- **CachedRegistry remains optional**: The main install path doesn't use `CachedRegistry` -- it goes through the Loader's simpler cache-then-fetch logic. Distributed registries get the same simple caching for free. `CachedRegistry` only needs updates if `update-registry` should refresh distributed sources too.

## Weaknesses

- **Directory layout assumption**: This approach assumes distributed registries use the same `recipes/{letter}/{name}.toml` layout as the central registry. If a distributed source organizes recipes differently (flat directory, different naming), the approach breaks. This is a design constraint, not just an implementation detail -- it limits what can be a "registry."

- **No recipe namespacing**: With multiple registries, recipe names can collide. The first-match-wins semantics mean a distributed registry's `fzf` recipe is silently ignored if the central registry has one. There's no way to explicitly request `someorg/fzf`. This is by design (central priority), but users who add a distributed registry specifically for a custom `fzf` recipe will be surprised.

- **Loader method duplication grows**: `fetchFromRegistry()`, `warnIfShadows()`, and `buildSatisfiesIndex()` all loop over registries. `ListAllWithSource()` and `listRegistryRecipes()` also need loops. The Loader gains 5+ loop sites that must stay in sync. This is manageable but increases the maintenance cost of the Loader, which is already 690 lines.

- **`Registry()` accessor breaks**: 17 command files reference `loader.Registry()`, which returns a single `*registry.Registry`. Changing this to `Registries()` or keeping `Registry()` for central-only creates an API change. Most callers only need the central registry, so a `CentralRegistry()` accessor works, but it's still a rename across many files.

- **Per-registry CacheManager means per-registry limits**: The global `tsuku cache info` output becomes more complex. Users see multiple caches with independent sizes and limits. The `cache cleanup` command needs to handle all registries. This isn't hard, but it's more surface area in the CLI.

- **Manifest per registry is deferred complexity**: The initial approach skips distributed registry manifests (satisfies lookups only use central). This means `tsuku search` won't find recipes from distributed sources unless they're cached, and satisfies resolution won't know about distributed recipes. This gap is acceptable initially but will need closing.

## Deal-Breaker Risks

- **Layout rigidity could block real use cases**: If the primary distributed source use case is "my company has a private GitHub repo of custom recipes," those repos are unlikely to follow the `recipes/{letter}/{name}.toml` layout convention. They'll probably use a flat `recipes/{name}.toml` layout. The Extended Registry approach would require either (a) forcing all distributed repos to adopt the bucketed layout, which is a high adoption barrier, or (b) making `recipeURL()` configurable per registry, which undermines the "just another Registry instance" simplicity. This isn't necessarily a deal-breaker, but it's the most likely point where the approach's assumptions collide with reality. The mitigation is straightforward: add a `PathStyle` field to `Registry` that selects between "bucketed" (default) and "flat" URL construction. This is a small change that preserves the rest of the approach.

None of the risks are true deal-breakers -- each has a known mitigation path. The layout rigidity risk is the closest, but a `PathStyle` enum is a 10-line change.

## Implementation Complexity

- **Files to modify**: ~12-15 files
  - `internal/registry/registry.go`: Add `PathStyle` field, make `recipeURL()` path-style-aware (~20 lines)
  - `internal/recipe/loader.go`: Change `registry` to `registries`, update 5 methods (~80 lines changed)
  - `internal/recipe/loader_test.go`: Update tests for multi-registry (~50 lines)
  - `internal/recipe/types.go`: Add `SourceDistributed` RecipeSource constant (~5 lines)
  - `internal/install/state.go`: No changes needed (Plan.RecipeSource is already string)
  - `internal/config/config.go`: Add distributed registry config parsing (~30 lines)
  - `cmd/tsuku/main.go`: Construct multiple registries from config (~20 lines)
  - `cmd/tsuku/recipes.go`: Handle distributed source display (~10 lines)
  - `cmd/tsuku/update_registry.go`: Refresh all registries (~30 lines)
  - `cmd/tsuku/search.go`: Search across registries (~15 lines)
  - `cmd/tsuku/info.go`: Show recipe source registry (~10 lines)
  - `cmd/tsuku/cache.go`: Handle per-registry cache stats (~20 lines)

- **New infrastructure**: No. Uses existing Registry type, HTTP client, and cache directory structure. The only new concept is a config file section for distributed registries.

- **Estimated scope**: Medium. The core change (multi-registry Loader) is small (~80 lines). The tail of command updates and test changes adds up, but each change is localized and low-risk.

## Summary

The Extended Registry approach works because the existing `Registry` type is already a generic HTTP-based recipe fetcher with configurable base URL and cache directory -- it just happens to only be instantiated once today. Extending the Loader to iterate a list of registries requires changes in ~12-15 files, but each change is small and localized. The main risk is the directory layout assumption (distributed repos must follow the same bucketed structure or a `PathStyle` escape hatch is needed), but this is a 10-line fix. This approach trades abstraction elegance for implementation pragmatism: there's no new interface hierarchy, no provider pattern, no strategy objects -- just a slice of registries tried in order, with the central registry always first.
