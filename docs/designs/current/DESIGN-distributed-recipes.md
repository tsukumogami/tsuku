---
status: Current
upstream: docs/prds/PRD-distributed-recipes.md
problem: |
  The recipe Loader uses a hardcoded priority chain (memory, local, embedded,
  registry) with no shared interface. Adding distributed sources means either
  threading another source type through every conditional, or extracting a
  RecipeProvider abstraction. State tracking has no source field, the caching
  subsystem is coupled to the central registry's URL model, and every command
  that resolves recipes would need ad-hoc distributed source handling.
decision: |
  Extract a RecipeProvider interface that all recipe sources implement. The Loader
  becomes an ordered chain of providers instead of a hardcoded priority sequence.
  Distributed sources are a new provider implementation that fetches recipes via
  HTTP from GitHub repositories containing a .tsuku-recipes/ directory.
rationale: |
  The interface pattern already exists implicitly (EmbeddedRegistry has Get/List/Has)
  and explicitly in the version provider system. Formalizing it eliminates duplicated
  chain logic in the Loader and makes distributed sources a single implementation
  rather than branches threaded through every command. The alternatives (extending
  the Registry type or URL resolution) either fail to deliver the unified abstraction
  the PRD requires or accumulate technical debt.
---

# DESIGN: Distributed Recipes

## Status

Current

## Context and Problem Statement

tsuku's recipe loading is built around a single Loader that checks four sources
in sequence: in-memory cache, local filesystem (`$TSUKU_HOME/recipes/`), embedded
recipes (compiled into the binary), and the central registry (fetched via HTTP,
cached with TTL and stale-if-error). Each source is accessed through distinct
code paths inside `Loader.GetWithContext()`, with no shared interface.

This creates three implementation problems:

**No provider abstraction.** The Loader directly calls `loadLocalRecipe()`,
`l.embedded.Get()`, and `l.registry.FetchRecipe()` in sequence. There's a
`RecipeSource` type (`local`, `embedded`, `registry`) but it's just a string
enum used for display, not a behavioral interface. Adding a fifth source
(distributed) means adding another branch to every recipe-loading path.

**State doesn't track source.** `ToolState` records versions, binaries, and
checksums but not where the recipe came from. `Plan.RecipeSource` exists but
it's a free-form string set during plan generation, not a structured field
on installed tool state. When `tsuku update` runs, it has no way to know which
source to check for a given tool.

**Caching is central-registry-specific.** The `Registry`, `CachedRegistry`, and
`CacheManager` types assume a single base URL with alphabetical subdirectories
(`registry/a/ansible.toml`). Distributed sources need per-source caching with
independent TTLs, but the cache directory layout doesn't accommodate multiple
origins.

The version provider system, by contrast, is already source-agnostic. A recipe's
`[version]` section resolves tool versions through pluggable providers (GitHub,
PyPI, crates.io, etc.) regardless of where the recipe itself came from. This
means the design can focus on recipe resolution and leave version resolution
untouched.

## Decision Drivers

**From the PRD:**

- **Uniform install experience (Goal 2).** `tsuku install ripgrep` and
  `tsuku install owner/repo` should feel identical. The underlying source
  difference must be invisible to the user beyond the name format.

- **Unified abstraction (Goal 3).** All recipe sources share a common model.
  Adding a new source type shouldn't require modifying every command.

- **Central registry priority (R5).** Unqualified names never consult distributed
  sources. This prevents name confusion and keeps the existing install path fast.

- **No new binary dependencies (R17).** Fetching must use HTTP with Go's stdlib.
  No `git` binary required.

- **Backward compatibility (R18).** Existing state.json files, CLI behavior,
  output format, and exit codes are unchanged. Lazy migration for the source field.

- **Minimal author friction (R19).** One directory, one TOML file. No manifest,
  no accounts, no registration.

**Implementation-specific:**

- **Loader refactoring scope.** The Loader is ~690 lines with recipe loading,
  satisfies-index building, and constraint lookup interleaved. The refactor
  should be surgical -- extract an interface without rewriting unrelated logic.

- **CachedRegistry reuse.** `CachedRegistry` (TTL, stale-if-error, LRU) is only
  used by `update-registry`. The main install path bypasses it. The design should
  either bring it into the main path or extend its model to distributed sources.

- **Command surface.** At least 10 commands need awareness of distributed sources:
  `install`, `remove`, `list`, `info`, `update`, `outdated`, `verify`, `recipes`,
  `update-registry`, and the new `registry` subcommands. The abstraction must
  minimize per-command changes.

- **HTTP fetching strategy.** GitHub's raw content API, archive API, and releases
  API each have different rate limits, auth models, and response formats. The
  design must pick one (or a fallback chain) for fetching `.tsuku-recipes/` from
  arbitrary repos.

- **Registry state persistence.** Registered distributed sources need to be stored
  somewhere. Options include state.json (existing), a separate config file, or
  individual files per registry.

## Considered Options

### Decision 1: Recipe source abstraction model

**Context:** The Loader needs to support distributed recipe sources alongside
existing local, embedded, and central registry sources. The core question is how
to structure this: new interface, extended existing type, or minimal URL routing.

**Chosen: RecipeProvider Interface.**

A Go interface with `Get`, `List`, and `Source` methods, implemented by adapters
wrapping each existing source. The Loader holds an ordered `[]RecipeProvider`
slice and iterates it, replacing the current sequence of `if` blocks.

This approach was selected because it delivers the PRD's unified abstraction goal
directly. The pattern already exists in the codebase: `EmbeddedRegistry` has
`Get(name)`, `List()`, and `Has(name)` -- it's one method signature away from a
formal interface. The version provider system in `internal/version/` uses the same
pluggable pattern. Formalizing it eliminates ~300 lines of duplicated chain logic
across four Loader methods (`GetWithContext`, `loadDirect`, `getEmbeddedOnly`,
`loadEmbeddedDirect`) and makes adding future source types a single
implementation. Each provider controls its own URL construction and caching
strategy, so distributed repos' flat `.tsuku-recipes/` layout doesn't conflict
with the central registry's bucketed `recipes/{letter}/{name}.toml` structure.

Two sub-decisions follow from this choice:

**In-memory cache stays in the Loader, not in providers.** The Loader's
`recipes map[string]*Recipe` stores parsed `*Recipe` objects, not raw bytes.
Providers return `[]byte` (TOML). Since parsing is a Loader concern and the
cache is shared across all providers, it sits above the provider layer. A
provider returning pre-parsed recipes would bypass validation.

**`update-registry` uses type-assertion, not interface methods.** The
`update-registry` command needs cache-level operations (TTL checking, forced
refresh, manifest fetching) that don't belong on the provider interface. The
Loader exposes `ProviderBySource()` and callers type-assert to the concrete
provider when they need internals. This is an intentional escape hatch for one
command, not a pattern to copy.

*Alternative rejected: Extended Registry.* Keeps the Loader's shape but changes
`registry *Registry` to `registries []*Registry`. This works for adding more
HTTP-based registries, but it doesn't unify local and embedded sources behind
the same model -- they remain special cases with their own code paths. It also
assumes distributed repos follow the central registry's directory layout, which
they won't (the PRD specifies `.tsuku-recipes/` as a flat directory). The
`PathStyle` escape hatch needed to fix this erodes the approach's main selling
point of "just another Registry instance." The Loader would gain 5+ loop sites
that must stay in sync, increasing maintenance cost rather than reducing it.

*Alternative rejected: URL Resolver.* Adds one branch to the Loader that resolves
`owner/repo` to a GitHub raw content URL and fetches through existing machinery.
Lowest ceremony (~300 lines of new code), but explicitly accumulates technical
debt. It doesn't deliver any abstraction -- distributed sources are a special
case handled by slash detection. No recipe discovery, no manifest support, and
GitHub raw content URL stability is a real concern. The advocate's own summary
acknowledged that "a provider abstraction would avoid" the tech debt.

### Decision 2: Satisfies index integration

**Context:** The Loader's satisfies index maps package names to recipe names
(e.g., `"python3"` -> `"python"`). Today it uses two strategies: full TOML parse
for embedded recipes, and a pre-computed manifest for the central registry. With
multiple provider types, the index needs a way to collect entries from each.

**Chosen: Optional `SatisfiesProvider` interface.**

Providers that can cheaply return satisfies entries implement an optional
interface:

```go
type SatisfiesProvider interface {
    SatisfiesEntries(ctx context.Context) (map[string]string, error)
}
```

The Loader checks `if sp, ok := provider.(SatisfiesProvider); ok` when building
the index. Each provider uses whatever strategy is efficient for it: embedded and
local providers do full TOML parsing, the central registry reads its manifest,
and distributed providers can use their own manifests. Providers that don't
implement the interface are skipped -- their recipes are only findable by exact
name.

The satisfies index also tracks which provider contributed each entry (as a
`satisfiesEntry{recipeName, source}` struct) so that `RequireEmbedded` can
filter the index to embedded-only entries. This is tag-based filtering: the
Loader uses `Source()` tags to select which providers participate in resolution,
generalizing to future flags like `RequireLocal` or `RequireOffline`.

*Alternative rejected: Loader-internal indexing.* Keep satisfies index building
as a Loader concern, with the Loader calling `List()` on some providers and
consulting a manifest for others. This requires the Loader to know provider
internals (which ones have manifests, which ones need full parsing), defeating
the purpose of the interface abstraction.

### Decision 3: HTTP fetching strategy

**Context:** The distributed provider needs to fetch `.tsuku-recipes/*.toml` from
GitHub repositories. GitHub offers several APIs with different trade-offs around
rate limits, auth requirements, and directory listing capability.

**Chosen: Two-tier approach (Contents API + raw content).**

1. One Contents API call (`GET /repos/{owner}/{repo}/contents/.tsuku-recipes`)
   lists available TOML files. This auto-resolves the default branch, returns
   file metadata including `download_url` fields pointing to
   `raw.githubusercontent.com`. Costs 1 rate-limited API request per repo.
2. Individual files are fetched via those `download_url` values --
   `raw.githubusercontent.com` URLs that have no rate limit and need no auth
   for public repos.

This minimizes API rate limit consumption (1 call per repo for discovery) while
supporting multi-recipe repos where filenames aren't known in advance. Auth via
`GITHUB_TOKEN` (already supported by the `secrets` package) raises the Contents
API limit from 60 to 5000 requests/hour.

Two sub-decisions follow from this choice:

**HTTP client: `httputil.NewSecureClient` for both tiers.** The existing registry
client (`newRegistryHTTPClient()`) lacks SSRF protection and redirect validation.
It's safe only because it hardcodes `raw.githubusercontent.com`. The distributed
provider accepts user-provided `owner/repo` values that influence URL
construction, so SSRF protection, DNS rebinding guards, and HTTPS-only redirect
enforcement are needed. Two separate client instances: an authenticated one for
Contents API calls (carries `GITHUB_TOKEN`), and an unauthenticated one for raw
content fetches (prevents token leakage to `download_url` targets).

**Default branch: Contents API auto-resolution, cached for subsequent fetches.**
The Contents API auto-resolves the default branch when no `ref` parameter is
given. The branch name is cached in `_source.json` alongside the directory
listing. For subsequent fetches (when the listing is cached but individual
recipes need refreshing), raw content URLs use the cached branch name. If the
Contents API is rate-limited on a cold cache, the provider tries `main` then
`master` as a fallback.

*Alternative rejected: Raw content only.* Uses
`raw.githubusercontent.com/{owner}/{repo}/{branch}/.tsuku-recipes/{file}` for
everything. No rate limits, but cannot list directory contents -- must know
filenames in advance. Requires default branch discovery (try `main`, then
`master`, wasting requests). Works for `owner/repo:recipe-name` where the
filename is known, but fails for bare `owner/repo` where the user wants all
recipes from a source.

*Alternative rejected: Archive/tarball API.* Downloads the entire repo as a
tarball in one request, then extracts `.tsuku-recipes/` files. Gets everything
at once, but downloads the full repo (overkill for 1-10 TOML files) and counts
against API rate limits. Only viable for very large multi-recipe repos, which
aren't the expected case.

### Decision 4: Registry storage location

**Context:** Registered distributed sources (the list of `owner/repo` values a
user trusts) need persistent storage. This is configuration -- "which sources
do I trust" -- not runtime state ("what's installed").

**Chosen: `config.toml` (new `[registries]` section).**

The `userconfig` package already manages `$TSUKU_HOME/config.toml` (TOML format,
atomic writes, 0600 permissions) with `Load()`, `Save()`, `Get()`, `Set()`
patterns. Registry entries fit naturally alongside existing user preferences
(telemetry, LLM config, secrets). The `strict_registries` flag also lives here.
Hand-editable for users who want to manage registries outside the CLI.

```toml
strict_registries = false

[registries."alice/tools"]
url = "https://github.com/alice/tools"
auto_registered = true
```

One trade-off: the install path currently never reads `config.toml`. Adding
registry lookup introduces a file read in a performance-sensitive path. This is
a single `os.ReadFile` of a small TOML file, mitigated by lazy loading with
in-process caching (read once per session).

*Alternative rejected: `state.json` (new top-level field).* Mixes configuration
(which sources to trust) with operational state (what's installed). The
`StateManager` uses exclusive file locks for writes -- registry edits would
contend with install operations. Changes to trusted sources would show up in
diffs alongside tool installation changes.

*Alternative rejected: Separate `registries.json`.* Clean separation, but
introduces yet another file and persistence layer. The `userconfig` package
already solves atomic writes, TOML parsing, and settings management. A separate
file duplicates that infrastructure for a single concern.

### Decision 5: Source tracking field placement

**Context:** Installed tools need to record which source they came from so that
`update`, `verify`, and `outdated` can fetch recipes from the right place.
`Plan.RecipeSource` already exists per-version inside cached plans, storing
`"registry"` or `"local"`.

**Chosen: Top-level `Source` field on `ToolState`.**

A tool comes from one source. You don't install v1 from `alice/tools` and v2
from `bob/tools` under the same name. The source determines where to check for
updates, which is a tool-level concern. The new `Source` field is the
authoritative source for future operations; `Plan.RecipeSource` remains as a
historical record of what was used at install time.

Lazy migration: entries with empty `Source` get `"central"` by default, inferred
from `Plan.RecipeSource` when available. This runs in the `Load()` path alongside
the existing `migrateToMultiVersion()` pattern. Since all currently installed
tools came from the central registry or embedded sources, defaulting to
`"central"` is safe.

*Alternative rejected: Per-version source tracking only.* Keep using
`Plan.RecipeSource` without a top-level field. This would require every command
that needs the source to find the active version, look up its VersionState, check
for a cached Plan, and extract RecipeSource. That's four levels of indirection
for a common operation. It also means a tool with no cached plan (pre-plan
installations) has no source at all.

### Decision 6: Distributed recipe cache architecture

**Context:** The central registry cache uses a letter-bucketed directory layout
(`$TSUKU_HOME/registry/{letter}/{name}.toml`) managed by a single `CacheManager`
with LRU eviction. Distributed recipes from different sources could collide on
names in this flat structure.

**Chosen: Separate `CacheManager` instance with its own directory tree.**

Distributed recipes get `$TSUKU_HOME/cache/distributed/{owner}/{repo}/` with
their own `CacheManager` instance and independent size limits. Each repo
directory contains recipe TOML files, metadata sidecars, and a `_source.json`
file with cached branch name, directory listing, and timestamp.

The central registry cache is bounded by the known recipe count (~1500).
Distributed sources are unbounded and user-driven. Separate limits prevent a
proliferation of distributed sources from evicting official registry cache
entries. The central registry cache location (`$TSUKU_HOME/registry/`) stays
unchanged for backward compatibility.

*Alternative rejected: Extend existing `CacheManager`.* The `CacheManager` walks
`{CacheDir}/{letter}/` looking for `.toml` files. Its `listEntries()` and
`Size()` methods only walk one level of letter directories. Making it handle the
distributed directory layout (nested `{owner}/{repo}/`) requires modifying its
traversal logic and size accounting. Since eviction semantics differ between
bounded (central) and unbounded (distributed) caches, a shared manager would need
conditional logic for each.

### Decision 7: Name collision handling across sources

**Context:** `state.Installed` is a `map[string]ToolState` keyed by tool name.
If `alice/tools` and `bob/tools` both define a recipe named `mytool`, there's a
namespace collision. This also applies to distributed recipes that share a name
with a central registry recipe (though R5 prevents unqualified names from
reaching distributed sources).

**Chosen: Last-install-wins with collision detection.**

`state.Installed["mytool"]` points to whichever source was installed last.
Source is tracked, so `tsuku info mytool` shows which source it came from.
When installing a tool whose name already exists from a different source, the
CLI prompts: "mytool is currently installed from bob/tools. Replace with
alice/tools? [y/N]". The `--force` flag skips the prompt.

This matches how most package managers work (Homebrew taps, npm scoped packages).
It keeps the state model simple and avoids PATH conflicts between multiple
binaries with the same name.

*Alternative rejected: Namespaced keys (e.g., `alice/tools/mytool`).* Both
`state.Installed["alice/tools/mytool"]` and `state.Installed["bob/tools/mytool"]`
could coexist, each installing different binaries. But this has massive blast
radius: every command that reads state by tool name needs updating. The `bin/`
directory can't have two `mytool` symlinks, so PATH conflicts remain unsolved.
And the user experience degrades -- `tsuku update mytool` becomes ambiguous,
requiring `tsuku update alice/tools/mytool` for every operation.

### Decision 8: Provider unification approach

**Context:** After implementing Decisions 1-7 (PR #2160), the codebase has four
separate `RecipeProvider` implementations, two independent cache systems, and
provider-specific logic scattered across the loader and CLI. The `RecipeProvider`
interface works, but each implementation duplicates satisfies parsing, layout
bucketing, and HTTP client setup. `GetFromSource()` has 50+ lines of per-source-type
switch logic, and the loader uses 5 type assertions to concrete provider types. The
original issue (#2073) called for a manifest format that "could unify how embedded,
central, and local registries are parsed," but that unification wasn't delivered.

**Chosen: Manifest-Driven Single Provider.**

Replace all four concrete providers (`EmbeddedProvider`, `LocalProvider`,
`CentralRegistryProvider`, `DistributedProvider`) with one `RegistryProvider` type
parameterized by:

1. **Manifest** -- declares layout (`flat` or `grouped`), optional `index_url` for
   pre-built recipe indexes, and source identity.
2. **BackingStore interface** -- three implementations:
   - `MemoryStore`: in-memory map from `go:embed` (replaces EmbeddedProvider)
   - `FSStore`: local filesystem reads (replaces LocalProvider)
   - `HTTPStore`: HTTP fetching with built-in disk cache (replaces both
     CentralRegistryProvider and DistributedProvider)

The manifest lives inside the registry directory (e.g., `.tsuku-recipes/manifest.json`).
tsuku probes for `.tsuku-recipes/` first, then `recipes/` as fallback. The manifest
doesn't declare the directory name -- tsuku discovers it.

```json
{
  "layout": "grouped",
  "index_url": "https://tsuku.dev/recipes.json"
}
```

Both fields are optional. No manifest means flat layout, no index.

The central registry's manifest is baked into the binary at compile time (layout:
grouped, index_url: tsuku.dev/recipes.json). The embedded recipes are another baked-in
registry with a `MemoryStore`. Both process through the same `RegistryProvider` code
path as any third-party registry.

`HTTPStore` owns its disk cache because caching is transport-specific -- TTL, eviction,
and conditional requests (ETag/If-Modified-Since) all depend on the HTTP backend. This
replaces both `internal/registry/cache*.go` and `internal/distributed/cache.go`.

The `update-registry` command uses a `CacheIntrospectable` optional interface instead
of casting to `*CentralRegistryProvider`. This matches the existing optional-interface
pattern (`SatisfiesProvider`, `RefreshableProvider`).

This eliminates:
- ~500 lines of duplicated code across providers and cache systems
- All 5 type assertions in the loader
- The 60-line `GetFromSource()` switch (becomes ~5 lines)
- Two separate cache implementations

*Alternative rejected: Layered Storage Abstraction.* Separates storage (byte
fetching) from registry logic with cache as composable middleware. Creates a design
tension: HTTP conditional requests don't fit a generic cache middleware. Either the
middleware becomes transport-aware (defeating its purpose) or HTTPStore handles its
own caching (duplicating the middleware). The extra layer adds complexity without
solving a problem we actually have.

*Alternative rejected: Progressive Extraction.* Keeps four provider types, extracts
shared helpers (satisfies parsing, bucketing). Doesn't deliver a single code path --
`GetFromSource()` switch stays, two cache systems persist, new registry types still
need new provider code. Too conservative for the stated goal given no users and
acceptable breaking changes.

## Decision Outcome

The seven decisions above compose into a layered system:

**Abstraction layer (Decisions 1-2).** A `RecipeProvider` interface with `Get`,
`List`, and `Source` methods replaces the Loader's hardcoded priority chain. The
satisfies index uses an optional `SatisfiesProvider` interface so each provider
can contribute entries efficiently. The Loader iterates providers in priority
order, stopping at the first hit. Its public API stays the same -- the interface
is an internal refactor that existing callers don't see.

**State layer (Decisions 4-5).** A top-level `Source` field on `ToolState`
records where each tool came from. Registry configuration lives in `config.toml`,
cleanly separating "where can I look" (config) from "where did this come from"
(state). Lazy migration defaults existing entries to `"central"`.

**Fetch and cache layer (Decisions 3, 6).** The distributed provider uses a
two-tier HTTP strategy: Contents API for discovery, raw content for file fetches.
Distributed recipes get their own cache tree and `CacheManager` instance, keeping
them isolated from the central registry cache.

**Namespace layer (Decision 7).** Last-install-wins with collision detection.
Tools are keyed by name in a flat namespace, with a confirmation prompt when
replacing a tool from a different source.

## Solution Architecture

### Overview

The design introduces three changes to tsuku's internals:

1. A `RecipeProvider` interface that all recipe sources implement, replacing the
   Loader's hardcoded priority chain with an ordered provider slice.
2. A `Source` field on `ToolState` that records where each installed tool came
   from, enabling source-directed operations (update, verify, outdated).
3. A distributed provider that fetches recipes from GitHub repositories via HTTP,
   with its own caching layer.

These changes are layered: the interface refactor (1) is a standalone improvement
that doesn't change behavior. Source tracking (2) builds on it. The distributed
provider (3) is the new capability.

### Components

**RecipeProvider interface** (`internal/recipe/provider.go`, new file)

```go
// RecipeProvider is a source of recipe TOML data. Providers are ordered
// by priority in the Loader's chain; earlier providers shadow later ones.
type RecipeProvider interface {
    Get(ctx context.Context, name string) ([]byte, error)
    List(ctx context.Context) ([]RecipeInfo, error)
    Source() RecipeSource
}

// SatisfiesProvider is optional. Providers that can cheaply return
// package-name-to-recipe-name mappings implement it for the satisfies index.
type SatisfiesProvider interface {
    SatisfiesEntries(ctx context.Context) (map[string]string, error)
}

// RefreshableProvider is optional. Providers with cached upstream data
// implement it for tsuku update-registry.
type RefreshableProvider interface {
    Refresh(ctx context.Context) error
}
```

**Concrete providers:**

| Provider | Package | Wraps | Source tag | Optional interfaces |
|----------|---------|-------|-----------|-------------------|
| `LocalProvider` | `internal/recipe` | `recipesDir` path | `SourceLocal` | SatisfiesProvider (full parse) |
| `EmbeddedProvider` | `internal/recipe` | `EmbeddedRegistry` | `SourceEmbedded` | SatisfiesProvider (full parse) |
| `CentralRegistryProvider` | `internal/registry` | `*Registry` | `SourceRegistry` | SatisfiesProvider (manifest), RefreshableProvider |
| `DistributedProvider` | `internal/distributed` (new) | HTTP client + cache | `SourceDistributed` (new) | RefreshableProvider |

**Refactored Loader** (`internal/recipe/loader.go`)

```go
type Loader struct {
    providers        []RecipeProvider
    recipes          map[string]*Recipe      // In-memory parsed cache
    constraintLookup ConstraintLookup
    satisfiesIndex   map[string]satisfiesEntry
    satisfiesOnce    sync.Once
}

type satisfiesEntry struct {
    recipeName string
    source     RecipeSource
}
```

The four current chain methods (`GetWithContext`, `loadDirect`, `getEmbeddedOnly`,
`loadEmbeddedDirect`) collapse into one:

```go
func (l *Loader) resolveFromChain(ctx context.Context, providers []RecipeProvider, name string, trySatisfies bool) ([]byte, RecipeSource, error)
```

- `GetWithContext`: calls `resolveFromChain(ctx, l.providers, name, true)`
- `RequireEmbedded`: filters providers to `Source() == SourceEmbedded`, calls
  `resolveFromChain` with the filtered list
- Satisfies fallback: if `resolveFromChain` returns ErrNotFound and
  `trySatisfies` is true, look up the satisfies index and retry with
  `trySatisfies=false` (recursion guard)

Three Loader constructors (`New`, `NewWithLocalRecipes`, `NewWithoutEmbedded`)
become one: `NewLoader(providers ...RecipeProvider)`.

**Interface disambiguation.** Two existing interfaces named `RecipeLoader` exist
in the codebase (`internal/actions/resolver.go:25` and `internal/verify/deps.go:19`).
These are consumer-side interfaces that the `*Loader` already satisfies. They are
unrelated to `RecipeProvider`, which is a supply-side interface that sources
implement. The Loader bridges the two: it accepts `RecipeProvider` implementations
and exposes the `RecipeLoader` consumer API. No changes to consumer interfaces.

**In-memory cache key scheme.** The `recipes` map uses the recipe name as the key.
For distributed sources, qualified names like `owner/repo:foo` could collide with
a central registry recipe named `foo`. The cache key for distributed recipes must
include the source qualifier: `"owner/repo:foo"` is a distinct key from `"foo"`.
The Loader strips the qualifier when passing to the distributed provider's `Get`
method but preserves it for caching.

**Source tracking** (`internal/install/state.go`)

```go
type ToolState struct {
    // ... existing fields ...
    Source string `json:"source,omitempty"` // "central", "embedded", "local", or "owner/repo"
}
```

Lazy migration in `Load()`: entries with empty `Source` get `"central"` by
default. If `Plan.RecipeSource` is available, it's used to infer a more
specific value.

**Registry configuration** (`internal/userconfig/userconfig.go`)

```go
type Config struct {
    // ... existing fields ...
    StrictRegistries bool                       `toml:"strict_registries,omitempty"`
    Registries       map[string]RegistryEntry   `toml:"registries,omitempty"`
}

type RegistryEntry struct {
    URL            string `toml:"url"`
    AutoRegistered bool   `toml:"auto_registered,omitempty"`
}
```

Registries are user configuration ("which sources do I trust"), stored alongside
telemetry and LLM preferences in `$TSUKU_HOME/config.toml`. This separates
configuration (where to look) from state (where things came from).

**Distributed provider** (`internal/distributed/`, new package)

Fetches recipes from GitHub repos containing `.tsuku-recipes/`. Uses a two-tier
HTTP strategy:

1. **Discovery**: GitHub Contents API (`api.github.com/repos/{owner}/{repo}/contents/.tsuku-recipes`)
   lists available TOML files. Auto-resolves default branch. Costs 1 rate-limited
   API request per repo. Returns `download_url` fields pointing to raw content.
2. **Fetch**: Individual files via `raw.githubusercontent.com` URLs from the
   Contents API response. Unlimited requests, no rate limit, no auth needed for
   public repos.

Auth: Uses `GITHUB_TOKEN` via the existing `secrets` package for Contents API
calls (raises limit from 60 to 5000 req/hr). Raw content fetches don't need auth.

HTTP clients: Two separate clients to prevent token leakage:
- **Authenticated client** for Contents API calls (`api.github.com` only). Uses
  `GITHUB_TOKEN` in the `Authorization` header.
- **Unauthenticated client** for raw content fetches. No auth headers. Used for
  `download_url` values after hostname validation (see Security Considerations).

Both clients use `httputil.NewSecureClient` (SSRF protection, DNS rebinding
guards, redirect validation) since user-provided `owner/repo` values influence
URL construction.

Input validation: The `owner/repo` parsing should reuse the existing
`ValidateGitHubURL()` function in `internal/discover/sanitize.go`, which
already handles owner/repo regex validation, path traversal detection, and
credential rejection. Don't build a parallel parser.

### Key Interfaces

**RecipeProvider** (defined above) is the central abstraction. All recipe-consuming
code interacts with providers through the Loader, which exposes the same public API
as today (`Get`, `GetWithContext`, `ListAllWithSource`).

**Source-directed loading**: For operations on already-installed tools (update,
verify, outdated), the Loader gains a `GetFromSource` method:

```go
func (l *Loader) GetFromSource(ctx context.Context, name string, source string) ([]byte, error)
```

This bypasses the priority chain and loads directly from the provider matching
the given source. When `source` is `"central"`, it uses the central registry
provider. When `source` is `"owner/repo"`, it uses the distributed provider
for that repo. `GetFromSource` does not consult or populate the in-memory
`recipes` cache -- it always fetches fresh data from the source.

**Name parsing**: The install command detects distributed sources by the presence
of a `/` in the tool name. The parsing rules:

| Input | Interpretation |
|-------|---------------|
| `ripgrep` | Unqualified: central registry only |
| `ripgrep@1.0` | Unqualified with version pin |
| `owner/repo` | Distributed: single-recipe repo |
| `owner/repo:recipe` | Distributed: specific recipe in multi-recipe repo |
| `owner/repo@1.0` | Distributed with version pin |
| `owner/repo:recipe@1.0` | Distributed, specific recipe, version pin |

### Data Flow

**First install from a distributed source** (`tsuku install owner/repo`):

1. Install command detects `/` in name, identifies as distributed source
2. Check `config.toml` for registered source `owner/repo`
   - If `strict_registries` is on and source is unregistered: error with
     `tsuku registry add` suggestion
   - If source is unregistered and strict mode is off: show confirmation prompt
     ("Installing from owner/repo for the first time. Distributed recipes can
     execute arbitrary commands with your user permissions. Continue? [y/N]").
     Pass `-y` to skip. On confirmation, auto-register.
3. Distributed provider fetches `.tsuku-recipes/` via Contents API
   - Single TOML file: use it
   - Multiple TOML files: error listing available recipes (unless `:recipe-name` given)
   - No `.tsuku-recipes/` directory: error
4. Recipe TOML is parsed and cached in `$TSUKU_HOME/cache/distributed/{owner}/{repo}/`
5. Normal install flow: plan generation, version resolution, download, extract, verify
6. State records `Source: "owner/repo"` on the ToolState entry

**Subsequent update** (`tsuku update <tool>`):

1. Read `ToolState.Source` for the tool
2. Call `loader.GetFromSource(ctx, name, source)` to fetch fresh recipe from the
   recorded source
3. Version provider resolves latest version from the recipe's `[version]` section
4. Normal upgrade flow if a newer version is available

**Cache layout:**

```
$TSUKU_HOME/
  registry/                           # Central registry cache (unchanged)
    a/ansible.toml
    a/ansible.meta.json
    manifest.json
  cache/
    distributed/                      # Distributed recipe cache (new)
      {owner}/{repo}/
        {recipe}.toml
        {recipe}.meta.json
        _source.json                  # Branch name, directory listing, timestamp
```

The central registry cache stays at `$TSUKU_HOME/registry/` (backward compatible).
Distributed sources get a separate tree under `$TSUKU_HOME/cache/distributed/`
with their own `CacheManager` instance and independent size limits.

## Implementation Approach

### Phases 1-5: Distributed Recipes (Complete)

Delivered by PR #2160. See the PLAN doc for details.

### Phase 6: BackingStore interface + MemoryStore + FSStore

Define the `BackingStore` interface. Port embedded and local providers to
`MemoryStore` and `FSStore`. Low risk -- these are the simplest providers with no
caching or HTTP.

### Phase 7: Unified disk cache + HTTPStore

Merge the two cache implementations (`internal/registry/cache*.go` and
`internal/distributed/cache.go`) into one. Build `HTTPStore` with configurable TTL,
size limits, and eviction. Port the central registry provider.

### Phase 8: Port distributed provider + manifest discovery

Port the distributed provider to `HTTPStore`. Add manifest fetching and directory
probing logic (`.tsuku-recipes/` then `recipes/`).

### Phase 9: Loader cleanup

Remove type assertions from loader. Collapse `GetFromSource()`. Move
provider-specific logic from `install_distributed.go` behind the interface. Add
`CacheIntrospectable` optional interface for `update-registry`.

### Original Phases (Reference)

<details>
<summary>Original implementation phases from the distributed recipes design</summary>

### Phase 1: RecipeProvider interface and Loader refactor

Extract the interface. Wrap local, embedded, and central registry sources in
provider adapters. Refactor the Loader to iterate a provider chain. No behavior
change -- this is a pure refactor.

Deliverables:
- `internal/recipe/provider.go` -- interface definitions
- `internal/recipe/provider_local.go` -- LocalProvider
- `internal/recipe/provider_embedded.go` -- EmbeddedProvider
- `internal/registry/provider_central.go` -- CentralRegistryProvider
- `internal/recipe/loader.go` -- refactored to use providers, including
  `warnIfShadows` (currently hardcoded to `l.registry.GetCached()`) refactored
  to detect shadowing across all providers
- Updated tests

### Phase 2: Source tracking in state

Add `Source` field to `ToolState`. Implement lazy migration. Wire source
recording into the install flow so new installs populate the field.

Deliverables:
- `internal/install/state.go` -- Source field + migration
- `cmd/tsuku/helpers.go` -- populate Source during plan generation
- `cmd/tsuku/list.go` -- show source in output

### Phase 3: Registry management

Add registry configuration to `config.toml`. Implement the `tsuku registry`
subcommands (list, add, remove). Implement strict mode.

Deliverables:
- `internal/userconfig/userconfig.go` -- RegistryEntry, StrictRegistries
- `cmd/tsuku/registry.go` -- registry subcommands
- Source-directed loading in `loader.GetFromSource()`

### Phase 4: Distributed provider

Implement the distributed provider with HTTP fetching, caching, and error handling.
Wire it into the Loader as the lowest-priority provider for qualified names.

Deliverables:
- `internal/distributed/provider.go` -- DistributedProvider
- `internal/distributed/github.go` -- GitHub Contents API + raw content fetching
- `internal/distributed/cache.go` -- per-source cache management
- `internal/registry/errors.go` -- new error types if needed

### Phase 5: Command updates

Update remaining commands (info, update, outdated, verify, recipes,
update-registry) for source awareness. This is the long tail of small changes.

Deliverables:
- `cmd/tsuku/info.go` -- show source in output
- `cmd/tsuku/update.go` -- source-directed recipe loading
- `cmd/tsuku/outdated.go` -- check correct source per tool
- `cmd/tsuku/verify.go` -- use cached recipe from source
- `cmd/tsuku/recipes.go` -- list from all registered sources
- `cmd/tsuku/update_registry.go` -- refresh distributed sources

</details>

## Security Considerations

**Trust model.** Distributed recipes can execute arbitrary shell commands via the
`run_command` action (which passes recipe-defined strings to `sh -c`) and have
access to all other actions (cargo_build, pip_install, configure_make, etc.)
with the invoking user's full permissions. This is the same trust model as
`go install`, `cargo install`, or `pip install` from arbitrary sources, but
unlike the central registry where recipes are reviewed via PR.

**Recipe integrity.** v1 does not verify recipe content integrity. Recipes are
fetched from HEAD over HTTPS, which protects against network-level tampering but
not against upstream compromise (account takeover, malicious force-push). Binary
integrity is protected by checksum/signature verification defined in the recipe,
but if the recipe itself is tampered, those verification parameters are also
compromised. Recipe-level integrity (content-hash pinning, change detection) is
a prerequisite for enterprise or high-security use and should be prioritized as
a fast follow.

Implementer requirements:
- Validate that `download_url` values returned by the GitHub Contents API use
  HTTPS **and** come from an allowed hostname (`raw.githubusercontent.com`,
  `objects.githubusercontent.com`). A compromised or spoofed API response could
  point to an arbitrary HTTPS host.
- Use separate HTTP clients for authenticated (Contents API) and unauthenticated
  (raw content) requests. If a single client carries the `Authorization` header
  at the transport level, the token leaks to any `download_url` target.
- Record `sha256(recipe_toml_bytes)` in `state.json` alongside the `Source`
  field. This doesn't block mutation in v1 but creates an audit trail. The
  intended future behavior: on update, if the hash changed, show a diff summary
  and require `--accept-recipe-changes` to proceed.
- Show an interactive confirmation prompt on first install from a new distributed
  source (e.g., "Installing from owner/repo for the first time. Distributed
  recipes can execute arbitrary commands with your user permissions. Continue?
  [y/N]"). Accept `-y` flag to skip for scripted use.

**Manifest index_url.** The `index_url` field in a third-party manifest is
author-declared. tsuku fetches a URL from an untrusted source. Mitigations:
- HTTPS-only (reject HTTP URLs)
- The index provides search metadata and satisfies mappings only, not recipe
  content. Recipe bytes always come from the registry directory itself.
- Satisfies mappings from untrusted indexes could misdirect tool resolution but
  can't inject code (the recipe still comes from the registry)
- A hostname allowlist was considered but deferred. HTTPS-only plus the limited
  scope of index data provides adequate protection for now.

**Strict mode.** Teams and CI environments should set `strict_registries = true`
to prevent auto-registration. Document this in the `tsuku registry` help text
and in the security section of the website.

**Token handling.** `GITHUB_TOKEN` is sent only to `api.github.com` over HTTPS.
Raw content fetches don't include authentication headers. The token is resolved
through the existing `secrets` package, which checks environment variables and
`config.toml` (stored with 0600 permissions).

**Telemetry.** Telemetry events for distributed installs should include an opaque
"distributed" source tag rather than the full `owner/repo` identifier. Full
identifiers reveal user-source relationships to the telemetry backend without
clear analytical benefit.

## Consequences

### Positive

- **Unified abstraction.** All recipe sources share the same interface. Adding
  a new source type (OCI registry, S3 bucket, local git repo) is one interface
  implementation. No Loader modifications needed.
- **Less code in the Loader.** Four chain methods collapse into one. The Loader
  shrinks by ~150 lines net despite gaining provider orchestration logic.
- **Testability.** Mock providers replace real filesystem and HTTP dependencies
  in tests. The three Loader constructors become one that accepts any providers.
- **Source tracking enables source-directed operations.** `tsuku update` checks
  the right source per tool instead of always hitting the central registry.
  `tsuku outdated` can check all sources in parallel.
- **Clean config/state separation.** "Where to look" lives in config.toml.
  "Where this came from" lives in state.json. Neither pollutes the other.

### Negative

- **Migration cost.** The Loader refactor touches ~12-15 files. While the public
  API stays the same, constructors change and tests need updating. Risk of
  subtle behavioral differences during the transition.
- **Contents API rate limits.** Unauthenticated access allows 60 requests/hour.
  A user installing from many distributed sources without `GITHUB_TOKEN` will
  hit this quickly. The error message must guide them to set the token.
- **config.toml loaded in install path.** Currently the install path never reads
  config.toml. Adding registry lookup adds a file read to every install. This
  is a single `os.ReadFile` of a small TOML file, so the cost is negligible,
  but it's a new dependency in a performance-sensitive path.
- **update-registry escape hatch.** The `update-registry` command needs
  type-assertion access to the central registry provider's internals for cache
  management operations. This is an intentional trade-off: the alternative is
  polluting the provider interface with cache-specific methods that most
  providers don't need.
- **GitHub-first assumption.** The distributed provider is GitHub-specific (Contents
  API, raw.githubusercontent.com). Non-GitHub sources require a full HTTPS URL
  via `tsuku registry add` and a different fetch strategy. This is acceptable
  for v1 but means the provider isn't truly generic.

### Mitigations

- **Migration risk**: Phase 1 is a pure refactor with no behavior change.
  Existing tests verify the refactor doesn't break anything before new
  capabilities are added.
- **Rate limits**: Clear error message with `GITHUB_TOKEN` setup instructions.
  The two-tier fetch strategy minimizes API calls (1 per repo for discovery,
  unlimited for content).
- **config.toml in install path**: Lazy loading with in-process caching. Read
  once per session, not per install.
- **update-registry coupling**: Well-documented as an intentional escape hatch.
  Only one command uses it. If future providers need similar operations, a
  `CacheManageable` optional interface can be extracted then.
- **GitHub-first**: The provider interface is generic. GitHub-specific logic is
  encapsulated in the distributed provider implementation. When non-GitHub
  support is needed, it's a new provider, not a change to the interface.

## Implementation Issues

Full plan: [Distributed Recipes](docs/plans/PLAN-distributed-recipes.md) (single-pr mode, 13 issues). All issues completed.

| Issue | Dependencies | Tier |
|-------|--------------|------|
| [#1: refactor(recipe): extract RecipeProvider interface](docs/plans/PLAN-distributed-recipes.md) | None | critical |
| _Extract RecipeProvider interface and refactor Loader from hardcoded four-source chain to ordered provider slice_ | | |
| [#2: feat(state): add source tracking to ToolState](docs/plans/PLAN-distributed-recipes.md) | [#1](docs/plans/PLAN-distributed-recipes.md) | testable |
| _Add Source field to ToolState recording where each tool's recipe came from_ | | |
| [#3: feat(config): add registry configuration and GetFromSource](docs/plans/PLAN-distributed-recipes.md) | [#1](docs/plans/PLAN-distributed-recipes.md) | testable |
| _Add registry config section and source-directed recipe loading_ | | |
| [#4: feat(cli): implement tsuku registry subcommands](docs/plans/PLAN-distributed-recipes.md) | [#3](docs/plans/PLAN-distributed-recipes.md) | testable |
| _Add registry list, add, remove CLI subcommands_ | | |
| [#5: feat(distributed): implement GitHub HTTP fetching and cache](docs/plans/PLAN-distributed-recipes.md) | [#1](docs/plans/PLAN-distributed-recipes.md) | critical |
| _Build GitHub HTTP client with hostname allowlist, auth, rate limiting, and local cache_ | | |
| [#6: feat(distributed): implement DistributedProvider](docs/plans/PLAN-distributed-recipes.md) | [#1](docs/plans/PLAN-distributed-recipes.md), [#5](docs/plans/PLAN-distributed-recipes.md) | testable |
| _Create DistributedProvider implementing RecipeProvider and RefreshableProvider_ | | |
| [#7: feat(install): integrate distributed sources into install flow](docs/plans/PLAN-distributed-recipes.md) | [#2](docs/plans/PLAN-distributed-recipes.md), [#4](docs/plans/PLAN-distributed-recipes.md), [#6](docs/plans/PLAN-distributed-recipes.md) | testable |
| _Wire distributed sources into install command with name parsing and collision detection_ | | |
| [#8: feat(cli): add source-directed loading to update, outdated, verify](docs/plans/PLAN-distributed-recipes.md) | [#2](docs/plans/PLAN-distributed-recipes.md), [#3](docs/plans/PLAN-distributed-recipes.md), [#6](docs/plans/PLAN-distributed-recipes.md) | testable |
| _Make update, outdated, verify use ToolState.Source for provider routing_ | | |
| [#9: feat(cli): add source display to info, list, recipes](docs/plans/PLAN-distributed-recipes.md) | [#2](docs/plans/PLAN-distributed-recipes.md), [#6](docs/plans/PLAN-distributed-recipes.md) | simple |
| _Show source annotations in info, list, and recipes output_ | | |
| [#10: feat(cli): extend update-registry for distributed sources](docs/plans/PLAN-distributed-recipes.md) | [#6](docs/plans/PLAN-distributed-recipes.md) | simple |
| _Add RefreshableProvider refresh loop to update-registry_ | | |
| [#11: feat(koto): create .tsuku-recipes/ in koto repo](docs/plans/PLAN-distributed-recipes.md) | [#6](docs/plans/PLAN-distributed-recipes.md) | simple |
| _Create .tsuku-recipes/ directory in koto with seed recipe_ | | |
| [#12: chore(recipes): migrate koto recipes to distributed](docs/plans/PLAN-distributed-recipes.md) | [#7](docs/plans/PLAN-distributed-recipes.md), [#11](docs/plans/PLAN-distributed-recipes.md) | simple |
| _Move koto recipes from central registry to koto's .tsuku-recipes/_ | | |
| [#13: test(distributed): end-to-end validation](docs/plans/PLAN-distributed-recipes.md) | [#11](docs/plans/PLAN-distributed-recipes.md), [#12](docs/plans/PLAN-distributed-recipes.md) | testable |
| _Full lifecycle validation of distributed install, update, remove_ | | |
