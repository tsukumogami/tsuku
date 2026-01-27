# Issue #1160 Baseline

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

`feature/1160-update-registry-enhancement` from `main` at `1b0fdfab`

## Dependencies

This issue builds on:
- #1156 (completed): Cache metadata infrastructure (CacheMetadata with CachedAt)
- #1157 (completed): TTL-based cache expiration (CachedRegistry)
- #1158 (completed): LRU cache size management (CacheManager)
- #1159 (completed): Stale-if-error fallback (CacheInfo with IsStale)

## Existing Code Review

### internal/registry/cached_registry.go
Current methods:
- `GetRecipe(ctx, name) ([]byte, *CacheInfo, error)` - Get with TTL check and stale fallback
- `SetCacheManager(cm)` / `CacheManager()` - Cache manager configuration
- `SetMaxStale(d)` / `SetStaleFallback(enabled)` - Stale fallback config
- `Registry()` - Access underlying registry

Need to add:
- `Refresh(ctx, name) ([]byte, error)` - Force refresh single recipe
- `RefreshAll(ctx) (*RefreshStats, error)` - Refresh all cached recipes
- `RefreshStats` and `RefreshDetail` structs

### internal/registry/registry.go
Has `ListCached()` method that returns list of cached recipe names.
This will be used by RefreshAll() to iterate all cached recipes.

### cmd/tsuku/update_registry.go
Need to examine current implementation and add flags:
- --dry-run
- --recipe <name>
- --all (default)

## Files to Create/Modify

1. **internal/registry/cached_registry.go** (modify)
   - Add RefreshStats and RefreshDetail structs
   - Add Refresh() method
   - Add RefreshAll() method

2. **internal/registry/cached_registry_test.go** (modify)
   - Add tests for Refresh() and RefreshAll()

3. **cmd/tsuku/update_registry.go** (modify)
   - Add --dry-run, --recipe, --all flags
   - Implement dry-run logic
   - Implement selective/all refresh logic

4. **docs/designs/DESIGN-registry-cache-policy.md** (modify)
   - Update issue status (strikethrough #1160 when complete)
