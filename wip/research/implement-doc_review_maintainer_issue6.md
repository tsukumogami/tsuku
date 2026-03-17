---
focus: maintainer
issue: 6
blocking_count: 2
advisory_count: 4
---

# Maintainer Review: Issue 6 (DistributedProvider)

## Overall Assessment

The DistributedProvider itself is clean and easy to follow -- thin wrapper over GitHubClient, clear method signatures, good separation of concerns. The Loader integration for qualified names is well-designed with proper cache key isolation. Two findings create real misread risk for the next developer.

## Blocking Findings

### B1: `GetWithSource` silently returns wrong source for qualified names

`internal/recipe/loader.go:160-171` -- `GetWithSource` delegates to `Get(name, opts)` which correctly routes `"acme/tools:dist-tool"` to the distributed provider and caches it under the qualified key `"acme/tools:dist-tool"`. But then it looks up `l.recipeSources[name]` using the same qualified key. This works today.

The problem: `getFromDistributed` (line 126) stores `l.recipeSources[cacheKey]` where `cacheKey` is the full qualified name. But `GetWithSource` looks up `l.recipeSources[name]` where `name` is also the full qualified name. So the lookup works. However, if `!ok` (line 166), the fallback is `SourceRegistry` -- silently wrong for a distributed recipe. The next developer reading this fallback will think "this only applies to old pre-migration recipes" (as the comment says), but it also applies if `getEmbeddedOnly` is ever called with a qualified name (which it can't be today, but the fallback masks the real problem instead of failing loudly).

More concretely: `getEmbeddedOnly` does `l.recipes[name] = recipe` but does NOT populate `l.recipeSources[name]`. So any recipe loaded via `getEmbeddedOnly` followed by `GetWithSource` silently gets `SourceRegistry`. Today this is benign (embedded IS central), but the pattern of silent fallback to a wrong source will bite when someone adds a new code path that caches without tracking source. The fallback should return `SourceEmbedded` for embedded, or return an error for unknown sources, not silently default to registry.

### B2: `splitQualifiedName` accepts qualifiers containing colons

`internal/recipe/loader.go:138` -- `splitQualifiedName` uses `LastIndex(":")`, so `"acme/tools:sub:recipe"` parses as qualifier `"acme/tools:sub"`, recipe `"recipe"`. The test at `loader_test.go:1659` documents this as intended: `"multiple colons uses last"`.

The next developer implementing Issue 7 (install flow) will need to parse `"owner/repo:recipe@version"`. If they follow this function's pattern, `"acme/tools:my-tool@1.0"` would split as qualifier `"acme/tools:my-tool@1.0"` -- that's wrong, the recipe name contains the version suffix. The `LastIndex` strategy doesn't compose with `@version` suffixes. Issue 7 will need to strip `@version` before calling `splitQualifiedName`, but nothing in this code or its comments warns about that interaction.

The colon-in-qualifier case (`"acme/tools:sub"`) can never match a real GitHub provider (GitHub repo names can't contain colons), so it silently parses into an unmatchable qualifier, producing a confusing "no provider registered" error instead of a parse error. Rejecting colons in the qualifier segment (or using `strings.Index` for first colon) would be safer and would prevent the version-suffix composition problem.

## Advisory Findings

### A1: `ListRecipes` vs `ForceListRecipes` naming

`internal/distributed/client.go:107,147` -- `ForceListRecipes` is `ListRecipes` minus the cache freshness check. The `Force` prefix is clear enough, but the duplication of the fallback logic (rate limit -> stale cache -> branch probe) between the two methods is a divergent twin. `ForceListRecipes` has a slightly different fallback order: it checks stale cache AFTER the API call fails, while `ListRecipes` checks stale cache BEFORE the API call when it's fresh. If someone fixes a bug in one fallback path, they'll need to remember to fix the other. Consider extracting the shared rate-limit fallback into a helper.

### A2: Shared `GitHubClient` across providers not documented

`cmd/tsuku/main.go:88-93` -- All distributed providers from `userCfg.Registries` share a single `GitHubClient` instance. This is correct (one auth client, one rate limit budget), but it means the `CacheManager` is also shared. The `CacheManager.evictOldest()` could evict one provider's cache to make room for another's. This isn't a bug, but the next developer adding per-provider cache configuration would need to understand that the client is shared. A brief comment on the sharing would help.

### A3: `TestDistributedProvider_Get_FetchesFromServer` doesn't test what it claims

`internal/distributed/provider_test.go:112-166` -- The test name says "fetches from server" but the test comment (line 153-156) explains it actually tests that the provider wires through the client correctly by checking the *error type*. The test always fails with an allowlist error because the test server hostname isn't in `allowedDownloadHosts`. This makes the test a hostname-validation test, not a server-fetch test. The test name will mislead the next developer into thinking server fetching is covered when it isn't. Rename to something like `TestDistributedProvider_Get_ValidatesDownloadHost` or add a comment at the function level.

### A4: Magic number for incomplete cache TTL

`internal/distributed/cache.go:132` -- `IsSourceFresh` uses a hardcoded `5 * time.Minute` for incomplete entries, separate from the configurable `cm.ttl`. The 5-minute choice makes sense (short enough to retry the full listing once rate limits ease), but the next developer tuning cache behavior might change `DefaultCacheTTL` and not realize this separate TTL exists. Consider making it a named constant like `incompleteCacheTTL` with a comment explaining why it differs from the main TTL.
