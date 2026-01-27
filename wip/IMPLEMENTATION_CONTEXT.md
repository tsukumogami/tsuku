---
summary:
  constraints:
    - Must match existing cache info output format (Downloads, Versions sections)
    - Use CacheManager.Info() which returns CacheStats struct
    - JSON output must include registry section in same structure as human output
    - Handle empty cache gracefully (no errors, show "Entries: 0")
  integration_points:
    - cmd/tsuku/cache.go (cacheInfoCmd) - add registry section to output
    - internal/registry/cache_manager.go - use existing Info() method
    - config.GetRecipeCacheTTL() - for determining stale entries
    - config.GetRecipeCacheSizeLimit() - for limit display
  risks:
    - Need to calculate stale count (TTL expired but still valid)
    - Need to find oldest/newest entry names from CacheStats
    - JSON output format must be backward compatible
  approach_notes: |
    This is a simple enhancement to cacheInfoCmd. CacheManager.Info() already returns
    CacheStats with EntryCount, TotalSize, OldestAccess, and NewestAccess. Need to:
    1. Add stale count calculation (entries with last_access older than TTL)
    2. Find recipe names for oldest/newest entries
    3. Format human-readable output with relative timestamps
    4. Add registry section to JSON output structure
---

# Implementation Context: Issue #1163

**Source**: docs/designs/DESIGN-registry-cache-policy.md

## Key Design Section

Stage 7 from the design doc specifies the target output format:

```
Cache Information:
  Directory: ~/.tsuku

Version Cache:
  Entries: 45
  Size: 128KB

Registry Cache:                    # NEW SECTION
  Entries: 23
  Size: 85KB
  Oldest: fzf (cached 5 days ago)
  Newest: ripgrep (cached 2 hours ago)
  Stale: 3 entries (require refresh)
  Limit: 50MB (0.17% used)
```

## Existing Infrastructure

- `CacheManager.Info() (*CacheStats, error)` returns:
  - EntryCount, TotalSize, OldestAccess, NewestAccess
- `CacheManager.SizeLimit() int64` - returns configured limit
- `config.GetRecipeCacheTTL()` - for TTL to calculate stale entries
- `config.GetRecipeCacheSizeLimit()` - for limit display

## Missing Pieces

1. **Stale count**: Need to iterate entries and count those with lastAccess older than TTL
2. **Oldest/Newest names**: CacheStats has timestamps but not recipe names
3. **formatAgeDuration()**: Human-readable relative time ("5 days ago")
