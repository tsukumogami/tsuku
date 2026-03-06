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
