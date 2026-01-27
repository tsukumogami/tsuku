# Issue #1159 Baseline

## Test Status

All relevant tests passing:

```
ok      github.com/tsukumogami/tsuku/internal/registry  (all cache tests)
ok      github.com/tsukumogami/tsuku/internal/config    (all config tests)
```

## Build Status

```
go build ./... - PASS
go vet ./... - PASS
```

## Branch

`feature/1159-stale-fallback` from `main` at `05413b1c`

## Dependencies

This issue builds on:
- #1156 (completed): Cache metadata infrastructure (CacheMetadata with CachedAt)
- #1157 (completed): TTL-based cache expiration (CachedRegistry)
- #1158 (completed): LRU cache size management (CacheManager)

## Existing Code Review

### internal/registry/cached_registry.go
Current flow in GetRecipe():
1. Check cache - if hit, check freshness via metadata
2. If fresh, return cached content
3. If expired, try network fetch
4. **Current behavior on network failure: return error (no stale fallback)**

This is where stale fallback logic needs to be added.

### internal/registry/errors.go
Has existing error types:
- ErrTypeNotFound
- ErrTypeNetwork
- etc.

Need to add:
- ErrTypeCacheTooStale (for when cache is too old)
- ErrTypeCacheStaleUsed (for warning context)

### internal/config/config.go
Has existing patterns:
- GetRecipeCacheTTL()
- GetRecipeCacheSizeLimit()

Need to add:
- GetRecipeCacheMaxStale() - duration, default 7 days
- GetRecipeCacheStaleFallback() - bool, default true

## Files to Create/Modify

1. **internal/config/config.go** (modify)
   - Add GetRecipeCacheMaxStale() and GetRecipeCacheStaleFallback()

2. **internal/config/config_test.go** (modify)
   - Add tests for new config functions

3. **internal/registry/errors.go** (modify)
   - Add ErrTypeCacheTooStale and ErrTypeCacheStaleUsed

4. **internal/registry/cached_registry.go** (modify)
   - Extend GetRecipe() with stale fallback logic
   - Add CacheInfo struct with IsStale flag
   - Update maxStale and staleFallback fields

5. **internal/registry/cached_registry_test.go** (modify)
   - Add tests for stale fallback scenarios

6. **docs/designs/DESIGN-registry-cache-policy.md** (modify)
   - Update issue status (strikethrough #1159 when complete)
