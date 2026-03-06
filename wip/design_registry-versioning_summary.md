# Design Summary: registry-versioning

## Input Context (Phase 0)
**Source:** /explore handoff
**Problem:** The CLI never validates the registry manifest's schema version. Breaking format changes cause silent failures, and there's no mechanism for registries to announce upcoming migrations or guide users to upgrade.
**Constraints:** Must work for both central and distributed registries. Must be additive (deploying the mechanism can't break old CLIs). Integer schema version preferred over semver, following the discovery registry's existing pattern.

## Approaches Investigated (Phase 1)
- Integer version with range acceptance: extends discovery registry's proven pattern, smallest scope, works for all registry types
- HTTP version negotiation: elegant for API registries but requires manifest-level fallback anyway (supplementary complexity)
- Dual-manifest endpoint: versioned URL paths, structural independence, but adds latency and dual generation burden

## Selected Approach (Phase 2)
Integer version with range acceptance. It's the only approach with codebase precedent, works for static and distributed registries without fallback mechanisms, and has the smallest implementation scope. HTTP negotiation and dual-manifest can be layered on top later if tsuku moves to dynamic APIs.

## Investigation Findings (Phase 3)
- Version transition: Go's json.Unmarshal won't coerce string↔int. Need custom UnmarshalJSON on Manifest. Legacy strings map to version 0. Generation script change must lag CLI change by one release.
- Warning UX: No centralized warning system exists. Need printWarning() helper respecting --quiet. Use sync.Once for per-session dedup. Trigger on manifest read, not CLI startup. No semver comparison exists yet -- need minimal parser or golang.org/x/mod/semver.
- Cache interaction: Manifest cache is simpler than expected -- no TTL, no stale-if-error. FetchManifest validates before caching, so incompatible data never reaches disk. Old cache survives server-side upgrades. Per-recipe CachedRegistry is independent and unaffected.

## Current Status
**Phase:** 3 - Deep Investigation
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
