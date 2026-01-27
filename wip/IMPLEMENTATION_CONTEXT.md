---
summary:
  constraints:
    - Must deliver CachedRegistry type with GetRecipe(ctx, name) method for downstream #1159 stale fallback
    - Must deliver CachedRegistry that supports adding Refresh() method for #1160
    - TTL bounds: minimum 5m, maximum 7d, default 24h
    - No breaking changes to existing Registry methods
    - Follows existing internal/version/cache.go patterns for consistency
  integration_points:
    - internal/config/config.go - Add EnvRecipeCacheTTL constant and GetRecipeCacheTTL() function
    - internal/registry/cache.go - Uses CacheMetadata, newCacheMetadata, DefaultCacheTTL from #1156
    - internal/registry/cached_registry.go (new) - CachedRegistry wrapper implementation
    - internal/registry/registry.go - Wraps existing Registry.FetchRecipe() and GetCached() methods
  risks:
    - CachedRegistry must be designed to support future stale-if-error extension (#1159)
    - CachedRegistry must support adding Refresh() method without breaking changes (#1160)
    - Don't implement stale fallback yet - that's #1159's scope
    - Don't implement LRU or CacheManager yet - that's #1158's scope
  approach_notes: |
    This is Stage 2 of the design (TTL Expiration). Stage 1 (#1156) is complete.

    Create CachedRegistry wrapper that:
    1. Checks if cache is fresh (within TTL) - return cached
    2. If expired, try FetchRecipe() from network
    3. On success, cache the result and return
    4. On failure, return error (no stale fallback yet)

    The wrapper uses the metadata infrastructure from #1156 to check ExpiresAt.
    TTL is configurable via TSUKU_RECIPE_CACHE_TTL env var.
---

# Implementation Context: Issue #1157

**Source**: docs/designs/DESIGN-registry-cache-policy.md

## Key Design Points

### Stage 2: TTL Expiration

From the design doc:

> **Steps:**
> 1. Add `TSUKU_RECIPE_CACHE_TTL` to `internal/config/config.go`
> 2. Create `internal/registry/cached_registry.go` with `CachedRegistry` wrapper
> 3. Implement `GetRecipe()` with TTL check:
>    - Fresh: return cached
>    - Expired: try refresh, return fresh or error
> 4. Update `recipe.Loader` to use `CachedRegistry`

### CachedRegistry Design

```go
type CachedRegistry struct {
    underlying   *Registry
    ttl          time.Duration
}

func (c *CachedRegistry) GetRecipe(ctx context.Context, name string) ([]byte, error)
```

The wrapper checks metadata's ExpiresAt:
- If `time.Now() < ExpiresAt`: return cached content
- If expired: try network fetch, return fresh or error

### Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `TSUKU_RECIPE_CACHE_TTL` | 24h | Time until cache is considered stale |

Min: 5m, Max: 7d

### What NOT to implement (future issues)

- Stale-if-error fallback (#1159)
- LRU size management (#1158)
- CacheManager (#1158)
- CLI command enhancements (#1160, #1162, #1163)
