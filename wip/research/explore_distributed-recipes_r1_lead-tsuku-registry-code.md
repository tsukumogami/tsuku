# Lead: tsuku registry code paths

## Findings

### Current Architecture: Sequential Priority Chain

Tsuku models four recipe sources (embedded, local, registry cache, and remote
registry) as a sequential priority chain within the `Loader` struct rather than
as instances of a unified abstraction.

The `Get()` method in `loader.go` (lines 87-143) hardcodes the precedence:
1. In-memory cache
2. Local filesystem
3. Embedded FS
4. Registry (which internally checks disk cache first, then fetches remote)

This creates a single load path with embedded source checks, but no
`RecipeSource` interface or registry abstraction that local/embedded/registry
recipes inherit from.

### Natural Seams for Unification

The natural seams already exist in the code:

- `FetchRecipe()` and `GetCached()` in Registry split local vs. remote
- `EmbeddedRegistry.Get()` is self-contained
- Local file loading is isolated in `loadLocalRecipe()`
- Registry caching (`CachedRegistry` wrapper, cache metadata in `.meta` sidecars)
  and the manifest system (`recipes.json`) are independent of recipe fetching

### CachedRegistry Wrapper

`CachedRegistry` is only used in the `update-registry` command. The Loader
directly calls `GetCached()` and `FetchRecipe()` without the wrapper's freshness
checks.

## Implications

Unification requires extracting a `RecipeProvider` interface (with methods like
`Get(ctx, name)`, `List()`, `IsCached()`, `GetCached()`) that both the Registry
struct and EmbeddedRegistry struct implement. The Loader would then compose
providers in priority order rather than embedding the fetch logic directly.

The existing code has clean seams -- this is more of a refactor than a rewrite.

## Surprises

The CachedRegistry wrapper isn't used in the main install path. The Loader
bypasses it and calls Get/Fetch directly on Registry. This means the caching
abstraction is already partially decoupled.

## Open Questions

Should the `CachedRegistry` wrapper be part of the unified abstraction, or
should caching (TTL, stale-if-error, LRU eviction) be pushed into the Loader
as a separate responsibility?

## Summary

Tsuku uses a hardcoded priority chain (memory -> local -> embedded -> registry) in the Loader rather than a unified registry interface. The natural seams exist for extracting a RecipeProvider interface that all sources implement. The biggest open question is whether caching belongs in the provider abstraction or the Loader.
