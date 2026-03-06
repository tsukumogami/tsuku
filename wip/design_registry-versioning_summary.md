# Design Summary: registry-versioning

## Input Context (Phase 0)
**Source:** /explore handoff
**Problem:** The CLI never validates the registry manifest's schema version. Breaking format changes cause silent failures, and there's no mechanism for registries to announce upcoming migrations or guide users to upgrade.
**Constraints:** Must work for both central and distributed registries. Must be additive (deploying the mechanism can't break old CLIs). Integer schema version preferred over semver, following the discovery registry's existing pattern.

## Current Status
**Phase:** 0 - Setup (Explore Handoff)
**Last Updated:** 2026-03-05

## Key Exploration Findings

### Codebase State
- `Manifest.SchemaVersion` exists (`manifest.go:28`) but is never validated
- Discovery registry (`discover/registry.go:52-53`) already uses integer version with hard check
- `parseManifest()` is the single chokepoint for all manifest parsing
- Existing `RegistryError` types and `Suggester` pattern provide error infrastructure
- `buildinfo.Version()` returns CLI version but no semver comparison exists yet
- Stale-if-error fallback in `CachedRegistry` assumes stale data is usable

### Design Decisions Pre-Resolved
- Registry-level versioning only (not per-recipe)
- Integer schema version (not semver string)
- Deprecation metadata inline in manifest (not separate endpoint)
- Migration lifecycle: new CLI first, then deprecation signal, then migration
- Each distributed registry authors its own deprecation block independently

### Research Files
- `wip/research/explore_registry-versioning_r1_lead-precedent-survey.md`
- `wip/research/explore_registry-versioning_r1_lead-versioning-model.md`
- `wip/research/explore_registry-versioning_r1_lead-mismatch-behavior.md`
- `wip/research/explore_registry-versioning_r1_lead-deprecation-metadata.md`
- `wip/research/explore_registry-versioning_r1_lead-embedded-compatibility.md`
