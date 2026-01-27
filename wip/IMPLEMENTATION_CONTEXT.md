---
summary:
  constraints:
    - Maximum staleness bound of 7 days (configurable via TSUKU_RECIPE_CACHE_MAX_STALE)
    - Stale fallback enabled by default (TSUKU_RECIPE_CACHE_STALE_FALLBACK=true)
    - Warning messages go to stderr, not stdout
    - CacheInfo struct must include IsStale flag for caller awareness
    - Must work with existing CachedRegistry from #1157 (extend, not replace)
  integration_points:
    - internal/config/config.go - Add GetRecipeCacheMaxStale() and GetRecipeCacheStaleFallback()
    - internal/registry/cached_registry.go - Extend GetRecipe() with stale fallback logic
    - internal/registry/errors.go - Add ErrTypeCacheTooStale and ErrTypeCacheStaleUsed
    - internal/registry/cache.go - Uses existing CacheMetadata.CachedAt for age calculation
  risks:
    - Must handle case where metadata doesn't exist (pre-migration recipes)
    - Warning output must not interfere with stdout (e.g., for piped commands)
    - ErrTypeCacheStaleUsed is a warning context, not an actual error to return
    - Setting TSUKU_RECIPE_CACHE_MAX_STALE=0 should disable stale fallback
  approach_notes: |
    This is Stage 3 of the design (Stale Fallback). Stages 1 (#1156), 2 (#1157), and 4 (#1158) are complete.

    Extend CachedRegistry.GetRecipe() to handle stale fallback:
    1. On network failure, check if cache age < maxStale
    2. If yes, log warning to stderr and return stale content
    3. If no, return ErrTypeCacheTooStale error
    4. Add CacheInfo return value with IsStale flag

    The CacheInfo struct allows callers to know when stale data was used.
    Warning message format from design: "Warning: Using cached recipe '{name}' (last updated {X} hours ago)..."
---

# Implementation Context: Issue #1159

**Source**: docs/designs/DESIGN-registry-cache-policy.md (Stage 3: Stale Fallback)

## Key Design Points

### Stale Fallback Logic (Stage 3)

From the design doc:

> **Bounded stale-if-error (2B)** chosen because:
> - Provides best user experience during transient network issues
> - Maximum staleness bound (7 days) limits security exposure
> - Configurable via environment variables
> - Matches industry patterns (apt, npm)

Decision flow:
```
TTL expired + network fail + age < maxStale → Warning + return stale cache
TTL expired + network fail + age >= maxStale → Error (cache too old)
TTL expired + network ok → Refresh cache, return fresh
```

### Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `TSUKU_RECIPE_CACHE_MAX_STALE` | 7d | Maximum age for stale-if-error fallback |
| `TSUKU_RECIPE_CACHE_STALE_FALLBACK` | true | Enable stale-if-error behavior |

### Error Messages (from design)

| Failure Mode | Message |
|--------------|---------|
| Network timeout, stale used | stderr: "Warning: Using cached recipe '{name}' (last updated {X} hours ago). Run 'tsuku update-registry' to refresh." |
| Network timeout, too stale | "Could not refresh recipe '{name}'. Cache expired {X} days ago (max {Y} days). Check your internet connection." |

### New Error Types

```go
ErrTypeCacheTooStale  ErrorType = 10 // Cache exists but exceeds max staleness
ErrTypeCacheStaleUsed ErrorType = 11 // Stale cache used (warning context)
```

### CacheInfo Struct

The design specifies returning CacheInfo with an IsStale flag:

```go
type CacheInfo struct {
    IsStale     bool
    CachedAt    time.Time
    // ... other fields as needed
}
```

This allows callers to be aware when stale data was returned.
