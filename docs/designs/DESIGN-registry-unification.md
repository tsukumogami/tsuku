---
status: Proposed
problem: |
  After implementing distributed recipes (PR #2160), the codebase has four separate
  provider implementations, two independent cache systems, and provider-specific logic
  scattered across the loader and CLI commands. Each registry type (embedded, central,
  local, distributed) uses its own code path for resolution, caching, and layout
  discovery. Adding a new registry type requires modifying multiple files with
  hardcoded type switches instead of extending a single interface.
decision: |
  TBD
rationale: |
  TBD
---

# DESIGN: Registry Unification

## Status

Proposed

## Context and Problem Statement

tsuku supports five registry types today: embedded (18 system library recipes compiled
into the binary), the official central registry (tsukumogami/tsuku, char-grouped layout
fetched via HTTP), local filesystem recipes ($TSUKU_HOME/recipes/), local cache (mirrors
of the central registry), and remote custom registries (configurable third-party GitHub
repos). Each has its own provider implementation, cache implementation, and path
resolution logic.

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

The goal is to collapse all registry types behind a single code path where the central
registry is not special -- it's just another registry whose manifest happens to be
baked into the binary at compile time.

## Decision Drivers

- **Single code path**: All registries must resolve through identical logic. Adding a
  new registry type should mean configuring an existing mechanism, not writing a new
  provider.
- **Manifest-driven discovery**: A manifest inside the registry directory declares
  layout (flat or char-grouped) and optional index URL. tsuku probes for known
  directory names (`.tsuku-recipes/` then `recipes/`).
- **No special cases for the central registry**: The central registry's manifest is
  baked into the binary as a compile-time optimization, but it processes through
  the same code path as any third-party registry.
- **No git dependency**: tsuku remains zero-dependency. HTTP fetching (GitHub Contents
  API + raw content) stays as the transport mechanism.
- **Breaking changes acceptable**: No users yet, so internal API changes, directory
  layout changes, and behavioral changes are all on the table.
- **Embedded recipes as a registry**: The 18 system library recipes are modeled as
  another baked-in registry with in-memory backing, same interface as all others.
- **recipes.json is a website asset, not a CLI dependency**: The CLI uses the
  manifest's optional `index_url` field instead. The central registry's baked-in
  manifest knows the URL; third-party registries can host their own index.
