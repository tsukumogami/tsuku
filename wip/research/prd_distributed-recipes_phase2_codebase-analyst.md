# Phase 2 Research: Codebase Analyst

## Lead 3: Recipe Name Conflict Resolution

### Findings

Current recipe loading uses a 4-tier priority chain in `loader.Get()`:
1. In-memory cache
2. Local recipes (`$TSUKU_HOME/recipes`)
3. Embedded recipes (compiled into binary)
4. Registry (remote/cached from tsuku.dev)

Unqualified names are resolved only by recipe name -- the first match in
priority order wins. There's no mechanism to specify "which registry" for a
name. The `satisfies` index provides fallback resolution for dependency names
but doesn't track source origin.

### Implications for Requirements

- Unqualified names (`tsuku install ripgrep`) must prefer the central registry
  for backward compatibility -- existing behavior can't break.
- Distributed sources need explicit qualification via slash notation
  (`owner/repo` or `owner/repo:recipe`) to avoid silent conflicts.
- If a user installs `ripgrep` from central and later a third-party registry
  also has `ripgrep`, the unqualified name should continue resolving to central.
- The PRD should define: when two sources have the same recipe name, is it an
  error, a warning, or silently resolved by priority?

### Open Questions

- Should `tsuku install ripgrep` ever resolve to a distributed source, or only
  when qualified with `owner/repo`?
- If a tool was installed from a distributed source, does `tsuku update ripgrep`
  (unqualified) know to check the original source?

## Lead 5: State Tracking for Distributed Installs

### Findings

Current state.json structure:
- `state.Installed` is a map keyed by tool name (string)
- `Plan.RecipeSource` captures source context but only as "registry" or a file path
- `VersionState` has no source tracking -- only version metadata (binaries,
  checksums, installation time)
- Tool identification uses only the tool name as key -- no field tracks which
  registry the recipe came from
- Backward compatibility is handled via `migrateToMultiVersion()` for
  single-to-multi-version migration

### Implications for Requirements

- state.json must extend to track source identity per installed version, not
  just per tool. A `source` field (e.g., `"tsuku"` for central, `"owner/repo"`
  for distributed) is needed.
- Existing installations should default to `"tsuku"` (central registry) source
  for backward compatibility.
- The source field enables `tsuku update` to check the correct registry for
  updates, and `tsuku list` to display where each tool came from.
- Auto-registration must be idempotent and traceable in state -- the list of
  known registries needs to be persisted somewhere (state.json or separate config).

### Open Questions

- Should the registry list live in state.json or a separate config file?
- What migration strategy for pre-existing installations (assume central registry)?

## Summary

The codebase resolves recipes by name-only priority chain with no source awareness, and state.json lacks fields to track which registry a tool came from. The PRD must require qualified names for distributed sources, central-registry preference for unqualified names, and a source identity field in state.json per installed version. Auto-registration state needs to be persisted alongside (or within) the existing state file.
