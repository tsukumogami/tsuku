---
status: Proposed
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

Proposed

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

A Go interface with `Get`, `List`, `Source`, and `Priority` methods, implemented
by adapters wrapping each existing source. The Loader holds an ordered
`[]RecipeProvider` slice and iterates it, replacing the current sequence of
`if` blocks.

This approach was selected because it delivers the PRD's unified abstraction goal
directly. The pattern already exists in the codebase: `EmbeddedRegistry` has
`Get(name)`, `List()`, and `Has(name)` -- it's one method signature away from a
formal interface. The version provider system in `internal/version/` uses the same
pluggable pattern. Formalizing it eliminates ~300 lines of duplicated chain logic
across three Loader methods (`GetWithContext`, `loadDirect`, `getEmbeddedOnly`)
and makes adding future source types a single implementation. Each provider
controls its own URL construction and caching strategy, so distributed repos'
flat `.tsuku-recipes/` layout doesn't conflict with the central registry's
bucketed `recipes/{letter}/{name}.toml` structure.

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

## Decision Outcome

The distributed recipes system will be built on a RecipeProvider interface that
all recipe sources implement. The Loader's priority chain becomes a configurable
list of providers rather than hardcoded conditionals.

Key properties:
- Each source type (local, embedded, central registry, distributed) implements
  the same interface with `Get`, `List`, and `Source` methods
- The Loader iterates providers in priority order, stopping at the first hit
- Distributed sources are a new provider that fetches `.tsuku-recipes/*.toml`
  from GitHub repos via HTTP
- Per-provider caching with independent TTLs and cache directories
- Source tracking flows naturally from which provider answered the request
- The Loader's public API (`Get`, `GetWithContext`) stays the same -- the
  interface is an internal refactor that existing callers don't see

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
