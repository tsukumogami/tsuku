# Issue 1157 Implementation Plan

## Goal

Add TTL-based expiration to the recipe registry cache via a `CachedRegistry` wrapper that checks cache freshness before returning recipes.

## Files to Modify/Create

1. **internal/config/config.go** - Add `EnvRecipeCacheTTL` and `GetRecipeCacheTTL()`
2. **internal/registry/cached_registry.go** (new) - CachedRegistry wrapper
3. **internal/registry/cached_registry_test.go** (new) - Unit tests

## Implementation Steps

### Step 1: Add TTL configuration to config.go

Add following the existing `GetVersionCacheTTL()` pattern:
- `EnvRecipeCacheTTL = "TSUKU_RECIPE_CACHE_TTL"`
- `GetRecipeCacheTTL()` with:
  - Default: 24h (matches DefaultCacheTTL in cache.go)
  - Minimum: 5m
  - Maximum: 7d
  - Warning on stderr for invalid/out-of-range values

### Step 2: Create CachedRegistry wrapper

```go
type CachedRegistry struct {
    registry *Registry
    ttl      time.Duration
}

func NewCachedRegistry(reg *Registry, ttl time.Duration) *CachedRegistry

func (c *CachedRegistry) GetRecipe(ctx context.Context, name string) ([]byte, error)
```

GetRecipe logic:
1. Check if recipe is cached: `registry.GetCached(name)`
2. Read metadata: `registry.ReadMeta(name)`
3. If metadata exists and `time.Now() < meta.ExpiresAt`: return cached (fresh hit)
4. Try to fetch from network: `registry.FetchRecipe(ctx, name)`
5. On success: cache it via `registry.CacheRecipe(name, content)`, return content
6. On failure: return error (no stale fallback in this issue)

Note: The wrapper recalculates ExpiresAt using the configured TTL, not the DefaultCacheTTL from #1156.

### Step 3: Create comprehensive tests

Test cases:
1. Fresh cache hit (within TTL)
2. Expired cache with successful refresh
3. Expired cache with failed refresh (returns error)
4. Cache miss with successful fetch
5. Cache miss with failed fetch
6. TTL configuration bounds

## Design Decisions

1. **CachedRegistry wraps Registry**: Rather than modifying Registry directly, we create a wrapper. This:
   - Preserves backwards compatibility
   - Allows downstream (#1159) to extend with stale fallback
   - Keeps concerns separated

2. **TTL applied at read time**: We use the configured TTL at read time, not the DefaultCacheTTL at write time. This allows users to change TTL without invalidating cache.

3. **No stale fallback yet**: Per issue scope, expired + network failure = error. #1159 adds stale-if-error.

## Success Criteria

- [ ] `TSUKU_RECIPE_CACHE_TTL` environment variable configures TTL (default 24h, min 5m, max 7d)
- [ ] `CachedRegistry` type wraps `*Registry` and provides `GetRecipe(ctx, name)` method
- [ ] `GetRecipe()` returns cached recipe immediately if within TTL
- [ ] `GetRecipe()` attempts refresh when TTL expired, returns fresh data on success
- [ ] `GetRecipe()` returns error when TTL expired and refresh fails (no stale fallback yet)
- [ ] Existing `Registry` methods remain unchanged (no breaking changes)
- [ ] Unit tests cover: fresh cache hit, expired cache with successful refresh, expired cache with failed refresh
