# Design Summary: binary-index-full-registry

## Input Context (Phase 0)
**Source:** Freeform revision of DESIGN-binary-index.md
**Problem:** The binary index only covers locally-cached recipes. On a clean
machine, tsuku which jq returns "not found" after update-registry because the
index is built from ListCached() (local cache only) rather than the full registry
manifest.
**Constraints:** BinaryIndex.Rebuild() signature must not change; manifest is
already downloaded by update-registry; rate limit safety required.

## Decisions Made
- **Option B chosen**: Manifest-driven enumeration with on-demand fetch
- **Bounded concurrency**: semaphore of 10 concurrent fetches
- **Skip on error**: unfetchable recipes are skipped with slog.Warn (non-fatal)

## Security Review (Phase 5)
**Outcome:** Option 2 — Document considerations
**Summary:** No new attack surface; existing TLS/trust model applies. Rate limit
retry gap documented as known limitation.

## Current Status
**Phase:** 5 → 6 (Final Review)
**Last Updated:** 2026-03-23
