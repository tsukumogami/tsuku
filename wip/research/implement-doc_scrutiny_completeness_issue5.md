# Issue 5 Completeness Review

## Acceptance Criteria Evaluation

### HTTP Client Setup

| AC | Status | Evidence |
|----|--------|----------|
| Two separate HTTP client instances via `httputil.NewSecureClient` | PASS | `NewGitHubClient` at lines 72-74 creates both `rawClient` and `apiClient` via `httputil.NewSecureClient(httputil.DefaultOptions())` |
| Authenticated client carries `GITHUB_TOKEN` via `secrets` package | PASS | Lines 77-83: token retrieved via `secrets.Get("github_token")`, wrapped in `authTransport` |
| Unauthenticated client has no auth headers | PASS | `rawClient` is a plain `NewSecureClient` with no transport wrapping |
| Token never sent to any host other than `api.github.com` | PASS | `authTransport.RoundTrip` (line 51) checks `strings.EqualFold(req.URL.Hostname(), contentsAPIHost)` before adding the header. Test `TestAuthTransport_TokenIsolation` validates non-API hosts don't receive the token. |

### Contents API Integration

| AC | Status | Evidence |
|----|--------|----------|
| `GET /repos/{owner}/{repo}/contents/.tsuku-recipes` lists TOML files | PASS | `listViaContentsAPI` at line 217 constructs the correct URL |
| Parse response to extract file names and `download_url` values | PASS | Lines 260-271 unmarshal `[]contentsEntry`, filter `.toml` files, build `files` map |
| Auto-resolve default branch; cache in `_source.json` | PASS | `extractBranchFromURL` parses branch from download URLs; `PutSourceMeta` writes `_source.json` |
| Rate-limited cold cache fallback: try `main` then `master` via raw URLs | PASS | Lines 131-140 in `ListRecipes` + `probeDefaultBranches` + `tryBranches` implement this |
| Clear error message when rate-limited, guiding user to set `GITHUB_TOKEN` | PASS | `ErrRateLimited.Error()` appends "Set GITHUB_TOKEN for higher rate limits" when `!HasToken` |

### Download URL Validation

| AC | Status | Evidence |
|----|--------|----------|
| Validate every `download_url` uses HTTPS | PASS | `validateDownloadURL` checks `u.Scheme != "https"` at line 345 |
| Hostname allowlist: `raw.githubusercontent.com`, `objects.githubusercontent.com` | PASS | `allowedDownloadHosts` map at lines 21-24 |
| Reject non-allowlisted hostnames with clear error | PASS | Returns `ErrInvalidDownloadURL` with reason including the hostname and allowed list |

### Cache Layer

| AC | Status | Evidence |
|----|--------|----------|
| Cache directory: `$TSUKU_HOME/cache/distributed/{owner}/{repo}/` | PASS | `CacheManager.repoDir` at line 58: `filepath.Join(cm.baseDir, owner, repo)`. Caller passes `$TSUKU_HOME/cache/distributed` as baseDir. |
| Store TOML as `{recipe}.toml`, metadata as `{recipe}.meta.json` | PASS | `PutRecipe` writes `name+".toml"` and `name+".meta.json"` |
| `_source.json` per repo: branch, directory listing, fetch timestamp | PASS | `SourceMeta` struct has `Branch`, `Files`, `FetchedAt`; stored as `_source.json` |
| Separate `CacheManager` instance with independent size limits | **BLOCKING** | `CacheManager` has TTL-based freshness but no size limit mechanism. There is no max cache size, no eviction policy, and no size configuration parameter. The AC explicitly says "independent size limits from central registry cache." |
| Cache lookup before HTTP fetch; return cached data when fresh | PASS | `ListRecipes` checks cache first (lines 113-116). `FetchRecipe` checks cache first (lines 153-156). |

### Input Validation

| AC | Status | Evidence |
|----|--------|----------|
| Reuse `ValidateGitHubURL()` from `internal/discover/sanitize.go` | PASS | `ListRecipes` calls `discover.ValidateGitHubURL(owner + "/" + repo)` at line 108 |
| Reject path traversal, credentials, invalid owner/repo | PASS | `ValidateGitHubURL` handles these. Cache layer (`repoDir`, `GetRecipe`) also rejects `..` and `/`. Test coverage in `TestGitHubClient_ListRecipes_ValidationRejectsInvalid` and `TestCacheManager_PathTraversal`. |

### Error Handling

| AC | Status | Evidence |
|----|--------|----------|
| Distinguish "repo not found", "no `.tsuku-recipes/`", "rate limited", network errors | PASS | Four distinct error types: `ErrRepoNotFound`, `ErrNoRecipeDir`, `ErrRateLimited`, `ErrNetwork` |
| Rate limit errors include remaining/reset from response headers | PASS | `parseRateLimitError` extracts `X-RateLimit-Remaining` and `X-RateLimit-Reset`; `ErrRateLimited` carries both fields |

---

## BLOCKING Issues

### B1: CacheManager has no size limit mechanism

**AC text:** "Separate `CacheManager` instance with independent size limits from central registry cache"

The current `CacheManager` only has a TTL (`time.Duration`) for freshness. There is:
- No `maxSize` or `maxEntries` field
- No eviction policy (LRU, FIFO, or any other)
- No method to compute cache size or prune entries
- No configuration for size limits

The AC specifically calls out "independent size limits from central registry cache," meaning this was a deliberate design requirement to prevent unbounded disk growth from distributed sources. The current implementation will grow without bound as users add more distributed recipe sources.

**Severity:** BLOCKING -- the AC is explicit about size limits and the feature is absent.

**Suggested fix:** Add a `maxBytes int64` (or similar) field to `CacheManager`, a method to compute total cache size, and an eviction pass in `PutRecipe`/`PutSourceMeta` when the limit is exceeded.

---

## ADVISORY Observations

### A1: `TestAuthTransport_TokenIsolation` first subtest is incomplete

The subtest "token sent to api.github.com" (lines 39-55) does setup work but never makes an assertion. It has comments explaining the difficulty of testing against the real hostname, then the subtest body just... ends. The *actual* hostname-matching logic is tested in the second subtest by checking the condition directly, so this isn't a correctness gap, but the dead test code should be removed or completed.

### A2: `TestGitHubClient_ListRecipes_ContentsAPI` doesn't exercise `ListRecipes`

This test (lines 85-154) creates a `GitHubClient` via `newGitHubClientWithHTTP` but never calls `gc.ListRecipes()`. Instead it calls the test server directly and manually reimplements the parsing logic. The `gc` and `ctx` variables are created then discarded with `_ = ctx; _ = gc`. The test validates the parsing logic works in isolation but doesn't test the actual `listViaContentsAPI` code path through the client.

This means the integration between `ListRecipes -> listViaContentsAPI -> response parsing -> cache write` is untested. The difficulty is that `listViaContentsAPI` hardcodes the `api.github.com` hostname. A more effective approach would be to make the API base URL injectable for testing, or to test `listViaContentsAPI` through a test that intercepts at the HTTP transport level.

### A3: `FetchRecipe` cache-hit path not fully tested through the public API

`TestGitHubClient_FetchRecipe_CacheLifecycle` tests the cache directly (`cache.GetRecipe`) rather than through `gc.FetchRecipe()`, because the download URL validation rejects test server URLs. The `FetchRecipe` method's cache-hit code path (lines 153-156) is therefore not exercised through the public API in tests.

### A4: `listViaContentsAPI` conflates 404 for "repo not found" and "no recipe dir"

Line 241 returns `ErrNoRecipeDir` for all 404s. The code comment (lines 238-240) acknowledges this but the AC says to distinguish "repo not found" from "no `.tsuku-recipes/`". In practice, differentiating these would require an additional API call, so this is a pragmatic tradeoff -- but it partially fails the "distinguish repo not found" criterion.

### A5: `GetSourceMeta` returns `(nil, nil)` for missing files, but `ListRecipes` treats this as cache miss correctly

In `ListRecipes` line 113-116, `err == nil` is checked first, then `IsSourceFresh(cached)`. If the file doesn't exist, `GetSourceMeta` returns `(nil, nil)`, so `err == nil` is true but `cached` is nil, making `IsSourceFresh(nil)` return false. This works correctly but is subtle -- a nil check on `cached` before calling `IsSourceFresh` would be clearer.

### A6: Recipe name `a..b` rejected by cache but may be valid

`CacheManager.GetRecipe` rejects any name containing `..` (line 120). A recipe literally named `a..b` (two consecutive dots mid-name, not a traversal) would be rejected. This is conservative and probably fine, but worth noting as a false positive.
