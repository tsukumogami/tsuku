# Phase 2 Research: Architecture Perspective

## Lead 2: Minimum Viable Third-Party Registry

### Findings

A single TOML file in `.tsuku-recipes/my-tool.toml` is sufficient for v1. The
existing recipe format already contains all necessary metadata (name, description,
homepage, version section, steps, verify). Tool authors need only provide the
standard recipe sections already understood by the community.

The manifest should be optional:
- **Single-recipe repos**: Just `.tsuku-recipes/tool.toml`. No manifest needed.
  tsuku discovers available recipes by listing TOML files in the directory.
- **Multi-recipe repos**: Optional `manifest.json` in `.tsuku-recipes/` that
  lists available recipes with metadata. Enables richer registry features
  (descriptions, categories, curated subsets).

This parallels Claude Code's marketplace model but with lower friction for the
simple case. Claude Code requires marketplace.json; tsuku should not require it
for single-recipe repos.

The koto recipe (currently in `recipes/k/koto.toml` in the central registry)
uses standard recipe format. Moving it to `tsukumogami/koto/.tsuku-recipes/koto.toml`
would require no format changes -- just relocating the file.

### Implications for Requirements

- The PRD should require that `.tsuku-recipes/*.toml` is the minimum viable
  registry. No manifest, no config, no special setup beyond the TOML file.
- Manifest is optional and adds value for multi-recipe registries (discoverability,
  metadata, curation).
- Tool author friction is: create directory, add TOML file, done. This is the
  lowest possible bar.
- The `.tsuku-recipes/` convention should be documented as a standard that any
  repo can adopt.

### Open Questions

- Should the directory be `.tsuku-recipes/` or `.tsuku/recipes/`? The former is
  more discoverable; the latter nests under a tsuku-specific directory.
- Should tsuku support a recipe at the repo root (no directory convention) for
  single-tool repos?

## Lead 4: @latest Semantics for Distributed Recipes

### Findings

Version resolution is already decoupled from recipe source. The key insight:
there are TWO versions at play for distributed recipes:

1. **Recipe version** -- which version of the recipe definition to use (git ref
   of the registry repo). This determines which TOML file content is active.
2. **Tool version** -- which version of the tool to install. This is controlled
   by the recipe's `[version]` section and its version provider.

The recipe's `[version]` section already handles tool version resolution via
providers (GitHub releases, PyPI, etc.). This works identically whether the
recipe comes from central or distributed sources.

For **recipe version** (`@latest` in `tsuku install owner/repo@latest`):
- `@latest` should mean the latest semver-sorted git tag, with HEAD of default
  branch as fallback if no tags exist.
- `@v1.2.3` pins to a specific tag.
- `@main` (or any branch name) tracks that branch.

This means zero changes to recipe TOML format. The git ref only affects which
recipe definition is fetched, not how the tool version is resolved.

### Implications for Requirements

- The PRD should distinguish recipe version (git ref) from tool version (version
  provider resolution).
- `@latest` = latest semver tag (fallback to HEAD). This provides reproducibility
  via tags while allowing repos without tags to work.
- Version pinning (`@v1.0.0`) is important for reproducibility and enterprise use.
- The version provider architecture is already provider-agnostic, so distributed
  recipes work with existing version resolution. No new version providers needed.

### Open Questions

- Should `tsuku install owner/repo` cache the resolved git ref so that subsequent
  operations use the same recipe version?
- How does `tsuku update` handle recipe version updates vs tool version updates?
  These are independent dimensions.

## Summary

A single `.tsuku-recipes/tool.toml` is sufficient for v1 -- no manifest required for simple repos. Version resolution splits into recipe version (git ref, @latest = latest semver tag) and tool version (existing provider system, unchanged). The recipe TOML format needs zero changes for distributed support; the version provider architecture is already source-agnostic.
