# Maintainer Review: Issue 5 -- Distributed Package HTTP Fetching and Cache

**Reviewer focus**: Can the next developer understand and modify this code with confidence?

## Overall

The code is well-structured. Naming is accurate, error types are specific and actionable, the cache layer has clean separation from the HTTP client, and the fallback strategy (API -> stale cache -> branch probing) is easy to follow. The two-client design (authenticated API client vs. unauthenticated raw client) is clearly motivated and correctly implemented.

---

## Findings

### 1. `GetSourceMeta` returns `nil, nil` -- callers must know the convention

`internal/distributed/cache.go:82` -- `GetSourceMeta` returns `(nil, nil)` for a cache miss. The doc comment says so, but callers in `client.go:113-114` rely on this: `if err == nil && gc.cache.IsSourceFresh(cached)` works only because `IsSourceFresh(nil)` returns `false`. The same pattern repeats for `GetRecipe` and `GetRecipeMeta`.

The next developer adding a new caller will likely write `if cached != nil` without checking `err`, or check `err == nil` and assume `cached` is non-nil. The `nil, nil` return is a Go convention for "not found" in cache-like APIs, but three separate methods all using it with interlocking nil checks creates a subtle contract.

**Advisory.** The pattern works and is documented, but a `CacheMiss` sentinel error would make the call sites more explicit. Low risk since the existing callers all handle it correctly and the tests cover it.

### 2. Magic 5-minute TTL for incomplete source metadata

`internal/distributed/cache.go:131` -- The incomplete TTL of `5 * time.Minute` is hardcoded with no named constant. The main TTL is configurable and has a well-named `DefaultCacheTTL` constant, but this secondary TTL is invisible unless you read `IsSourceFresh`. A developer changing `DefaultCacheTTL` won't know there's a second, shorter TTL hiding inside the freshness check.

**Advisory.** Extract to a named constant like `incompleteCacheTTL` with a comment explaining why 5 minutes (enough time for rate limits to partially reset).

### 3. `ErrNetwork` used for non-network errors

`internal/distributed/client.go:259` -- A JSON parse failure from `json.Unmarshal` is wrapped in `ErrNetwork{Operation: "parsing API response"}`. JSON parsing is not a network error; the next developer debugging a malformed API response will see `ErrNetwork` and look at network configuration, not at the response body.

Similarly, line 196: `fmt.Errorf("unexpected status %d")` wrapped in `ErrNetwork` -- an HTTP 500 is arguably a server error, not a network error. The name `ErrNetwork` implies transport-level failures (DNS, timeouts, connection refused), not application-level HTTP status codes or parse failures.

**Blocking.** A developer matching on `*ErrNetwork` to decide whether to retry will catch parse errors too. Rename to `ErrRemote` or `ErrRequest`, or split `ErrNetwork` (transport) from `ErrAPIResponse` (unexpected status/parse failure).

### 4. Test `TestGitHubClient_ListRecipes_ContentsAPI` doesn't test what it claims

`internal/distributed/client_test.go:79` -- The test name says it tests `ListRecipes` via the Contents API, but it never calls `ListRecipes` or `listViaContentsAPI`. It manually replicates the parsing logic from `listViaContentsAPI` inline (lines 128-137), then asserts on its own copy. If the real parsing logic changes, this test will still pass.

Lines 117-118 (`_ = ctx; _ = gc`) confirm the test was originally intended to call the client but the author couldn't route the request to the test server due to URL construction. The `gc` variable is created and never used.

**Blocking.** The next developer will trust this test as coverage for the Contents API path and won't write a replacement. Either refactor `listViaContentsAPI` to accept a base URL (making it testable) or rename the test to `TestContentsAPIResponseParsing` so its actual scope is clear.

### 5. `TestGitHubClient_FetchRecipe_CacheLifecycle` tests cache directly, not `FetchRecipe`

`internal/distributed/client_test.go:213` -- Named as a `FetchRecipe` cache lifecycle test (scenario-16), but it calls `cache.GetRecipe` and `cache.PutRecipe` directly, never testing the actual cache-hit path through `FetchRecipe`. The only `FetchRecipe` call (line 254) tests URL validation rejection, not caching.

The test server, `downloadURL`, `callCount`, and `gc` are all created but unused (`_ = ts; _ = downloadURL`), suggesting the same testability gap as finding #4.

**Blocking.** Same reasoning -- the test name claims coverage that doesn't exist. The cache round-trip is already covered in `cache_test.go`. This test should either exercise `FetchRecipe`'s cache integration or be renamed to avoid giving false confidence.

### 6. Duplicated path-traversal validation

`internal/distributed/cache.go:72-74` (in `repoDir`) and `cache.go:145` (in `GetRecipe`) and `cache.go:166` (in `PutRecipe`) and `cache.go:214` (in `GetRecipeMeta`) -- The recipe name sanitization check (`strings.Contains(name, "..")`) is copy-pasted in three methods. The `repoDir` check handles owner/repo, but recipe name validation has no shared helper.

If someone adds a `DeleteRecipe` method, they must remember to copy the check. More importantly, the `".."` check in recipe names will reject legitimate names like `a..b` (two consecutive dots in a tool name), which may or may not be intentional -- a test (`TestCacheManager_InvalidRecipeName`) confirms `"a..b"` is rejected, but there's no comment explaining whether this is deliberate policy or an overly broad check.

**Advisory.** Extract a `validateRecipeName` helper. Add a comment explaining whether `a..b` is intentionally blocked or a side effect.

### 7. `FetchRecipe` returns `ErrRepoNotFound` on 404, but the 404 could be the file, not the repo

`internal/distributed/client.go:188-189` -- When `FetchRecipe` gets a 404 for a specific recipe download URL, it returns `ErrRepoNotFound`. But the caller already knows the repo exists (they got the URL from `ListRecipes`). The 404 means the file was deleted or the URL is stale, not that the repo is missing.

The next developer handling `ErrRepoNotFound` will think the entire repository is inaccessible and skip all recipes from that source, when actually only one file is gone.

**Blocking.** Use a different error type (e.g., `ErrRecipeNotFound{Owner, Repo, Name}`) or return the generic network error with the status code.

---

## Summary

| # | Location | Issue | Severity |
|---|----------|-------|----------|
| 1 | `cache.go:82` | `nil, nil` return convention for cache miss | Advisory |
| 2 | `cache.go:131` | Hardcoded 5-minute incomplete TTL | Advisory |
| 3 | `client.go:259` | `ErrNetwork` wraps parse/status errors | Blocking |
| 4 | `client_test.go:79` | Test replicates parsing logic instead of calling `ListRecipes` | Blocking |
| 5 | `client_test.go:213` | Test named for `FetchRecipe` cache lifecycle but tests cache directly | Blocking |
| 6 | `cache.go:145,166,214` | Duplicated recipe name validation, unclear `..` policy | Advisory |
| 7 | `client.go:188-189` | `FetchRecipe` 404 returns `ErrRepoNotFound` instead of recipe-level error | Blocking |
