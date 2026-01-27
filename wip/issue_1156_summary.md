# Issue 1156 Summary

## What Was Done

Added cache metadata infrastructure to track registry recipe cache state with JSON sidecar files. This is Stage 1 of the registry cache policy implementation that enables TTL-based expiration and LRU eviction in subsequent issues.

## Files Changed

- `internal/registry/cache.go` (new): CacheMetadata struct and helper functions
  - CacheMetadata struct with CachedAt, ExpiresAt, LastAccess, Size, ContentHash fields
  - metaPath(), WriteMeta(), ReadMeta(), UpdateLastAccess(), DeleteMeta() methods
  - newCacheMetadata() and newCacheMetadataFromFile() constructors
  - computeContentHash() SHA256 helper
  - ListCachedWithMeta() for cache statistics
  - DefaultCacheTTL constant (24h)

- `internal/registry/registry.go`: Updated to write metadata
  - CacheRecipe() now writes metadata sidecar after recipe file
  - GetCached() creates metadata for existing recipes (migration)
  - GetCached() updates LastAccess on read

- `internal/registry/cache_test.go` (new): Comprehensive tests
  - TestCacheMetadata_WriteMeta
  - TestCacheMetadata_ReadMeta
  - TestCacheMetadata_ReadMeta_InvalidJSON
  - TestCacheRecipe_WritesMetadata
  - TestGetCached_MigratesMetadata
  - TestGetCached_UpdatesLastAccess
  - TestComputeContentHash
  - TestMetaPath
  - TestDeleteMeta
  - TestListCachedWithMeta

## Test Results

- All 17 registry tests passing
- All existing tests in other packages passing
- Validation script from issue passes
- Linter passes

## Design Decisions Made

1. **Sidecar file naming**: Used `.meta.json` suffix matching version cache pattern
2. **Migration strategy**: Create metadata on GetCached() using file mtime for CachedAt
3. **Error handling**: Metadata failures don't fail recipe operations (best effort)
4. **DefaultCacheTTL**: 24 hours as constant; configurable TTL comes in #1157

## Notes for Reviewers

- The implementation follows the existing `internal/version/cache.go` pattern closely
- Metadata writes are best-effort - if they fail, the recipe operation still succeeds
- ListCachedWithMeta() returns nil for recipes without metadata (useful for identifying pre-migration recipes)
