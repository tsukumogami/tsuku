# Lead: Unified manifest schema

## Findings

### Current Schema is Central-Registry-Specific

The current manifest.go schema (schemaVersion, generatedAt, deprecation notice,
recipes array with name/description/homepage/dependencies) is tightly coupled to
the central registry's distribution needs. It includes schema versioning and
deprecation notices that serve HTTP-based registry evolution, not individual
recipe sources.

### Per-Recipe vs Registry-Level Concerns

To unify embedded recipes (simple TOML in binary), repo `.tsuku-recipes/`
directories (local filesystem), local user recipes ($TSUKU_HOME/recipes), and the
central registry under one abstraction, the schema must separate two layers:

**Per-recipe metadata** (needed everywhere):
- name, description, homepage
- dependencies, runtime_dependencies
- satisfies (ecosystem aliasing)

**Registry-level metadata** (only for remote/distributed registries):
- schema_version
- deprecation notices
- generated_at timestamp

### Format Agnosticism

Recipes can remain format-agnostic (TOML internally, JSON externally) while the
manifest becomes a registry envelope that's optional for embedded/local sources
but required for remote/third-party registries.

### The satisfies Field

The `satisfies` field (ecosystem aliasing, e.g. `bat` satisfies `cargo:bat`)
currently lives in the manifest recipe entries. Third-party repos may not need
this -- it's primarily useful for the central registry's discovery integration.

## Implications

The unified schema should be two layers:
1. A `RecipeMetadata` layer extracted from individual recipe TOML files (always present)
2. A `RegistryManifest` envelope wrapping recipe metadata with registry-level
   concerns (optional, only for remote registries)

This means a `.tsuku-recipes/` directory in a repo could work with just TOML
files and no manifest at all, while the central registry continues using its
JSON manifest for HTTP distribution.

## Surprises

The satisfies field creates an interesting tension -- it's a registry concern
(how does this recipe relate to ecosystem packages?) but it's stored per-recipe.
Third-party repos probably don't need it, which suggests it belongs in the
registry envelope, not the recipe itself.

## Open Questions

Whether to make `satisfies` part of the minimal per-recipe schema or keep it as
a registry-level optimization. Third-party repos may not need ecosystem aliasing.

## Summary

The current manifest schema is tightly coupled to central registry distribution needs (schema versioning, deprecation notices) and should be split into per-recipe metadata (always present) and a registry envelope (optional, for remote registries). This means a `.tsuku-recipes/` directory could work with just TOML files and no manifest, while the central registry keeps its JSON manifest. The biggest open question is whether `satisfies` belongs per-recipe or in the registry envelope.
