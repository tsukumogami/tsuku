# Implementation Plan: Issue 1156

## Summary

Add cache metadata infrastructure to track recipe cache state with JSON sidecar files. This is the foundation for TTL-based expiration and LRU eviction in subsequent issues.

## Files to Create

- `internal/registry/cache.go`: CacheMetadata struct and helper functions

## Files to Modify

- `internal/registry/registry.go`: Add metaPath(), WriteMeta(), ReadMeta() methods; update CacheRecipe() to write metadata

## Implementation Steps

1. **Create cache.go with CacheMetadata struct**
   ```go
   type CacheMetadata struct {
       CachedAt    time.Time `json:"cached_at"`
       ExpiresAt   time.Time `json:"expires_at"`
       LastAccess  time.Time `json:"last_access"`
       Size        int64     `json:"size"`
       ContentHash string    `json:"content_hash"`
   }
   ```

2. **Add metaPath() function to Registry**
   - Returns `{recipe}.meta.json` path (e.g., `registry/f/fzf.meta.json`)
   - Pattern: same directory structure as cachePath(), but with `.meta.json` extension

3. **Add WriteMeta() method to Registry**
   - Takes recipe name and CacheMetadata
   - Writes JSON to metaPath(name)
   - Uses json.MarshalIndent for readability (matches version cache pattern)

4. **Add ReadMeta() method to Registry**
   - Takes recipe name
   - Reads and unmarshals JSON from metaPath(name)
   - Returns nil, nil if file doesn't exist (cache miss)
   - Returns nil, error if file exists but can't be read/parsed

5. **Add computeContentHash() helper**
   - Takes content bytes
   - Returns SHA256 hex string

6. **Define DefaultCacheTTL constant**
   - 24 hours (actual config comes in issue #1157)

7. **Update CacheRecipe() to write metadata**
   - After writing recipe content, create CacheMetadata with:
     - CachedAt: time.Now()
     - ExpiresAt: time.Now().Add(DefaultCacheTTL)
     - LastAccess: time.Now()
     - Size: len(content)
     - ContentHash: computeContentHash(content)
   - Call WriteMeta()

8. **Update GetCached() for migration**
   - After reading recipe content, check if metadata exists
   - If no metadata, create it using file mtime for CachedAt
   - Set ExpiresAt to CachedAt + DefaultCacheTTL
   - Write the new metadata

9. **Add tests for new functionality**
   - TestCacheMetadata_WriteMeta
   - TestCacheMetadata_ReadMeta
   - TestCacheRecipe_WritesMetadata
   - TestGetCached_MigratesMetadata

## Test Strategy

- Unit tests for WriteMeta/ReadMeta functions
- Integration test for CacheRecipe writing metadata
- Migration test for GetCached creating metadata for existing recipes
- Ensure all existing tests pass unchanged

## Design Decisions

1. **Sidecar file naming**: Use `.meta.json` suffix (e.g., `fzf.meta.json`) to match version cache pattern
2. **DefaultCacheTTL as constant**: 24 hours, matching design. Actual config in issue #1157
3. **Migration on read**: Create metadata during GetCached() if missing, using file mtime
4. **SHA256 for ContentHash**: Standard choice, matches design security considerations
5. **Keep existing behavior**: Don't fail operations if metadata write fails (log only)

## Open Questions

None - design doc is clear on all requirements.
