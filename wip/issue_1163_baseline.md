# Issue #1163 Baseline

## Test Status

All relevant tests passing:

```
ok  github.com/tsukumogami/tsuku/internal/registry
ok  github.com/tsukumogami/tsuku/cmd/tsuku
```

## Build Status

```
go build ./... - PASS
```

## Branch

`feature/1163-cache-info-enhancement` from `main` at `208485be`

## Dependencies

This issue builds on:
- #1158 (completed): CacheManager with Info() method returning CacheStats
- #1162 (completed): Cache cleanup command (just merged)

## Existing Code Review

### internal/registry/cache_manager.go
Current methods:
- `Info() (*CacheStats, error)` - Returns EntryCount, TotalSize, OldestAccess, NewestAccess
- `SizeLimit() int64` - Returns configured size limit
- `listEntries() ([]cacheEntry, error)` - Private, lists all entries with metadata

### cmd/tsuku/cache.go
- `cacheInfoCmd` - Currently shows Downloads and Versions sections
- `formatBytes(bytes int64) string` - Human-readable byte formatting

## Files to Modify

1. **internal/registry/cache_manager.go** - May need to extend Info() or add helper methods
2. **cmd/tsuku/cache.go** - Add Registry section to cacheInfoCmd output
