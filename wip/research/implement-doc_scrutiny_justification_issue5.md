# Justification Review: Issue 5 -- GitHub HTTP Fetching and Cache

**Focus**: justification (technical decisions, pattern consistency, security)
**Files reviewed**:
- `internal/distributed/client.go`
- `internal/distributed/cache.go`
- `internal/distributed/errors.go`
- `internal/distributed/client_test.go`
- `internal/distributed/cache_test.go`

**Reference patterns**:
- `internal/httputil/client.go` (HTTP client patterns)
- `internal/secrets/secrets.go` (token handling)
- `internal/config/config.go` (cache directory patterns)

---

## BLOCKING Issues

### B1: FetchRecipe returns nil on 304 when cache has no content

**File**: `client.go:183`
**Code**:
```go
if resp.StatusCode == http.StatusNotModified && cached != nil {
    return cached, nil
}
```

The `cached` variable is loaded on line 153-155 from `gc.cache.GetRecipe()`. However, the cache check at line 154 only guards against `err`, not empty content. `GetRecipe` returns `nil, nil` when the file doesn't exist. If the TOML file was deleted from disk but the `.meta.json` sidecar still exists (containing ETag/Last-Modified), the client would:
1. Get `cached = nil` (TOML file missing)
2. Still find `recipeMeta != nil` (sidecar exists)
3. Send conditional headers
4. Receive 304
5. Return `nil, nil` -- a silent data loss

The `cached != nil` check on line 183 does prevent returning nil, but the request still goes out with conditional headers that can't be fulfilled. If the server returns 304, the subsequent code falls through to the 404 and generic error checks, which would return a confusing `ErrRepoNotFound` or `ErrNetwork` for what is actually a local cache corruption.

**Fix**: Only send conditional headers when `cached != nil`. Move the conditional header block inside a `if cached != nil` guard.

### B2: Test for auth token isolation doesn't actually test the transport

**File**: `client_test.go:39-55`

The subtest "token sent to api.github.com" creates a request, overwrites the host twice, then ends with a comment "Let's test the logic more directly" -- and does nothing. This is a no-op test body. The "transport adds token for api.github.com hostname" subtest at line 57 manually reimplements the transport logic instead of calling `authTransport.RoundTrip`. Neither test actually exercises the real `authTransport.RoundTrip` method for the positive case (token IS sent to api.github.com).

The negative case at line 69 is correct -- it sends to the test server (127.0.0.1) and verifies no token leaks.

**Impact**: The critical security property (token sent to api.github.com) is asserted by duplicating logic, not by testing the actual code path. A refactoring bug in `authTransport.RoundTrip` would not be caught.

**Fix**: Create a test that calls `authTransport.RoundTrip` directly with a request whose `URL.Hostname()` is `api.github.com`, using a mock base transport that captures the request. Verify the Authorization header is present on the captured request.

---

## ADVISORY Observations

### A1: Cache TTL inconsistency with existing patterns

**File**: `cache.go:13`

`DefaultCacheTTL` is 1 hour, while `config.go` defines `DefaultRecipeCacheTTL` as 24 hours for the central registry cache. The distributed cache is fetching the same kind of artifact (recipe TOML files) but with a 24x shorter TTL. This may be intentional (distributed sources change more often than the curated central registry), but there's no comment explaining the rationale.

The acceptance criteria say "Separate CacheManager instance with independent size limits from central registry cache" which justifies independent settings, but doesn't explain the TTL choice.

**Suggestion**: Add a comment explaining the 1-hour TTL choice, or consider making it configurable via an environment variable like the existing `TSUKU_RECIPE_CACHE_TTL`.

### A2: No cache size limit enforcement

**File**: `cache.go`

The central registry cache has `DefaultRecipeCacheSizeLimit` (50MB) and `GetRecipeCacheSizeLimit()`. The distributed `CacheManager` has no size limit. Over time, registering many distributed sources could accumulate unbounded cache data. This is acceptable for an initial implementation since the per-recipe size is limited to 1MB (via `io.LimitReader` in `client.go:198`), but worth noting for future work.

### A3: Path traversal validation uses substring match instead of component check

**File**: `cache.go:53-57`

The traversal check uses `strings.Contains(owner, "..")`. This rejects `a..b` as well as `../etc`, which is overly strict. GitHub allows repository names containing `..` (e.g., `my..repo`). The test at `cache_test.go:151` shows `"a..b"` is rejected for recipe names too.

The `discover.ValidateGitHubURL()` is called in `ListRecipes` and handles real GitHub name validation. The cache-level check is a defense-in-depth layer, so being overly strict is acceptable -- but it means the two validation layers have different acceptance criteria for the same input.

### A4: `listViaContentsAPI` test doesn't exercise the actual method

**File**: `client_test.go:85-154`

`TestGitHubClient_ListRecipes_ContentsAPI` creates a test server and GitHubClient, but then bypasses the client entirely by making a direct HTTP request to the test server and manually re-implementing the parsing logic. The `gc` variable is created but never used (lines 109, 123-124). This tests the response format expectations and `validateDownloadURL`, but not the `listViaContentsAPI` method itself.

This is understandable -- `listViaContentsAPI` constructs a hardcoded `api.github.com` URL, making it difficult to redirect to a test server. However, the test name and scenario reference (scenario-14) suggest it validates the Contents API flow.

**Suggestion**: Either rename the test to `TestContentsAPIResponseParsing` to match what it actually tests, or refactor `listViaContentsAPI` to accept a base URL parameter (or use the test server URL with a custom transport).

### A5: Error types don't implement `Is()` for sentinel matching

**File**: `errors.go`

`ErrRepoNotFound`, `ErrNoRecipeDir`, `ErrRateLimited`, and `ErrInvalidDownloadURL` are struct types without `Is()` or `As()` methods. While `errors.As()` works for type matching, `errors.Is()` doesn't. This is fine for the current codebase usage (which uses type assertion `_, ok := apiErr.(*ErrRateLimited)`), but diverges from the Go idiom of sentinel errors for well-known conditions.

The existing codebase doesn't consistently use sentinel errors either, so this is consistent with project patterns.

### A6: `probeDefaultBranches` fetches stale cache it already has

**File**: `client.go:287-288`

In `ListRecipes`, when the code reaches `probeDefaultBranches` (line 133), it has already loaded `cached` at line 113. But `probeDefaultBranches` calls `gc.cache.GetSourceMeta(owner, repo)` again at line 288. This is a minor inefficiency -- the method could accept the already-loaded cache entry as a parameter. Not a bug, just a redundant disk read.

### A7: HTTP response body read after 304 check could be cleaner

**File**: `client.go:183-198`

After the 304 check, if the status is neither 304, 404, nor 200, the response body is never read before the function returns an error. The `defer resp.Body.Close()` at line 180 handles cleanup, but not reading the body means the connection can't be reused by the HTTP client pool. This is a minor performance concern for error paths.

---

## Pattern Consistency Assessment

### HTTP Security: GOOD
- Uses `httputil.NewSecureClient` for both clients (SSRF protection, HTTPS-only redirects, DNS rebinding guards)
- Download URL allowlist prevents SSRF via API response poisoning
- 1MB response limit prevents memory exhaustion
- Auth token isolated to api.github.com via `authTransport` with hostname check

### Token Handling: GOOD
- Uses `secrets.Get("github_token")` consistent with existing pattern
- Token absence is graceful (unauthenticated mode with lower rate limits)
- Rate limit errors guide users to set GITHUB_TOKEN

### Cache Layer: ACCEPTABLE
- Directory structure follows `{owner}/{repo}/` pattern, consistent with how `config.go` organizes under `$TSUKU_HOME/cache/`
- Path traversal protection present (overly strict but safe)
- Missing: cache directory not registered in `config.Config` struct (no `DistributedCacheDir` field alongside `TapCacheDir`, `KeyCacheDir`, etc.)

### Error Types: GOOD
- Clear domain-specific errors for each failure mode
- `ErrNetwork.Unwrap()` supports error chain inspection
- Error messages are actionable (rate limit includes guidance about GITHUB_TOKEN)

---

## Summary

| Category | Count |
|----------|-------|
| BLOCKING | 2 |
| ADVISORY | 7 |

The two blocking issues are:
1. A conditional-header/cache-miss interaction that could produce confusing errors on cache corruption
2. A test that claims to verify auth token isolation for the positive case but doesn't actually call the code under test

The implementation is solid on security fundamentals (SSRF, auth isolation, URL validation). The cache design is reasonable. Advisory items are mostly about test coverage gaps and minor inconsistencies with existing patterns.
