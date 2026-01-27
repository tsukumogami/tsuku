---
summary:
  constraints:
    - Must match existing internal/version/cache.go pattern for consistency
    - JSON sidecar files named {recipe}.meta.json alongside .toml files
    - CacheMetadata must include all fields for downstream issues (CachedAt, ExpiresAt, LastAccess, Size, ContentHash)
    - Migration required - existing cached recipes without metadata must work
  integration_points:
    - internal/registry/registry.go - modify CacheRecipe(), add metaPath/WriteMeta/ReadMeta
    - internal/registry/cache.go (new) - CacheMetadata struct
    - internal/version/cache.go - reference pattern to follow
  risks:
    - Breaking existing caching behavior (mitigate with tests)
    - Migration edge cases for corrupted or partial cache entries
    - File permission issues on metadata writes
  approach_notes: |
    Follow the version cache pattern exactly. Create cache.go with CacheMetadata struct.
    Add methods to Registry: metaPath() returns path, WriteMeta/ReadMeta handle JSON sidecar.
    Update CacheRecipe() to write metadata after recipe. In GetCached(), if no metadata
    exists, create it using file mtime as CachedAt. Use a default TTL constant (actual
    config comes in issue #1157). ContentHash uses SHA256 for integrity verification.
---

# Implementation Context: Issue #1156

**Source**: docs/designs/DESIGN-registry-cache-policy.md

## Key Design Decisions

1. **JSON Sidecar Files (Option 1A)**: Store metadata in `{recipe}.meta.json` alongside cached recipes
   - Matches `internal/version/cache.go` pattern exactly
   - Easy to inspect manually
   - Atomic updates possible

2. **CacheMetadata Struct**:
   ```go
   type CacheMetadata struct {
       CachedAt    time.Time `json:"cached_at"`
       ExpiresAt   time.Time `json:"expires_at"`
       LastAccess  time.Time `json:"last_access"`
       Size        int64     `json:"size"`
       ContentHash string    `json:"content_hash"` // SHA256
   }
   ```

3. **Migration Behavior**: Existing cached recipes without metadata are treated as expired but valid. First read creates metadata with `cached_at` set to file's modification time.

## Files to Create/Modify

- **Create**: `internal/registry/cache.go` - CacheMetadata struct and related functions
- **Modify**: `internal/registry/registry.go` - Add metaPath(), WriteMeta(), ReadMeta(), update CacheRecipe()

## Reference Implementation

See `internal/version/cache.go` for the pattern to follow - it uses similar JSON sidecar approach.

## Downstream Dependencies

- **#1157** (TTL expiration) needs: CacheMetadata, metaPath(), ReadMeta(), WriteMeta()
- **#1158** (LRU management) needs: CacheMetadata.Size, CacheMetadata.LastAccess, ReadMeta()
