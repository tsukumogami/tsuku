# Pragmatic Review: Issue 5 -- GitHub HTTP Fetching and Cache

**Focus:** Over-engineering, dead code, speculative generality, YAGNI violations.

---

## BLOCKING

### B1: `SetMaxSize` is speculative API with no production caller

**Location:** `cache.go:63-67`

`SetMaxSize` is exported, has zero callers outside `cache_test.go`, and `NewCacheManager` already hardcodes `DefaultMaxCacheSize`. The constructor doesn't accept `maxBytes` as a parameter either, so the only way to change it is this setter -- which nothing calls. Either add `maxBytes` to the constructor if it's needed now, or remove `SetMaxSize` and the field until a caller exists. The current shape is a dead public API that suggests configurability that doesn't exist.

**Fix:** Delete `SetMaxSize`. If a future issue needs configurable cache size, add it to `NewCacheManager` parameters at that point.

### B2: `DefaultMaxCacheSize` exported constant with no external consumer

**Location:** `cache.go:19`

Exported but only referenced internally (hardcoded in `NewCacheManager`). No code outside the `distributed` package reads this value. Unexport it or inline it.

**Fix:** Rename to `defaultMaxCacheSize`.

---

## ADVISORY

### A1: Duplicated path-traversal validation in cache methods

**Location:** `cache.go:145-147`, `cache.go:166-167`, `cache.go:214-215`

The recipe name sanitization check (`strings.Contains(name, "..")` etc.) is copy-pasted across `GetRecipe`, `PutRecipe`, and `GetRecipeMeta`. This isn't over-engineering to flag -- it's the opposite: three copies of the same guard invites drift. A `validateRecipeName(name)` helper would be justified here (three callers, identical logic, security-relevant).

### A2: `evictOldest` walks the entire cache tree twice per eviction

**Location:** `cache.go:191-193`, `cache.go:235-247`

`PutRecipe` calls `cm.Size()` (which walks the full tree) and then `evictOldest()` (which walks it again). For the current 20MB cap this is fine, but worth a comment noting it's O(n) per write. Not blocking because the cache is small and writes are infrequent.

### A3: Test `TestGitHubClient_ListRecipes_ContentsAPI` doesn't test `ListRecipes`

**Location:** `client_test.go:79-148`

Despite the name, this test never calls `gc.ListRecipes()`. It calls the test server directly, manually parses the response, and re-implements the filtering logic from `listViaContentsAPI`. The `gc` and `ctx` variables are created and then discarded (`_ = ctx; _ = gc`). This is dead test code that gives false confidence. Either make it actually test the client method (which requires making the API URL injectable) or rename it to `TestContentsAPIResponseParsing` and drop the unused client setup.

### A4: `TestGitHubClient_FetchRecipe_CacheLifecycle` mostly tests cache, not `FetchRecipe`

**Location:** `client_test.go:213-261`

Similar to A3: the test sets up a server and client but then only exercises `cache.GetRecipe` and `cache.PutRecipe` directly. The only `FetchRecipe` call tests URL rejection. The `ts` and `downloadURL` variables are unused (`_ = ts; _ = downloadURL`). The "scenario-16: Cache read/write/expiry lifecycle" claim in the comment is misleading -- expiry is never tested here.

---

## Not Flagged

- Error type hierarchy in `errors.go`: five types for five distinct failure modes, all used. Correctly scoped.
- `authTransport`: single-purpose RoundTripper wrapping is idiomatic Go, not over-abstraction.
- `extractBranchFromURL` helper: called once but encapsulates URL parsing with clear naming. Inlining would hurt readability. Pass.
- `probeDefaultBranches` / `tryBranches` split: `probeDefaultBranches` adds real logic (cached branch priority) before delegating. Not a thin wrapper.
- Two HTTP clients (api vs raw): justified by different auth requirements. Not speculative.
