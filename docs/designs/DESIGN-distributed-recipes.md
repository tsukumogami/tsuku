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
