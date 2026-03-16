# Advocate: URL Resolver

## Approach Description

The URL Resolver approach treats distributed recipe sources as a URL translation
problem rather than an abstraction layer problem. When a user runs `tsuku install
owner/repo`, the system detects the slash in the name and resolves it to a GitHub
raw content URL pointing to `.tsuku-recipes/` in that repository. The resolved URL
feeds into the existing `Registry.FetchRecipe()` / `Registry.CacheRecipe()` /
`Registry.GetCached()` machinery.

Concretely:

1. **Name detection**: The Loader's `GetWithContext()` gains one new branch before
   the registry fallback. If `name` contains a `/`, treat it as `owner/repo/tool`
   (or `owner/repo` where tool = repo basename). Resolve to a raw GitHub URL like
   `https://raw.githubusercontent.com/{owner}/{repo}/main/.tsuku-recipes/{tool}.toml`.

2. **Fetch via existing Registry**: Create or reuse a `Registry` instance with
   `BaseURL` set to the resolved repo URL prefix. Call the same `FetchRecipe()`,
   `CacheRecipe()`, and `GetCached()` methods. The cache directory would be
   namespaced under `$TSUKU_HOME/registry/distributed/{owner}/{repo}/`.

3. **Source tracking**: Add a `Source` field to `ToolState` in `state.json`
   recording the origin (e.g., `"central"`, `"github:owner/repo"`). The existing
   `Plan.RecipeSource` string gets populated with the same value.

4. **Source management**: A configuration file (e.g., `$TSUKU_HOME/sources.json`)
   lists known `owner/repo` mappings. Commands like `tsuku source add owner/repo`
   and `tsuku source list` manage this file. These are thin wrappers over the list.

## Investigation

### What I read

- `internal/recipe/loader.go` -- the full Loader with its 4-tier chain
- `internal/registry/registry.go` -- Registry struct, FetchRecipe, CacheRecipe, GetCached, recipeURL
- `internal/registry/cached_registry.go` -- CachedRegistry with TTL/stale-if-error
- `internal/registry/cache.go` -- CacheMetadata, sidecar metadata files
- `internal/registry/manifest.go` -- Manifest fetch/parse, schema versioning
- `internal/recipe/types.go` -- Recipe, RecipeSource, Plan structs
- `internal/install/state.go` -- ToolState, Plan, VersionState
- `cmd/tsuku/install.go` -- install command flow, name parsing, discovery fallback
- `internal/registry/cache_manager.go` -- CacheManager stats

### How the approach fits

**Registry is already URL-flexible.** The `Registry` struct has a `BaseURL` field
and an `isLocal` bool. It already supports both HTTP URLs and local filesystem
paths. The `recipeURL()` method constructs `{BaseURL}/recipes/{letter}/{name}.toml`.
For distributed sources, we'd need to either:
- Override `recipeURL()` behavior for non-standard directory layouts, or
- Accept that distributed repos must follow the `recipes/{letter}/{name}.toml`
  convention (unlikely for third-party repos), or
- Construct a custom URL outside `recipeURL()` and call the HTTP client directly.

The third option is most realistic. The raw fetch/cache/read methods on `Registry`
are simple enough that wrapping them for a different URL pattern is straightforward.

**Cache namespacing works naturally.** The cache uses `{CacheDir}/{letter}/{name}.toml`
with sidecar `.meta.json` files. For distributed sources, using a separate
`CacheDir` like `$TSUKU_HOME/registry/distributed/owner/repo/` keeps caches
isolated without any changes to cache logic.

**Name parsing has a natural hook.** The install command already splits on `@` for
versions (line 180-184 in install.go). Adding a slash-detection step is trivial.
The Loader's `GetWithContext()` has a clean insertion point between "embedded recipes"
and "registry fallback" (around line 126).

**Plan.RecipeSource is already free-form.** It stores strings like `"registry"`,
file paths, `"create"`, `"validation"`. Adding `"github:owner/repo"` fits the
existing pattern without schema changes.

## Strengths

- **Minimal new abstractions**: No new interfaces, no provider pattern, no
  registry abstraction layer. The approach adds a URL resolution function and a
  new branch in the Loader. This keeps the codebase's current concrete style
  intact.

- **Reuses proven infrastructure**: `FetchRecipe()` already handles HTTP GET with
  proper timeouts, rate limit detection, and error types. `CacheRecipe()` /
  `GetCached()` already handle disk caching with metadata sidecars. The
  `CachedRegistry` already implements TTL and stale-if-error semantics. All of
  this applies to distributed sources with minimal adaptation.

- **Central registry priority is trivially enforced**: The Loader's chain is
  explicit and sequential. Unqualified names (no slash) never reach the
  distributed resolution code. The slash acts as a syntactic firewall. There is
  zero risk of distributed sources shadowing central recipes by accident.

- **No new binary dependencies**: Pure HTTP. GitHub raw content URLs are
  unauthenticated for public repos. No git binary, no SSH keys, no submodule
  complexity.

- **Low blast radius**: The core Loader and Registry types don't change
  signatures. Existing tests pass untouched. New behavior is additive and gated
  behind name-contains-slash detection.

- **Backward compatible by design**: Existing `tsuku install fzf` behavior is
  completely unchanged. The new code path only activates for `owner/repo` or
  `owner/repo/tool` patterns.

- **Cache isolation is clean**: Each distributed source gets its own cache
  subdirectory, reusing the same metadata and TTL machinery. No collision with
  central registry cache entries.

## Weaknesses

- **URL construction is fragile**: GitHub raw content URLs depend on default
  branch naming (`main` vs `master`). The resolver would need to either assume
  `main`, require explicit branch specification, or probe both (adding latency).
  The GitHub API to determine the default branch requires authentication for
  private repos.

- **recipeURL() doesn't fit distributed layouts**: The existing `recipeURL()`
  method hardcodes the `recipes/{letter}/{name}.toml` directory structure. Third-
  party repos won't follow this convention. The resolver needs its own URL
  construction, meaning it can't fully reuse `FetchRecipe()` as-is -- it needs to
  either bypass `recipeURL()` or use the HTTP client directly.

- **No recipe discovery for distributed sources**: The central registry has a
  manifest (`recipes.json`) that enables `tsuku search`, `tsuku recipes`, and
  satisfies-index lookups. Distributed sources won't have manifests. Commands
  like `tsuku search` and `tsuku outdated` won't know about recipes in
  distributed sources unless they're individually fetched.

- **No integrity verification for distributed sources**: The central registry's
  recipes are reviewed and merged via PR. A distributed source is an arbitrary
  GitHub repo where the owner can change recipes at any time. There's no signing,
  no pinning to commits, and no content-hash verification beyond the cache
  metadata (which only detects local corruption, not upstream tampering).

- **Version resolution across sources is unclear**: If `owner/repo` provides a
  recipe for `fzf`, and the central registry also has `fzf`, what happens with
  `tsuku outdated`? The approach doesn't define how version providers interact
  with source tracking. A tool installed from a distributed source would need its
  version provider to resolve against the same source's recipe.

- **Multiple tools per repo require verbose syntax**: If a distributed source has
  10 recipes, installing each requires `tsuku install owner/repo/tool1`,
  `owner/repo/tool2`, etc. There's no `tsuku install --source owner/repo tool1
  tool2` batch syntax without additional command plumbing.

- **CachedRegistry isn't reusable as-is**: `CachedRegistry` wraps a single
  `Registry` instance. For distributed sources, you'd need either one
  `CachedRegistry` per source (memory/complexity cost) or a way to route
  through the existing one with different base URLs (which it doesn't support).

## Deal-Breaker Risks

- **GitHub raw content URL format is not a stable API**: GitHub doesn't guarantee
  the raw.githubusercontent.com URL format or availability. Rate limiting on raw
  content access is undocumented and could change. If GitHub changes their CDN
  structure or adds authentication requirements, every distributed source breaks.
  This isn't hypothetical -- GitHub has changed raw URL behavior before (requiring
  authentication for private repos, adding CORS restrictions). **Mitigation**: the
  approach could support GitHub API URLs as a fallback, but this adds complexity
  and requires authentication tokens. This risk is significant but not a true
  deal-breaker because: (a) the raw URL format has been stable for public repos
  for years, and (b) the approach could be extended to support the GitHub Contents
  API as an alternative.

- **No deal-breaker identified for the core use case**: For public repos with
  standard branch names, the approach works. The fragility risks are real but
  manageable with reasonable defaults (try `main`, fall back to `master`, error
  with guidance).

## Implementation Complexity

- **Files to modify**:
  - `internal/recipe/loader.go` -- add slash detection and distributed fetch
    branch (~30 lines)
  - `internal/registry/registry.go` -- add method to fetch from arbitrary URL
    (not just `recipeURL()` pattern) (~20 lines)
  - `internal/install/state.go` -- add `Source` field to `ToolState` (~5 lines)
  - `cmd/tsuku/install.go` -- pass source info through to state saving (~10 lines)
  - `cmd/tsuku/main.go` -- wire up new source management commands
  - New file: `cmd/tsuku/source.go` -- `tsuku source add/list/remove` (~100 lines)
  - New file: `internal/source/resolver.go` -- URL resolution logic (~80 lines)
  - New file: `internal/source/config.go` -- sources.json management (~60 lines)

- **New infrastructure**: sources.json config file, distributed cache subdirectory
  structure. No external services.

- **Estimated scope**: Medium. Core fetch path changes are small (~50 lines of
  logic). The bulk is the new `source` subcommand and its config management.
  Testing requires mocking GitHub raw URLs or using a local HTTP server.

- **Files affected but not modified** (backward compatible):
  - All 10+ commands that call `loader.Get()` -- they pass through unchanged
    because the Loader's signature doesn't change.
  - `internal/registry/cached_registry.go` -- not directly used by main install
    path (only by `update-registry`), so distributed sources can start without
    TTL-based refresh.

## Summary

The URL Resolver approach is the lowest-ceremony path to distributed recipes. It
adds one branch to the Loader, reuses the existing HTTP fetch and disk cache
machinery, and enforces central registry priority through syntax (slash vs no
slash) rather than configuration. Its main weakness is fragility around GitHub URL
conventions and the lack of recipe discovery or integrity verification for
distributed sources. For an MVP where the goal is "let users install from a
GitHub repo's `.tsuku-recipes/` directory," this approach delivers with roughly
300 lines of new code and no architectural changes -- but it accumulates technical
debt that a provider abstraction would avoid.
