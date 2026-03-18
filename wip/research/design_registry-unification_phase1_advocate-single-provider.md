# Advocate: Manifest-Driven Single Provider

## Approach Description

Replace the four concrete provider types (`LocalProvider`, `EmbeddedProvider`, `CentralRegistryProvider`, `DistributedProvider`) with a single `RegistryProvider` struct parameterized by two axes:

1. **Manifest configuration** -- determines layout (flat vs. grouped/bucketed), index URL, source identity, and cache TTL.
2. **Backing store** -- an interface that determines how bytes are fetched. Three implementations:
   - `MemoryStore` -- serves bytes from an in-memory map (replaces `EmbeddedRegistry`)
   - `FilesystemStore` -- reads from a local directory (replaces `LocalProvider`)
   - `HTTPStore` -- fetches via HTTP with disk cache (replaces both `CentralRegistryProvider`'s `registry.Registry` and `DistributedProvider`'s `GitHubClient`)

Each registry becomes an instance of `RegistryProvider` created from a manifest:

```go
type RegistryProvider struct {
    name     string          // human-readable identifier ("central", "local", "owner/repo")
    source   RecipeSource    // for Source() method
    layout   Layout          // flat or grouped (first-letter bucketing)
    indexURL string          // optional manifest/index endpoint
    store    BackingStore    // how to get bytes
}

type BackingStore interface {
    Get(ctx context.Context, path string) ([]byte, error)
    List(ctx context.Context) ([]string, error)
    // Optional: SatisfiesEntries, Refresh -- via interface assertions or method on store
}
```

The central registry's baked-in manifest provides its layout (grouped), base URL, and index URL. Embedded recipes get an in-memory store pre-populated at compile time. Local recipes get a filesystem store. Distributed registries get an HTTP store backed by a disk cache with the GitHub client logic behind it.

The `Loader` stops caring about provider types entirely. No type assertions, no source-specific switch statements.

## Investigation

### Current Provider Implementations

I read all four provider files plus the loader, registry package, and distributed package. Here's what I found:

**RecipeProvider interface** (`internal/recipe/provider.go`): Clean 3-method interface (`Get`, `List`, `Source`) plus two optional interfaces (`SatisfiesProvider`, `RefreshableProvider`). This interface is already the right shape for unification.

**LocalProvider** (`provider_local.go`, 133 lines): Filesystem-backed. Flat layout (`dir/name.toml`). `SatisfiesEntries` parses every TOML file to extract satisfies mappings.

**EmbeddedProvider** (`provider_embedded.go`, 75 lines): Memory-backed via `EmbeddedRegistry` (`embedded.go`, 103 lines). Flat layout (no directory bucketing). `SatisfiesEntries` also parses every TOML file.

**CentralRegistryProvider** (`provider_registry.go`, 134 lines): Wraps `registry.Registry` which handles grouped layout (first-letter bucketing), HTTP fetch, and disk cache with metadata sidecars. `SatisfiesEntries` reads from the cached manifest (doesn't parse individual recipes). Has escape hatch `Registry()` method used by `update-registry` command.

**DistributedProvider** (`internal/distributed/provider.go`, 83 lines): Wraps `GitHubClient` which handles GitHub Contents API listing, raw content downloads, its own cache with different TTL/eviction strategy. No `SatisfiesEntries` implementation.

### Duplication Confirmed

1. **SatisfiesEntries**: Three providers implement it with nearly identical parsing logic (Local and Embedded parse all TOML files; Central reads from manifest). A unified provider could have a single strategy: parse from manifest if available, fall back to parsing all recipes.

2. **firstLetter bucketing**: Duplicated between `registry/cache.go` and `registry/registry.go`. Under unification, layout becomes a config field and bucketing is a single function.

3. **Two cache systems**: `registry.CacheManager` (50MB, LRU, 24h TTL) and `distributed.CacheManager` (20MB, oldest-repo eviction, 1h TTL). Both do the same fundamental thing: store TOML files on disk with metadata sidecars. A unified cache with per-registry configuration would eliminate this.

4. **GetFromSource** (`loader.go:185-248`): 60+ lines of switch logic routing to specific provider types. With uniform providers, this becomes a simple `ProviderBySource(source).Get(name)` call.

5. **Type assertions** (`loader.go`): 4 casts to concrete types -- `*EmbeddedProvider`, `*CentralRegistryProvider` (2x), `*LocalProvider` (2x). These disappear if the provider is uniform.

### How Get/List/Source Work Generically

**Get(ctx, name)**: The unified provider computes the path based on layout (flat: `name.toml`, grouped: `firstLetter(name)/name.toml`), then calls `store.Get(ctx, path)`. The store handles caching, HTTP, or memory lookup.

**List(ctx)**: The store returns all available names. For memory and filesystem stores, this is straightforward. For HTTP stores, this uses the cached source listing (equivalent to current `SourceMeta.Files` or manifest entries).

**Source()**: Returns the configured source tag. No change from current behavior.

**SatisfiesEntries**: Two strategies:
- If the registry has an index/manifest with satisfies data, read from that (fast path, current central behavior).
- Otherwise, parse available recipes (current local/embedded behavior). This can be a method on the store or a fallback in the provider.

### Backing Store Mapping

| Current Type | Store | Layout | Cache | Index |
|---|---|---|---|---|
| EmbeddedProvider | MemoryStore | flat | none | none (parse recipes) |
| LocalProvider | FilesystemStore | flat | none | none (parse recipes) |
| CentralRegistryProvider | HTTPStore + DiskCache | grouped | 24h TTL, LRU, 50MB | recipes.json manifest |
| DistributedProvider | HTTPStore + DiskCache | flat | 1h TTL, oldest-repo, 20MB | GitHub Contents API |

## Strengths

- **Eliminates all type assertions in loader.go**: The 4 casts to `*EmbeddedProvider`, `*CentralRegistryProvider`, and `*LocalProvider` go away. The loader deals only with the interface. This is the single biggest code quality win -- `warnIfShadows` and `RecipesDir`/`SetRecipesDir` become generic operations.

- **GetFromSource collapses to trivial logic**: The 60-line switch with per-source-type branching becomes `provider := l.ProviderBySource(source); return provider.Get(ctx, name)`. Every provider already knows how to fetch from its own source.

- **Single SatisfiesEntries implementation**: The identical parsing loop in Local and Embedded providers merges into one. Central's manifest-based path becomes the fast path for any provider that has an index, with recipe-parsing as the universal fallback.

- **Cache unification is natural**: The backing store abstraction lets us have one `DiskCache` implementation with per-instance configuration (TTL, size limit, eviction strategy). Currently two separate `CacheManager` types with ~400 lines of overlapping code exist.

- **New registry types are free**: Adding a new registry type (e.g., an S3-backed registry, a local HTTP server) means creating a new `BackingStore` implementation. No changes to the loader, no new provider files, no new type assertions.

- **Manifest-driven means self-describing registries**: The registry directory probing logic (`.tsuku-recipes/` then `recipes/`) can be encoded in the manifest. Each registry declares its own layout rather than the CLI hardcoding assumptions.

- **Central registry stops being special**: The `SourceCentral` / `SourceRegistry` / `SourceEmbedded` distinction that requires mapping logic (`SourceCentral = "central"` encompasses both `SourceRegistry` and `SourceEmbedded`) simplifies. Central becomes just another HTTP-backed registry that happens to ship a baked-in fallback.

## Weaknesses

- **EmbeddedProvider's "baked-in" nature is genuinely different**: The embedded registry uses `go:embed` to bake recipes into the binary. It's not configurable at runtime, doesn't have a manifest file, and its "store" is a compile-time artifact. Forcing it into the same config-driven model is awkward -- you'd need a "manifest" that's also compiled in, or special-case the memory store's initialization. The abstraction doesn't perfectly model this.

- **CentralRegistryProvider has an escape hatch (`Registry()`) for good reason**: The `update-registry` command needs `CachedRegistry` internals for refresh stats, stale-if-error, and per-recipe cache management. These are ~380 lines of cache-aware logic in `cached_registry.go` that don't map cleanly to a generic `BackingStore.Get()`. The escape hatch would need to survive in some form, or the store interface would need to grow.

- **Distributed provider's GitHub-specific logic is complex**: The `GitHubClient` handles Contents API listing, branch probing fallback, rate limit handling, conditional requests (ETag/If-Modified-Since), and download URL validation. This is ~400 lines of GitHub-specific code that doesn't become simpler by putting it behind a store interface. It just moves.

- **Migration cost for tests**: 15+ test files reference specific provider constructors. All would need updating. The `source_directed_test.go` and `update_registry_test.go` files are particularly test-heavy.

- **The "two cache" problem might not actually be a problem to solve here**: The central and distributed caches have different eviction strategies (LRU vs. oldest-repo) and different TTLs for good reasons. Unifying the cache type could force a lowest-common-denominator design that loses the per-type optimization.

## Deal-Breaker Risks

- **update-registry command's deep dependency on Registry internals**: `CachedRegistry.RefreshAll()`, `CachedRegistry.Refresh()`, `CacheManager.Info()`, `CacheManager.CleanupWithDetails()` -- these are called directly from the update-registry and cache commands. They depend on the central registry's specific cache structure (metadata sidecars, LRU tracking, stale-if-error). If the unified store interface can't expose these capabilities, either the interface balloons or the escape hatch stays, undermining the unification.

  **Mitigation**: Accept that stores can expose optional interfaces (like the current `RefreshableProvider` and `SatisfiesProvider` pattern). The `update-registry` command would assert on a `CacheIntrospectable` interface rather than casting to `*CentralRegistryProvider`. This is less clean than zero assertions but preserves the pattern the codebase already uses.

  **Assessment**: Not a true deal-breaker. The escape hatch pattern already exists and is documented (`ProviderBySource` comment calls it "the documented escape hatch"). Converting type assertions on concrete types to interface assertions is still a win.

## Implementation Complexity

- **Files to modify**: ~15-20 files
  - `internal/recipe/provider.go` -- expand or keep as-is
  - `internal/recipe/provider_local.go` -- rewrite as `FilesystemStore`
  - `internal/recipe/provider_embedded.go` -- rewrite as `MemoryStore`
  - `internal/recipe/provider_registry.go` -- rewrite as `HTTPStore` wrapper
  - `internal/recipe/loader.go` -- remove type assertions, simplify `GetFromSource`, `warnIfShadows`, `RecipesDir`
  - `internal/recipe/embedded.go` -- keep for `go:embed`, but wire into `MemoryStore`
  - `internal/distributed/provider.go` -- rewrite as `HTTPStore` configuration
  - `internal/registry/registry.go` -- refactor into shared cache/HTTP primitives
  - `internal/registry/cache.go` -- merge with distributed cache into unified cache
  - `internal/registry/cache_manager.go` -- consolidate
  - `cmd/tsuku/main.go` -- update provider construction
  - `cmd/tsuku/install_distributed.go` -- simplify (currently 230 lines, much of it provider management)
  - ~8 test files need updating

- **New infrastructure**: Yes
  - `BackingStore` interface and 3 implementations (~200-300 lines net new, but replaces ~500+ lines of duplicated code)
  - Unified disk cache (~300 lines, replacing ~700 lines across two cache packages)

- **Estimated scope**: Large (2-3 weeks of focused work)
  - Phase 1: Define `BackingStore` interface, implement `MemoryStore` and `FilesystemStore`, port Embedded and Local providers (low risk, ~3 days)
  - Phase 2: Unify cache implementations, implement `HTTPStore`, port Central provider (~5 days)
  - Phase 3: Port Distributed provider, clean up loader type assertions (~3 days)
  - Phase 4: Update tests, integration testing (~3 days)

## Summary

The manifest-driven single provider approach directly addresses the root cause of duplication: four separate implementations of fundamentally the same "get recipe bytes by name" operation. The backing store abstraction cleanly separates "what bytes to fetch" (manifest/layout config) from "how to fetch them" (memory, filesystem, HTTP), and the existing optional-interface pattern (`SatisfiesProvider`, `RefreshableProvider`) already demonstrates the codebase can handle capability differences without type assertions on concrete types. The main cost is the update-registry command's deep coupling to central registry cache internals, but this is manageable through interface assertions rather than concrete type casts. Net effect: ~500 lines of duplicated code eliminated, zero concrete type assertions in the loader, and new registry types become a single-interface implementation.
