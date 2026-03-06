# Exploration Findings: registry-versioning

## Core Question

How should the CLI and registries (central and distributed) negotiate format compatibility, communicate deprecation timelines, and guide users through upgrades -- without silent failures or unilateral breakage from either side?

## Round 1

### Key Insights

- **The discovery registry already implements integer schema versioning with hard validation** (`internal/discover/registry.go:52-53`). The manifest has the same field but never checks it. The proven pattern exists in the codebase. *(versioning-model)*
- **Integer versions are sufficient for schema negotiation.** Semver minor/patch distinction is meaningless for schemas. The manifest's `"1.2.0"` has never been bumped and is the only string-based schema version in the codebase. *(versioning-model, precedent-survey)*
- **`parseManifest()` is the single chokepoint** for all manifest parsing (cached and fresh). Adding a version check there catches all paths. Existing `RegistryError` types and `Suggester` pattern provide the error infrastructure. *(mismatch-behavior)*
- **Deprecation metadata is additive and self-bootstrapping.** An optional `deprecation` object in `recipes.json` works without breaking old CLIs (JSON ignores unknown fields). Each distributed registry authors its own block independently. *(deprecation-metadata)*
- **Embedded recipes are the highest-risk surface** for schema changes (19 bootstrap recipes frozen at build time), but this concern is about recipe format evolution, not registry manifest versioning. *(embedded-compatibility)*
- **Most package managers avoid breaking schema changes entirely.** When forced, declarative version fields + long deprecation timelines (6-12 months) work best. Cargo's integer version with "skip what you don't understand" is the simplest proven pattern. *(precedent-survey)*

### Tensions

- **Stale cache vs version safety.** A version-incompatible stale manifest is worse than no manifest, inverting the "stale is better than nothing" assumption in `CachedRegistry`.
- **CLI version comparison doesn't exist yet.** `buildinfo.Version()` returns strings like `"v0.1.0"` or `"dev"`. Comparing against a `min_cli_version` requires semver parsing not yet in the codebase.

### Gaps

- No self-update mechanism exists. Upgrade path is informational only.
- Multi-registry deprecation UX (when registries have different timelines) is unresolved.

### User Focus

The user clarified the scope: this is about **registry-level schema versioning and migration lifecycle**, not per-recipe versioning. The design should define a protocol where:

1. New CLI is released supporting both old and new manifest formats
2. Registry announces upcoming migration via deprecation metadata (still serving old format)
3. Old CLI users see warnings with upgrade instructions
4. Registry migrates to new format
5. Upgraded CLIs work seamlessly; non-upgraded CLIs get actionable hard errors

Per-recipe schema versioning is explicitly out of scope.

## Accumulated Understanding

The problem is straightforward: the registry manifest has a `schema_version` field that nobody reads, and there's no mechanism for registries to announce format migrations.

**What we know:**
- The manifest needs an integer `schema_version` (not semver), following the discovery registry's proven pattern
- The CLI should accept a range `[1, MAX_SUPPORTED]` and hard-fail outside that range with actionable errors
- A `deprecation` block in the manifest lets registries pre-announce migrations: `deprecated_after`, `removed_after`, `min_cli_version`, `message`, `upgrade_url`
- This is additive JSON -- deploying it doesn't break old CLIs (they ignore unknown fields)
- The check belongs in `parseManifest()`, the single chokepoint for all manifest parsing
- Distributed registries author their own deprecation blocks independently
- Stale cached manifests that are version-incompatible should be treated as unusable
- The CLI needs semver comparison for `min_cli_version` vs `buildinfo.Version()`

**What's settled:**
- Registry-level versioning only (not per-recipe)
- Integer schema version (not semver)
- Deprecation metadata inline in manifest (not separate endpoint)
- Migration lifecycle: new CLI first, then deprecation signal, then migration

**What the design doc needs to define:**
- Exact manifest schema changes (integer version, deprecation block)
- CLI behavior at each state: compatible, deprecated, incompatible
- Interaction with caching (stale-if-error when version-incompatible)
- What constitutes a breaking vs additive manifest change (version bump policy)
- Multi-registry behavior (per-registry deprecation tracking)
- Implementation issues for filing after acceptance

## Decision: Crystallize
