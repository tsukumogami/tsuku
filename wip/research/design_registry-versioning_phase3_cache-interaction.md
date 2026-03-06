# Phase 3 Research: Stale Cache Interaction

## Questions Investigated

1. How does `CachedRegistry` implement stale-if-error? Full code path for fresh, expired+network success, expired+network failure within/beyond maxStale.
2. What happens when `parseManifest()` fails today?
3. Schema version incompatibility with network down -- what should happen?
4. Should the version check happen before or after caching?
5. How does the manifest cache interact with `update-registry`?
6. Is there a per-recipe stale-if-error path that also needs the version check?

## Findings

### 1. CachedRegistry stale-if-error implementation

The stale-if-error system is implemented in `CachedRegistry.GetRecipe()` (`internal/registry/cached_registry.go:92-135`) with four distinct paths:

**Fresh cache hit (line 108-111):** `ReadMeta` succeeds, `isFresh(meta)` returns true. Returns cached bytes directly with `CacheInfo{IsStale: false}`. No network call.

**Expired cache + network success (line 113-127):** `isFresh` returns false. Calls `registry.FetchRecipe()`. On success, calls `cacheWithTTL()` to update cache and metadata, returns fresh content.

**Expired cache + network failure within maxStale (line 115-118, then `handleStaleFallback` line 139-168):** Network fetch fails. `handleStaleFallback` checks: (a) `staleFallback` is enabled and `maxStale > 0`, (b) `meta != nil`, (c) `time.Since(meta.CachedAt) < maxStale` (default 7 days). If all pass, prints a stderr warning and returns stale bytes with `CacheInfo{IsStale: true}`.

**Expired cache + network failure beyond maxStale (line 162-167):** Same path but age exceeds `maxStale`. Returns `RegistryError{Type: ErrTypeCacheTooStale}` with a message like "cache expired 10 days ago (max 7 days)".

**Cache miss + network failure (line 129-134):** Calls `fetchAndCache`, which calls `FetchRecipe`. Error propagated directly -- no fallback possible since there's nothing cached.

Key defaults: TTL defaults to `DefaultCacheTTL` (24h, `cache.go:16`), maxStale defaults to 7 days (`cached_registry.go:53`), staleFallback defaults to true (`cached_registry.go:54`).

### 2. What happens when parseManifest() fails today

`parseManifest()` (`manifest.go:158-164`) does only JSON unmarshalling. If it fails:

- **In `FetchManifest`** (line 125-128): The parse error is returned immediately. The raw data is NOT cached (caching happens at line 131, after the parse check). This is significant -- invalid JSON never reaches disk.
- **In `GetCachedManifest`** (line 59): The parse error is returned to the caller. There is no retry or fallback logic in `GetCachedManifest` itself.

There is no stale-if-error for the manifest. `GetCachedManifest` and `FetchManifest` live on the `Registry` type, not `CachedRegistry`. The manifest has no TTL metadata, no sidecar `.meta.json`, and no freshness checks. It's a simple read-or-fetch model.

### 3. Schema version incompatibility + network down

If a cached `manifest.json` has `schema_version: 2` but the CLI only supports up to version 1:

Today nothing happens because `parseManifest()` doesn't check `schema_version` at all. The field is deserialized into `Manifest.SchemaVersion` (a string, line 28) but never validated.

With the proposed version check in `parseManifest()`:
- `GetCachedManifest` would return an error (version incompatible).
- The caller (`internal/recipe/loader.go:403`) calls `GetCachedManifest` but doesn't call `FetchManifest` on failure -- it just proceeds without the manifest. So a version-incompatible stale cache would cause the satisfies index to be empty, but wouldn't block installation.
- There's no mechanism to recover: the network is down, and the cached data is rejected. The CLI degrades gracefully since the manifest is optional for core operations.

This is actually the correct behavior for the design requirement ("version-incompatible stale manifest should be treated as unusable"). The manifest is used for the satisfies index (ecosystem name resolution), not for core recipe loading.

### 4. Version check timing relative to caching

Current `FetchManifest` flow (line 124-136):
1. Fetch raw bytes from network
2. Call `parseManifest(data)` -- validates JSON
3. Only if parse succeeds: call `CacheManifest(data)` (writes raw bytes to disk)
4. Return parsed manifest

This means the version check (added to `parseManifest`) would naturally gate caching. A version-incompatible manifest fetched from the network would NOT be written to disk. This is the right behavior:

- If the server sends schema v2 and the CLI only supports v1, the CLI refuses to cache it. The old v1 cache (if present) remains on disk untouched.
- The old cached manifest can still be read by `GetCachedManifest` -- but only if it passes the same version check in `parseManifest`.

One edge case: if the user upgrades their CLI from v1-only to v2-supporting, and the cached manifest is still v1, that's fine -- the range acceptance model means v2-supporting CLIs accept v1 manifests.

The reverse is the problem case: downgrading the CLI (or a server-side schema bump). The stale v2 manifest on disk would be rejected by the downgraded CLI. But since `CacheManifest` just overwrites the file, the next successful `FetchManifest` with a compatible version will replace it.

### 5. Manifest cache and update-registry interaction

`update-registry` (`cmd/tsuku/update_registry.go`) does two things:

1. **Manifest refresh** (line 51): Calls `refreshManifest(ctx, reg)` which calls `reg.FetchManifest(ctx)`. This always hits the network -- there's no TTL check for the manifest. Errors are non-fatal (logged to stderr, line 233).

2. **Recipe refresh** (lines 53-58): Either refreshes a single recipe or all cached recipes via `CachedRegistry.RefreshAll()`. `RefreshAll` (line 316-378) DOES respect TTL -- it skips fresh entries (line 334). But `--all` flag forces refresh of everything via `forceRegistryRefreshAll` (line 147-150).

The manifest refresh in `update-registry` always fetches fresh. With the version check, if the server returns an incompatible schema version:
- `FetchManifest` would return an error from `parseManifest`
- `refreshManifest` would log a warning to stderr (line 233)
- The old cached manifest remains on disk (since `CacheManifest` is only called after successful parse)
- Recipe refresh continues normally -- the manifest is independent of per-recipe operations

### 6. Per-recipe stale-if-error and the version check

The per-recipe stale-if-error path (`CachedRegistry.GetRecipe` and `handleStaleFallback`) deals with TOML recipe files, not the JSON manifest. These are entirely separate systems:

- **Manifest**: Single `manifest.json` file, no TTL metadata, no stale-if-error, read by `GetCachedManifest`/`FetchManifest` on `Registry`.
- **Recipes**: Per-recipe `.toml` files with `.meta.json` sidecars, TTL + stale-if-error, managed by `CachedRegistry`.

The manifest version check does NOT need to be added to the per-recipe path. Recipes are TOML files with their own schema (defined in `internal/recipe/`). If recipe schema versioning is ever needed, it would be a separate concern.

However, one indirect interaction exists: `internal/recipe/loader.go:403` calls `GetCachedManifest()` during recipe loading to populate the satisfies index. If the cached manifest fails version validation, the loader continues without it. This means a version-incompatible manifest doesn't block recipe operations -- it only degrades ecosystem name resolution.

## Implications for Design

**The manifest has no stale-if-error, which simplifies the version check.** Since there's no fallback logic for the manifest (unlike per-recipe caching), the version check in `parseManifest` is a clean gate: either the manifest parses and passes the version check, or it doesn't. No need to decide "should we fall back to a stale but version-incompatible manifest" because there's no fallback mechanism to begin with.

**Cache-before-check vs check-before-cache is already solved.** `FetchManifest` validates before caching (line 124-136). Adding the version check to `parseManifest` preserves this ordering automatically. Incompatible data stays off disk.

**The old cache survives server-side upgrades.** When the server starts serving schema v2 and the CLI only supports v1, `FetchManifest` will reject the new data and leave the old v1 cache intact. `GetCachedManifest` will still serve the old v1 data (which passes the version check). This provides natural degradation without explicit fallback logic.

**update-registry won't break on version mismatch.** Since manifest errors in `update-registry` are non-fatal warnings, a schema version bump on the server won't prevent recipe refresh operations. Users will see a warning but their tools will continue to install.

**No changes needed in `CachedRegistry` or `cache.go`.** The version check is entirely within the manifest subsystem. The per-recipe stale-if-error in `CachedRegistry` is unaffected.

**One gap: no user-visible signal for persistent version mismatch.** If the manifest version is bumped server-side and the user never upgrades their CLI, they'll get the stderr warning on every `update-registry` and silently use a stale manifest for all other operations. Consider adding a "please upgrade tsuku" message when the version check fails, distinct from the generic parse error.

## Surprises

1. **The manifest has NO stale-if-error or TTL.** Despite `CachedRegistry` having a complete stale-if-error system for per-recipe caching, the manifest bypasses all of it. `GetCachedManifest` and `FetchManifest` are methods on `Registry`, not `CachedRegistry`. The manifest is a simple "read from disk or fetch from network" with no freshness semantics.

2. **`SchemaVersion` is a string, not an int.** The `Manifest` struct (`manifest.go:28`) defines `SchemaVersion string`. The design calls for integer version with range acceptance, which means `parseManifest` will need to parse this string to an int (or change the struct field type). Current manifests use `"1.0"` format based on the test fixtures, which would need conversion logic.

3. **`parseManifest` does zero validation today.** It only checks JSON syntax via `json.Unmarshal`. No field validation, no required-field checks, nothing. The version check would be the first semantic validation in this function.

4. **The manifest is optional for core operations.** The loader at `internal/recipe/loader.go:403` treats `GetCachedManifest` failure as non-fatal. This means the version check can be strict (reject incompatible versions) without risking breakage of core install/update flows. This is a strong argument for strict rejection over lenient degradation.

## Summary

The manifest cache system is simpler than expected -- it has no TTL, no stale-if-error, and no sidecar metadata, unlike the per-recipe cache in `CachedRegistry`. Adding the version check to `parseManifest()` works cleanly because `FetchManifest` already validates before caching, so incompatible data never reaches disk while the old compatible cache survives untouched. The per-recipe stale-if-error system in `CachedRegistry` is completely independent and needs no changes.
