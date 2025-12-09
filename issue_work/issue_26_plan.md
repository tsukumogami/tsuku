# Issue 26 Implementation Plan

## Summary

Add a caching layer for version lists with configurable TTL to reduce API calls and improve performance. The cache wraps version providers and stores results in `$TSUKU_HOME/cache/versions/`.

## Approach

Create a decorator pattern cache wrapper that implements `VersionLister` and wraps an underlying provider. This allows caching to be added transparently without modifying existing providers.

### Alternatives Considered

- **Modify each provider**: Would require changes to every provider and duplicate caching logic. Not chosen because it violates DRY.
- **Cache at resolver level**: Would cache all resolver calls, but version lists are the main concern. Caching at provider level gives finer control.

## Files to Modify

- `internal/config/config.go` - Add `CacheDir` and `VersionCacheDir` fields, add `EnvVersionCacheTTL` constant, add `GetVersionCacheTTL()` function
- `cmd/tsuku/versions.go` - Add `--refresh` flag to bypass cache, show cache status in output

## Files to Create

- `internal/version/cache.go` - Cache implementation with TTL support
- `internal/version/cache_test.go` - Tests for cache functionality

## Implementation Steps

- [ ] Add cache directory fields to Config struct
- [ ] Add `TSUKU_VERSION_CACHE_TTL` env var support in config package
- [ ] Create `internal/version/cache.go` with CachedVersionLister wrapper
- [ ] Add cache file read/write with TTL checking
- [ ] Update `cmd/tsuku/versions.go` to wrap provider with cache
- [ ] Add `--refresh` flag to versions command
- [ ] Write tests for cache behavior

## Testing Strategy

- Unit tests: Test cache hit/miss logic, TTL expiration, file read/write
- Unit tests: Test env var parsing for TTL configuration
- Unit tests: Test cache key generation from source description
- Manual verification: Run `tsuku versions` twice and observe cache behavior

## Risks and Mitigations

- **Stale cache**: Mitigated by default 1-hour TTL and `--refresh` flag for force refresh
- **Disk space**: Version lists are small (few KB each), not a concern
- **File corruption**: Use atomic writes (write temp, rename)

## Success Criteria

- [ ] Version lists are cached to `$TSUKU_HOME/cache/versions/`
- [ ] Default TTL is 1 hour
- [ ] TTL is configurable via `TSUKU_VERSION_CACHE_TTL`
- [ ] `--refresh` flag bypasses cache
- [ ] Cache status shown in output (e.g., "Using cached versions" vs "Fetching fresh")
- [ ] All tests pass

## Open Questions

None - requirements are clear from the issue.
