# Issue 1157 Summary

## What Was Done

Added TTL-based expiration to the recipe registry cache via a `CachedRegistry` wrapper that checks cache freshness before returning recipes.

## Files Changed

- `internal/config/config.go`: Added `EnvRecipeCacheTTL` constant and `GetRecipeCacheTTL()` function
  - Default: 24h, Min: 5m, Max: 7d
  - Validates bounds and warns on invalid/out-of-range values

- `internal/config/config_test.go`: Added 5 new tests for `GetRecipeCacheTTL()`

- `internal/registry/cached_registry.go` (new): CachedRegistry wrapper implementation
  - `NewCachedRegistry(reg, ttl)` constructor
  - `GetRecipe(ctx, name)` with TTL-based freshness check
  - `Registry()` for underlying registry access

- `internal/registry/cached_registry_test.go` (new): Comprehensive tests
  - TestCachedRegistry_FreshCacheHit
  - TestCachedRegistry_ExpiredCacheRefresh
  - TestCachedRegistry_ExpiredCacheNetworkFailure
  - TestCachedRegistry_CacheMissNetworkSuccess
  - TestCachedRegistry_CacheMissNetworkFailure
  - TestCachedRegistry_TTLRespected
  - TestCachedRegistry_Registry

## Test Results

- All registry tests passing (17 existing + 7 new)
- All config tests passing (existing + 5 new)
- Linter passes

## Design Decisions Made

1. **Wrapper pattern**: CachedRegistry wraps Registry rather than modifying it, preserving backwards compatibility and enabling downstream extensions.

2. **TTL at read time**: The configured TTL is applied at read time using `CachedAt + ttl`, not the stored `ExpiresAt`. This allows TTL changes to take effect without invalidating the cache.

3. **No stale fallback**: Per issue scope, expired cache + network failure returns error. Stale-if-error fallback is #1159's scope.

4. **Double-write for TTL**: After `CacheRecipe()` (which uses DefaultCacheTTL), we update metadata with the configured TTL to ensure correct expiration.

## Notes for Reviewers

- The `CachedRegistry` is designed to be extended in #1159 with stale-if-error fallback
- The `Registry()` method provides access to underlying registry for #1160's CLI integration
- Test uses short TTL (100ms) to verify expiration behavior without slow tests
