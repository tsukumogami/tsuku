# Advocate: RecipeProvider Interface

## Approach Description

Define a Go interface that all recipe sources implement:

```go
type RecipeProvider interface {
    // Get returns raw TOML bytes for a recipe, or nil if not found.
    Get(ctx context.Context, name string) ([]byte, error)
    // List returns all available recipe names from this provider.
    List(ctx context.Context) ([]RecipeInfo, error)
    // Source returns the RecipeSource tag for recipes from this provider.
    Source() RecipeSource
    // Priority returns the provider's position in the resolution chain.
    // Lower values are checked first.
    Priority() int
}
```

The Loader stops holding direct references to `*registry.Registry`, `*EmbeddedRegistry`, and `recipesDir string`. Instead, it holds an ordered `[]RecipeProvider` slice. `GetWithContext()` iterates through providers in priority order, calling `Get()` on each until one returns data. Each provider encapsulates its own fetching logic (filesystem reads, HTTP calls, embed.FS access, distributed HTTP).

New source types (distributed registries, organization-internal registries) are added by implementing the interface and registering with the Loader via something like `loader.AddProvider(provider)`.

## Investigation

### Current Loader structure (loader.go, 688 lines)

The Loader has three distinct source backends wired in as struct fields:

1. **In-memory cache** (`map[string]*Recipe`) -- always checked first, not a "source" in the provider sense.
2. **Local filesystem** (`recipesDir string`) -- accessed via `loadLocalRecipe()` which does `os.ReadFile(filepath.Join(recipesDir, name+".toml"))`.
3. **Embedded FS** (`*EmbeddedRegistry`) -- accessed via `l.embedded.Get(name)` returning raw `[]byte`.
4. **Central registry** (`*registry.Registry`) -- accessed via `l.registry.GetCached()` then `l.registry.FetchRecipe()`, with manual caching in `fetchFromRegistry()`.

`GetWithContext()` (lines 87-143) contains the priority chain as a series of `if` blocks. `loadDirect()` (lines 187-226) duplicates the same chain minus the satisfies fallback. `getEmbeddedOnly()` (lines 147-181) is a restricted variant. Three methods doing roughly the same thing with minor variations.

### What each source already does behind the scenes

- **EmbeddedRegistry** (embedded.go): Has `Get(name) ([]byte, bool)`, `List() []string`, `ListWithInfo() ([]RecipeInfo, error)`, `Has(name) bool`. This already looks like a provider interface -- it just doesn't declare one.

- **Registry** (registry/registry.go): Has `FetchRecipe(ctx, name) ([]byte, error)`, `GetCached(name) ([]byte, error)`, `ListCached() ([]string, error)`, `CacheRecipe(name, data) error`. The fetching-and-caching dance is split between the Loader's `fetchFromRegistry()` and Registry methods.

- **Local recipes**: Just `os.ReadFile` with path construction. Simplest source.

### Coupling surface

The `Loader.Registry()` method exposes the raw `*registry.Registry` to callers. Grepping shows only two call sites:
- `cmd/tsuku/update_registry.go:32` -- needs the Registry to create a `CachedRegistry` for TTL-based refresh.
- `internal/recipe/loader_test.go:217` -- test assertion.

This is very light coupling. The update-registry command could be handled as a provider-level operation (e.g., `provider.Refresh(ctx)`) or keep a separate reference to the registry provider.

### ListAllWithSource pattern

`ListAllWithSource()` (lines 474-514) manually iterates local, embedded, and registry sources to build a deduplicated list. With providers, this becomes a loop: iterate providers in priority order, collect `RecipeInfo` from each, deduplicate by name. The existing `RecipeInfo` struct with its `Source` field maps directly to what providers would return.

### Satisfies index

The `buildSatisfiesIndex()` method (lines 370-418) scans embedded recipes and the registry manifest. With providers, each provider could expose its satisfies entries, or the index-building could iterate all providers calling a `ListSatisfies()` method. This is the trickiest part: the satisfies index currently uses a mix of full recipe parsing (embedded) and manifest-based shortcuts (registry), so the interface might need to accommodate both strategies.

### Plan generator integration

`PlanConfig.RecipeSource` is a free-form string set by callers (lines 29-30 in plan_generator.go). The provider's `Source()` method could produce this string. When `GetWithContext` finds a recipe, it could return which provider served it, and the caller passes that to the plan generator. This maps cleanly.

### State tracking

`ToolState` in state.go has no recipe source field. `Plan.RecipeSource` exists but is a free-form string (line 37 of plan.go). The provider approach makes it natural to record the source: whoever resolves the recipe knows which provider answered. No state schema changes are required, just populating the existing field consistently.

## Strengths

- **Eliminates code duplication in the Loader.** `GetWithContext`, `loadDirect`, and `getEmbeddedOnly` share the same pattern (iterate sources, check each one). A provider chain collapses these into a single loop with filter parameters (e.g., "embedded only" becomes filtering by provider type). This cuts the Loader from ~450 lines of source-specific code to ~150 lines of generic chain logic.

- **Maps cleanly to existing source APIs.** `EmbeddedRegistry` already has `Get(name) ([]byte, bool)` and `ListWithInfo()`. The local filesystem source is a trivial wrapper. The registry source wraps `GetCached` + `FetchRecipe`. Each adapter is 30-50 lines. No existing behavior needs to change -- the adapters call the same underlying code.

- **Adding distributed sources is just another provider.** A `DistributedProvider` implements the same interface. The Loader doesn't need to know about HTTP endpoints, authentication, or caching strategies specific to distributed sources. This is the core design goal and the approach delivers it directly.

- **Testability improves significantly.** Currently, testing the Loader requires constructing a real `*registry.Registry` with filesystem paths and optionally HTTP backends. With providers, tests can inject mock providers that return canned data. The existing `CacheRecipe()` method on the Loader hints at this need (line 359: "useful for testing or loading recipes from non-standard sources").

- **RecipeSource tracking becomes automatic.** Each provider knows its source type. The Loader can return `(recipe, source)` tuples without callers needing to guess. The existing `RecipeSource` string in `Plan` and `RecipeInfo.Source` get populated consistently from the same place.

- **Aligns with the version provider pattern.** The codebase already has a pluggable provider pattern in `internal/version/` where different version sources implement a common interface. A recipe provider interface follows the same architectural pattern, making the codebase more consistent.

## Weaknesses

- **The satisfies index doesn't fit the simple Get/List interface.** Building the satisfies index requires either parsing every recipe's metadata (expensive for registry sources) or accessing a manifest/index file (registry-specific optimization). A naive interface forces full recipe enumeration. The fix is either a separate `SatisfiesEntries()` method on the interface (which some providers leave empty) or keeping the manifest-based shortcut as a separate mechanism. Either way, it adds complexity to the interface.

- **CachedRegistry becomes awkward.** `CachedRegistry` wraps `Registry` and is only used by the `update-registry` command. With providers, where does TTL-based refresh live? Options: (a) the registry provider itself handles caching internally, (b) a decorator pattern wraps any provider with caching, (c) refresh stays as a separate concern outside the provider chain. Option (a) is simplest but mixes concerns. The current code already has this mixing (Registry handles both fetching and caching), so it's not a regression.

- **`loader.Registry()` accessor breaks encapsulation.** Two callers access the raw `*registry.Registry` through the Loader. With providers, this accessor either survives (defeating the abstraction) or those callers need refactoring. The update-registry command is the harder case: it needs cache-level operations (TTL refresh, dry-run status checks) that don't belong on the generic provider interface.

- **Migration cost is non-trivial.** The Loader is used across 19 files via `loader.Get` or `loader.GetWithContext`. While the Loader's public API can stay the same (the chain is an internal detail), the constructor functions (`New`, `NewWithLocalRecipes`, `NewWithoutEmbedded`) need to change to accept providers. Tests that construct Loaders will need updating.

- **RequireEmbedded mode needs special handling.** The `LoaderOptions.RequireEmbedded` flag restricts loading to embedded sources only. With providers, this becomes a filter on the chain ("only try providers of type embedded"). This works but the interface needs to support type identification, adding a method or type assertion to the hot path.

## Deal-Breaker Risks

- **None identified.** The existing source backends are already structured as semi-independent code paths with similar signatures. The refactoring is a formalization of an existing implicit pattern, not an architectural overhaul. The main risk -- satisfies index complexity -- is manageable because the current implementation already handles two different strategies (full parse vs. manifest lookup) and the interface can accommodate both via an optional method.

The closest thing to a deal-breaker would be if the `update-registry` command's cache management operations required deep integration with the Loader's resolution chain. But inspection shows `update-registry` creates its own `CachedRegistry` independently (line 41 of update_registry.go: `cachedReg := registry.NewCachedRegistry(reg, ttl)`) and doesn't go through the Loader at all. This means cache refresh can stay as a registry-specific concern without polluting the provider interface.

## Implementation Complexity

- **Files to modify:** ~12-15 files
  - `internal/recipe/loader.go` -- major refactor (core change)
  - `internal/recipe/types.go` -- add interface definition
  - `internal/recipe/embedded.go` -- adapter wrapping EmbeddedRegistry
  - `internal/recipe/loader_test.go` -- update tests for new construction
  - `internal/recipe/satisfies_test.go` -- may need updates
  - `internal/registry/registry.go` -- possibly add adapter type
  - `cmd/tsuku/main.go` -- update Loader construction
  - `cmd/tsuku/update_registry.go` -- may need minor changes
  - `cmd/tsuku/helpers.go` -- if it constructs Loaders
  - `internal/executor/plan_generator.go` -- minor (RecipeSource propagation)
  - `internal/actions/resolver.go` -- if it uses Loader directly
  - 2-3 new files for provider adapter implementations

- **New infrastructure:** Yes
  - Interface definition (~20 lines)
  - 3 adapter types (local, embedded, registry) -- ~50 lines each
  - Provider chain logic in Loader -- ~100 lines replacing ~300 lines of current code

- **Estimated scope:** Medium
  - Core refactor is contained in `internal/recipe/` and `internal/registry/`
  - Public API of `Loader.Get()` and `Loader.GetWithContext()` can remain identical
  - The blast radius is kept small because the interface sits behind the existing Loader API

## Summary

The RecipeProvider interface formalizes a pattern that already exists implicitly in the codebase. The three current sources (local, embedded, registry) each have their own Get/List operations with similar shapes but no shared contract. Wrapping them in a common interface eliminates ~300 lines of duplicated chain logic in the Loader and makes distributed sources a matter of adding one more provider implementation. The main complexity cost is handling the satisfies index, which needs either an optional interface method or a parallel indexing mechanism, but the current code already deals with two different indexing strategies, so this isn't new complexity -- just relocated complexity.
