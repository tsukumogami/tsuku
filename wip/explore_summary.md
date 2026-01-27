# Exploration Summary: Registry Recipe Cache Policy

## Problem (Phase 1)

The registry recipe cache stores recipes indefinitely with no TTL, no size limits, and no metadata tracking. Network failures cause immediate hard failures instead of falling back to cached data. Users have no visibility into cache state and no way to manage cache growth.

## Decision Drivers (Phase 1)

- **User experience during network issues**: Stale data beats hard failures for most use cases
- **Disk space management**: Unbounded cache growth is problematic for constrained environments
- **Operational visibility**: Users need to understand cache behavior and debug issues
- **Consistency with existing patterns**: Follow `version/cache.go` patterns for metadata and TTL
- **Backwards compatibility**: Existing workflows must continue working
- **Configurable defaults**: Power users need escape hatches

## Research Findings (Phase 2)

**Upstream constraints from DESIGN-recipe-registry-separation.md:**
- 24-hour default TTL (configurable via `TSUKU_RECIPE_CACHE_TTL`)
- Stale cache fallback with warning on network failure
- 500MB LRU cache limit (configurable via `TSUKU_CACHE_SIZE_LIMIT`)
- Enhanced `tsuku update-registry` and new `tsuku cache cleanup` commands
- Defined error messages for all failure modes (Gap 6 in design)

**Existing codebase patterns:**
- `internal/version/cache.go`: Well-structured TTL cache with JSON metadata sidecar files
  - `cacheEntry` struct with CachedAt, ExpiresAt, Source fields
  - Atomic writes via temp file + rename
  - `CacheInfo` type for introspection
- `internal/registry/errors.go`: 9 error types with classification and suggestions
- `internal/config/config.go`: Environment variable patterns with validation

**Industry patterns (from research):**
- Most package managers don't implement automatic size limits (Homebrew, npm, pip, Cargo)
- apt's stale fallback with warning is the most user-friendly pattern
- HTTP stale-if-error concept: serve stale content if origin returns 5xx or is unreachable
- Warnings belong on stderr; should include staleness info and refresh command

## Options (Phase 3)

1. **Metadata storage**: JSON sidecars (like version cache) vs embedded headers vs central DB
2. **Stale fallback**: Strict TTL vs stale-if-error (RFC 5861) vs always-prefer-cached
3. **LRU eviction**: On-write vs threshold-based vs manual-only
4. **Error messages**: Structured error types vs message templates

## Decision (Phase 5)

**Problem:** The registry recipe cache stores recipes indefinitely with no TTL, no size limits, and no metadata tracking, causing network failures to result in hard failures instead of graceful degradation.
**Decision:** Implement TTL-based caching with JSON metadata sidecars, bounded stale-if-error fallback (max 7 days), threshold-based LRU eviction, and structured error types.
**Rationale:** Follows existing version cache patterns for consistency, provides best user experience during network issues with bounded staleness for security, and enables configurable cache management.

## Current Status

**Phase:** 5 - Decision (completed)
**Last Updated:** 2026-01-26
