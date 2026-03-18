# Advocate: Layered Storage Abstraction

## Approach Description

Separate the current provider system into two distinct layers:

1. **Storage layer** -- a `Storage` interface responsible only for reading bytes from a backing store. Implementations would include:
   - `FSStorage` -- reads from a local filesystem directory (covers local recipes and the central registry cache)
   - `EmbeddedStorage` -- wraps an `embed.FS` or `map[string][]byte` (covers embedded system library recipes)
   - `HTTPStorage` -- fetches bytes via HTTP from raw GitHub URLs or any HTTPS endpoint (covers central registry remote and distributed repos)
   - `GitHubContentsStorage` -- wraps the Contents API for directory listings, delegates file reads to `HTTPStorage`

2. **Cache middleware** -- a `CachingStorage` that wraps any `Storage` backend. It intercepts `Read` and `List` calls, checks a local cache directory with TTL-based freshness, and falls back to the wrapped backend on miss/expiry. A single implementation replaces both `internal/registry/cache.go` + `cached_registry.go` and `internal/distributed/cache.go`.

3. **Registry layer** -- a single `Registry` struct that combines a `Storage` backend with a `Manifest`. The manifest declares the layout (flat vs. grouped) and an optional index URL. The `Registry` handles recipe name-to-path resolution using the manifest's layout, satisfies index construction (once, generically), and refresh logic. This is the only type that implements `RecipeProvider`.

The `Loader` continues to hold a priority-ordered list of `RecipeProvider`s, but every provider is now the same type (`Registry`) configured with different storage backends and manifests.

### Concrete Storage Interface

```go
type Storage interface {
    // Read returns the raw bytes at the given path, or an error.
    Read(ctx context.Context, path string) ([]byte, error)

    // List returns all recipe names available (without extension).
    List(ctx context.Context) ([]string, error)

    // Exists returns true if the given path exists.
    Exists(ctx context.Context, path string) (bool, error)
}
```

### Cache Middleware

```go
type CachingStorage struct {
    backend  Storage
    cacheDir string
    ttl      time.Duration
    maxSize  int64
}
```

`CachingStorage` implements `Storage`. On `Read`, it checks `cacheDir` for a fresh copy (using sidecar `.meta.json` files with TTL, ETag, content hash -- the same metadata schema already used in both existing cache implementations). On miss or expiry, it delegates to `backend`, writes the result to disk, and returns. Eviction uses the same LRU high-water/low-water strategy already in `registry/cache_manager.go`.

### Registry as Provider

```go
type Registry struct {
    source   recipe.RecipeSource
    storage  Storage           // possibly CachingStorage wrapping HTTPStorage
    manifest *Manifest         // declares layout, index_url, satisfies
}
```

`Registry` implements `RecipeProvider`, `SatisfiesProvider`, and `RefreshableProvider`. When the manifest declares a `grouped` layout, `Get("fzf")` translates to `storage.Read("f/fzf.toml")`. When `flat`, it's `storage.Read("fzf.toml")`. Satisfies entries come from the manifest, not by parsing every recipe file.

## Investigation

### What I Read

- `internal/recipe/provider.go` -- `RecipeProvider`, `SatisfiesProvider`, `RefreshableProvider` interfaces
- `internal/recipe/provider_embedded.go` -- `EmbeddedProvider` wrapping `EmbeddedRegistry`
- `internal/recipe/provider_local.go` -- `LocalProvider` reading from filesystem
- `internal/recipe/provider_registry.go` -- `CentralRegistryProvider` wrapping `registry.Registry`
- `internal/recipe/loader.go` -- `Loader` with provider chain, satisfies index, `GetFromSource()` switch
- `internal/distributed/provider.go` -- `DistributedProvider` wrapping `GitHubClient`
- `internal/distributed/cache.go` -- `CacheManager` with TTL, eviction, per-repo directories
- `internal/distributed/client.go` -- `GitHubClient` with Contents API + raw URL fetching
- `internal/registry/registry.go` -- `Registry` with HTTP fetch, local path support, cache read/write
- `internal/registry/cache.go` -- `CacheMetadata` with TTL, content hash, LRU timestamps
- `internal/registry/cached_registry.go` -- `CachedRegistry` with stale-if-error fallback
- `internal/registry/cache_manager.go` -- `CacheManager` with high-water/low-water eviction
- `internal/registry/manifest.go` -- `Manifest` with schema versioning, recipe entries
- `docs/designs/DESIGN-registry-unification.md` -- problem statement and decision drivers

### Key Findings

**Duplicated cache logic.** The central registry (`internal/registry/`) and distributed system (`internal/distributed/cache.go`) implement nearly identical caching patterns: sidecar metadata files, TTL-based freshness checks, size-limited eviction. The main differences are:
- Central: first-letter bucketed directory structure, SHA256 content hashes, high-water/low-water eviction
- Distributed: owner/repo directory structure, ETag/Last-Modified conditional headers, oldest-repo eviction

A `CachingStorage` middleware can absorb both by parameterizing the cache directory layout (delegated to the `Registry` layer's path resolution) and supporting both content-hash and ETag freshness strategies.

**SatisfiesEntries duplication.** Three providers (`EmbeddedProvider`, `LocalProvider`, `CentralRegistryProvider`) implement `SatisfiesEntries`. The embedded and local versions parse every TOML file to extract satisfies mappings -- identical logic. The central registry version reads from the manifest instead. Under this approach, all registries would have manifests (even embedded ones, generated at build time), so satisfies resolution becomes a single manifest-reading implementation.

**GetFromSource is a hardcoded switch.** `loader.go` lines 185-248 contain a `switch` on source strings with per-type behavior (central tries registry then embedded; local searches local providers; distributed matches owner/repo pattern). With a unified `Registry` type, `GetFromSource` becomes: find the provider whose `Source()` matches, call `Get()`. The 50+ lines collapse to ~5.

**Type assertions in warnIfShadows.** The loader casts providers to `*EmbeddedProvider` and `*CentralRegistryProvider` to check for shadowing (lines 359-374). With a uniform provider type, shadowing detection uses the `Storage.Exists()` method without knowing the concrete type.

**GitHubClient complexity.** The distributed `GitHubClient` handles two concerns: directory listing (Contents API) and file fetching (raw URLs). These map cleanly to `List()` and `Read()` on a `Storage` interface. The Contents API with rate-limit fallback stays inside a `GitHubContentsStorage` implementation. The central registry's HTTP fetch (`registry.fetchRemoteRecipe`) is a simpler version of the same pattern -- both collapse into an `HTTPStorage` that knows how to fetch a URL.

**EmbeddedRegistry is already a map.** `internal/recipe/embedded.go` holds `map[string][]byte`. This is trivially wrapped as an `EmbeddedStorage` implementing `Storage`. The `fs.WalkDir` loading stays in the constructor.

### How the Pieces Compose

Central registry (remote):
```
Registry(source="central", manifest=baked-in)
  -> CachingStorage(cacheDir="$TSUKU_HOME/registry", ttl=24h, maxSize=50MB)
    -> HTTPStorage(baseURL="https://raw.githubusercontent.com/tsukumogami/tsuku/main")
```

Central registry (embedded fallback):
```
Registry(source="embedded", manifest=baked-in-subset)
  -> EmbeddedStorage(map[string][]byte from embed.FS)
```

Local recipes:
```
Registry(source="local", manifest=auto-detected)
  -> FSStorage(dir="$TSUKU_HOME/recipes")
```

Distributed (owner/repo):
```
Registry(source="owner/repo", manifest=fetched-from-repo)
  -> CachingStorage(cacheDir="$TSUKU_HOME/cache/distributed/owner/repo", ttl=1h, maxSize=20MB)
    -> GitHubContentsStorage(owner, repo, apiClient, rawClient)
```

## Strengths

- **Eliminates both cache implementations.** The central `registry/cache.go` + `cache_manager.go` (~360 lines) and distributed `distributed/cache.go` (~290 lines) collapse into a single `CachingStorage` (~200 lines). TTL, eviction strategy, and stale-if-error are configuration parameters, not separate implementations. This is the single largest duplication win.

- **GetFromSource becomes trivial.** The current 60-line switch with per-source-type behavior reduces to a loop that finds the matching provider and calls `Get()`. No type assertions, no special cases for "central" mapping to both registry and embedded.

- **SatisfiesEntries becomes a single implementation.** All registries have manifests, so the three separate implementations (two parsing TOML, one reading manifest JSON) collapse to one manifest-reading path. Embedded and local registries generate their manifests at load time.

- **New registry types require zero new code.** Adding support for GitLab, S3, or local `.tsuku-recipes/` directories means writing a new `Storage` implementation (one struct, three methods) and configuring it with the existing `Registry` + `CachingStorage`. No loader changes, no new provider types, no new cache implementations.

- **Cache is composable and testable.** `CachingStorage` wraps any `Storage`, so tests can verify caching behavior independently of the backend. The current `CachedRegistry` is tightly coupled to `Registry` and can only cache HTTP-fetched recipes. The middleware pattern makes it easy to add caching to local filesystem registries in the future (e.g., for network-mounted directories).

- **Type assertions disappear from the loader.** The five type assertions in `loader.go` (`*EmbeddedProvider`, `*CentralRegistryProvider`, `*LocalProvider`) are replaced by uniform interface calls. `warnIfShadows` uses `Storage.Exists()`, `RecipesDir()` uses a `Dir()` method on `FSStorage` (or a type switch on a much smaller surface), and `ProviderBySource` returns the same concrete type for all providers.

- **Aligns with the design doc's stated goals.** The DESIGN-registry-unification.md explicitly calls for "single code path", "no special cases for the central registry", and "adding a new registry type should mean configuring an existing mechanism". This approach directly delivers all three.

## Weaknesses

- **Manifest generation for embedded and local registries.** The central and distributed registries already have manifests. Embedded and local registries don't. Embedded recipes would need a build-time manifest generator (scanning the recipes directory and outputting JSON). Local recipes would need runtime manifest generation on first access. This is new infrastructure that doesn't exist today.

- **Two-phase initialization for distributed registries.** The current `GitHubClient.ListRecipes()` fetches the directory listing (equivalent to a manifest) before individual recipes can be read. Under this approach, the `GitHubContentsStorage.List()` call must complete before `Read()` works because the download URLs come from the listing. This couples `List` and `Read` in a way that pure `Storage` semantics don't naturally express -- the implementation needs internal state (cached download URLs) that bleeds through the interface.

- **Conditional request headers (ETag/If-Modified-Since) don't fit a generic cache.** The distributed cache uses ETag and Last-Modified headers for conditional HTTP requests, which are transport-specific. A generic `CachingStorage` operates on opaque bytes and TTL. To support conditional requests, either the `HTTPStorage` needs to handle its own caching (defeating the middleware purpose), or `CachingStorage` needs transport-aware hooks. This complicates the "transparent middleware" claim.

- **Stale-if-error fallback is cache-policy logic, not storage logic.** The central registry's `CachedRegistry` has a sophisticated stale-if-error policy with configurable `maxStale`. This is registry-level policy (should a user see stale data?) rather than storage-level (where are the bytes?). Putting it in `CachingStorage` contaminates the storage layer with policy decisions. Keeping it in the `Registry` layer means `CachingStorage` can't fully replace `CachedRegistry`.

- **Migration complexity.** The refactor touches core data paths. Every recipe resolution, every cache read/write, every registry refresh flows through the new types. While the interfaces are clean, the migration risk is real -- silent behavior changes in TTL handling, cache path layout, or error propagation could cause regressions in recipe resolution.

- **The `Registry` layer still needs layout awareness.** Path translation (name to `f/fzf.toml` vs. `fzf.toml`) lives in the `Registry`, not in `Storage`. This means `Storage` works with raw paths but the caller (Registry) must know the layout. If a `Storage` backend internally uses a different path scheme (like the distributed cache's `owner/repo/name.toml`), the abstraction leaks.

## Deal-Breaker Risks

- **Conditional HTTP requests may force a hybrid cache.** If the cache middleware can't handle ETag/If-Modified-Since without becoming transport-aware, you end up with two cache patterns: the generic `CachingStorage` for simple TTL cases, and a transport-aware variant for HTTP backends. This partially recreates the current duplication. However, this is mitigatable: the `HTTPStorage.Read()` method can internally handle conditional requests and return fresh bytes or cached bytes, making the caching transparent to `CachingStorage`. The cost is that `HTTPStorage` needs a local cache path for conditional request handling, which overlaps with `CachingStorage`. This is a design tension but not a deal-breaker because you can choose to put conditional request logic in `HTTPStorage` and TTL/eviction logic in `CachingStorage`, splitting responsibilities cleanly.

- **No identified deal-breakers that would make this approach fail entirely.** The weaknesses are engineering costs and design tensions, not fundamental incompatibilities. The layered model fits the existing codebase's shape -- every current provider already does "read bytes from somewhere" + "apply caching" + "resolve recipe names", just without formal separation.

## Implementation Complexity

- **Files to modify**: ~15-20 files
  - New: `internal/storage/storage.go` (interface), `internal/storage/fs.go`, `internal/storage/embedded.go`, `internal/storage/http.go`, `internal/storage/github.go`, `internal/storage/caching.go` (~6 new files, ~600-800 lines total)
  - Modify: `internal/recipe/provider.go`, `internal/recipe/loader.go`, `internal/recipe/provider_registry.go` (rewrite to unified Registry)
  - Delete: `internal/recipe/provider_embedded.go`, `internal/recipe/provider_local.go`, `internal/distributed/cache.go` (absorbed into caching storage), most of `internal/registry/cached_registry.go`
  - Modify: `internal/registry/registry.go` (slim down to manifest + URL config), `internal/registry/cache.go` (metadata types reused, management logic moves to caching storage)
  - Modify: CLI commands that type-assert providers (`cmd/tsuku/` -- update-registry, install commands)
  - Modify: `internal/distributed/client.go` (refactor into GitHubContentsStorage)

- **New infrastructure**: Yes
  - `internal/storage/` package with Storage interface and 5 implementations
  - Build-time manifest generator for embedded recipes (small script or Go generate)
  - Runtime manifest builder for local recipe directories

- **Estimated scope**: Large (3-4 weeks for a careful implementation with incremental PRs)
  - Phase 1: Storage interface + FSStorage + EmbeddedStorage (no behavior change, wraps existing code)
  - Phase 2: CachingStorage replacing both cache implementations
  - Phase 3: Unified Registry type replacing all four providers
  - Phase 4: Loader simplification (GetFromSource, type assertions, satisfies index)

## Summary

The layered storage abstraction directly targets the root cause of the duplication: every registry type reimplements the same three concerns (byte fetching, caching, recipe resolution) in a tightly coupled way. By separating storage from registry logic and making caching a composable middleware, you eliminate the two parallel cache implementations, the SatisfiesEntries duplication, and the GetFromSource switch -- roughly 500+ lines of redundant code. The main engineering cost is handling HTTP-specific caching (conditional requests) within the generic middleware pattern, which requires careful interface design but has workable solutions. The approach is a natural fit for the codebase's existing shape and delivers on all three goals stated in the design doc.
