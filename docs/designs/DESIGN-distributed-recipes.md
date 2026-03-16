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
  TBD -- to be filled after approach selection.
rationale: |
  TBD -- to be filled after approach selection.
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
