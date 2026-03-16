# Phase 3 Research: Interface Design & Loader Integration

## Questions Investigated
1. What methods touch the priority chain in `loader.go`, and what minimal interface collapses them into one?
2. How close is `EmbeddedRegistry` to the target interface?
3. How should the satisfies index work with providers?
4. How does `RequireEmbedded` work, and can it be modeled as a filter on the provider chain?
5. Is the in-memory cache a provider or a Loader-level concern?
6. How does `ListAllWithSource()` work, and how does listing change with providers?
7. How should `loader.Registry()` and `update-registry` work after the refactor?

## Findings

### 1. Priority chain methods in loader.go

Four methods implement variations of the priority chain:

| Method | Cache | Local | Embedded | Registry | Satisfies fallback |
|--------|-------|-------|----------|----------|--------------------|
| `GetWithContext` (L87-143) | yes | yes | yes | yes | yes (calls `loadDirect`) |
| `loadDirect` (L187-226) | yes | yes | yes | yes | no |
| `getEmbeddedOnly` (L147-181) | yes | no | yes | no | embedded-only variant |
| `loadEmbeddedDirect` (L231-252) | yes | no | yes | no | no |

The satisfies fallback is a recursion guard: `GetWithContext` may resolve a name through the satisfies index and then calls `loadDirect` (the same chain minus satisfies) to avoid infinite loops. `getEmbeddedOnly` does the same with `loadEmbeddedDirect`.

**Minimal interface to collapse these:**

```go
type RecipeProvider interface {
    // Get returns raw TOML bytes for a named recipe, or ErrNotFound.
    Get(ctx context.Context, name string) ([]byte, error)

    // List returns metadata for all recipes this provider can serve.
    List(ctx context.Context) ([]RecipeInfo, error)

    // Source returns the source tag for attribution in listings.
    Source() RecipeSource
}
```

The four methods collapse into a single `resolveFromChain(ctx, providers, name)` function that iterates an ordered slice of providers. The satisfies fallback lives in the Loader, not in the providers: after the chain returns ErrNotFound, the Loader consults the satisfies index and retries with the canonical name (with a `noSatisfies` flag to prevent recursion).

`RequireEmbedded` becomes: pass only the embedded provider(s) to `resolveFromChain`.

### 2. EmbeddedRegistry's existing methods

`EmbeddedRegistry` (embedded.go) has:
- `Get(name string) ([]byte, bool)` -- returns raw TOML bytes
- `Has(name string) bool`
- `List() []string` -- bare names
- `ListWithInfo() ([]RecipeInfo, error)` -- names + descriptions + source tag
- `Count() int`

This is very close to `RecipeProvider`. Differences:
- `Get` returns `([]byte, bool)` instead of `([]byte, error)` -- trivial adapter
- `Get` doesn't take `context.Context` -- no network IO, so context is unused but easy to add for interface compliance
- `ListWithInfo` already returns `[]RecipeInfo` with `Source: SourceEmbedded` set

An `EmbeddedProvider` wrapping `EmbeddedRegistry` is approximately 20 lines of adapter code. Alternatively, `EmbeddedRegistry` itself could grow the interface methods directly.

### 3. Satisfies index and providers

`buildSatisfiesIndex()` (L370-418) uses two strategies:

1. **Embedded**: full TOML parse of every embedded recipe, extracting `r.Metadata.Satisfies`
2. **Registry manifest**: reads `ManifestRecipe.Satisfies` from the cached JSON manifest -- no TOML parsing

These strategies exist for performance: parsing ~1500 TOML recipes on every startup would be expensive, so the manifest provides a pre-computed shortcut.

**Options for the provider interface:**

**Option A: Optional `SatisfiesEntries` method.** Add an optional interface:

```go
type SatisfiesProvider interface {
    // SatisfiesEntries returns package_name -> recipe_name mappings.
    // Providers that can return this cheaply (manifest, embedded scan) implement it.
    SatisfiesEntries(ctx context.Context) (map[string]string, error)
}
```

The Loader checks `if sp, ok := provider.(SatisfiesProvider); ok { ... }` when building the index. Providers without it are skipped (their recipes are only findable by exact name). This preserves the two-strategy approach: `EmbeddedProvider` does full parse, `RegistryProvider` reads its manifest.

**Option B: Separate indexing mechanism.** Keep satisfies index building as a Loader concern. The Loader iterates providers, calling `List()` on embedded-like providers and consulting a manifest for registry-like providers. This is messier because the Loader needs to know provider internals.

**Recommendation: Option A.** It keeps provider-specific optimization internal to the provider, which is the whole point of the interface. Distributed providers could pre-compute satisfies entries in their own manifests.

### 4. RequireEmbedded as a chain filter

`RequireEmbedded` (L88-91) short-circuits `GetWithContext` to call `getEmbeddedOnly`, which only checks cache + embedded + embedded-only satisfies.

With providers, this becomes a filter on which providers participate in resolution. Two approaches:

**Tag-based filtering:** Each provider has a tag (e.g., `Source() RecipeSource`). When `RequireEmbedded` is set, the Loader filters to only providers where `Source() == SourceEmbedded`.

**Explicit provider subset:** The Loader holds a named reference to the embedded provider and constructs a single-element chain for `RequireEmbedded` lookups.

**Recommendation: Tag-based.** It generalizes better. Future flags like `RequireLocal` or `RequireOffline` become tag filters without new code paths. The `Source()` method already exists in the proposed interface.

However, the satisfies fallback also needs filtering: `lookupSatisfiesEmbeddedOnly` (L431-442) verifies the canonical recipe exists in embedded FS before returning it. With the tag-based approach, the Loader would filter the satisfies index to entries contributed by embedded-tagged providers. This requires the satisfies index to track which provider contributed each entry -- a `map[string]satisfiesEntry` where `satisfiesEntry` has both `recipeName` and `source RecipeSource`.

### 5. In-memory cache: provider or Loader concern?

The cache (`l.recipes map[string]*Recipe`) stores **parsed** `*Recipe` objects, not raw bytes. Providers return raw `[]byte` (TOML). The cache sits above the provider layer: it caches the result of `provider.Get()` + `parseBytes()`.

It should remain a Loader-level concern, not a provider:
- It stores parsed recipes, not raw data
- It's shared across all providers (a recipe cached from any source serves future lookups)
- It handles the `CacheRecipe()` / `ClearCache()` API for callers

The flow becomes: `Loader.GetWithContext` checks `l.recipes` first, then iterates providers, then parses, then caches. Same as today but with the provider loop replacing the hardcoded conditionals.

### 6. ListAllWithSource and RecipeInfo

`ListAllWithSource()` (L474-514) iterates three sources in priority order, deduplicating by name:
1. `listLocalRecipes()` -- scans `recipesDir` directory, parses each TOML for description
2. `embedded.ListWithInfo()` -- iterates embedded map, parses each for description
3. `listRegistryRecipes()` -- calls `registry.ListCached()`, parses each for description

With providers, this becomes:

```go
func (l *Loader) ListAllWithSource() ([]RecipeInfo, error) {
    seen := make(map[string]bool)
    var result []RecipeInfo
    for _, p := range l.providers {
        infos, err := p.List(ctx)
        if err != nil { return nil, err }
        for _, info := range infos {
            if !seen[info.Name] {
                seen[info.Name] = true
                result = append(result, info)
            }
        }
    }
    return result, nil
}
```

The provider ordering (local first, embedded second, registry third) handles the deduplication priority naturally. Each provider's `List` returns `RecipeInfo` with the correct `Source` tag.

Note: `ListLocal()` and `ListEmbedded()` (L517-527) are convenience methods that filter by source. With providers, these become `ListBySource(SourceLocal)` or similar.

### 7. loader.Registry() and update-registry

`loader.Registry()` (L344-346) returns `*registry.Registry` directly. Three call sites:

1. `cmd/tsuku/update_registry.go:32` -- creates `CachedRegistry` for TTL refresh
2. `cmd/tsuku/update_registry.go:70` -- `cachedReg.Registry().ListCached()` for dry-run
3. `cmd/tsuku/update_registry.go:132` -- same for refresh-all

The `update-registry` command needs operations that don't belong on `RecipeProvider`:
- Cache TTL checking (`GetCacheStatus`)
- Forced refresh (`Refresh`, `RefreshAll`)
- Manifest fetching (`FetchManifest`)
- Cache clearing (`ClearCache`)

**Approach:** Keep `Registry()` as a method but source it from the provider chain. The Loader can expose `CentralRegistry()` that type-asserts or searches its providers for the central registry provider, returning its inner `*registry.Registry`. Alternatively, `update-registry` could hold its own reference to the `*registry.Registry` at construction time (it already gets it from `loader.Registry()` at line 32).

The cleanest approach: the `RegistryProvider` (wrapper around `*registry.Registry`) exposes a `Registry() *registry.Registry` method. The Loader provides a `ProviderBySource(source RecipeSource) RecipeProvider` method. Callers that need cache operations type-assert:

```go
if rp, ok := loader.ProviderBySource(SourceRegistry).(*RegistryProvider); ok {
    reg := rp.Registry()
    cachedReg := registry.NewCachedRegistry(reg, ttl)
    // ...
}
```

This is mildly ugly but keeps the interface clean and limits the coupling to one command that genuinely needs registry internals.

## Proposed Interface Definition

```go
// RecipeProvider is a source of recipe TOML data.
// Providers are ordered by priority in the Loader's chain;
// earlier providers shadow later ones for the same recipe name.
type RecipeProvider interface {
    // Get returns raw TOML bytes for the named recipe.
    // Returns ErrNotFound if the recipe doesn't exist in this provider.
    Get(ctx context.Context, name string) ([]byte, error)

    // List returns metadata for all recipes available from this provider.
    List(ctx context.Context) ([]RecipeInfo, error)

    // Source returns the source tag for this provider.
    Source() RecipeSource
}

// SatisfiesProvider is an optional interface for providers that can
// efficiently return satisfies-index entries without full recipe parsing.
type SatisfiesProvider interface {
    // SatisfiesEntries returns package_name -> recipe_name mappings
    // from this provider's recipes. Used to build the Loader's
    // satisfies fallback index.
    SatisfiesEntries(ctx context.Context) (map[string]string, error)
}

// RefreshableProvider is an optional interface for providers whose
// cached data can be refreshed from an upstream source.
type RefreshableProvider interface {
    // Refresh re-fetches cached data from upstream.
    Refresh(ctx context.Context) error
}
```

**Concrete providers:**

| Provider | Source tag | SatisfiesProvider | RefreshableProvider | Notes |
|----------|-----------|-------------------|---------------------|-------|
| `LocalProvider` | `SourceLocal` | yes (full parse) | no | Wraps `recipesDir` path |
| `EmbeddedProvider` | `SourceEmbedded` | yes (full parse) | no | Wraps `EmbeddedRegistry` |
| `CentralRegistryProvider` | `SourceRegistry` | yes (manifest) | yes | Wraps `*registry.Registry` |
| `DistributedProvider` | `SourceDistributed` (new) | yes (own manifest) | yes | Future: third-party registries |

**Loader changes:**

```go
type Loader struct {
    providers        []RecipeProvider       // Ordered by priority
    recipes          map[string]*Recipe     // In-memory parsed cache
    constraintLookup ConstraintLookup
    satisfiesIndex   map[string]satisfiesEntry
    satisfiesOnce    sync.Once
}

type satisfiesEntry struct {
    recipeName string
    source     RecipeSource // Tracks which provider contributed this entry
}
```

## Implications for Design

1. **The refactor is incremental.** Each existing source becomes a provider one at a time. The Loader can hold both old fields and providers during migration, with old fields removed once all three are converted.

2. **Satisfies needs source tracking.** The `RequireEmbedded` satisfies filter means the index must know which provider contributed each entry. This is a small addition to the index data structure.

3. **`update-registry` is the messiest caller.** It needs cache-level operations that live below the provider interface. The type-assertion approach works but should be well-documented as an intentional escape hatch, not a pattern to copy.

4. **`warnIfShadows` becomes simpler.** Today it checks embedded and registry separately. With providers, shadowing detection is implicit: if a recipe is found by an earlier provider that shadows a later one, the Loader can check if later providers also have it.

5. **Three constructor variants collapse.** `New`, `NewWithLocalRecipes`, `NewWithoutEmbedded` become one `NewLoader(providers ...RecipeProvider)` or a builder pattern. Test code passes whatever providers it wants.

## Surprises

1. **Four methods, not three.** The phase 1 advocate report identified three chain methods (`GetWithContext`, `loadDirect`, `getEmbeddedOnly`). There's actually a fourth: `loadEmbeddedDirect` (L231-252), the non-recursive counterpart of `getEmbeddedOnly`. Both `loadDirect` and `loadEmbeddedDirect` exist purely as recursion guards for the satisfies fallback. With the provider refactor, all four collapse into one `resolveFromChain` function with a boolean `noSatisfies` parameter.

2. **The satisfies index builds eagerly for embedded but lazily overall.** `buildSatisfiesIndex` is called via `sync.Once` on first satisfies lookup. Embedded recipes are fully parsed at this point (hundreds of TOML parses). This is acceptable today (~1600 embedded recipes) but could matter for distributed providers with thousands of recipes. The `SatisfiesProvider` optional interface lets distributed providers use manifests instead of full parsing.

3. **The cache stores parsed recipes, not raw bytes.** This means providers return bytes, the Loader parses and caches the result. A provider returning pre-parsed `*Recipe` objects would bypass validation and step analysis. Keeping parsing in the Loader is the right call.

4. **`ListAllWithSource` parses every recipe for its description.** Both `listLocalRecipes` and `listRegistryRecipes` parse each recipe just to extract the description field. This is O(n) full TOML parses. The provider `List` method should ideally avoid this -- local and registry providers could parse lazily or use a lighter metadata extraction.

## Summary

The `RecipeProvider` interface needs only three methods (`Get`, `List`, `Source`) with two optional interfaces (`SatisfiesProvider` for the satisfies index, `RefreshableProvider` for cache refresh). Four loader methods collapse into one chain-walking function with a recursion guard flag. The in-memory cache stays as a Loader concern (it stores parsed recipes, not raw bytes), and `RequireEmbedded` becomes a source-tag filter on the provider chain, which requires the satisfies index to track contributing providers. The messiest integration point is `update-registry`, which needs type-assertion access to registry internals -- an acceptable escape hatch for one command.
