# Explore Scope: registry-versioning

## Core Question

How should the CLI and registries (central and distributed) negotiate format compatibility, communicate deprecation timelines, and guide users through upgrades -- without silent failures or unilateral breakage from either side?

## Context

The CLI parses `recipes.json` and recipe TOML files with no format compatibility check. The `Manifest` struct declares a `SchemaVersion` field (`internal/registry/manifest.go:28`), and the generation script writes `"1.2.0"`, but the CLI never reads or validates it. TOML silently drops unknown fields, so additive changes are safe, but breaking changes produce no useful signal.

Three recipe sources exist: embedded (frozen at build time), local (`$TSUKU_HOME/recipes`), and remote registry. Embedded recipes are the hardest case -- old binaries can't be updated.

The mechanism must serve both the central registry and future distributed registries (#2073). Registries are the variable; the CLI is always tsuku. Neither side should break the other unilaterally: the CLI shouldn't force registries to upgrade immediately, and registries shouldn't silently break old CLIs.

Related: #1091 (version manifests), #2073 (distributed recipes from remote sources).

## In Scope

- Versioning model for manifest and recipe schemas (breaking vs additive)
- CLI behavior on version mismatch (negotiation, warnings, hard stops)
- Deprecation metadata structure (how registries announce upcoming changes)
- Embedded recipe compatibility (old binaries vs new-format recipes)
- Multi-registry compatibility (central + distributed at different schema versions)
- Upgrade path communication (how the user learns what to do)

## Out of Scope

- Recipe content changes (new fields within recipes -- additive and safe via TOML)
- Distributed registry discovery/install mechanics (#2073)
- Version manifest design (#1091)
- Support for third-party CLI clients (only tsuku CLI)

## Research Leads

1. **How do other package managers handle schema versioning between client and registry?**
   Homebrew taps, Cargo registries, npm, Terraform providers -- what negotiation protocols exist? Which approaches worked, which failed?

2. **What versioning model fits tsuku's registry format?**
   Semver, single integer, min-version? What constitutes a breaking change given TOML's silent field-dropping? Where does the version live (manifest-level, per-recipe, both)?

3. **What should the CLI do at each mismatch severity level?**
   Warn, block, partial-parse? How does this interact with the 4-tier resolution chain (cache, local, embedded, registry)? How are warnings surfaced, how often, and what actionable info do they include?

4. **How should deprecation metadata be structured for registry-agnostic use?**
   What fields (dates, reason, required CLI version, upgrade URL)? Where in the manifest? How do distributed registries author it independently without central coordination?

5. **How do embedded recipes interact with schema evolution?**
   Frozen recipes in the binary vs evolving registry format. Should the binary declare a "supported schema range"? What happens when embedded and registry recipes are at different schema versions?
