# Intent Review: Issue 5 -- GitHub HTTP Fetching and Cache

**Focus:** Does the implementation serve the broader design goals from the design doc, and does it set up Issues 6, 7, and beyond for success?

## Summary

The implementation in `internal/distributed/` (client.go, cache.go, errors.go) aligns well with the design doc's Decisions 3 and 6. The two-tier HTTP strategy, separate auth/unauth clients, hostname allowlist, and per-repo cache directory layout all match the design spec. The API surface is consumable by Issue 6's DistributedProvider, and the error types cover the install flow's needs.

Two blocking issues and four advisory observations follow.

---

## BLOCKING Issues

### B1: FetchRecipe cache hit path skips freshness check

**Location:** `client.go:153-155`

```go
cached, err := gc.cache.GetRecipe(owner, repo, name)
if err == nil && cached != nil {
    return cached, nil
}
```

`FetchRecipe` returns cached recipe content if the bytes exist on disk, with no TTL or freshness check. Contrast with `ListRecipes` which calls `gc.cache.IsSourceFresh(cached)`. A recipe cached from a previous session will be served indefinitely, even if the source repo has been updated.

The design doc specifies conditional requests using ETag/If-Modified-Since (which the code does implement for the download path), but the early return on line 153-155 short-circuits before those conditional headers are ever sent. The `RecipeMeta` with ETag data is fetched on line 166-174 but only reached if the cache GetRecipe returns nil.

**Impact on Issue 6:** The DistributedProvider's `Get()` will silently return stale recipes. The `RefreshableProvider.Refresh()` interface (Issue 6 acceptance criteria) needs to be able to force re-validation, but there's no mechanism to bypass this cache hit.

**Suggested fix:** Either add a TTL check similar to `IsSourceFresh` for individual recipes, or always proceed to the conditional request path and only return early on a 304. The conditional headers already protect against unnecessary re-downloads.

### B2: probeDefaultBranches returns SourceMeta with nil Files map

**Location:** `client.go:323-325`

```go
return &SourceMeta{
    Branch:    branch,
    Files:     nil, // Can't list via raw URLs; will populate on next successful API call
```

When the Contents API is rate-limited on a cold cache, the fallback returns a `SourceMeta` with `Branch` set but `Files` as nil. This gets cached via `PutSourceMeta` (line 138). On the next call, `IsSourceFresh` returns true for this nil-Files entry, meaning `ListRecipes` returns a `SourceMeta` where the caller has no recipe names or download URLs.

**Impact on Issue 6:** `DistributedProvider.List()` needs to return recipe names from `SourceMeta.Files`. A nil map means an empty listing for up to 1 hour (the cache TTL). `DistributedProvider.Get()` can't look up a download URL either. The provider will need defensive nil checks and possibly its own re-fetch logic, which undermines the clean separation between the HTTP layer (Issue 5) and the provider layer (Issue 6).

**Suggested fix:** Either (a) mark the probed SourceMeta with a flag indicating it's incomplete so `IsSourceFresh` treats it differently, or (b) use a shorter TTL for probe-only results.

---

## ADVISORY Observations

### A1: No `IsRecipeFresh` method on CacheManager

The cache has `IsSourceFresh` for directory listings but no equivalent for individual recipes. Issue 6's provider will need to decide when to re-fetch a recipe. Currently that logic is embedded in `FetchRecipe` (the conditional request path), but as noted in B1, it's unreachable when cache bytes exist. Adding an `IsRecipeFresh(*RecipeMeta) bool` method to `CacheManager` would give the provider explicit freshness control and fix B1 at the same time.

### A2: No cache eviction or size limit mechanism

The design doc mentions "independent size limits" for the distributed cache (Decision 6). The current `CacheManager` has no eviction logic. This is fine for v1 since the number of distributed sources will be small, but the API should accommodate it later. The current interface doesn't expose a `Size()` or `Prune()` method. Not blocking because the design doc's implementation approach (Phase 4) lists this under the distributed provider deliverables, not the cache layer, and the number of cached repos in early usage will be trivially small.

### A3: ListRecipes validates with `owner + "/" + repo` concatenation

**Location:** `client.go:108`

```go
if err := discover.ValidateGitHubURL(owner + "/" + repo); err != nil {
```

This works because `ValidateGitHubURL` accepts `owner/repo` format. But if the caller passes an owner or repo with characters that happen to form a valid combined string but are individually invalid (unlikely given the regex, but fragile), this could pass validation. A cleaner approach would validate owner and repo separately. Not blocking since the regex in `ValidateGitHubURL` handles the known attack vectors (path traversal, credentials), and the cache's `repoDir` method has its own traversal checks.

### A4: Test coverage for FetchRecipe is indirect

`TestGitHubClient_FetchRecipe_CacheLifecycle` tests the cache layer directly and URL validation, but doesn't exercise the actual `FetchRecipe` download path (conditional headers, 304 handling, 1MB limit, ETag caching) because test server URLs fail the hostname allowlist. This means the integration between FetchRecipe and a real HTTP response is untested. Issue 6 tests could cover this with an injectable URL validator or by adding allowlist entries for test use, but this gap means bugs in the download path won't surface until integration testing.

---

## Intent Alignment Assessment

### Does GitHubClient's API surface work for Issue 6?

**Yes, with the B1/B2 fixes.** The two methods Issue 6 needs are:
- `ListRecipes(ctx, owner, repo) -> *SourceMeta` -- maps to `DistributedProvider.List()`
- `FetchRecipe(ctx, owner, repo, name, downloadURL) -> []byte` -- maps to `DistributedProvider.Get()`

The `SourceMeta.Files` map provides the recipe-name-to-download-URL mapping that bridges listing to fetching. The separation is clean.

### Does CacheManager's API fit?

**Yes.** The provider doesn't need to call cache methods directly -- they're encapsulated inside `GitHubClient`. The `NewCacheManager(baseDir, ttl)` constructor lets the provider control cache location and TTL independently from the central registry.

### Are error types sufficient for Issue 7?

**Yes.** The install flow needs to distinguish:
- User error (bad repo name) -- covered by `ValidateGitHubURL` errors
- Repo/recipe not found -- `ErrRepoNotFound`, `ErrNoRecipeDir`
- Transient failure -- `ErrRateLimited`, `ErrNetwork`
- Security violation -- `ErrInvalidDownloadURL`

Issue 7's confirmation prompt flow can use type assertions on these to decide whether to show "not found" vs "try again later" vs "set GITHUB_TOKEN" guidance.

### Is the code organized for future issues?

**Yes.** The package boundary is clean. `client.go` handles HTTP, `cache.go` handles disk persistence, `errors.go` defines the error taxonomy. Issue 6 adds `provider.go` to this package without modifying the existing files. The `newGitHubClientWithHTTP` constructor enables test injection.
