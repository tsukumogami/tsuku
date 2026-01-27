# Architecture Review: Registry Recipe Cache Policy Design

## Executive Summary

The proposed architecture is well-designed and follows established patterns in the codebase. The design is implementable with clear component boundaries and reasonable sequencing. However, there are a few areas where simplification or clarification would improve the implementation.

**Overall Assessment:** The architecture is clear enough to implement. The design demonstrates good judgment in its decision-making process and aligns well with existing code patterns.

---

## Detailed Analysis

### 1. Architecture Clarity Assessment

#### Strengths

1. **Pattern Consistency**: The design correctly identifies `internal/version/cache.go` as the pattern to follow. The existing `CachedVersionLister` wrapper provides an excellent template for `CachedRegistry`.

2. **Clear Component Boundaries**: The three-layer architecture (recipe/loader -> CachedRegistry -> Registry) maintains separation of concerns:
   - `CachedRegistry` handles TTL/stale logic
   - `CacheManager` handles LRU/size limits
   - `Registry` remains unchanged (good for backwards compatibility)

3. **Well-Defined Data Flows**: The design document includes explicit flow diagrams for fresh hit, expired refresh, stale fallback, and too-stale error cases.

4. **Structured Error Types**: Adding `ErrTypeCacheTooStale` follows the established pattern in `internal/registry/errors.go` (9 existing error types).

#### Areas Needing Clarification

1. **CachedRegistry vs CacheManager Interaction**: The design shows both components but doesn't clearly specify who calls `EnforceLimit()`. The data flow suggests `CachedRegistry.GetRecipe()` calls it after caching, but this should be explicit.

2. **Metadata Migration Path**: Stage 1 mentions "Migrate existing cached recipes: create metadata on first read" but doesn't specify the migration details. What happens if metadata doesn't exist when reading a cached recipe?
   - **Recommendation**: Specify that missing metadata means "treat as expired, fetch fresh"

3. **CacheInfo Return Value**: The design mentions `CachedRegistry.GetRecipe()` returns `([]byte, *CacheInfo, error)` but the existing `Registry.GetCached()` returns `([]byte, error)`. The Loader integration needs updating.
   - **Recommendation**: Document how `Loader.fetchFromRegistry()` should handle `CacheInfo`

### 2. Missing Components and Interfaces

#### Identified Gaps

1. **No Interface for CachedRegistry**: The design proposes a concrete `CachedRegistry` struct but `Loader` currently takes `*registry.Registry`. For testing and flexibility, consider:
   ```go
   type RecipeFetcher interface {
       GetRecipe(ctx context.Context, name string) ([]byte, error)
       CacheRecipe(name string, data []byte) error
       // ... other required methods
   }
   ```

2. **Missing Refresh Batch Method**: The `update-registry --all` feature requires refreshing all cached recipes, but `CachedRegistry` only has single-recipe methods. Add:
   ```go
   func (c *CachedRegistry) RefreshAll(ctx context.Context) (refreshed, failed int, error)
   ```

3. **No Configuration Loader**: The design mentions 4 environment variables but doesn't show how they're loaded. Extend `internal/config/config.go` or create `internal/registry/config.go`.

4. **ListCached Metadata**: `CacheManager.Info()` needs to enumerate metadata files to show oldest/newest/stale counts, but the design doesn't show how to efficiently scan metadata files.
   - **Recommendation**: Add `CacheManager.ListEntries() ([]CacheEntryInfo, error)` returning per-entry metadata

#### Not Missing (Correctly Excluded)

- Background refresh (explicitly out of scope)
- Binary cache management (different lifecycle)
- Recipe signing (separate future work)

### 3. Implementation Phase Sequencing Analysis

The 7-stage plan is well-sequenced with one exception:

| Stage | Depends On | Assessment |
|-------|------------|------------|
| 1. Metadata Infrastructure | None | Correct starting point |
| 2. TTL Expiration | Stage 1 | Correct, uses metadata |
| 3. Stale Fallback | Stage 2 | Correct, extends TTL logic |
| 4. LRU Size Management | Stage 1 | **Could parallel with Stage 2** |
| 5. Update-Registry Enhancement | Stages 2-3 | Correct, needs TTL context |
| 6. Cache Cleanup Command | Stage 4 | Correct, uses CacheManager |
| 7. Cache Info Enhancement | Stages 4-6 | Correct, final integration |

**Observation**: Stage 4 (LRU) only depends on Stage 1 (metadata with `last_access` and `size` fields). It could run in parallel with Stage 2-3 development by different contributors.

**Potential Issue**: Stage 2 creates `CachedRegistry` but Stage 4 creates `CacheManager`. The design doesn't specify when they get wired together.
- **Recommendation**: Stage 4 should include wiring `CacheManager` into `CachedRegistry`

### 4. Alternative Approaches Considered

#### Simpler Alternative: Extend Existing Registry

Instead of creating `CachedRegistry` wrapper, extend `Registry` directly:

```go
type Registry struct {
    BaseURL    string
    CacheDir   string
    client     *http.Client
    ttl        time.Duration    // NEW
    maxStale   time.Duration    // NEW
    sizeLimit  int64            // NEW
}
```

**Pros**:
- No new types to introduce
- Simpler call chain
- Fewer files to navigate

**Cons**:
- Mixes concerns (network vs cache policy)
- Harder to test cache logic in isolation
- Breaks single responsibility principle

**Verdict**: The design's wrapper approach is superior for maintainability.

#### Simpler Alternative: No CacheManager (Manual Cleanup Only)

Drop automatic LRU eviction, rely entirely on `tsuku cache cleanup`:

**Pros**:
- Simpler implementation
- Predictable behavior
- No surprise evictions

**Cons**:
- Poor UX in constrained environments (containers, small VMs)
- Users must remember to cleanup
- Contrary to "self-contained, no system dependencies" philosophy

**Verdict**: The design correctly rejected this in Option 3C analysis.

#### Simpler Alternative: Single Metadata File

Use one JSON file for all metadata instead of sidecars:

```json
{
  "fzf": {"cached_at": "...", "last_access": "..."},
  "ripgrep": {"cached_at": "...", "last_access": "..."}
}
```

**Pros**:
- Single file read for all metadata
- Simpler directory structure

**Cons**:
- Concurrent write issues (multiple recipe installs)
- Single point of corruption
- Must keep in sync with actual files

**Verdict**: The design correctly chose sidecars (Option 1A) due to atomic update safety.

### 5. Code Integration Points

The implementation will need to modify these existing files:

| File | Changes |
|------|---------|
| `internal/config/config.go` | Add 4 new env var getters |
| `internal/registry/errors.go` | Add `ErrTypeCacheTooStale`, `ErrTypeCacheStaleUsed` |
| `internal/recipe/loader.go` | Update `fetchFromRegistry()` to use `CachedRegistry` |
| `cmd/tsuku/cache.go` | Add registry cache section to `cacheInfoCmd`, add `cacheCleanupCmd` |
| `cmd/tsuku/update_registry.go` | Add `--dry-run`, `--recipe`, `--all` flags |

New files to create:

| File | Purpose |
|------|---------|
| `internal/registry/cache.go` | `CacheMetadata` struct |
| `internal/registry/cached_registry.go` | `CachedRegistry` wrapper |
| `internal/registry/cache_manager.go` | `CacheManager` for LRU |
| `cmd/tsuku/cache_cleanup.go` | Cleanup subcommand |

### 6. Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Metadata file corruption | Low | Medium | ContentHash verification, re-fetch on mismatch |
| LRU evicts actively used recipe | Low | Low | Recipe re-fetched transparently |
| Stale cache masks critical bug fix | Medium | Medium | 7-day max staleness bound, warning on use |
| Configuration complexity for users | Low | Low | Good defaults, power-user escape hatches |

### 7. Testing Strategy Recommendations

1. **Unit Tests for CachedRegistry**:
   - Fresh cache hit (returns cached)
   - Expired cache with network success (refreshes)
   - Expired cache with network failure (stale fallback)
   - Too-stale cache (returns error)
   - Mock time for TTL testing

2. **Unit Tests for CacheManager**:
   - Size calculation accuracy
   - LRU eviction order
   - Threshold-based triggering (80%/60%)

3. **Integration Tests**:
   - End-to-end install with cache
   - `update-registry` command output
   - `cache cleanup` removes old entries
   - `cache info` shows correct statistics

4. **Migration Tests**:
   - Recipe without metadata gets metadata on access
   - Mixed old/new cache directory works correctly

---

## Summary

### Key Findings

1. **Architecture is sound**: The wrapper pattern matches existing code, component boundaries are clear, and the sequencing is mostly correct.

2. **Minor gaps exist**: The CachedRegistry/CacheManager wiring point isn't explicit, batch refresh method is missing, and metadata migration details need specification.

3. **No simpler alternatives were overlooked**: The design document thoroughly evaluated and correctly rejected simpler approaches.

4. **Implementation is feasible**: The 7-stage plan provides clear deliverables, though Stage 4 could parallel with Stage 2-3.

### Top 3 Recommendations

1. **Add explicit wiring specification**: Document in Stage 4 exactly how `CacheManager` gets injected into `CachedRegistry`. Consider dependency injection via constructor: `NewCachedRegistry(reg *Registry, mgr *CacheManager, opts CacheOptions)`.

2. **Add RefreshAll method**: Stage 5's `--all` flag requires batch refresh capability. Add `RefreshAll(ctx context.Context) (RefreshStats, error)` to `CachedRegistry` where `RefreshStats` contains counts of refreshed/skipped/failed recipes.

3. **Specify metadata migration behavior**: In Stage 1 validation criteria, explicitly state: "Cached recipes without metadata are treated as expired (TTL exceeded) but valid for stale fallback. First read creates metadata with `cached_at = file modification time`."

### Overall Verdict

The architecture is ready for implementation. The identified gaps are minor clarifications rather than fundamental issues. The design demonstrates careful consideration of trade-offs and follows established codebase patterns effectively.
