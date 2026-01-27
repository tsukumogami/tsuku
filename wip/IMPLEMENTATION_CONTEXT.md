---
summary:
  constraints:
    - Threshold-based eviction (80% high water, 60% low water) for better UX than evict-on-write
    - LRU ordering uses last_access from metadata sidecars (filesystem atime is unreliable)
    - Default 50MB limit (configurable via TSUKU_RECIPE_CACHE_SIZE_LIMIT)
    - Must deliver Cleanup(maxAge) method for downstream #1162 cache cleanup command
    - Must deliver Info() method and CacheStats struct for downstream #1163 cache info
  integration_points:
    - internal/config/config.go - Add GetRecipeCacheSizeLimit() function with human-readable parsing
    - internal/registry/cache_manager.go (new) - CacheManager struct with Size, EnforceLimit, Cleanup, Info
    - internal/registry/cached_registry.go - Call EnforceLimit() after caching recipes
    - internal/registry/cache.go - Uses existing ReadMeta, DeleteMeta, ListCachedWithMeta from #1156
  risks:
    - Must handle cases where metadata doesn't exist (pre-migration recipes)
    - EnforceLimit should be efficient - don't walk filesystem on every cache write unnecessarily
    - Deletion must remove both .toml and .meta.json atomically
    - Warning output should go to stderr, not stdout
  approach_notes: |
    This is Stage 4 of the design (LRU Size Management). Stages 1 (#1156) and 2 (#1157) are complete.

    Create CacheManager with:
    1. Size() - walk cache directory summing file sizes
    2. EnforceLimit() - check 80% threshold, evict LRU until 60%
    3. Cleanup(maxAge) - remove entries not accessed within maxAge
    4. Info() - return CacheStats struct with entry count, total size, etc.

    Integrate with CachedRegistry by calling EnforceLimit() after successful cache writes.
---

# Implementation Context: Issue #1158

**Source**: docs/designs/DESIGN-registry-cache-policy.md

## Key Design Points

### Stage 4: LRU Size Management

From the design doc:

> **Steps:**
> 1. Add `TSUKU_RECIPE_CACHE_SIZE_LIMIT` config (default: 50MB)
> 2. Create `internal/registry/cache_manager.go` with `CacheManager`
> 3. Implement `Size()`, `EnforceLimit()`, and eviction logic
> 4. Call `EnforceLimit()` after `CacheRecipe()` writes
> 5. Add warning at 80% capacity

### CacheManager Design

```go
type CacheManager struct {
    cacheDir   string
    sizeLimit  int64   // Default: 50MB
    highWater  float64 // 0.80 (80%)
    lowWater   float64 // 0.60 (60%)
}

func (m *CacheManager) Size() (int64, error)
func (m *CacheManager) Info() (*CacheStats, error)
func (m *CacheManager) EnforceLimit() (int, error)
func (m *CacheManager) Cleanup(maxAge time.Duration) (int, error)
```

### Eviction Algorithm

1. On cache write, check if size > sizeLimit * highWater (80%)
2. If so, sort entries by last_access ascending (oldest first)
3. Evict until size < sizeLimit * lowWater (60%)

### Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `TSUKU_RECIPE_CACHE_SIZE_LIMIT` | 50MB | LRU eviction threshold |

Parse human-readable sizes: 50MB, 50M, 52428800
