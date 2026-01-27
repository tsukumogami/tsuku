# Issue #1162 Baseline

## Test Status

All relevant tests passing:

```
ok      github.com/tsukumogami/tsuku/internal/registry  (all cache tests)
```

## Build Status

```
go build ./... - PASS
go vet ./... - PASS
```

## Branch

`feature/1162-cache-cleanup-command` from `main` at `76adec9e`

## Dependencies

This issue builds on:
- #1156 (completed): Cache metadata infrastructure (CacheMetadata with CachedAt, LastAccess)
- #1158 (completed): LRU cache size management (CacheManager with Cleanup, EnforceLimit)
- #1160 (completed): Update-registry enhancement

## Existing Code Review

### internal/registry/cache_manager.go
Current methods:
- `Cleanup(maxAge time.Duration) (int, error)` - Remove entries older than maxAge
- `EnforceLimit() (int, error)` - LRU eviction when above 80%
- `Size() (int64, error)` - Total cache size
- `Info() (*CacheStats, error)` - Cache statistics
- `listEntries() ([]cacheEntry, error)` - List all entries (private)
- `deleteEntry(name string) error` - Delete single entry (private)

### cmd/tsuku/cache.go
- `cacheCmd` parent command
- `cacheClearCmd` for clearing caches
- `cacheInfoCmd` for showing cache info
- `formatBytes(bytes int64) string` helper for human-readable sizes

## Files to Create/Modify

1. **cmd/tsuku/cache_cleanup.go** (create)
   - Add cacheCleanupCmd subcommand
   - Flags: --dry-run, --max-age, --force-limit
   - Use CacheManager for cleanup operations

2. **cmd/tsuku/cache.go** (modify)
   - Register cacheCleanupCmd in init()

3. **internal/registry/cache_manager.go** (modify)
   - Add CleanupWithDetails() for dry-run and detailed output
   - Or add ListStaleEntries() for entry listing

4. **internal/registry/cache_manager_test.go** (modify)
   - Add tests for new methods

5. **docs/designs/DESIGN-registry-cache-policy.md** (modify)
   - Update issue status (strikethrough #1162 when complete)
