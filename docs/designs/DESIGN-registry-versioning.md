---
status: Proposed
problem: |
  The CLI parses the registry manifest with no format compatibility check. The Manifest struct has a SchemaVersion field that's never validated. A breaking registry format change would cause silent parse failures or confusing errors, and old CLI versions have no way to know they're incompatible. There's no mechanism for registries to announce upcoming format changes or guide users through upgrades.
---

# DESIGN: Registry Schema Versioning and Deprecation Signaling

## Status

Proposed

## Context and Problem Statement

The registry manifest (`recipes.json`) includes a `schema_version` field set to `"1.2.0"` by the generation script, but the CLI never reads or validates it. TOML's behavior of silently dropping unknown fields makes additive changes safe, but breaking changes produce no useful signal.

This means:
- A breaking registry format change causes silent parse failures or confusing errors
- Old CLI versions can't detect incompatibility with the current registry
- There's no way to communicate deprecation timelines, upgrade instructions, or migration paths
- Distributed registries (#2073) will face the same problem independently

The discovery registry (`internal/discover/registry.go`) already implements integer schema versioning with hard validation. That pattern works but wasn't applied to the main manifest.

This design defines a protocol where registries announce upcoming format changes in advance, CLIs negotiate what they understand, and neither side breaks the other unilaterally. The expected migration lifecycle:

1. New manifest format is designed
2. New CLI is released supporting both old and new formats
3. Registry starts emitting deprecation signal (still serving old format)
4. Old CLI users see warnings with upgrade instructions
5. Registry migrates to new format
6. Upgraded CLIs work seamlessly; non-upgraded CLIs get an actionable error

## Decision Drivers

- **No silent breakage.** A CLI that can't parse the registry should fail with a clear message, not silently drop data.
- **Registry independence.** Each registry (central or distributed) controls its own migration timeline. No central coordination required.
- **Additive deployment.** The versioning and deprecation mechanism itself must be deployable without breaking old CLIs.
- **Simplicity.** The discovery registry's integer version with hard check is the proven pattern in this codebase. Semver is overkill for schema negotiation.
- **Cache safety.** A version-incompatible stale manifest is worse than no manifest. The stale-if-error fallback needs to be version-aware.

## Considered Options

### Decision 1: Version negotiation mechanism

**Context:** How the CLI detects whether it can parse a registry's manifest format, and how registries signal upcoming format changes.

**Chosen: Integer version with range acceptance.**

The manifest carries an integer `schema_version`. The CLI embeds a supported range `[MinVersion, MaxVersion]`. On parse, `parseManifest()` checks the manifest's version against the range: in-range proceeds normally, above-range returns a hard error with "upgrade tsuku" messaging. A separate optional `deprecation` object in the manifest pre-announces migrations. This directly extends the pattern already proven in the discovery registry (`internal/discover/registry.go:52-53`), works identically for static file registries and future HTTP API registries, and gets cache safety for free because `parseManifest()` is the single chokepoint for all manifest parsing paths.

*Alternative rejected: HTTP version negotiation.* The CLI would send `Accept` headers declaring supported schema versions, and smart registries would serve the appropriate format. Elegant for API-backed registries, but tsuku's central registry is a static file on Cloudflare Pages, and distributed registries will likely be static or git-based too. Every non-HTTP code path (cache reads, local registries, git-based registries) requires manifest-level version checking as a fallback, so the HTTP negotiation layer becomes supplementary complexity on top of the mechanism you must build anyway.

*Alternative rejected: Dual-manifest endpoint.* Versioned URL paths (`/v1/recipes.json`, `/v2/recipes.json`) where the URL IS the version contract. Old CLIs keep hitting their known URL; new CLIs probe upward with 404 fallback. Makes registry independence structural, but adds latency (one extra round-trip per version probe for new-CLI-on-old-registry), requires dual file generation during transitions, creates URL asymmetry with recipe paths, and introduces N*M probing complexity with multiple registries. The manifest-level integer check achieves the same goals with less infrastructure.

## Decision Outcome

The registry manifest will carry an integer `schema_version` field. The CLI will validate this against a compiled-in supported range on every manifest parse. An optional `deprecation` object in the manifest will let registries pre-announce format migrations with timelines and upgrade instructions.

Key properties:
- Integer schema version replaces the current unused semver string `"1.2.0"`
- Version check in `parseManifest()` protects all code paths (fetch, cache, local)
- Deprecation block is additive JSON -- old CLIs ignore it, new CLIs surface warnings
- Each registry (central or distributed) authors its own deprecation independently
- Stale cached manifests that are version-incompatible are treated as unusable
