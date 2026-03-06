---
status: Proposed
problem: |
  The CLI parses the registry manifest with no format compatibility check. The Manifest struct has a SchemaVersion field that's never validated. A breaking registry format change would cause silent parse failures or confusing errors, and old CLI versions have no way to know they're incompatible. There's no mechanism for registries to announce upcoming format changes or guide users through upgrades.
decision: |
  The manifest carries an integer schema_version validated against a compiled-in range [Min, Max] in parseManifest(). An optional deprecation object in the manifest lets registries pre-announce format migrations with timelines, minimum CLI version requirements, and upgrade URLs. The version check gates all manifest parsing paths (fetch, cache, local). Legacy string-format versions are handled via custom UnmarshalJSON during the transition. Deprecation warnings are displayed by the cmd/ layer (not the registry package) using sync.Once for per-session dedup. The existing version.CompareVersions() checks CLI version against min_cli_version.
rationale: |
  Integer versioning follows the pattern already proven by the discovery registry in this codebase. It's simpler than semver (which is overkill for schema negotiation), works identically for static file and HTTP API registries, and gets cache safety for free through the parseManifest() chokepoint. HTTP content negotiation and dual-manifest endpoints were rejected because they add infrastructure complexity on top of the manifest-level check you must build anyway.
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

How the CLI detects whether it can parse a registry's manifest format, and how registries signal upcoming format changes.

#### Chosen: Integer version with range acceptance

The manifest carries an integer `schema_version`. The CLI embeds a supported range `[MinVersion, MaxVersion]`. On parse, `parseManifest()` checks the manifest's version against the range: in-range proceeds normally, above-range returns a hard error with "upgrade tsuku" messaging. A separate optional `deprecation` object in the manifest pre-announces migrations. This directly extends the pattern already proven in the discovery registry (`internal/discover/registry.go:52-53`), works identically for static file registries and future HTTP API registries, and gets cache safety for free because `parseManifest()` is the single chokepoint for all manifest parsing paths.

#### Alternatives Considered

**HTTP version negotiation.** The CLI sends `Accept` headers declaring supported schema versions, and smart registries serve the appropriate format. Rejected because tsuku's central registry is a static file on Cloudflare Pages, and distributed registries will likely be static or git-based too. Every non-HTTP code path (cache reads, local registries, git-based registries) requires manifest-level version checking as a fallback, so the HTTP negotiation layer becomes supplementary complexity on top of the mechanism you must build anyway.

**Dual-manifest endpoint.** Versioned URL paths (`/v1/recipes.json`, `/v2/recipes.json`) where the URL IS the version contract. Old CLIs keep hitting their known URL; new CLIs probe upward with 404 fallback. Rejected because it adds latency (one extra round-trip per version probe for new-CLI-on-old-registry), requires dual file generation during transitions, creates URL asymmetry with recipe paths, and introduces N*M probing complexity with multiple registries. The manifest-level integer check achieves the same goals with less infrastructure.

### Decision 2: Legacy version transition strategy

The `Manifest.SchemaVersion` field is currently a `string` (`"1.2.0"`), but the new design needs an `int`. Go's `json.Unmarshal` is strict about type matching -- it won't coerce a JSON string into an int field or vice versa. Users have cached `manifest.json` files with the string format, and old CLIs in the wild expect it. The transition must not break either direction: new CLI reading old cache, or old CLI reading new manifest.

#### Chosen: Custom `UnmarshalJSON` on `Manifest`

Implement `UnmarshalJSON` on the `Manifest` struct. The public field stays `SchemaVersion int`. The custom unmarshaler decodes `schema_version` into an `interface{}`, then type-switches: `float64` (JSON number) converts to `int`, `string` (legacy format like `"1.2.0"`) maps to version 0 (pre-versioning era). This keeps the public API clean, handles both formats transparently, and requires no caller changes since `parseManifest()` just calls `json.Unmarshal` which invokes the custom method automatically.

The generation script change (`"1.2.0"` to `1`) must lag the CLI change by at least one release. If the script emits an integer before users have the new CLI, old CLIs fail with no recovery path. Sequence: Release N ships the dual-type unmarshaler (script still emits string), Release N+1 changes the script to emit integer.

#### Alternatives Considered

**`json.Number` field type.** `json.Number` accepts JSON numbers but not JSON strings when used as a struct field with default `json.Unmarshal`. It would handle the new integer format but fail on the old string `"1.2.0"` -- the opposite of what backward compatibility requires. Rejected because it solves the wrong direction.

**`interface{}` field with accessor method.** Store `SchemaVersionRaw interface{}` and provide a `SchemaVersion() (int, error)` method. This works but exposes a raw field on the struct, requires every access point to call the method instead of reading a field, and leaks the transition concern into the public API. Rejected because the custom `UnmarshalJSON` achieves the same result without API degradation.

**Immediate type change (no transition).** Change the field to `int` and the generation script simultaneously. Users with cached string-format manifests get a parse error, recoverable by running `tsuku update-registry`. Rejected because old CLIs (which expect a string field) would hard-fail on the new integer manifest with no recovery path except upgrading the CLI -- exactly the kind of silent breakage this design aims to prevent.

### Decision 3: Warning trigger point

When a manifest contains a `deprecation` object, the CLI needs to surface a warning. The question is where in the code path this happens, and how to avoid spamming the user with repeated warnings.

#### Chosen: Manifest parse stores, `cmd/` layer displays with `sync.Once`

`parseManifest()` parses and stores the `Deprecation` field on the struct but does not write to stderr -- it stays in `internal/registry`, which shouldn't own display concerns. The `cmd/` layer checks `manifest.Deprecation` after a successful parse and calls `printWarning()` if present. A `sync.Once` ensures the warning fires at most once per CLI invocation, even if multiple code paths read the manifest. This covers both `update-registry` (which fetches fresh) and recipe-using commands (which read from cache), without adding latency to commands that don't touch the manifest (like `--help` or `--version`).

#### Alternatives Considered

**Trigger at CLI startup.** Check the cached manifest's deprecation state during `main.go` initialization. Rejected because it adds I/O latency to every command, including `--help` and `--version`, which never need the manifest. It also creates a dependency between CLI startup and the registry cache.

**Trigger inside `parseManifest()` directly.** Have the parsing function write to stderr when it encounters a deprecation block. Rejected because `parseManifest()` lives in `internal/registry`, and writing to stderr from a library-level parsing function violates the display boundary. It also makes the warning impossible to suppress with `--quiet` (which is a `cmd/`-level flag) without threading CLI flags into the registry package.

**Trigger on first recipe load in the Loader.** Check deprecation state when the Loader initializes or on first recipe access. This covers all recipe-using commands but misses `update-registry`, which is the most natural place for users to see the warning. Rejected because it requires plumbing manifest access into the Loader's initialization path, and the `cmd/` layer already has access to the manifest after fetch or cache read.

### Decision 4: Semver comparison for `min_cli_version`

The deprecation notice includes `min_cli_version` so the CLI can tell users whether they need to upgrade. Comparing `buildinfo.Version()` (e.g., `"v0.3.0"`) against a semver string like `"v0.5.0"` requires version parsing.

#### Chosen: Reuse `version.CompareVersions()`

The codebase already has `version.CompareVersions()` (`internal/version/version_utils.go`) which handles semver with `v` prefix stripping, prerelease ordering, and calver formats. The `Masterminds/semver/v3` library is also already a dependency, used by multiple version providers for sorting. For the deprecation check, `CompareVersions(buildinfo.Version(), minCLIVersion)` gives us exactly what we need with no new code. Dev builds (`dev-*`, `dev`, `unknown`) skip the comparison entirely and are treated as current -- developers running from source are assumed to be on the latest code.

#### Alternatives Considered

**Hand-rolled minimal parser.** Strip `v` prefix, split on `.`, compare integers. Rejected because it duplicates logic that `CompareVersions` already handles, including edge cases like prerelease ordering and prefix normalization.

**Direct use of `Masterminds/semver/v3`.** Call `semver.NewVersion()` and use its comparison methods directly. This would work but adds a direct coupling to the library in `internal/buildinfo/` or `cmd/`. Using the existing `CompareVersions` wrapper keeps the version comparison logic centralized in `internal/version/`.

## Decision Outcome

The manifest carries an integer `schema_version` validated against a compiled-in `[Min, Max]` range in `parseManifest()`. Legacy string-format versions are handled transparently via a custom `UnmarshalJSON` on the `Manifest` struct, mapping old strings to version 0. The generation script migrates from string to integer one release after the CLI ships with the dual-type unmarshaler, so neither old CLIs nor new CLIs ever see a type they can't handle.

An optional `deprecation` object in the manifest lets registries pre-announce format migrations. The `internal/registry` layer parses and stores the deprecation data; the `cmd/` layer displays it as a stderr warning via a new `printWarning()` helper that respects `--quiet`. Warnings fire once per CLI invocation using `sync.Once`. Before displaying upgrade instructions, the CLI compares `buildinfo.Version()` against the deprecation's `min_cli_version` using the existing `version.CompareVersions()`, and adjusts the message accordingly -- either "upgrade to vX.Y" or "your CLI already supports the new format."

Key properties:
- Integer schema version replaces the current unused semver string `"1.2.0"`
- Version check in `parseManifest()` protects all code paths (fetch, cache, local)
- Deprecation block is additive JSON -- old CLIs ignore it, new CLIs surface warnings
- Each registry (central or distributed) authors its own deprecation independently
- Stale cached manifests that are version-incompatible are treated as unusable
- Two-release rollout ensures neither direction of the string-to-int transition breaks

## Solution Architecture

### Overview

The manifest gains two new capabilities: a validated integer schema version and an optional deprecation notice. The version check gates all manifest parsing. The deprecation notice surfaces warnings before a registry migrates formats.

### Manifest Schema Changes

The `schema_version` field changes from an ignored semver string to a validated integer. A new optional `deprecation` object is added.

**Current format:**
```json
{
  "schema_version": "1.2.0",
  "generated_at": "2026-03-05T00:00:00Z",
  "recipes": [...]
}
```

**New format:**
```json
{
  "schema_version": 1,
  "generated_at": "2026-03-05T00:00:00Z",
  "deprecation": {
    "sunset_date": "2026-09-01",
    "min_cli_version": "v0.5.0",
    "message": "This registry will adopt schema v2 on 2026-09-01. Update tsuku to v0.5.0+.",
    "upgrade_url": "https://tsuku.dev/upgrade"
  },
  "recipes": [...]
}
```

The `deprecation` object is optional. When absent (the normal state), no warnings are shown. Fields:

| Field | Type | Required | Purpose |
|-------|------|----------|---------|
| `sunset_date` | string (YYYY-MM-DD) | Yes | Date after which the registry may stop serving this schema version |
| `min_cli_version` | string (semver) | Yes | Minimum CLI version that supports the replacement schema |
| `message` | string | Yes | Human-readable explanation. Each registry writes its own. |
| `upgrade_url` | string (URL) | No | Link to upgrade instructions or release notes |

### Components

**`Manifest` struct** (`internal/registry/manifest.go`): `SchemaVersion` changes from `string` to `int`. New `Deprecation *DeprecationNotice` pointer field (nil when absent). Custom `UnmarshalJSON` handles the string-to-integer transition: legacy string values (like `"1.2.0"`) map to version 0.

**Version constants** (`internal/registry/manifest.go`): `MinManifestSchemaVersion = 0` and `MaxManifestSchemaVersion = 1`. Version 0 represents pre-versioning manifests (the legacy string era). Version 1 is the first explicitly versioned format. Future breaking changes increment `MaxManifestSchemaVersion`.

**`parseManifest()` validation** (`internal/registry/manifest.go`): After JSON unmarshal, checks `manifest.SchemaVersion` against `[Min, Max]`. Above-range returns a new `RegistryError` with `ErrTypeSchemaVersion` (dedicated error type, not overloading `ErrTypeValidation`) and a suggestion to upgrade. The suggestion also mentions `tsuku update-registry` for the CLI-downgrade case where the cache has a higher version than the CLI supports. `parseManifest()` only validates and stores the `Deprecation` field on the struct -- it does not write to stderr. Warning display is the caller's responsibility.

**`DeprecationNotice` struct** (`internal/registry/manifest.go`): Holds `SunsetDate`, `MinCLIVersion`, `Message`, `UpgradeURL`. Parsed from the manifest's `deprecation` JSON object.

**Warning display** (`cmd/tsuku/helpers.go`): New `printWarning()` helper that writes to stderr and respects `--quiet`. After a successful manifest parse, the `cmd/` layer checks `manifest.Deprecation` and calls `printWarning()` if present. Warnings fire once per CLI invocation via `sync.Once`. The warning identifies the source registry by URL (the actual fetch URL, not a hardcoded default) so users know which registry issued the notice. When multi-registry support ships (#2073), the `sync.Once` should become per-registry dedup.

**Downgrade prevention rule:** Before displaying deprecation warnings, the CLI compares `buildinfo.Version()` against `min_cli_version`. If the current CLI version is >= `min_cli_version`, it shows "your CLI already supports the new format." If below, it shows "upgrade to vX.Y." The CLI never renders a message suggesting a version older than the running one -- this prevents a malicious registry from using `min_cli_version` to suggest downgrading.

**Semver comparison**: Uses `version.CompareVersions()` (`internal/version/version_utils.go`) to check `buildinfo.Version()` against `min_cli_version`. This function already handles `v` prefix stripping, semver ordering, and prerelease comparison. Dev builds (`dev-*`, `dev`, `unknown`) skip the comparison and are treated as current.

### Data Flow

```
Registry serves recipes.json with schema_version + optional deprecation
  |
  v
FetchManifest() fetches raw bytes
  |
  v
parseManifest() -> json.Unmarshal (custom UnmarshalJSON handles string/int)
  |
  +-- schema_version out of [Min, Max] range?
  |     -> Return RegistryError with upgrade suggestion
  |     -> Raw bytes NOT cached (FetchManifest validates before caching)
  |     -> Old compatible cache on disk survives untouched
  |
  +-- schema_version in range, deprecation present?
  |     -> Store deprecation info, warn once per session on stderr
  |     -> Compare min_cli_version against buildinfo.Version()
  |     -> Include actionable message: "upgrade to vX.Y" or "you're already current"
  |
  +-- schema_version in range, no deprecation?
        -> Normal operation, no user-visible change
```

### Key Interfaces

**Version check error:**
```go
&RegistryError{
    Type:    ErrTypeSchemaVersion,
    Recipe:  "manifest",
    Message: fmt.Sprintf("registry uses schema version %d, but this CLI supports versions %d-%d",
        manifest.SchemaVersion, MinManifestSchemaVersion, MaxManifestSchemaVersion),
}
```

The `Suggestion()` method returns: `"Update tsuku to the latest version, or run 'tsuku update-registry' to refresh the cache. See https://tsuku.dev/upgrade"`

**Deprecation warning (stderr):**
```
Warning: Registry at https://tsuku.dev reports: This registry will adopt schema v2 on 2026-09-01.
  Update tsuku to v0.5.0 or later: https://tsuku.dev/upgrade
```

If the user's CLI already meets `min_cli_version`:
```
Warning: Registry at https://tsuku.dev reports: This registry will adopt schema v2 on 2026-09-01.
  Your CLI (v0.5.0) already supports the new format. Run 'tsuku update-registry' after the migration.
```

The registry URL shown is the actual fetch URL (from `manifestURL()`), not a hardcoded default.

## Implementation Approach

### Phase 1: Version validation infrastructure

Add the version check to `parseManifest()` with custom `UnmarshalJSON` on `Manifest`. This ships with the generation script still emitting `"1.2.0"` (string), which the custom unmarshaler maps to version 0. The CLI starts validating but nothing changes for users since version 0 is in range.

Deliverables:
- `Manifest.SchemaVersion` type change from `string` to `int`
- Custom `UnmarshalJSON` on `Manifest` (accepts both string and int)
- Version range constants and validation in `parseManifest()`
- New `ErrTypeSchemaVersion` error type with dedicated upgrade suggestion
- Test updates (9 locations across `manifest_test.go` and `satisfies_test.go`)
- New tests for: integer version parsing, legacy string parsing, out-of-range rejection

### Phase 2: Deprecation notice support

Add the `DeprecationNotice` struct, parsing, and warning display. Add `printWarning()` helper. Use existing `version.CompareVersions()` for `min_cli_version` checking.

Deliverables:
- `DeprecationNotice` struct and `Deprecation *DeprecationNotice` field on `Manifest`
- `printWarning()` in `cmd/tsuku/helpers.go` (stderr, respects `--quiet`)
- `sync.Once` warning dedup per CLI invocation
- Dev build detection for skipping `min_cli_version` comparison
- Tests for deprecation parsing, warning display, dev build handling

### Phase 3: Generation script migration

After at least one CLI release ships with the custom unmarshaler, change the generation script to emit integer `schema_version: 1`.

Deliverables:
- `scripts/generate-registry.py`: change `SCHEMA_VERSION = "1.2.0"` to `SCHEMA_VERSION = 1`
- Verify deployed `recipes.json` emits integer

### Phase 4 (optional): Cleanup

After sufficient adoption of the new CLI, remove the string-handling path from the custom `UnmarshalJSON`. This is optional since the string path has zero cost.

## Security Considerations

### Download verification
Not applicable. This design changes manifest parsing logic, not download or binary verification. The existing checksum and signature verification paths are unaffected.

### Execution isolation
Not applicable. No new code execution paths are introduced. The version check and deprecation warning are pure data parsing and string formatting.

### Supply chain risks
**Moderate consideration.** The `deprecation` object includes a `message` and `upgrade_url` authored by each registry independently. A compromised or malicious registry could use these fields to direct users to a fake upgrade page.

Mitigation: The CLI should not auto-open URLs. The `upgrade_url` is displayed as text only. Users must copy-paste it themselves. The warning message is clearly labeled as coming from the registry, not from tsuku itself:

```
Warning: Registry at <actual-fetch-url> reports: <message>
```

The URL shown is the actual fetch URL (from `manifestURL()`), not hardcoded. This ensures that if `TSUKU_MANIFEST_URL` or `TSUKU_REGISTRY_URL` is overridden, the warning correctly attributes the message to the actual source rather than giving a malicious override the trust halo of the default registry.

Additionally, the CLI never renders a deprecation message that suggests installing a version older than the running one. The `min_cli_version` comparison happens before displaying any upgrade instructions.

For the central registry, the message originates from the tsuku maintainers (trusted). For distributed registries, users have already opted into trusting that source.

### User data exposure
Not applicable. No new data is collected or transmitted. The deprecation check is purely local: parse a JSON field, compare against compiled-in constants, print to stderr.

## Consequences

### Positive
- Breaking registry format changes produce clear, actionable errors instead of silent failures
- Registries can pre-announce migrations, giving users months of warning
- Distributed registries get the same mechanism for free (it's in the manifest they already serve)
- The version check protects all code paths (fetch, cache, local) via the `parseManifest()` chokepoint
- Incompatible manifests are never cached, so old compatible caches survive server-side upgrades

### Negative
- The string-to-integer migration for `SchemaVersion` requires a two-release rollout (CLI change first, generation script second)
- Dev build detection needs special handling to skip the `min_cli_version` comparison (dev builds don't follow semver)
- The `deprecation.message` is registry-authored free text, which could be used for phishing in a malicious distributed registry
- A compromised registry could serve `schema_version: 0` with altered recipe data, and it would pass validation and be cached. This isn't a new attack vector (the same works today without versioning), but the version check doesn't prevent it. A future ratchet mechanism (refuse to cache a schema version lower than the highest previously seen) could close this gap.

### Mitigations
- Two-release rollout: the custom `UnmarshalJSON` handles both formats transparently, so users never see an error during transition
- Semver comparison: reuses existing `version.CompareVersions()` with a dev-build guard. No new parsing code needed.
- Phishing risk: the CLI labels the message source ("Registry at X reports: ...") and never auto-opens URLs. This matches the trust model users already accepted by adding the registry.
