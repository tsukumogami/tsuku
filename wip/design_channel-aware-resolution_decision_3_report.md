<!-- decision:start id="resolve-latest-caching" status="assumed" -->
### Decision: ResolveLatest Caching Strategy

**Context**

Tsuku's CachedVersionLister caches ListVersions results in `$TSUKU_HOME/cache/versions/` with configurable TTL and atomic writes, but ResolveLatest passes through to the network on every call. The channel-aware resolution feature needs both absolute-latest and pin-aware queries ("latest within 18.x"). Pin-aware queries inherently require a version list to filter, making ListVersions the authoritative data source for constrained resolution.

The codebase has two interface levels: VersionResolver (ResolveLatest, ResolveVersion) implemented by all providers, and VersionLister (adds ListVersions) implemented by most but not all providers. CustomProvider and FossilTimelineProvider only implement VersionResolver.

**Assumptions**

- Pin-aware resolution will filter cached ListVersions results rather than introducing a new ResolveLatest variant per constraint. If wrong, a per-constraint cache key scheme would be needed.
- The semantic difference between "first entry in sorted version list" and "latest release from API" is negligible for real-world providers. If wrong, specific providers may need override behavior.
- VersionResolver-only providers (CustomProvider, FossilTimelineProvider) are rare enough that leaving them uncached is acceptable for now.

**Chosen: Derive Latest from Cached ListVersions**

For providers that implement VersionLister, CachedVersionLister.ResolveLatest() returns the first entry from the cached version list instead of calling the underlying provider's ResolveLatest. When the version list cache is warm, no network call happens. When the cache is cold or expired, ListVersions is fetched (populating the cache), and the first result is returned.

Pin-aware queries filter the same cached list by the constraint prefix. A query for "latest within 18.x" loads the cached version list, filters to entries matching `18.*`, and returns the highest match.

For VersionResolver-only providers where no version list is available, ResolveLatest continues to delegate to the underlying provider (network call). This can be extended with per-result caching later if the need arises.

The Refresh() method and `--force` flag clear the ListVersions cache, which implicitly invalidates derived resolution results. No separate cache invalidation path is needed.

**Rationale**

This approach unifies cached data access for both pinned and unpinned queries behind a single cache entry (the version list). It adds no new cache files, no new cache key schemes, and no new structs. It follows the existing CachedVersionLister pattern -- the change is in how ResolveLatest delegates, not in the caching infrastructure itself. The atomic write constraint (R21) is already satisfied by the existing writeCache implementation.

**Alternatives Considered**

- **Extend CachedVersionLister with separate resolution cache entries**: Stores ResolveLatest results in additional `<hash>_latest.json` files alongside list cache files. Rejected because it doubles cache file count per provider and creates two invalidation paths (list cache and resolution cache) that must stay synchronized. Pin-aware queries would still need the list cache, making the resolution cache redundant for the primary use case.

- **Separate CachedVersionResolver layer**: A new struct wrapping VersionResolver independently from CachedVersionLister. Rejected because it adds structural complexity (two cache wrappers to compose and configure) for minimal benefit. The only providers that would use it exclusively are CustomProvider and FossilTimelineProvider, which are uncommon.

**Consequences**

- ResolveLatest for VersionLister providers becomes a cache-backed operation with the same TTL as ListVersions.
- A cold cache will trigger a ListVersions fetch even when only ResolveLatest was requested. This is slightly more expensive on the first call but populates the cache for subsequent pin-aware queries.
- VersionResolver-only providers remain uncached. If this becomes a problem, the "extend with resolution cache" approach can be layered on later without breaking changes.
- The CachedVersionLister.ResolveLatest method changes from a pass-through to a cache-aware method, which is a behavior change for existing callers -- but the result should be identical.
<!-- decision:end -->
