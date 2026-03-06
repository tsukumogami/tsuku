# Lead: How should deprecation metadata be structured?

## Findings

### Current state

The `Manifest` struct (`internal/registry/manifest.go:27-31`) has three fields:

```go
type Manifest struct {
    SchemaVersion string           `json:"schema_version"`
    GeneratedAt   string           `json:"generated_at"`
    Recipes       []ManifestRecipe `json:"recipes"`
}
```

`SchemaVersion` is set to `"1.2.0"` by `scripts/generate-registry.py` (line 23) but **never read or validated by the CLI**. The `parseManifest` function (line 158) does a bare `json.Unmarshal` with no version check. This contrasts with the discovery registry (`internal/discover/registry.go:52-53`), which hard-fails on version mismatch:

```go
if reg.SchemaVersion != 1 {
    return nil, fmt.Errorf("unsupported discovery registry schema version %d (expected 1)", reg.SchemaVersion)
}
```

The CLI exposes its own version via `internal/buildinfo/version.go` (goreleaser-injected `version` var or dev pseudo-version), but no mechanism exists to compare CLI version against registry requirements.

The registry is single-source today: one `BaseURL` resolved from `TSUKU_REGISTRY_URL` env var or `DefaultRegistryURL` constant (`internal/registry/registry.go:20-23`). There's no multi-registry configuration.

### Proposed deprecation metadata structure

Deprecation metadata belongs at the manifest level (top-level field in `recipes.json`), not per-recipe. Reasoning:

1. Schema deprecation is a property of the **format**, not individual recipes.
2. Per-recipe deprecation (e.g., "tool X is abandoned, use Y") is a different concern that belongs in recipe metadata, not the manifest.
3. Distributed registries need a single place to announce format changes.

Proposed fields in a new `deprecation` object:

```json
{
  "schema_version": "1.2.0",
  "generated_at": "2026-03-05T00:00:00Z",
  "deprecation": {
    "deprecated_after": "2026-06-01T00:00:00Z",
    "removed_after": "2026-09-01T00:00:00Z",
    "min_cli_version": "v0.5.0",
    "message": "Schema v1 is deprecated. Update tsuku to v0.5.0+ for schema v2 support.",
    "upgrade_url": "https://tsuku.dev/upgrade"
  },
  "recipes": [...]
}
```

Field definitions:

| Field | Type | Required | Purpose |
|-------|------|----------|---------|
| `deprecated_after` | ISO 8601 datetime | Yes | Date after which the CLI should warn. Soft deadline. |
| `removed_after` | ISO 8601 datetime | No | Date after which the registry may stop serving this schema version. Hard deadline. Omit if no hard cutoff is planned. |
| `min_cli_version` | semver string | Yes | Minimum CLI version that supports the replacement schema. Users need this to know what to upgrade to. |
| `message` | string | Yes | Human-readable explanation. Registries write this independently -- no coordination needed. |
| `upgrade_url` | URL string | No | Optional link to upgrade instructions or release notes. |

### Why this structure works for distributed registries

Each registry authors its own `deprecation` block. No central coordination is needed because:

- The `deprecation` block describes **this registry's** timeline, not a global one.
- A distributed registry can deprecate at its own pace. Registry A might move to schema v2 on June 1; Registry B might stay on v1 indefinitely.
- The CLI evaluates deprecation per-registry, not globally. When fetching from Registry A, it sees A's deprecation notice. When fetching from B, it sees B's (or none).
- The `min_cli_version` field tells users which CLI version to target, regardless of which registry they're talking to.

### CLI behavior on encountering deprecation

The CLI should implement three states based on the current date vs. the deprecation fields:

1. **Before `deprecated_after`**: No message. Normal operation.
2. **Between `deprecated_after` and `removed_after`**: Warning on stderr, once per session (not per command). Print `message` and `upgrade_url` if present. Continue normal operation.
3. **After `removed_after`** (if set): Warning becomes more urgent ("this registry may stop working"). Still attempt to use the data -- the CLI shouldn't refuse to work based on a date alone, since the registry might not have actually removed the old format yet.

The CLI should also compare `min_cli_version` against `buildinfo.Version()`:
- If current CLI >= `min_cli_version`: "You already support the new format. Run `tsuku update-registry` to refresh."
- If current CLI < `min_cli_version`: "Update tsuku to {min_cli_version} or later: {upgrade_url}"

### Placement: manifest-level vs separate endpoint

Two options:

**Option A: Inline in `recipes.json`** (recommended)
- Zero additional HTTP requests.
- The CLI already fetches the manifest; it just needs to read one more field.
- Backward-compatible: old CLIs ignore unknown JSON fields (Go's `json.Unmarshal` silently drops them).
- Distributed registries already generate their own `recipes.json`.

**Option B: Separate `/.well-known/tsuku-deprecation.json` endpoint**
- Cleaner separation of concerns.
- But adds a network request, a new cache file, and a new failure mode.
- Distributed registries would need to serve an additional file.

Option A wins on simplicity. The deprecation block is small (5 fields) and directly relevant to the manifest it accompanies.

### Authoring workflow

For the central registry:
1. Maintainer adds a `DEPRECATION` dict to `scripts/generate-registry.py` constants (alongside `SCHEMA_VERSION`).
2. The script includes it in the generated JSON when non-empty.
3. To announce deprecation: set the fields, merge, deploy. The next `recipes.json` generation includes the notice.
4. To clear deprecation: remove or empty the dict.

For distributed registries:
1. Registry operator adds the `deprecation` block to their manifest generation tooling.
2. No PR to the central repo needed. No coordination required.
3. The `message` field lets each registry explain its own migration path.

### Per-recipe deprecation (separate concern)

Tool-level deprecation ("use `ripgrep` instead of `grep`") is different from schema deprecation. It would live in the `ManifestRecipe` struct or recipe TOML `[metadata]` section, not the manifest-level deprecation block. This lead focuses on schema/format deprecation only.

## Implications

1. **Go struct change required**: Add an optional `Deprecation *DeprecationNotice` field to `Manifest`. Since Go's JSON unmarshaler ignores unknown fields and uses nil for missing pointer fields, old CLIs safely ignore it.

2. **CLI must start reading `SchemaVersion`**: The deprecation mechanism only makes sense if the CLI also validates the schema version. These two features should ship together.

3. **`buildinfo.Version()` becomes load-bearing**: Today it's only used for display (`tsuku --version`). If the CLI compares it against `min_cli_version`, it needs to handle dev builds gracefully (dev versions should be treated as "latest" to avoid annoying developers).

4. **Multi-registry means per-registry deprecation state**: When the CLI supports multiple registries, it needs to track and display deprecation warnings per source. A single global warning wouldn't make sense.

5. **No breaking change to deploy**: Adding `deprecation` to `recipes.json` is additive JSON. Old CLIs ignore it. New CLIs read it. The mechanism is self-bootstrapping.

## Surprises

1. **The discovery registry already validates schema version** (`internal/discover/registry.go:52-53`), but the main manifest does not. This inconsistency means the pattern for version validation exists in the codebase but wasn't applied to the primary manifest path.

2. **`SchemaVersion` is a string (`"1.2.0"`) in the manifest but an int (`1`) in every other schema-versioned struct** (discovery registry, seed queue, batch queue, blocker, dashboard, results). This inconsistency will need resolution -- semver string comparison is more complex than integer comparison.

3. **No mechanism exists to compare CLI version against anything**. `buildinfo.Version()` returns strings like `"v0.1.0"`, `"dev"`, or `"dev-abc1234-dirty"`. Comparing these against a `min_cli_version` requires semver parsing that doesn't exist in the codebase today.

## Open Questions

1. **Should manifest `SchemaVersion` be normalized to an integer like every other schema version in the codebase?** The `"1.2.0"` string format is unique to the manifest and complicates comparison logic. Switching to an integer would align with the rest of the codebase but is itself a (minor) format change.

2. **How should the CLI handle conflicting deprecation signals from multiple registries?** If Registry A says "upgrade by June" and Registry B says "no changes planned", should the CLI show both? Only the most urgent? This becomes relevant once #2073 lands.

3. **Should there be a `tsuku doctor` check for deprecation status?** Currently `tsuku doctor` validates the installation. Adding a "your registry is about to change formats" check would give users a single command to check health.

4. **What about recipe-level `min_cli_version`?** Individual recipes might use features (actions, version providers) that require a newer CLI. This is orthogonal to manifest-level deprecation but would use similar version comparison infrastructure.

5. **How does caching interact with deprecation?** If a user has a cached `recipes.json` from before the deprecation notice was added, they won't see it until `tsuku update-registry`. Should the CLI check for deprecation on a different cadence than the normal cache TTL?

## Summary

Deprecation metadata should be an optional top-level `deprecation` object in `recipes.json` containing `deprecated_after`, `removed_after`, `min_cli_version`, `message`, and `upgrade_url` -- this is additive JSON that old CLIs safely ignore, and each distributed registry can author independently without coordination. The main implication is that the CLI must start actually validating `SchemaVersion` (which it currently ignores despite the field existing since schema 1.2.0) and must gain semver comparison capability to check `min_cli_version` against `buildinfo.Version()`. The biggest open question is whether to normalize the manifest's string-based `SchemaVersion` to an integer to match every other versioned struct in the codebase, since that inconsistency complicates the comparison logic this feature depends on.
