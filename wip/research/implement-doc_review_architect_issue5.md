# Architect Review: Issue 5 -- Distributed Package HTTP Fetching and Cache

**Files reviewed:**
- `internal/distributed/client.go`
- `internal/distributed/cache.go`
- `internal/distributed/errors.go`
- `internal/distributed/client_test.go`
- `internal/distributed/cache_test.go`

**Context files:**
- `internal/recipe/provider.go` (RecipeProvider interface)
- `internal/recipe/provider_registry.go` (CentralRegistryProvider pattern)
- `internal/httputil/client.go` (HTTP client factory)
- `internal/config/config.go` (config and cache dir patterns)

---

## Structural Fit

The `internal/distributed` package sits at the right level: it depends downward on `internal/httputil`, `internal/discover`, and `internal/secrets`. No circular dependencies. It does not import `internal/recipe`, `internal/config`, or `cmd/` -- dependency direction is correct.

The package is designed as a building block that Issue 6 will wrap with a `RecipeProvider` implementation. This is the right layering: `distributed` handles GitHub HTTP and disk cache, while a future provider adapter will map it to the `RecipeProvider` interface. The existing `CentralRegistryProvider` wraps `*registry.Registry` the same way.

HTTP client construction reuses `httputil.NewSecureClient(httputil.DefaultOptions())`, consistent with how the rest of the codebase creates HTTP clients. The `authTransport` wrapping pattern is clean.

Error types are package-scoped, typed, and follow the existing error patterns (struct types with `Error() string`). `ErrNetwork` implements `Unwrap()`.

---

## Findings

### 1. Cache TTL and size limit are hardcoded -- parallel config pattern (Advisory)

`cache.go:15-19` defines `DefaultCacheTTL = 1 * time.Hour` and `DefaultMaxCacheSize = 20MB` as package-level constants. The existing config pattern in `internal/config/config.go` centralizes all cache defaults (TTL, size limits) with env-var overrides (`TSUKU_RECIPE_CACHE_TTL`, `TSUKU_RECIPE_CACHE_SIZE_LIMIT`, etc.).

The distributed cache creates a second source of cache configuration that lives in a different package with no env-var override path. When Issue 6 wires this up, the caller will need to bridge between `config.Get*()` functions and `NewCacheManager()` parameters. That's fine as-is -- the `CacheManager` accepts TTL and size as constructor args, so the wiring code can pass config values through.

This is advisory, not blocking, because:
- The defaults are reasonable and documented.
- The constructor accepts overrides, so Issue 6 can wire in config values.
- No env-var constants are duplicated -- they simply don't exist yet for distributed cache.

**Recommendation:** When Issue 6 wires this up, add `TSUKU_DISTRIBUTED_CACHE_TTL` and `TSUKU_DISTRIBUTED_CACHE_SIZE_LIMIT` to `internal/config/config.go` rather than adding them in `internal/distributed/`.

### 2. No `DistributedCacheDir` in Config struct (Advisory)

`internal/config/config.go` defines all cache directory paths (`VersionCacheDir`, `DownloadCacheDir`, `TapCacheDir`, etc.) on the `Config` struct, and `EnsureDirectories()` creates them. The `CacheManager` takes `baseDir` as a string arg but there's no corresponding field in `Config`.

This is a gap that Issue 6 will need to fill. Not blocking because the `CacheManager` constructor is flexible -- it accepts any path. But it means Issue 6 must add `DistributedCacheDir` to `Config` and `EnsureDirectories()` for consistency, rather than constructing the path ad-hoc.

**Recommendation:** Issue 6 should add `DistributedCacheDir string` to `Config` (path: `$TSUKU_HOME/cache/distributed`) and include it in `EnsureDirectories()`.

### 3. `GitHubClient` is a concrete type, not an interface (Advisory)

The `RecipeProvider` contract in `internal/recipe/provider.go` is interface-based. The `GitHubClient` in `internal/distributed/client.go` is a concrete struct. Issue 6 will need to either:
(a) wrap `*GitHubClient` inside a provider struct (like `CentralRegistryProvider` wraps `*registry.Registry`), or
(b) make `GitHubClient` implement `RecipeProvider` directly.

Option (a) matches the existing pattern. The current design supports this because `GitHubClient` methods (`ListRecipes`, `FetchRecipe`) have different signatures than `RecipeProvider` (`Get`, `List`, `Source`), so a wrapper adapter is the natural fit.

No action needed now. This is noted to confirm the design leaves room for correct integration.

### 4. `ListRecipes` validates via `discover.ValidateGitHubURL(owner + "/" + repo)` -- input coupling (Advisory)

`client.go:108` concatenates `owner + "/" + repo` to pass to `ValidateGitHubURL`, which expects a URL or `owner/repo` format. This works but creates a subtle coupling: if `ValidateGitHubURL`'s parsing changes, this concatenation might break. The `FetchRecipe` method doesn't call this validation at all -- it validates only the download URL.

Not blocking because the function is well-tested and the concatenation matches the documented format. But the asymmetry between `ListRecipes` (validates owner/repo) and `FetchRecipe` (validates only download URL) means input validation is split across two layers. Issue 6's provider wrapper should be the single validation point.

### 5. `evictOldest` walks the full directory tree on every write exceeding `maxBytes` (Advisory)

`cache.go:191-193` calls `cm.Size()` (which walks the entire cache directory tree) after every `PutRecipe`, then `evictOldest()` walks it again if over limit. For a 20MB cache this is negligible, but the pattern doesn't match how the central registry handles cache management (which doesn't have size-based eviction at all -- it uses TTL-only).

This is a new pattern rather than a parallel one. It's contained within `CacheManager` with no external callers, so it won't diverge. Advisory only.

---

## Summary

No blocking findings. The package layers correctly, reuses existing infrastructure (`httputil`, `discover`, `secrets`), and leaves clean seams for Issue 6 integration. The main architectural concern is ensuring Issue 6 wires cache configuration through `internal/config` rather than adding distributed-specific env vars in the `distributed` package itself.

| Level | Count |
|-------|-------|
| Blocking | 0 |
| Advisory | 5 |
