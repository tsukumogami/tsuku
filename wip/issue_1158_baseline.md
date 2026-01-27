# Issue #1158 Baseline

## Test Status

All relevant tests passing:

```
ok      github.com/tsukumogami/tsuku/internal/registry  (10 cache tests + 12 error tests)
ok      github.com/tsukumogami/tsuku/internal/config    (18 tests)
```

## Build Status

```
go build ./... - PASS
go vet ./... - PASS
```

## Branch

`feature/1158-lru-cache-management` from `main` at `985ca60e`

## Dependencies

This issue builds on:
- #1156 (completed): Cache metadata infrastructure (CacheMetadata, WriteMeta, ReadMeta, ListCachedWithMeta)
- #1157 (completed): TTL-based cache expiration (CachedRegistry, GetRecipeCacheTTL)

## Existing Code Review

### internal/registry/cache.go
Contains metadata infrastructure from #1156:
- `CacheMetadata` struct with LastAccess field (needed for LRU)
- `ListCachedWithMeta()` returns all cached recipes with metadata (useful for Size/EnforceLimit)
- `DeleteMeta()` removes metadata sidecar
- `metaPath()` helper for metadata file paths

### internal/config/config.go
Contains configuration pattern to follow:
- `GetRecipeCacheTTL()` - template for new `GetRecipeCacheSizeLimit()`
- Pattern: env var → parse → validate min/max → return default on error

### internal/registry/cached_registry.go
Integration point from #1157:
- `CachedRegistry.cacheAndReturn()` - where to call `EnforceLimit()` after caching

## Files to Create/Modify

1. **internal/registry/cache_manager.go** (new)
   - CacheManager struct
   - Size() method
   - EnforceLimit() method
   - Cleanup(maxAge) method
   - Info() method returning CacheStats

2. **internal/registry/cache_manager_test.go** (new)
   - Tests for all CacheManager methods

3. **internal/config/config.go** (modify)
   - Add GetRecipeCacheSizeLimit() with human-readable parsing

4. **internal/config/config_test.go** (modify)
   - Add tests for GetRecipeCacheSizeLimit()

5. **internal/registry/cached_registry.go** (modify)
   - Integrate CacheManager, call EnforceLimit() after cache writes

6. **docs/designs/DESIGN-registry-cache-policy.md** (modify)
   - Update issue status (strikethrough #1158 when complete)
