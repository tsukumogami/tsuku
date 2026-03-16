# Architecture Review: DESIGN-distributed-recipes

## 1. Does the architecture fit the existing codebase?

**Yes, with caveats.** The RecipeProvider interface follows the same pattern as `version.VersionResolver` / `version.VersionLister` -- a core interface plus optional extension interfaces (`SatisfiesProvider`, `RefreshableProvider`). This is the established extensibility pattern in the codebase and the right structural choice.

The design correctly identifies that `EmbeddedRegistry` already has `Get(name) ([]byte, bool)`, `List() []string`, and `Has(name) bool` -- it's essentially an unformalized provider. Wrapping it in an adapter is low-risk.

## 2. Findings

### Finding 1: `Loader.Registry()` accessor creates a leak that the design underestimates

**Severity: Advisory (borderline blocking)**

The design acknowledges that `update-registry` needs "type-assertion access to the central registry provider's internals." But the actual coupling is broader than one command.

`cmd/tsuku/update_registry.go:32` calls `loader.Registry()` to get the raw `*registry.Registry`, then wraps it in `registry.NewCachedRegistry()`. This isn't just a type assertion -- it's constructing a separate object (`CachedRegistry`) from the provider's internals and calling methods like `Refresh()`, `RefreshAll()`, `GetCacheStatus()`, `ListCached()` on it.

The design proposes `RefreshableProvider` with a single `Refresh(ctx) error` method. That's insufficient for what `update-registry` actually does:
- Per-recipe dry-run status checking (`GetCacheStatus`)
- Per-recipe refresh with age reporting (`Refresh` returning `*RefreshDetail`)
- Bulk refresh with statistics (`RefreshAll` returning `*RefreshStats`)

Either `RefreshableProvider` needs to be much richer, or `update-registry` keeps reaching through the abstraction. The design should explicitly decide which, because this will be the first thing an implementer hits.

**Recommendation:** Accept the escape hatch for `update-registry` but formalize it. Add a `CacheManageableProvider` optional interface now rather than deferring it. The interface only needs to match what `update-registry` actually calls. Deferring this creates a situation where Phase 1 ships a `RefreshableProvider` that Phase 5 immediately bypasses.

### Finding 2: `RecipeProvider.Get` returns `[]byte` but the Loader parses and caches `*Recipe`

**Severity: Advisory**

The proposed interface has `Get(ctx, name) ([]byte, error)` -- providers return raw TOML bytes. The Loader then parses via `parseBytes()` and caches the `*Recipe` in its `recipes` map. This is fine structurally, but creates a subtle issue: the Loader's in-memory cache (`map[string]*Recipe`) has no source tag. When a recipe is served from cache, the Loader can't tell which provider originally supplied it.

The design shows `resolveFromChain` returning `([]byte, RecipeSource, error)`, which means the source is known at resolution time. But the in-memory cache key is just `name` with no source annotation. If a distributed recipe `owner/repo:foo` and a central recipe `foo` coexist, the cache can serve the wrong one.

The name-parsing rules (qualified vs. unqualified) likely prevent this in practice -- `owner/repo:foo` would be keyed differently than `foo`. But the design doesn't explicitly address the cache key scheme for qualified names. It should.

**Recommendation:** Specify that the in-memory cache key for distributed recipes is the full qualified name (e.g., `owner/repo:recipe`), not the bare recipe name.

### Finding 3: Two existing `RecipeLoader` interfaces are not reconciled

**Severity: Blocking**

The codebase already has two `RecipeLoader` interfaces:

1. `internal/actions/resolver.go:25` -- `RecipeLoader` with `GetWithContext(ctx, name, opts) (*Recipe, error)`
2. `internal/verify/deps.go:19` -- `RecipeLoader` with `LoadRecipe(name) (*Recipe, error)`

These are consumer-side interfaces satisfied by `*Loader`. The design introduces `RecipeProvider` as a supply-side interface for sources. That's the right separation. However, the design doesn't mention these existing interfaces or how the Loader continues to satisfy them after refactoring.

The current `Loader.GetWithContext` signature won't change (the design says "the Loader's public API stays the same"), so `actions.RecipeLoader` is fine. But if the constructor changes from `NewWithLocalRecipes(reg, dir)` to `NewLoader(providers...)`, every callsite that constructs a Loader needs updating. The design's Phase 1 deliverables list "Updated tests" but don't mention `cmd/tsuku/main.go:71` where the production Loader is constructed, or `cmd/tsuku/create_test.go` which constructs test Loaders.

This isn't blocking per se -- it's just incomplete enumeration. But the omission of the two existing `RecipeLoader` interfaces from the design is a gap. If an implementer sees `RecipeLoader` and `RecipeProvider` as overlapping, they might try to merge them, which would be wrong.

**Recommendation:** Add a section clarifying the distinction: `RecipeProvider` is the supply-side interface (sources implement it). `RecipeLoader` (in `actions` and `verify`) is the consumer-side interface (the Loader satisfies it). The Loader bridges the two. No changes to consumer interfaces.

### Finding 4: `GetFromSource` bypasses the priority chain -- what about the in-memory cache?

**Severity: Advisory**

The design introduces `GetFromSource(ctx, name, source) ([]byte, error)` for source-directed operations (update, verify). This method "bypasses the priority chain and loads directly from the provider matching the given source."

But the Loader's first check in `GetWithContext` is always the in-memory cache. If a recipe was loaded via the chain (e.g., from embedded), and then `GetFromSource` is called with `source="owner/repo"`, should it skip the cache? The design doesn't specify.

For update/outdated, you want the fresh version from the source, not the cached parse. The method should either bypass the in-memory cache entirely or use a source-qualified cache key.

**Recommendation:** Specify that `GetFromSource` does not consult or populate the in-memory `recipes` map.

### Finding 5: Phase sequencing is correct but Phase 3 has an ordering dependency

**Severity: Advisory**

Phase 3 (Registry management) creates `tsuku registry add/remove/list` subcommands and `GetFromSource()`. Phase 4 (Distributed provider) implements the actual HTTP fetching. But `GetFromSource` with a distributed source can't work until Phase 4 exists.

This means Phase 3 can only be tested with mock providers or with central/local/embedded sources. That's fine for development, but the design should note it explicitly so implementers don't try to integration-test distributed `GetFromSource` in Phase 3.

The phases are otherwise well-sequenced. Phase 1 (pure refactor, no behavior change) as a standalone deliverable is the right call -- it can be reviewed and merged independently.

### Finding 6: `Source` field on `ToolState` vs. existing `Plan.RecipeSource`

**Severity: Advisory**

`Plan.RecipeSource` (line 35 of `state.go`) already stores a string like `"local"`, `"embedded"`, or `"registry"`. The design adds `ToolState.Source` as a new top-level field. The design mentions lazy migration that infers from `Plan.RecipeSource`, which is correct.

But the two fields will coexist. `Plan.RecipeSource` describes where the recipe came from at plan time. `ToolState.Source` describes where the tool's recipe lives for future operations. These can diverge -- e.g., a user installs from `owner/repo`, then the recipe gets upstreamed to the central registry. The `Plan.RecipeSource` in the stored plan will say `owner/repo` but `ToolState.Source` should be updated.

The design doesn't specify whether `Source` is immutable after install or can be updated. For `tsuku update`, if the user runs `tsuku install ripgrep` (central) to replace a previously distributed install, should `Source` change?

**Recommendation:** Specify that `Source` is set at install time and updated on reinstall/update. It reflects the current source of truth for where to find this tool's recipe.

### Finding 7: httputil.NewSecureClient exists and is the right choice

**Severity: Not a finding -- confirming the design's claim**

The design references `httputil.NewSecureClient` for SSRF protection. This exists at `internal/httputil/client.go` and is already used by `internal/version/resolver.go`, `internal/actions/download.go`, and `internal/validate/predownload.go`. Using it for the distributed provider is structurally consistent.

### Finding 8: No consideration of the `warnIfShadows` interaction

**Severity: Advisory**

`Loader.warnIfShadows()` (line 265) directly calls `l.registry.GetCached(name)` to check if a local recipe shadows a registry recipe. After the refactor, this should iterate providers to check for shadowing. Otherwise `warnIfShadows` becomes a hardcoded reference to the central registry that doesn't warn about distributed recipe shadowing.

This is a small detail but exactly the kind of thing that gets missed in Phase 1 and creates inconsistency.

**Recommendation:** Phase 1 should refactor `warnIfShadows` to use the provider chain (check if any lower-priority provider has the recipe).

## 3. Missing components or interfaces

**Config loading in the Loader construction path.** The design puts registry config in `$TSUKU_HOME/config.toml` (userconfig). Currently, the Loader is constructed in `cmd/tsuku/main.go:71` with just `recipe.NewWithLocalRecipes(reg, cfg.RecipesDir)`. After the refactor, constructing the distributed provider requires reading `config.toml` to know which repos are registered. The design mentions lazy loading with in-process caching but doesn't show where in the construction sequence this happens. Should `main.go` read userconfig and pass registered repos to the Loader constructor? Or should the Loader read config internally?

The Loader currently has no dependency on `userconfig`. Adding one would be a new dependency direction (recipe -> userconfig). The cleaner path: `main.go` reads userconfig, constructs distributed providers from it, and passes them to `NewLoader(providers...)`. This keeps the Loader agnostic about where provider configuration comes from.

## 4. Simpler alternatives overlooked

The design considered three alternatives and chose reasonably. The URL Resolver was the simplest but the design correctly identified it as tech debt accumulation.

One alternative not considered: **extend the existing `Registry` type to accept multiple base URLs.** The design rejected "Extended Registry" but framed it as `[]*Registry` on the Loader. A variant would be making `Registry` itself multi-origin -- it already handles both local and remote via `isLocal`. Adding a `[]Origin` with per-origin URL patterns and cache dirs keeps the abstraction inside `Registry` without a new interface. This would be less clean than the provider interface but dramatically less code to ship. Worth noting as a "if the refactor proves too risky" fallback.

## 5. Edge cases not covered in data flow

1. **Race between auto-register and strict mode toggle.** User starts an install, auto-registration fires, then (in another terminal) enables strict mode. The install succeeds but the registry entry persists. Not harmful but the design should note that strict mode only applies at install time, not retroactively.

2. **Offline behavior for distributed sources.** The central registry has stale-if-error via `CachedRegistry`. The distributed provider's cache (`$TSUKU_HOME/cache/distributed/`) has `_source.json` with timestamps, but the design doesn't specify stale-if-error behavior. If GitHub is down, can a cached distributed recipe still be used?

3. **Recipe name collision across distributed sources.** Two different `owner/repo` sources could have a recipe with the same name (e.g., both have `.tsuku-recipes/foo.toml`). The qualified name (`owner/repo:foo`) disambiguates at install time, but `tsuku list` and `tsuku recipes` need to handle the display. The design mentions `ListAllWithSource` but doesn't specify how collisions render.

4. **Satisfies index for distributed recipes.** The design's `SatisfiesProvider` optional interface means distributed recipes can participate in the satisfies index. But the satisfies index is built lazily on first fallback and includes all providers. If a distributed recipe claims to satisfy `gcc`, it could shadow the central registry's `gcc` recipe. The priority ordering (distributed is lowest) should prevent this, but the satisfies index currently uses "first match wins" without priority awareness. After refactoring, the index build order must match provider priority.

## 6. Summary assessment

The design is architecturally sound and implementable. The RecipeProvider interface is the right abstraction -- it follows the version provider pattern, eliminates duplicated chain logic, and makes distributed sources a single implementation.

**Blocking finding:** The existing `RecipeLoader` interfaces in `actions` and `verify` must be acknowledged in the design to prevent a confused implementer from merging or conflicting with them.

**Key advisory items to address before implementation:**
- Specify the in-memory cache key scheme for qualified names
- Decide on `GetFromSource` cache behavior
- Enrich `RefreshableProvider` or formalize the `update-registry` escape hatch now
- Refactor `warnIfShadows` in Phase 1
- Specify that `main.go` constructs providers from userconfig, not the Loader itself

The phases are correctly sequenced. Phase 1 as a standalone pure refactor is the right first step.
