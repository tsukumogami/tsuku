---
status: Current
problem: The registry recipe cache stores recipes indefinitely with no TTL, no size limits, and no metadata tracking, causing network failures to result in hard failures instead of graceful degradation.
decision: Implement TTL-based caching with JSON metadata sidecars, stale-if-error fallback, and LRU eviction.
rationale: Follows existing version cache patterns for consistency, provides best user experience during network issues, and enables configurable cache management.
---

# Registry Recipe Cache Policy

## Status

**Current**

## Implementation Issues

### Milestone: [Registry Cache Policy](https://github.com/tsukumogami/tsuku/milestone/48)

| Issue | Title | Dependencies | Tier |
|-------|-------|--------------|------|
| ~~[#1156](https://github.com/tsukumogami/tsuku/issues/1156)~~ | Add registry cache metadata infrastructure | None | testable |
| ~~[#1157](https://github.com/tsukumogami/tsuku/issues/1157)~~ | Implement TTL-based cache expiration | ~~[#1156](https://github.com/tsukumogami/tsuku/issues/1156)~~ | testable |
| ~~[#1159](https://github.com/tsukumogami/tsuku/issues/1159)~~ | Add stale-if-error fallback | ~~[#1157](https://github.com/tsukumogami/tsuku/issues/1157)~~ | testable |
| ~~[#1158](https://github.com/tsukumogami/tsuku/issues/1158)~~ | Implement LRU cache size management | ~~[#1156](https://github.com/tsukumogami/tsuku/issues/1156)~~ | testable |
| ~~[#1160](https://github.com/tsukumogami/tsuku/issues/1160)~~ | Enhance update-registry with status and selective refresh | ~~[#1157](https://github.com/tsukumogami/tsuku/issues/1157)~~ | testable |
| ~~[#1162](https://github.com/tsukumogami/tsuku/issues/1162)~~ | Add cache cleanup command | ~~[#1158](https://github.com/tsukumogami/tsuku/issues/1158)~~ | testable |
| ~~[#1163](https://github.com/tsukumogami/tsuku/issues/1163)~~ | Enhance cache info with registry statistics | ~~[#1158](https://github.com/tsukumogami/tsuku/issues/1158)~~ | simple |

## Upstream Design Reference

This design implements Stage 5 from [DESIGN-recipe-registry-separation.md](DESIGN-recipe-registry-separation.md).

**Relevant sections:**
- Stage 5: Cache Policy Implementation
- Gap 5 (Review Feedback): Cache size unbounded
- Gap 6 (Review Feedback): Error messages undefined

## Context and Problem Statement

The registry recipe cache (`$TSUKU_HOME/registry/`) stores fetched recipes to disk for subsequent installations. Currently, this cache operates with minimal policy:

**Current behavior:**
- Recipes are cached indefinitely once fetched (no TTL)
- No metadata tracking (no timestamps, no access times)
- Network failures result in hard errors even when cached data exists
- No size limits (cache grows unbounded)
- `tsuku update-registry` only clears cache, doesn't refresh

**Problems this creates:**

1. **Network failures cause unnecessary failures**: If GitHub is temporarily unreachable and cache is "expired" (conceptually), users get hard errors even though cached recipes would work fine for their use case.

2. **No cache freshness visibility**: Users can't tell if they're using stale recipes or when the cache was last updated. Recipe bug fixes might not propagate if users never clear cache.

3. **Unbounded disk growth**: In environments with many recipes or constrained disk space, the cache grows without limit. A user installing hundreds of recipes accumulates GB of cached data.

4. **Poor error messages**: Network failures produce technical errors without guidance on cache state or remediation steps.

### Why Now

The recipe registry separation (issue #1033, now completed) moves most recipes from embedded to registry-fetched. This amplifies existing cache issues:
- More recipes will be cached (150+ vs previous ~0)
- Network dependency increases
- Cache management becomes a real user concern

### Scope

**In scope:**
- TTL-based cache expiration with configurable duration
- Stale cache fallback on network failure with warnings
- LRU-based cache size limits with configurable maximum
- Enhanced `update-registry` command with status and selective refresh
- New `cache cleanup` subcommand for manual cache management
- Cache statistics in `tsuku cache info`
- Defined error message templates for all failure modes

**Out of scope:**
- Version cache policy changes (already has TTL via `internal/version/cache.go`)
- Downloaded binary cache management (different lifecycle)
- Recipe content validation/signing (separate future work)
- Background refresh or async cache updates

## Decision Drivers

- **User experience during network issues**: Users should get working installations, not cryptic errors, when cached data suffices
- **Disk space management**: Cache should be bounded for constrained environments (containers, small VMs)
- **Operational visibility**: Users need to understand cache state and debug staleness issues
- **Consistency with existing patterns**: Follow `internal/version/cache.go` for familiarity and code reuse
- **Backwards compatibility**: Existing workflows must continue working; migration should be automatic
- **Configurability**: Power users need escape hatches for non-standard environments

## Considered Options

### Decision 1: Metadata Storage Format

How should cache metadata (timestamps, access times) be stored?

#### Option 1A: JSON Sidecar Files

Store metadata in `{recipe}.meta.json` files alongside cached recipes.

```
$TSUKU_HOME/registry/
├── f/
│   ├── fzf.toml          # Recipe content
│   └── fzf.meta.json     # Metadata: cached_at, expires_at, last_access, size
```

**Pros:**
- Matches existing `internal/version/cache.go` pattern exactly
- Easy to inspect manually (`cat fzf.meta.json`)
- Atomic updates possible (write metadata after recipe)
- Can add fields without format migration

**Cons:**
- Double the file count in cache directory
- Two files to manage per recipe (slight complexity)
- Extra disk I/O for metadata reads

#### Option 1B: Embedded Metadata Header

Prepend metadata as a comment block in the TOML file itself.

```toml
# tsuku-cache-metadata: {"cached_at":"2025-01-26T...","expires_at":"..."}
[metadata]
name = "fzf"
...
```

**Pros:**
- Single file per recipe
- Metadata travels with content
- Simpler directory structure

**Cons:**
- Non-standard TOML extension (comment parsing)
- Harder to update metadata without rewriting whole file
- Recipe content is modified from source (debugging confusion)
- No precedent in codebase

#### Option 1C: Central Metadata Database

Store all metadata in a single `$TSUKU_HOME/registry/cache.json` file.

```json
{
  "fzf": {"cached_at": "...", "expires_at": "...", "size": 1234},
  "ripgrep": {"cached_at": "...", "expires_at": "...", "size": 2345}
}
```

**Pros:**
- Single file to manage
- Fast lookups (read once, query in memory)
- Easy to enumerate all cache state

**Cons:**
- Concurrent write issues (recipe installs in parallel)
- File corruption loses all metadata
- Must keep in sync with actual recipe files
- No codebase precedent

### Decision 2: Stale Fallback Behavior

How should the cache behave when TTL expires and network is unavailable?

#### Option 2A: Strict TTL (No Stale Fallback)

Expired cache is treated as invalid. Network failure with expired cache = error.

```
TTL expired + network fail → Error: "Could not refresh recipe. Cache expired."
```

**Pros:**
- Simple mental model
- Users always get fresh-ish data
- No risk of using very old cached recipes

**Cons:**
- Transient network issues cause failures
- Airplane mode breaks everything
- Poor offline experience

#### Option 2B: Bounded Stale-If-Error (Industry Standard)

Use stale cache on network failure, with warning and maximum staleness bound. Inspired by HTTP `stale-if-error` (RFC 5861).

```
TTL expired + network fail + age < 7 days → Warning + return stale cache
TTL expired + network fail + age >= 7 days → Error (cache too old)
TTL expired + network ok → Refresh cache, return fresh
```

**Pros:**
- Best user experience (operations succeed when possible)
- Matches apt, npm behavior
- Maximum staleness bound (7 days default) limits security risk
- Configurable via `TSUKU_RECIPE_CACHE_STALE_FALLBACK` and `TSUKU_RECIPE_CACHE_MAX_STALE`
- Warning communicates the situation clearly

**Cons:**
- Users might unknowingly use outdated recipes (limited to 7 days)
- Bug fixes in recipes may not propagate during network issues
- Slightly more complex logic

#### Option 2C: Always-Prefer-Cached

Always use cached recipes if they exist, only fetch on cache miss.

```
Cache exists → Return cached (regardless of age)
Cache miss   → Fetch from network
```

**Pros:**
- Maximum offline capability
- Fastest (no network checks when cached)
- Predictable behavior

**Cons:**
- Recipe updates never propagate automatically
- Users must explicitly `update-registry` to get fixes
- Could cause confusion when recipe behavior differs from registry

### Decision 3: LRU Eviction Trigger

When should LRU eviction occur?

#### Option 3A: Evict on Write

Check and evict after each `CacheRecipe()` call.

**Pros:**
- Cache never exceeds limit
- Simple to reason about
- Eviction is predictable

**Cons:**
- Eviction during install flow (user waiting)
- Could evict recipes user needs soon
- Slight latency on cache writes

#### Option 3B: Evict on Threshold

Evict when cache exceeds 80% of limit, removing until below 60%.

**Pros:**
- Batched eviction is more efficient
- User sees eviction less often
- Headroom prevents constant eviction

**Cons:**
- Cache can temporarily exceed limit
- More complex threshold logic
- Harder to predict when eviction happens

#### Option 3C: Manual Eviction Only

No automatic eviction. Users must run `tsuku cache cleanup`.

**Pros:**
- No surprise evictions
- User controls cache entirely
- Simplest implementation

**Cons:**
- Users must remember to clean up
- Cache can grow unbounded until manual action
- Poor experience in constrained environments

### Decision 4: Error Message Approach

How should cache-related errors be communicated?

#### Option 4A: Structured Error Types

Extend `internal/registry/errors.go` with cache-specific error types.

```go
ErrTypeCacheExpired
ErrTypeCacheStaleUsed
ErrTypeCacheMiss
```

**Pros:**
- Programmatic error handling possible
- Consistent with existing error classification
- Can provide targeted suggestions

**Cons:**
- More error types to maintain
- Users don't see error types directly

#### Option 4B: Message Templates Only

Define message templates without new error types. Use existing types with specific messages.

**Pros:**
- Simpler implementation
- Messages are what users see anyway
- Less type proliferation

**Cons:**
- Harder to handle errors programmatically
- Less structured
- May duplicate message logic

### Evaluation Against Decision Drivers

| Decision | User Experience | Disk Management | Visibility | Consistency | Compatibility | Configurability |
|----------|----------------|-----------------|------------|-------------|---------------|-----------------|
| **1A: JSON Sidecar** | Good | Good | Good | Good (matches version cache) | Good | Good |
| **1B: Embedded Header** | Good | Good | Fair | Poor (no precedent) | Fair | Good |
| **1C: Central DB** | Good | Fair | Good | Poor (no precedent) | Fair | Good |
| **2A: Strict TTL** | Poor | Good | Good | Fair | Good | Good |
| **2B: Stale-If-Error** | Good | Good | Good | Good (industry standard) | Good | Good |
| **2C: Always-Prefer-Cached** | Fair | Good | Poor | Poor | Fair | Fair |
| **3A: Evict on Write** | Fair | Good | Good | Good | Good | Good |
| **3B: Evict on Threshold** | Good | Good | Good | Good | Good | Good |
| **3C: Manual Only** | Fair | Poor | Good | Poor | Good | Good |
| **4A: Structured Errors** | Good | N/A | Good | Good | Good | Good |
| **4B: Templates Only** | Good | N/A | Good | Fair | Good | Good |

### Uncertainties

- **Actual cache sizes in practice**: At ~3KB per recipe, 150 recipes total ~450KB. The 500MB upstream limit may be oversized; a 50MB limit would accommodate ~16,000 recipes with headroom
- **LRU effectiveness**: Recipe access patterns may not follow LRU assumptions (users may reinstall the same few tools repeatedly)
- **Stale fallback risk**: How often do registry recipe bugs get fixed, and how quickly do users need those fixes?

### Key Assumptions

- **Recipe files remain small**: Recipes are TOML metadata (~2-5KB each), not binary assets
- **Filesystem atime unreliable**: Many systems mount with `noatime` or `relatime`, so access time tracking must use sidecar metadata, not filesystem timestamps
- **Single user per cache**: No concurrent user access to `$TSUKU_HOME` (standard for CLI tools)
- **Network failures are transient**: Persistent offline scenarios should use explicit offline mode (future work)

## Decision Outcome

**Chosen: 1A (JSON Sidecar) + 2B (Bounded Stale-If-Error) + 3B (Evict on Threshold) + 4A (Structured Errors)**

### Summary

Implement TTL-based caching with JSON metadata sidecar files, bounded stale-if-error fallback (max 7 days), threshold-based LRU eviction (trigger at 80%, evict to 60%), and structured error types for programmatic handling.

### Rationale

**JSON sidecar files (1A)** chosen because:
- Matches existing `internal/version/cache.go` pattern exactly (consistency driver)
- Enables tracking last_access time independent of unreliable filesystem atime
- Allows atomic updates and future field additions without migration

**Bounded stale-if-error (2B)** chosen because:
- Provides best user experience during transient network issues (UX driver)
- Maximum staleness bound (7 days) limits security exposure from stale cache poisoning
- Configurable via environment variables (configurability driver)
- Matches industry patterns (apt, npm)

**Threshold-based eviction (3B)** chosen because:
- Batched eviction is more efficient and less disruptive to users
- 80%/60% thresholds provide headroom to avoid constant eviction
- Better UX than on-write eviction interrupting install flows
- Respects disk space constraints (disk management driver)

**Structured error types (4A)** chosen because:
- Enables programmatic error handling for tools wrapping tsuku
- Consistent with existing 9 error types in `internal/registry/errors.go`
- Can provide targeted suggestions based on error type (visibility driver)

**Alternatives rejected:**

- **1B (Embedded header)**: Modifies recipe content from source (debugging confusion), no codebase precedent
- **1C (Central DB)**: Concurrent write issues, single point of failure, no codebase precedent
- **2A (Strict TTL)**: Poor offline experience, transient network issues cause unnecessary failures
- **2C (Always-prefer-cached)**: Recipe updates never propagate, poor visibility into freshness
- **3A (Evict on write)**: Eviction during install flow hurts UX, slight latency concern
- **3C (Manual only)**: Unbounded growth in constrained environments, contrary to disk management driver
- **4B (Templates only)**: Less structured, harder to handle errors programmatically

### Trade-offs Accepted

By choosing these options:

1. **Double file count in cache directory**: Metadata sidecars mean two files per recipe. Acceptable because recipe count is bounded (~150 registry recipes), and the pattern is established in version cache.

2. **7-day maximum staleness**: Stale cache can persist up to 7 days during network issues. Acceptable because this bounds security exposure while providing offline resilience. Users needing stricter freshness can set `TSUKU_RECIPE_CACHE_MAX_STALE=0`.

3. **Cache can temporarily exceed limit**: Threshold-based eviction means cache may briefly exceed 80% before eviction runs. Acceptable because the overage is temporary and bounded (eviction runs before next cache write completes).

4. **More error types to maintain**: Three new cache-specific error types increase codebase surface. Acceptable because they follow established patterns and improve user experience with targeted suggestions.

## Solution Architecture

### Overview

The solution adds a cache management layer between the recipe loader and the existing registry, handling TTL expiration, stale fallback, LRU eviction, and cache statistics. The implementation follows the established `internal/version/cache.go` patterns.

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  recipe/loader  │────▶│  CachedRegistry  │────▶│    Registry     │
│   GetWithCtx()  │     │    (new layer)   │     │  (unchanged)    │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                               │
                               ▼
                        ┌──────────────────┐
                        │  CacheManager    │
                        │  (LRU, stats)    │
                        └──────────────────┘
                               │
                               ▼
                        ┌──────────────────┐
                        │   Disk Cache     │
                        │ *.toml + *.meta  │
                        └──────────────────┘
```

### Components

#### 1. CacheMetadata Struct

Stored in `{recipe}.meta.json` alongside the recipe file:

```go
// internal/registry/cache.go (new file)
type CacheMetadata struct {
    CachedAt    time.Time `json:"cached_at"`
    ExpiresAt   time.Time `json:"expires_at"`
    LastAccess  time.Time `json:"last_access"`
    Size        int64     `json:"size"`
    ContentHash string    `json:"content_hash"` // SHA256 of recipe content
}
```

#### 2. CachedRegistry Wrapper

Wraps the existing `Registry` to add TTL and stale fallback:

```go
// internal/registry/cached_registry.go (new file)
type CachedRegistry struct {
    underlying   *Registry
    cacheDir     string
    ttl          time.Duration
    maxStale     time.Duration
    staleFallback bool
    manager      *CacheManager
}

func (c *CachedRegistry) GetRecipe(ctx context.Context, name string) ([]byte, *CacheInfo, error)
func (c *CachedRegistry) GetCacheInfo(name string) *CacheInfo
func (c *CachedRegistry) Refresh(ctx context.Context, name string) ([]byte, error)
func (c *CachedRegistry) RefreshAll(ctx context.Context) (*RefreshStats, error)

func NewCachedRegistry(reg *Registry, mgr *CacheManager, opts CacheOptions) *CachedRegistry
```

**Fetch flow:**
1. Check metadata: if fresh (age < TTL), return cached with updated last_access
2. If expired, try network fetch:
   - Success: update cache and metadata, return fresh
   - Failure + stale allowed (age < maxStale): warn and return stale
   - Failure + too stale: return error with `ErrTypeCacheTooStale`

#### 3. CacheManager for LRU and Stats

Manages cache size limits and provides statistics:

```go
// internal/registry/cache_manager.go (new file)
type CacheManager struct {
    cacheDir   string
    sizeLimit  int64  // Default: 50MB
    highWater  float64 // 0.80 (80%)
    lowWater   float64 // 0.60 (60%)
}

func (m *CacheManager) Size() (int64, error)
func (m *CacheManager) Info() (*CacheStats, error)
func (m *CacheManager) EnforceLimit() (int, error)  // Returns count of evicted entries
func (m *CacheManager) Cleanup(maxAge time.Duration) (int, error)
```

**Eviction algorithm:**
1. On cache write, check if size > sizeLimit * highWater
2. If so, sort entries by last_access ascending
3. Evict until size < sizeLimit * lowWater

#### 4. Cache-Specific Error Types

Add to `internal/registry/errors.go`:

```go
const (
    // ... existing types ...
    ErrTypeCacheTooStale  ErrorType = 10 // Cache exists but exceeds max staleness
    ErrTypeCacheStaleUsed ErrorType = 11 // Stale cache used (warning context)
)
```

### Data Flow

**Fresh cache hit:**
```
GetRecipe("fzf")
  └─▶ Read fzf.meta.json → age < TTL
      └─▶ Update last_access
      └─▶ Return fzf.toml content
```

**Expired cache with successful refresh:**
```
GetRecipe("fzf")
  └─▶ Read fzf.meta.json → age >= TTL
      └─▶ FetchRecipe() from GitHub → success
          └─▶ Write fzf.toml, fzf.meta.json
          └─▶ Check size limit → evict if needed
          └─▶ Return fresh content
```

**Expired cache with network failure (stale-if-error):**
```
GetRecipe("fzf")
  └─▶ Read fzf.meta.json → age >= TTL, age < maxStale
      └─▶ FetchRecipe() from GitHub → network error
          └─▶ Log warning: "Using cached recipe (last updated X hours ago)"
          └─▶ Update last_access
          └─▶ Return stale content + CacheInfo{IsStale: true}
```

**Expired cache with network failure (too stale):**
```
GetRecipe("fzf")
  └─▶ Read fzf.meta.json → age >= maxStale
      └─▶ FetchRecipe() from GitHub → network error
          └─▶ Return ErrTypeCacheTooStale error
              "Could not refresh recipe. Cache expired 8 days ago (max 7 days)."
```

### Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `TSUKU_RECIPE_CACHE_TTL` | 24h | Time until cache is considered stale |
| `TSUKU_RECIPE_CACHE_MAX_STALE` | 7d | Maximum age for stale-if-error fallback |
| `TSUKU_RECIPE_CACHE_SIZE_LIMIT` | 50MB | LRU eviction threshold |
| `TSUKU_RECIPE_CACHE_STALE_FALLBACK` | true | Enable stale-if-error behavior |

### Error Messages

Per Gap 6 from upstream design, exact error messages:

| Failure Mode | Message |
|--------------|---------|
| Network timeout, no cache | "Could not reach recipe registry. Check your internet connection." |
| Network timeout, stale used | stderr: "Warning: Using cached recipe '{name}' (last updated {X} hours ago). Run 'tsuku update-registry' to refresh." |
| Network timeout, too stale | "Could not refresh recipe '{name}'. Cache expired {X} days ago (max {Y} days). Check your internet connection." |
| Recipe doesn't exist | "No recipe found for '{name}'. Run 'tsuku search {name}' to find similar recipes." |
| GitHub rate limited | "Recipe registry temporarily unavailable (rate limited). Try again in a few minutes." |
| Recipe parse error | "Recipe '{name}' has syntax errors. This may be a registry issue. Run 'tsuku update-registry --recipe {name}' to refresh." |
| Cache full | stderr: "Warning: Recipe cache is {X}% full ({Y}MB of {Z}MB). Run 'tsuku cache cleanup' to free space." |

### Command Changes

#### Enhanced `tsuku update-registry`

```
tsuku update-registry [flags]

Flags:
  --dry-run          Show what would be refreshed without fetching
  --recipe <name>    Refresh a specific recipe only
  --all              Refresh all cached recipes (default if no --recipe)
```

Output:
```
Refreshing recipe cache...
  fzf: refreshed (was 2 days old)
  ripgrep: already fresh
  bat: refreshed (was 5 days old)

Refreshed 2 of 3 cached recipes.
```

#### New `tsuku cache cleanup`

```
tsuku cache cleanup [flags]

Flags:
  --dry-run          Show what would be removed without deleting
  --max-age <dur>    Remove entries older than duration (default: 30d)
  --force-limit      Enforce size limit regardless of age
```

Output:
```
Cleaning up recipe cache...
  Removing fzf (not accessed in 45 days)
  Removing old-tool (not accessed in 60 days)

Removed 2 entries, freed 8KB.
Cache: 12KB of 50MB (0.02%)
```

#### Enhanced `tsuku cache info`

Add registry section to existing output:

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

## Implementation Approach

### Stage 1: Cache Metadata Infrastructure

**Goal:** Add metadata tracking without changing cache behavior.

**Steps:**
1. Create `internal/registry/cache.go` with `CacheMetadata` struct
2. Add `metaPath()` function to `Registry` (returns `{recipe}.meta.json` path)
3. Add `WriteMeta()` and `ReadMeta()` methods
4. Update `CacheRecipe()` to write metadata alongside recipe file
5. Migrate existing cached recipes: create metadata on first read

**Migration behavior:** Existing cached recipes without metadata are treated as expired but valid for stale fallback. First read creates metadata with `cached_at` set to the file's modification time.

**Validation:** Existing tests pass. Cached recipes gain metadata files on access.

### Stage 2: TTL Expiration

**Goal:** Check cache freshness before returning cached recipes.

**Steps:**
1. Add `TSUKU_RECIPE_CACHE_TTL` to `internal/config/config.go`
2. Create `internal/registry/cached_registry.go` with `CachedRegistry` wrapper
3. Implement `GetRecipe()` with TTL check:
   - Fresh: return cached
   - Expired: try refresh, return fresh or error
4. Update `recipe.Loader` to use `CachedRegistry`

**Validation:** Cache expires after TTL. Fresh fetches work. Old tests pass.

### Stage 3: Stale Fallback

**Goal:** Use stale cache on network failure with warning.

**Steps:**
1. Add `TSUKU_RECIPE_CACHE_MAX_STALE` and `TSUKU_RECIPE_CACHE_STALE_FALLBACK` configs
2. Add `ErrTypeCacheTooStale` to `internal/registry/errors.go`
3. Extend `CachedRegistry.GetRecipe()`:
   - On network error: check maxStale, return stale with warning or error
4. Add `CacheInfo` return with `IsStale` flag for caller awareness

**Validation:** Network failures fall back to stale cache with stderr warning. Too-stale returns error.

### Stage 4: LRU Size Management

**Goal:** Enforce cache size limits with LRU eviction.

**Steps:**
1. Add `TSUKU_RECIPE_CACHE_SIZE_LIMIT` config (default: 50MB)
2. Create `internal/registry/cache_manager.go` with `CacheManager`
3. Implement `Size()`, `EnforceLimit()`, and eviction logic
4. Call `EnforceLimit()` after `CacheRecipe()` writes
5. Add warning at 80% capacity

**Validation:** Cache stays within limit. Least-recently-used entries evicted first.

### Stage 5: Update-Registry Command Enhancement

**Goal:** Add status reporting and selective refresh.

**Steps:**
1. Add `--dry-run` and `--recipe` flags to `cmd/tsuku/update_registry.go`
2. Implement `Refresh()` method on `CachedRegistry`
3. Show cache age before/after refresh
4. Report count of refreshed recipes

**Validation:** `update-registry --dry-run` shows what would refresh. `--recipe foo` refreshes only foo.

### Stage 6: Cache Cleanup Command

**Goal:** Manual cache management via `tsuku cache cleanup`.

**Steps:**
1. Create `cmd/tsuku/cache_cleanup.go`
2. Implement `--dry-run`, `--max-age`, `--force-limit` flags
3. Add `Cleanup()` method to `CacheManager`
4. Show reclaimed space and new cache size

**Validation:** Cleanup removes old entries. Dry-run shows what would be removed.

### Stage 7: Cache Info Enhancement

**Goal:** Show registry cache statistics in `tsuku cache info`.

**Steps:**
1. Add `Info()` method to `CacheManager`
2. Extend `cmd/tsuku/cache.go` to display registry cache section
3. Show oldest/newest, stale count, percent of limit used

**Validation:** `tsuku cache info` shows comprehensive registry cache statistics.

## Security Considerations

### Download Verification

**Context:** This design affects recipe files (TOML metadata), not the binaries those recipes install. Binary verification is handled by checksums in recipes, not by this cache layer.

**Recipe file verification:** Recipe files are fetched over HTTPS from GitHub's raw content CDN. No additional signature verification is implemented. The trust model relies on:
- GitHub's TLS certificate chain
- Repository access controls (PR review required for main branch)
- Existing content-based checksums in the recipe metadata sidecar

**Cache integrity:** The `CacheMetadata.ContentHash` field stores SHA256 of cached content. On read, content is verified against this hash. If verification fails, the cached entry is discarded and a fresh fetch is attempted. This prevents use of corrupted or tampered cache entries.

### Execution Isolation

**Not directly applicable:** This design does not execute code. It manages TOML files that describe how to install software. The actual execution isolation is handled by the recipe execution layer.

**File system access:** Cache operations require read/write access to `$TSUKU_HOME/registry/`. This is the same scope as current implementation. No new permissions required.

**Network access:** Requires outbound HTTPS to the registry (GitHub raw content). No changes from current implementation.

### Supply Chain Risks

**Stale cache attack surface:** The stale-if-error feature introduces a new attack vector: a compromised recipe could persist in the cache for up to 7 days even if the registry is corrected.

**Mitigations:**
1. **Maximum staleness bound (7 days):** Limits the persistence window for any cached malicious content
2. **Warning on stale use:** Users are informed when stale cache is used, enabling awareness of potential issues
3. **Manual refresh:** Users can immediately get fresh content via `tsuku update-registry --recipe <name>`
4. **Configurable:** Users can disable stale fallback entirely via `TSUKU_RECIPE_CACHE_STALE_FALLBACK=false`

**Cache poisoning:** If an attacker gains write access to `$TSUKU_HOME/registry/`, they could inject malicious recipe files. This is not new - the current implementation has the same vulnerability. This design doesn't make it worse.

**Local privilege:** Cache files are written with mode 0644 (world-readable). This matches current behavior and doesn't expose new attack surface.

### User Data Exposure

**No change from current behavior:** This design doesn't collect or transmit any user data beyond what's already sent (HTTP requests to GitHub for recipes).

**Cache statistics:** The `tsuku cache info` command exposes local cache statistics (entry count, size, ages). This information is only displayed locally and is not transmitted anywhere.

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Stale malicious recipe persists | 7-day max staleness bound | Compromised recipe usable for up to 7 days during network issues |
| Cache poisoning via local write | Standard file permissions (0644) | Requires local file system access (same as current) |
| Recipe tampered in transit | HTTPS to GitHub | Relies on GitHub's TLS and certificate chain |
| Cache content corruption | SHA256 content hash verification | Hash collision (cryptographically infeasible) |
| Unwanted stale cache use | User warning on stderr | User must notice warning in output |

## Consequences

### Positive

- **Better offline experience**: Transient network issues no longer block installations when stale cache is available
- **Cache freshness visibility**: Users can see cache age, stale status, and refresh as needed
- **Bounded disk usage**: LRU eviction prevents unbounded cache growth
- **Clear error messages**: Specific messages for each failure mode with actionable suggestions
- **Configurable behavior**: Power users can tune TTL, max staleness, and size limits
- **Consistent implementation**: Follows established patterns from version cache

### Negative

- **Complexity increase**: New wrapper layer, metadata files, and configuration options add cognitive load
- **Potential for stale data**: Up to 7 days of stale content during network issues
- **Breaking change for cache directory**: Existing recipes will have metadata added on first access (minor)
- **Additional disk I/O**: Metadata file reads on every cache access

### Mitigations

- **Complexity**: Consistent patterns with version cache reduce learning curve. Good defaults mean most users don't need to configure anything.
- **Stale data**: Clear warnings communicate staleness. Users can disable fallback if strict freshness is required.
- **Migration**: Automatic migration on first access means no manual intervention required.
- **I/O overhead**: Metadata files are tiny (<1KB). Impact is negligible compared to network latency saved by caching.
