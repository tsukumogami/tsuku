# Issue #1163 Implementation Plan

## Summary

Extend `tsuku cache info` to display registry cache statistics including entry count, size, oldest/newest entries with names, stale count, and limit utilization.

## Files to Modify

### 1. internal/registry/cache_manager.go

Extend CacheStats struct with new fields:
- OldestName, NewestName (string) - recipe names
- StaleCount (int) - count of TTL-expired entries

Modify Info() method to populate new fields:
- Track names while finding oldest/newest
- Accept TTL parameter for stale calculation OR add separate method

**Approach**: Add InfoWithTTL(ttl time.Duration) method to avoid changing Info() signature.

```go
type CacheStats struct {
    TotalSize    int64
    EntryCount   int
    OldestAccess time.Time
    NewestAccess time.Time
    OldestName   string  // NEW
    NewestName   string  // NEW
    StaleCount   int     // NEW
}

func (m *CacheManager) InfoWithTTL(ttl time.Duration) (*CacheStats, error)
```

### 2. internal/registry/cache_manager_test.go

Add test for InfoWithTTL():
- TestCacheManager_InfoWithTTL_StaleCount

### 3. cmd/tsuku/cache.go

Add registry section to cacheInfoCmd:
1. Import registry package
2. Get CacheManager from config
3. Call InfoWithTTL() with config.GetRecipeCacheTTL()
4. Format human output with new section
5. Add registry to JSON output struct

Helper function:
```go
func formatRelativeTime(t time.Time) string
```

## Implementation Order

1. Extend CacheStats struct with OldestName, NewestName, StaleCount
2. Add InfoWithTTL() method to CacheManager
3. Add test for InfoWithTTL()
4. Add formatRelativeTime() helper to cache.go
5. Add registry section to human output
6. Add registry to JSON output
7. Test with actual registry cache

## Acceptance Criteria Mapping

- [x] Registry section with entry count and size → Step 5
- [x] Oldest and newest with relative timestamps → Steps 1, 4, 5
- [x] Stale entries count → Steps 1, 2, 5
- [x] Size limit and utilization percentage → Step 5
- [x] JSON output includes registry stats → Step 6
- [x] Handle empty cache gracefully → Step 2 (return zeros)
- [x] Unit tests for formatting → Step 3
