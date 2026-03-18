---
status: Proposed
problem: |
  After implementing distributed recipes (PR #2160), the codebase has four separate
  provider implementations, two independent cache systems, and provider-specific logic
  scattered across the loader and CLI commands. Each registry type uses its own code
  path for resolution, caching, and layout discovery. Adding a new registry type
  requires modifying multiple files with hardcoded type switches instead of extending
  a single interface.
decision: |
  Replace all four provider types with a single RegistryProvider configured by a
  manifest (layout, index URL) and a BackingStore interface (MemoryStore, FSStore,
  HTTPStore). The central registry and embedded recipes become instances with baked-in
  manifests. Caching lives inside HTTPStore where transport-specific logic belongs.
rationale: |
  A layered storage approach was considered but adds a cache middleware abstraction
  that creates design tension with HTTP conditional requests (ETag/If-Modified-Since)
  without solving a real problem. Progressive extraction was too conservative for the
  stated goal. The single-provider approach delivers full unification with the
  simplest abstraction.
---

# DESIGN: Registry Unification

## Status

Proposed

## Context and Problem Statement

tsuku supports five registry types today: embedded (18 system library recipes compiled
into the binary), the official central registry (tsukumogami/tsuku, char-grouped layout
fetched via HTTP), local filesystem recipes (`$TSUKU_HOME/recipes/`), local cache
(mirrors of the central registry), and remote custom registries (configurable
third-party GitHub repos). Each has its own provider implementation, cache
implementation, and path resolution logic.

The distributed recipes feature (PR #2160) added a fifth type but followed the existing
pattern of creating a separate provider rather than unifying the architecture. This left
the codebase with:

- Four `RecipeProvider` implementations sharing duplicated logic (satisfies parsing,
  layout bucketing, HTTP client setup)
- Two completely separate cache implementations (`internal/registry/` and
  `internal/distributed/cache.go`) with different eviction strategies but identical
  architecture
- `GetFromSource()` in the loader with 50+ lines of hardcoded per-source-type switch
  logic
- 230+ lines of provider-specific code in `cmd/tsuku/install_distributed.go` that
  should live behind the provider interface
- Type assertions casting to concrete provider types instead of using the interface
- No support for local `.tsuku-recipes/` directories despite the design calling for it

The goal: collapse all registry types behind a single code path where the central
registry isn't special. It's just another registry whose manifest happens to be baked
into the binary at compile time.

## Decision Drivers

- **Single code path**: All registries resolve through identical logic. Adding a new
  registry type means configuring an existing mechanism, not writing a new provider.
- **Manifest-driven discovery**: A manifest inside the registry directory declares
  layout (flat or char-grouped) and optional index URL. tsuku probes for known
  directory names (`.tsuku-recipes/` first, then `recipes/`).
- **No special cases for the central registry**: The central registry's manifest is
  baked into the binary as a compile-time optimization, but processes through the same
  code path as any third-party registry.
- **No git dependency**: tsuku stays zero-dependency. HTTP fetching (GitHub Contents
  API + raw content) remains the transport mechanism.
- **Breaking changes acceptable**: No users yet, so internal API changes, directory
  layout changes, and behavioral changes are all on the table.
- **Embedded recipes as a registry**: The 18 system library recipes are modeled as
  another baked-in registry with in-memory backing, same interface as all others.
- **recipes.json is a website asset, not a CLI dependency**: The CLI uses the
  manifest's optional `index_url` field instead. The central registry's baked-in
  manifest knows the URL; third-party registries can optionally host their own index.

## Considered Options

### How should the four provider types be unified?

#### Chosen: Manifest-Driven Single Provider

Replace all four concrete provider types (`EmbeddedProvider`, `LocalProvider`,
`CentralRegistryProvider`, `DistributedProvider`) with one `RegistryProvider` struct
parameterized by:

1. **Manifest** -- declares layout (flat vs grouped), index URL, source identity
2. **BackingStore** -- interface for byte fetching with three implementations:
   - `MemoryStore` (replaces EmbeddedProvider)
   - `FSStore` (replaces LocalProvider)
   - `HTTPStore` (replaces both CentralRegistryProvider and DistributedProvider)

Each registry becomes an instance of `RegistryProvider`. The loader stops caring about
provider types. No type assertions, no per-source switch statements.

This approach was chosen because it directly addresses the root cause: four separate
implementations of the same "get recipe bytes by name" operation. The BackingStore
interface cleanly separates "what to fetch" from "how to fetch it," and the existing
optional-interface pattern (`SatisfiesProvider`, `RefreshableProvider`) already shows
the codebase can handle capability differences without concrete type assertions.

#### Rejected: Layered Storage Abstraction

Separate storage from registry logic with cache as composable middleware. This adds a
`Storage` interface, a `CachingStorage` middleware wrapping any backend, and a
`Registry` type on top.

Rejected because the composable cache middleware creates a design tension with HTTP
conditional requests (ETag/If-Modified-Since). Either the middleware becomes
transport-aware (defeating its purpose) or `HTTPStore` handles its own caching
(duplicating the middleware). The extra abstraction layer doesn't solve a problem we
actually have -- we don't need to cache filesystem reads or swap caching strategies.

#### Rejected: Progressive Extraction

Keep four provider types, extract shared helpers (satisfies parsing, bucketing, cache
interface). Low risk, incremental.

Rejected because it doesn't deliver the stated goal. `GetFromSource()` switch stays,
two cache systems persist, adding new registry types still requires new provider code
and loader modifications. For a codebase with no users and acceptable breaking changes,
this does less than it should.

## Decision Outcome

### Manifest Schema

A manifest declares how a registry is organized. It lives inside the registry
directory (e.g., `.tsuku-recipes/manifest.json`):

```json
{
  "layout": "grouped",
  "index_url": "https://tsuku.dev/recipes.json"
}
```

- `layout`: `"flat"` (default) or `"grouped"` (first-letter subdirectories)
- `index_url`: optional URL to a pre-built recipe index for search and satisfies

Both fields are optional. No manifest means flat layout, no index.

### Registry Directory Probing

tsuku probes for known directory names when discovering a registry:
1. `.tsuku-recipes/` (preferred)
2. `recipes/` (fallback, for backward compatibility with tsukumogami/tsuku)

The manifest doesn't declare the directory name. tsuku discovers it.

### RegistryProvider Structure

```go
type RegistryProvider struct {
    name     string        // "central", "embedded", "local", "owner/repo"
    source   RecipeSource
    manifest Manifest      // layout + optional index_url
    store    BackingStore
}

type BackingStore interface {
    Get(ctx context.Context, path string) ([]byte, error)
    List(ctx context.Context) ([]string, error)
}
```

`Get` resolves layout internally: flat layout passes `name.toml`, grouped layout
passes `f/fzf.toml`. The store just fetches bytes at the given path.

### Backing Store Implementations

| Store | Backs | Cache | Notes |
|-------|-------|-------|-------|
| `MemoryStore` | Embedded recipes | None (in-memory) | Populated from `go:embed` at compile time |
| `FSStore` | Local recipes, local cache | None (reads disk directly) | `$TSUKU_HOME/recipes/` |
| `HTTPStore` | Central registry, distributed registries | Disk cache with TTL | Handles ETags, rate limits, conditional requests internally |

`HTTPStore` owns its disk cache because caching is transport-specific. TTL, eviction
strategy, and conditional request headers all depend on the HTTP backend. This replaces
both `internal/registry/cache*.go` and `internal/distributed/cache.go` with a single
implementation parameterized by TTL, size limit, and eviction strategy.

### Baked-In Registries

Two registries have their manifests compiled into the binary:

**Central registry** (tsukumogami/tsuku):
- Directory: `recipes/`
- Layout: `grouped`
- Index URL: `https://tsuku.dev/recipes.json`
- Store: `HTTPStore` with 24h TTL, 50MB cache

**Embedded recipes** (18 system libraries):
- Layout: `flat`
- No index URL (satisfies parsed from recipes at load time)
- Store: `MemoryStore` populated from `go:embed`

Both process through the exact same `RegistryProvider` code path. The only difference
is that tsuku doesn't fetch their manifests because it already has them.

### What Changes in the Loader

- `resolveFromChain()` stays as-is (already generic)
- `GetFromSource()` collapses from 60 lines to ~5: find provider by source, call Get
- All 5 type assertions in `loader.go` are eliminated
- `install_distributed.go` shrinks significantly (provider management moves behind the
  interface)
- Satisfies resolution uses a single implementation: read from index if available,
  parse recipes as fallback

### Optional Interfaces

Some capabilities aren't universal. The existing pattern continues:

- `SatisfiesProvider`: registries with an index or parseable recipes
- `RefreshableProvider`: HTTP-backed registries that support cache refresh
- `CacheIntrospectable` (new): for `update-registry` command to inspect cache stats

The `update-registry` command asserts on `CacheIntrospectable` instead of casting to
`*CentralRegistryProvider`. This is less clean than zero assertions but matches a
pattern the codebase already uses.

## Solution Architecture

### Component Diagram

```
Loader
  |
  +-- RegistryProvider (embedded)
  |     manifest: {layout: flat}
  |     store: MemoryStore
  |
  +-- RegistryProvider (local)
  |     manifest: {layout: flat}
  |     store: FSStore($TSUKU_HOME/recipes/)
  |
  +-- RegistryProvider (central)
  |     manifest: {layout: grouped, index_url: tsuku.dev/recipes.json}
  |     store: HTTPStore(cache: $TSUKU_HOME/registry/, ttl: 24h)
  |
  +-- RegistryProvider (owner/repo)
        manifest: {layout: flat}  // fetched from repo
        store: HTTPStore(cache: $TSUKU_HOME/cache/distributed/owner/repo/, ttl: 1h)
```

### Data Flow: Recipe Resolution

1. Loader iterates providers in priority order (local > embedded > central > distributed)
2. Each `RegistryProvider.Get(name)` computes path from manifest layout
3. Calls `store.Get(path)` -- store handles caching, HTTP, or memory lookup
4. First provider to return bytes wins

### Data Flow: Manifest Discovery (New Registries)

1. User runs `tsuku install owner/repo:tool`
2. tsuku probes `owner/repo` for `.tsuku-recipes/manifest.json` via GitHub Contents API
3. Falls back to `recipes/manifest.json` if not found
4. Falls back to no manifest (flat layout, no index) if neither exists
5. Creates `RegistryProvider` with discovered manifest and `HTTPStore` backend

## Implementation Approach

### Phase 1: BackingStore interface + MemoryStore + FSStore

Define the interface. Port embedded and local providers. Low risk since these are
the simplest providers with no caching or HTTP.

### Phase 2: Unified disk cache + HTTPStore

Merge the two cache implementations into one. Build HTTPStore with configurable TTL,
size limits, and eviction. Port central registry provider.

### Phase 3: Port distributed provider + manifest discovery

Port the distributed provider to HTTPStore. Add manifest fetching and directory
probing logic. This is where the GitHub Contents API client gets wrapped.

### Phase 4: Loader cleanup + install_distributed simplification

Remove type assertions from loader. Collapse `GetFromSource()`. Move provider-specific
logic from `install_distributed.go` behind the interface.

## Security Considerations

### Download verification

No change from current behavior. Recipes are fetched over HTTPS. Binary downloads
within recipes use checksums declared in the recipe TOML.

### Execution isolation

No change. The manifest only affects recipe discovery, not execution.

### Supply chain risks

The `index_url` field in third-party manifests is author-declared. tsuku follows a
URL from an untrusted manifest. Mitigations:

- HTTPS-only (reject HTTP URLs)
- The index is used for search and satisfies mappings only, not for recipe content.
  Recipe bytes always come from the registry directory itself.
- Satisfies mappings from untrusted indexes could direct users to install the wrong
  tool for a given command, but can't inject arbitrary code (the recipe still comes
  from the registry)

A hostname allowlist was considered but deferred -- HTTPS-only plus the limited scope
of index data (search metadata, not executable content) provides adequate protection
for now.

### User data exposure

No change. tsuku doesn't transmit user data. HTTP requests to registries reveal which
tools the user is looking for (same as today).

## Consequences

### Positive

- Adding a new registry type requires one `BackingStore` implementation, zero loader
  changes
- ~500 lines of duplicated code eliminated across providers and cache systems
- All type assertions removed from the loader
- `GetFromSource()` shrinks from 60 lines to ~5
- Local `.tsuku-recipes/` support becomes possible (FSStore pointed at a local
  directory with a manifest)

### Negative

- Large refactor touching core data paths (~15-20 files). Risk of subtle regressions
  in recipe resolution or caching behavior.
- `update-registry` command still needs an interface assertion for cache introspection,
  though it's now an interface assertion rather than a concrete type cast.
- Embedded provider's `go:embed` initialization is a compile-time artifact that doesn't
  perfectly fit the "configured at runtime" model. The MemoryStore constructor papers
  over this but it's a slight abstraction mismatch.

### Mitigations

- Phased implementation reduces blast radius. Each phase is independently testable.
- Existing test suite (`go test ./...`) validates behavior preservation at each step.
- No users means regressions in edge cases won't break anyone.
