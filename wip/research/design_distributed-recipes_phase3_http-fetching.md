# Phase 3 Research: HTTP Fetching & Caching

## Questions Investigated
1. How does the existing HTTP fetching work? What HTTP client is used? What error handling exists?
2. How does the TTL, stale-if-error, and metadata sidecar system work?
3. How does LRU eviction work?
4. Which GitHub API is best for fetching 1-10 small TOML files from `.tsuku-recipes/`?
5. How do we discover the default branch?
6. How should per-provider caching work?
7. What SSRF protections and timeouts exist?
8. How do we list multiple TOML files in `.tsuku-recipes/` via HTTP?

## Findings

### Q1: Existing HTTP Fetching

The `Registry` struct (`internal/registry/registry.go:27-32`) uses a dedicated `*http.Client` created by `newRegistryHTTPClient()` (lines 37-51). This client:

- Uses `config.GetAPITimeout()` for the overall timeout (default 30s, configurable via `TSUKU_API_TIMEOUT`)
- Disables compression (decompression bomb protection)
- Sets dial timeout (10s), TLS handshake timeout (10s), response header timeout (10s)
- Does NOT use `httputil.NewSecureClient` -- it lacks SSRF protection and redirect validation

The registry currently fetches individual recipes by constructing a URL: `{BaseURL}/recipes/{first-letter}/{name}.toml` (line 86). The base URL defaults to `https://raw.githubusercontent.com/tsukumogami/tsuku/main` (line 20).

Error handling is thorough via `RegistryError` typed errors (`internal/registry/errors.go`). The error classification system (`classifyError`, line 99) detects DNS errors, TLS errors, timeouts, connection failures, and rate limits. Each error type has a user-facing suggestion (`Suggestion()`, line 70).

Key observation: The registry HTTP client has no `Authorization` header support. It relies entirely on unauthenticated `raw.githubusercontent.com` access. The version resolver (`internal/version/resolver.go`) handles GitHub auth separately using `go-github` with `oauth2` and the `secrets` package for `GITHUB_TOKEN`.

### Q2: TTL, Stale-if-Error, and Metadata Sidecar

`CachedRegistry` (`internal/registry/cached_registry.go:39-45`) wraps `Registry` and adds:

- **TTL**: Configurable, default 24h (`DefaultCacheTTL`). Checked via `isFresh()` (line 195) which computes expiration from `meta.CachedAt + ttl`, ignoring the stored `ExpiresAt` field. This means TTL changes take effect immediately without cache invalidation.
- **Stale-if-error**: When network fetch fails for an expired entry, `handleStaleFallback()` (line 139) returns stale content if age < `maxStale` (default 7 days). Prints a warning to stderr.
- **Metadata sidecar**: Each cached recipe `{name}.toml` has a companion `{name}.meta.json` (`internal/registry/cache.go:20-35`) storing `CachedAt`, `ExpiresAt`, `LastAccess`, `Size`, and `ContentHash` (SHA256).

The `GetRecipe()` flow (line 92):
1. Check cache -> if fresh, return
2. If expired, try network refresh
3. If network fails, try stale fallback
4. If not cached, fetch from network

### Q3: LRU Eviction

`CacheManager` (`internal/registry/cache_manager.go:51-56`) uses high/low water marks:
- **High water**: 80% of `sizeLimit` (default 50MB) -- triggers eviction
- **Low water**: 60% -- eviction target
- Entries sorted by `LastAccess` ascending (oldest first)
- Deletes both `.toml` and `.meta.json` files
- `EnforceLimit()` (line 216) is called after each cache write when CacheManager is configured
- `Cleanup()` (line 263) removes entries not accessed within `maxAge`

The cache directory structure is `{CacheDir}/{first-letter}/{name}.toml` with sidecar `.meta.json`.

### Q4: Best GitHub API for Fetching `.tsuku-recipes/`

Three options analyzed:

**Option A: Raw Content (`raw.githubusercontent.com`)**
- URL: `https://raw.githubusercontent.com/{owner}/{repo}/{branch}/.tsuku-recipes/{file}`
- Pros: No rate limits, no auth needed, simple GET, fast CDN-served
- Cons: Cannot list directory contents (must know filenames), requires knowing the branch name
- File size limit: None practical (serves raw files)

**Option B: Contents API (`api.github.com`)**
- URL: `https://api.github.com/repos/{owner}/{repo}/contents/.tsuku-recipes`
- Pros: Can list directory contents (returns JSON array), returns file content inline (base64), default branch resolved automatically
- Cons: 60 requests/hour unauthenticated, 5000 with token. Files limited to 1MB. Each directory listing is 1 request, each file fetch via API is another.
- For a single-recipe repo: 1 request (list) + 0 requests (content inline if < 1MB) = 1 request
- For multi-recipe repo: 1 request (list returns all files with content)

**Option C: Archive/Tarball API**
- URL: `https://api.github.com/repos/{owner}/{repo}/tarball/{ref}`
- Pros: Gets everything in one request, follows default branch ref
- Cons: Downloads entire repo as tarball (overkill for 1-10 TOML files), counts against API rate limit

**Recommendation: Two-phase approach using Contents API + raw content fallback.**

Phase 1 (discovery): Use the Contents API to list `.tsuku-recipes/` directory and discover file names. This returns file metadata including `download_url` fields that point to `raw.githubusercontent.com`. Cost: 1 API request.

Phase 2 (content): If the Contents API response includes file content inline (files < 1MB, which TOML files always are), extract it directly. Otherwise, use the `download_url` from the listing to fetch via raw content (no rate limit).

Actually, there's an even better approach. The Contents API for a directory returns objects with `name`, `path`, `size`, `sha`, and `download_url` but NOT inline content. You'd need individual file requests. However, the `download_url` values are raw.githubusercontent.com URLs, which are unlimited.

**Best strategy:**
1. One Contents API call to list `.tsuku-recipes/` directory (1 rate-limited request)
2. Fetch each file via its `download_url` (raw.githubusercontent.com, unlimited)
3. If Contents API is rate-limited, fall back to well-known filename conventions or cached directory listings

This minimizes API rate limit consumption while supporting discovery of multi-recipe repos.

### Q5: Default Branch Discovery

Several approaches:

1. **Contents API handles it implicitly**: `GET /repos/{owner}/{repo}/contents/.tsuku-recipes` uses the repo's default branch automatically when no `ref` parameter is specified.

2. **`HEAD` as branch name with raw content**: `raw.githubusercontent.com/{owner}/{repo}/HEAD/...` does NOT work. You must specify an actual branch name.

3. **Repos API**: `GET /repos/{owner}/{repo}` returns `default_branch` field. Costs 1 API request.

4. **Hardcoded `main` with `master` fallback**: Try `main`, if 404, try `master`. Works for raw content but wastes requests on repos using `master`.

**Recommendation**: Use the Contents API for initial discovery (which auto-resolves default branch). Cache the branch name from the response headers or derive it. For subsequent raw content fetches, use the cached branch name. If Contents API is unavailable (rate limited), try `main` then `master` for raw content.

Actually, the Contents API response doesn't directly tell you the branch name. But the `Repos API` call (`GET /repos/{owner}/{repo}`) returns it. Given we need 1 API call for directory listing anyway, we could:
1. Fetch repo metadata (1 API call) -> get `default_branch`
2. List directory via raw content... but raw content can't list directories.

The pragmatic answer: **Use Contents API for everything initial (auto-resolves branch), cache the directory listing, then use raw content URLs from the cached listing for refresh.**

### Q6: Per-Provider Caching

The existing cache structure is flat: `$TSUKU_HOME/registry/{letter}/{name}.toml`. Distributed recipes from different sources could collide on names.

**Proposed structure:**
```
$TSUKU_HOME/registry/
  {letter}/{name}.toml              # Official registry (unchanged)
  {letter}/{name}.meta.json         # Official metadata (unchanged)
  distributed/
    {owner}/{repo}/
      {name}.toml                   # Distributed recipe
      {name}.meta.json              # Distributed metadata
      _source.json                  # Source metadata (branch, last listing, etc.)
```

Key considerations:
- The existing `CacheManager` (`cache_manager.go`) walks `{CacheDir}/{letter}/` looking for `.toml` files. It would NOT see files under `distributed/` without modification.
- `CacheManager.listEntries()` (line 123) and `Size()` (line 71) only walk one level of letter directories.
- Either: (a) extend CacheManager to also walk `distributed/`, or (b) create a separate CacheManager instance for distributed recipes with its own size limit.

**Recommendation**: Option (b) -- separate CacheManager. Distributed recipes are a different provider with different eviction semantics. The official registry cache is bounded by the known recipe count (~1500). Distributed sources are unbounded and user-driven. Separate limits prevent a proliferation of distributed sources from evicting official registry cache entries.

The `_source.json` file per repo would store:
```json
{
  "default_branch": "main",
  "last_listing": ["tool-a.toml", "tool-b.toml"],
  "last_listed_at": "2026-03-15T10:00:00Z"
}
```

### Q7: SSRF Protections and Timeouts

Two HTTP client implementations exist:

1. **Registry client** (`registry.go:37-51`): Basic `http.Transport` with timeouts. NO SSRF protection, no redirect validation. Only connects to `raw.githubusercontent.com` (hardcoded safe URL).

2. **Secure client** (`internal/httputil/client.go:57-109`): Full security stack:
   - SSRF protection via `ValidateIP()` (`internal/httputil/ssrf.go:18`)
   - DNS rebinding protection (resolves all IPs and validates each)
   - HTTPS-only redirect enforcement
   - Redirect chain limit
   - Decompression bomb protection

For distributed recipes, the user provides `owner/repo` which gets resolved to GitHub URLs. While the domain is always `api.github.com` or `raw.githubusercontent.com`, we should still use the secure client because:
- Future providers might not be GitHub-only
- Redirects from GitHub could theoretically land elsewhere
- Defense in depth

**Recommendation**: Use `httputil.NewSecureClient` for the distributed source HTTP client. The existing registry client should arguably be migrated too, but that's out of scope.

**Auth token handling**: The `secrets` package (`internal/secrets/specs.go`) already knows about `github_token` with env var `GITHUB_TOKEN`. The version resolver uses it via `secrets.Get("github_token")`. The distributed source fetcher should use the same mechanism to get authenticated API access (5000 req/hr vs 60).

### Q8: Listing Multiple TOML Files via HTTP

As analyzed in Q4, `raw.githubusercontent.com` cannot list directory contents. The Contents API is the only HTTP-based way to discover file names in `.tsuku-recipes/`.

For the `owner/repo:recipe-name` shorthand (R1), listing isn't needed -- we know the filename: `.tsuku-recipes/{recipe-name}.toml`. This can use raw content directly if we know the branch.

For `owner/repo` without a recipe name (install all recipes from a source, or search), we must list the directory via Contents API.

**Edge cases:**
- Repo has no `.tsuku-recipes/` directory: Contents API returns 404. Map to `ErrTypeNotFound`.
- Repo is private: Returns 404 (without auth) or 403 (with insufficient auth). Need clear error messaging.
- Directory has non-TOML files: Filter by `.toml` extension from listing.
- Nested directories inside `.tsuku-recipes/`: The PRD says "one or more TOML files" in `.tsuku-recipes/`. Contents API returns only immediate children (not recursive). This is fine -- keep it flat.

## Implications for Design

### Fetching Strategy (Proposed)

```
tsuku install owner/repo:tool-name
```
1. Check distributed cache for `{owner}/{repo}/{tool-name}.toml`
2. If fresh, use cached recipe
3. If miss or stale:
   a. Try raw content: `https://raw.githubusercontent.com/{owner}/{repo}/main/.tsuku-recipes/{tool-name}.toml`
   b. If 404 on `main`, try `master`
   c. If still 404, try Contents API to check if repo/directory exists (gives better error)
   d. Cache result with source metadata

```
tsuku install owner/repo  (all recipes)
```
1. Check `_source.json` for cached directory listing
2. If stale or missing, call Contents API: `GET /repos/{owner}/{repo}/contents/.tsuku-recipes`
3. For each TOML file in listing, fetch via `download_url` (raw content, unlimited)
4. Cache all recipes and update `_source.json`

### Auth Integration

Create a shared function (or use existing `secrets.Get("github_token")`) for the distributed fetcher. Add `Authorization: Bearer {token}` header to Contents API calls. Raw content doesn't need auth for public repos.

### New Error Types

Add to `errors.go`:
- `ErrTypeSourceNotFound` -- repo or `.tsuku-recipes/` directory doesn't exist
- `ErrTypeSourceRateLimit` -- GitHub API rate limit (with suggestion to set `GITHUB_TOKEN`)

Or reuse existing `ErrTypeNotFound` and `ErrTypeRateLimit` which already have appropriate suggestions.

### Config Additions

New env vars (following existing pattern in `config.go`):
- `TSUKU_DISTRIBUTED_CACHE_SIZE_LIMIT` -- separate size limit for distributed recipe cache (default 10MB)
- `TSUKU_DISTRIBUTED_CACHE_TTL` -- TTL for distributed recipes (could default shorter, e.g., 1h, since these are more volatile)

### Interface Alignment

The `RecipeProvider` interface should expose:
- `GetRecipe(ctx, name) ([]byte, *CacheInfo, error)` -- matching CachedRegistry's signature
- `ListRecipes(ctx) ([]string, error)` -- for discovery
- `Refresh(ctx, name) error` -- for update-registry

The distributed provider's `GetRecipe` would accept names like `owner/repo:tool-name` and handle the fetching/caching internally.

## Surprises

1. **The registry HTTP client lacks SSRF protection.** It's safe today because it only hits `raw.githubusercontent.com`, but the distributed source fetcher must use `httputil.NewSecureClient` since user-provided `owner/repo` values influence URL construction.

2. **No auth in registry client.** The version resolver has GitHub auth via `go-github` + `oauth2`, but the registry fetcher has no auth at all. Since distributed sources use the GitHub Contents API (rate limited), auth is needed. The `secrets` package already handles `GITHUB_TOKEN` -- we just need to wire it in.

3. **CacheManager only walks letter-bucketed directories.** It can't manage distributed recipe cache without modification. A separate CacheManager instance with its own root is the clean path.

4. **The Contents API returns `download_url` fields pointing to raw.githubusercontent.com.** This means one rate-limited API call for directory listing gives us unlimited-access URLs for actual file content. This is the key insight for the fetching strategy.

5. **`raw.githubusercontent.com` does not support `HEAD` as a branch ref.** Default branch discovery requires either an API call or trying `main`/`master` in sequence.

6. **Contents API auto-resolves default branch.** When no `ref` query parameter is provided, it uses the repo's default branch. This eliminates the need for a separate branch-discovery API call in the common case (listing the directory).

## Summary

The existing registry infrastructure provides a solid foundation: typed errors, TTL-based caching with stale-if-error fallback, LRU eviction via CacheManager, and metadata sidecars. For distributed sources, the optimal fetching strategy is a two-tier approach: use the GitHub Contents API (1 rate-limited call) to discover and list `.tsuku-recipes/` directory contents, then fetch individual files via `raw.githubusercontent.com` URLs (unlimited, no rate limit). Auth via `GITHUB_TOKEN` (already supported by the `secrets` package) raises the Contents API limit from 60 to 5000 requests/hour. Distributed recipes should get their own cache subdirectory and separate CacheManager instance to avoid interfering with the official registry cache.
