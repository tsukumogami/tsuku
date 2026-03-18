# Advocate: Progressive Extraction

## Approach Description

Progressive Extraction keeps the four existing provider types (`EmbeddedProvider`, `LocalProvider`, `CentralRegistryProvider`, `DistributedProvider`) as distinct structs but eliminates duplicated logic by extracting shared helpers. The providers become thin wrappers around:

1. A **shared `SatisfiesExtractor`** that all providers delegate to, replacing 3 near-identical `SatisfiesEntries()` implementations.
2. A **unified cache abstraction** that both `registry.CacheManager` and `distributed.CacheManager` implement, normalizing TTL checks, eviction, and metadata handling.
3. Shared **directory-scanning helpers** for listing TOML files and parsing descriptions.
4. A **manifest-aware cross-cutting concern** that providers can opt into, letting any registry declare its layout and index URL.

The loader's `GetFromSource()` switch statement and `warnIfShadows()` type assertions remain but shrink as providers expose behavior through interfaces rather than concrete types. No new provider type is introduced; the existing four just share more code.

## Investigation

### SatisfiesEntries duplication

Three providers implement `SatisfiesEntries()` with near-identical logic:

- **`EmbeddedProvider.SatisfiesEntries()`** (provider_embedded.go:44-69): Iterates all embedded recipes, parses each with `toml.Unmarshal`, extracts `Satisfies` map, builds `map[string]string` with duplicate warnings.
- **`LocalProvider.SatisfiesEntries()`** (provider_local.go:84-127): Same algorithm but reads from disk via `os.ReadDir` + `os.ReadFile`. Has identical inner loop extracting satisfies entries with the same duplicate-warning pattern.
- **`CentralRegistryProvider.SatisfiesEntries()`** (provider_registry.go:82-103): Different data source (cached manifest JSON rather than TOML), but produces the same `map[string]string` output with the same dedup logic.

The core satisfies-extraction loop (lines ~55-65 in embedded, ~114-123 in local) is copy-pasted. A shared function `ExtractSatisfies(satisfiesMap map[string][]string, recipeName string, result map[string]string)` would eliminate this. The embedded and local providers would still need their own iteration code to get recipe bytes, but the inner loop collapses.

**Extractable**: ~15 lines per provider -> 1 shared function + 3 callers of ~5 lines each. Net savings: ~30 lines.

### firstLetter bucketing duplication

Two independent implementations exist:

- **`registry/cache.go:39`**: `firstLetter()` function with `_` fallback for non-alpha.
- **`registry/registry.go:85-86, 94-95`**: Inline `strings.ToLower(string(name[0]))` without the `_` fallback.

The `registry.go` versions lack the edge-case handling of `cache.go`. This is a minor bug waiting to happen. Extracting to a single package-level function is trivial.

**Extractable**: Move `firstLetter()` to a shared location, update 3 call sites. ~5 minutes of work.

### Cache layer duplication

Two completely separate cache systems:

| Aspect | `registry.CacheManager` | `distributed.CacheManager` |
|--------|------------------------|---------------------------|
| Location | `internal/registry/cache_manager.go` | `internal/distributed/cache.go` |
| Structure | `{letter}/{name}.toml` + `.meta.json` | `{owner}/{repo}/{name}.toml` + `.meta.json` |
| TTL | 24h default | 1h default |
| Size limit | Configurable with high/low water mark | 20MB with oldest-repo eviction |
| Freshness | `CacheMetadata.CachedAt + TTL` | `RecipeMeta.FetchedAt + TTL` |
| Eviction | LRU by `LastAccess` | Oldest repo directory |

Despite different directory layouts, both caches do the same thing: store TOML files with JSON metadata sidecars, check TTL-based freshness, and evict when size limits are exceeded. A shared `RecipeCache` interface could unify freshness-checking and size-management while allowing different directory layouts.

However, the metadata schemas differ (`CacheMetadata` has `ContentHash`, `ExpiresAt`, `LastAccess`; `RecipeMeta` has `ETag`, `LastModified`). Unifying these requires either a superset struct or an interface. The superset approach is simpler.

**Extractable with effort**: A `CacheStore` interface with `Get(key)`, `Put(key, data, meta)`, `IsFresh(key)`, `Size()` methods. Both caches implement it. The distributed cache adds owner/repo key namespacing. Estimated: ~150 lines of new interface + adapter code, replacing ~200 lines of duplicated logic.

### GetFromSource switch statement

`loader.go:185-248` has a 60-line switch on source type with per-type provider iteration patterns. The `"central"` case has special fallback from registry to embedded (13 lines). The `"local"` case is 10 lines. The `default` distributed case is 15 lines.

Under Progressive Extraction, this doesn't fully collapse because each source type has genuinely different resolution behavior (central falls through to embedded, distributed matches by owner/repo string). But the repetitive "iterate providers, find matching source, call Get" pattern (~8 lines repeated 3 times) can be extracted to a `findProvider(source)` helper.

**Partially extractable**: ~20 lines saved, switch statement shrinks but doesn't disappear.

### Type assertions in loader.go

Five type assertion sites in loader.go:

1. **Line 359**: `p.(*EmbeddedProvider)` in `warnIfShadows` -- checks if embedded has recipe.
2. **Line 366**: `p.(*CentralRegistryProvider)` in `warnIfShadows` -- checks registry cache.
3. **Line 569**: `p.(*LocalProvider)` in `RecipesDir()` -- gets directory path.
4. **Line 579**: `p.(*LocalProvider)` in `SetRecipesDir()` -- replaces provider.
5. **update_registry.go:38**: `p.(*CentralRegistryProvider)` -- accesses underlying Registry.

Progressive Extraction can reduce these by adding methods to the `RecipeProvider` interface or creating optional interfaces. For example:
- `HasRecipe(name string) bool` interface eliminates assertions #1 and #2.
- `ProviderDir() string` method or `DirProvider` interface eliminates #3.
- #4 and #5 are legitimate escape hatches that are hard to generalize without over-engineering.

**Partially extractable**: 3 of 5 assertions can be replaced with interfaces. 2 remain as documented escape hatches (which the code already acknowledges via `ProviderBySource()`).

### HTTP client duplication

- `registry/registry.go`: Creates its own `newRegistryHTTPClient()` with custom transport settings.
- `distributed/client.go`: Uses `httputil.NewSecureClient()` with auth transport wrapping.

These aren't identical, but they're both building hardened HTTP clients for GitHub content. The auth-transport pattern in distributed could be reused if the central registry ever needs authentication. For now this is not a high-value extraction target.

**Low priority**: Different enough that unifying adds complexity without clear benefit.

## Strengths

- **Minimal blast radius**: Each extraction is a focused refactoring that touches 2-4 files. A bug in the `SatisfiesExtractor` helper doesn't break the cache layer. Changes can be reviewed and tested in isolation.

- **No migration path needed**: Since provider types remain, all existing code that references `*CentralRegistryProvider` or `*LocalProvider` continues to compile. The `update_registry.go` escape hatch (line 38) works unchanged.

- **Incremental delivery**: Can be shipped as 3-4 independent PRs (satisfies helper, cache interface, firstLetter fix, GetFromSource cleanup). Each PR is independently valuable and independently revertable.

- **Low risk of over-abstraction**: The providers genuinely differ in their backing stores (in-memory map, filesystem, HTTP+cache, GitHub API+cache). Keeping separate types acknowledges this reality. Shared helpers eliminate copy-paste without forcing a single abstraction that papers over real differences.

- **Compatible with future unification**: If a later architectural change replaces all providers with a single `UnifiedProvider`, the extracted helpers become the foundation. Progressive Extraction is a stepping stone, not a dead end.

- **Manifest support integrates naturally**: Adding manifest awareness (layout declaration, index URL) as a cross-cutting concern that providers read from their backing store fits the "providers remain separate, share helpers" model. Each provider type decides how to obtain its manifest (embedded: compiled-in, local: read from disk, central: fetch+cache, distributed: GitHub API).

## Weaknesses

- **Doesn't eliminate the GetFromSource switch**: The per-source-type branching in `GetFromSource()` shrinks but persists. Adding a new source type still requires modifying this switch. The Unified Provider approach would eliminate it entirely.

- **Type assertions survive**: 2 of 5 type assertion sites remain. These are documented escape hatches but still represent coupling to concrete types that a unified approach would remove.

- **Two cache systems persist**: Even with a shared interface, the registry and distributed caches remain separate implementations with different directory layouts, TTL defaults, and eviction strategies. Operational complexity (cache inspection, debugging) stays roughly the same.

- **Doesn't address the loader's source-tracking complexity**: The `recipeSources` map, `SourceCentral` vs `SourceRegistry` vs `SourceEmbedded` distinction, and the "central = registry or embedded" mapping remain untouched. This is a source of confusion that Progressive Extraction doesn't simplify.

- **Testing burden increases modestly**: Extracted helpers need their own unit tests. The total test code grows even as production code shrinks, because the helpers need edge-case coverage that was previously tested implicitly through provider tests.

## Deal-Breaker Risks

None identified. The approach is inherently low-risk because it only extracts existing logic without changing behavior. Each extraction step can be validated by running the existing test suite (`go test ./...`). The worst-case outcome is "not enough value to justify the effort," not "breaks something fundamental."

The one scenario that could make Progressive Extraction a poor choice is if the registry unification design requires all providers to become a single code path within a single PR (e.g., for manifest-based routing where the manifest determines which providers exist). In that case, Progressive Extraction would be wasted intermediate work. But given that breaking changes are acceptable and there are no users yet, a staged approach is not inherently worse than a big-bang rewrite -- it's just slower to reach the end state.

## Implementation Complexity

- **Files to modify**: 8-10
  - `internal/recipe/provider_embedded.go` (delegate to shared satisfies helper)
  - `internal/recipe/provider_local.go` (delegate to shared satisfies helper)
  - `internal/recipe/provider_registry.go` (delegate to shared satisfies helper)
  - `internal/recipe/provider.go` (add optional interfaces: `HasChecker`, `DirProvider`)
  - `internal/recipe/loader.go` (use new interfaces, extract findProvider helper)
  - `internal/recipe/satisfies.go` (new: shared satisfies extraction function)
  - `internal/registry/cache.go` (export `firstLetter` or move to shared package)
  - `internal/registry/registry.go` (use shared `firstLetter`)
  - Optionally: `internal/cache/store.go` (new: shared cache interface, if pursuing cache unification)

- **New infrastructure**: Minimal
  - 1 new file for shared satisfies helper (~30 lines)
  - 1-2 new optional interfaces on `RecipeProvider` (~10 lines)
  - Optionally 1 new package `internal/cache` for shared cache interface (~100 lines)

- **Estimated scope**: Small to medium
  - Without cache unification: small (3-4 focused PRs, ~2 days of work)
  - With cache unification: medium (add 1-2 more PRs, ~4 days total)

## Summary

Progressive Extraction is the conservative play: it eliminates the most visible duplication (SatisfiesEntries parsing, firstLetter bucketing, provider-finding loops) through focused helper extraction without changing the system's provider-based architecture. It's low-risk, incrementally deliverable, and compatible with a later architectural unification if one proves necessary. The tradeoff is that it doesn't fully resolve the structural complexity -- the GetFromSource switch, dual cache systems, and source-tracking confusion all persist in reduced form. For a codebase with no external users and acceptable breaking changes, this approach may be doing less than it could, but it won't make anything worse.
