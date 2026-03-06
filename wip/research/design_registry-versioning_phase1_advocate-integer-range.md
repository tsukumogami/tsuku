# Advocate: Integer Version with Range Acceptance

## Approach Description

The manifest carries an integer `schema_version` field (replacing the current semver string). The CLI embeds two constants: `MinSchemaVersion = 1` and `MaxSchemaVersion = N`. On parse, the CLI checks the manifest's version against this range:

- **In range** `[Min, Max]`: parse normally.
- **Above Max**: hard error with "please upgrade tsuku to use this registry" message.
- **Below Min** (or zero/missing): hard error with "registry format too old or corrupt" message.

A separate optional `deprecation` object in the manifest pre-announces migrations:

```json
{
  "schema_version": 1,
  "deprecation": {
    "min_cli_version": "v0.5.0",
    "sunset_date": "2026-06-01",
    "message": "Schema version 1 will be replaced by version 2. Please upgrade tsuku."
  },
  "recipes": [...]
}
```

When the CLI sees a `deprecation` object while the `schema_version` is still in its supported range, it prints a warning to stderr but proceeds normally. This gives users lead time before the registry bumps to the new schema version.

## Investigation

### Existing precedent: discovery registry

The discovery registry in `internal/discover/registry.go` already validates an integer `schema_version`:

```go
// registry.go:52-54
if reg.SchemaVersion != 1 {
    return nil, fmt.Errorf("unsupported discovery registry schema version %d (expected 1)", reg.SchemaVersion)
}
```

This is a strict equality check (`== 1`). The proposed approach extends this to a range check, which is a natural evolution: the discovery registry's pattern proves the integer version concept works, the range just adds forward-compatibility.

### Current manifest gap

The manifest in `internal/registry/manifest.go` declares `SchemaVersion` as a string and never validates it:

```go
// manifest.go:28
type Manifest struct {
    SchemaVersion string           `json:"schema_version"`
    // ...
}
```

The `parseManifest` function (lines 158-163) just unmarshals JSON and returns. No version check at all. The generator (`scripts/generate-registry.py`, line 23) currently emits `"1.2.0"` as a semver string.

This means today, if the registry shipped a breaking schema change, every existing CLI would silently produce wrong results or crash with an unhelpful JSON parse error.

### Cache interaction

When `FetchManifest` succeeds parsing, it caches the raw bytes (line 131). `GetCachedManifest` reads from this cache on next use. The version check in `parseManifest` would gate both paths: a version-incompatible manifest would fail to parse whether freshly fetched or read from cache. This means stale caches with incompatible versions won't silently serve bad data.

### Error infrastructure

The `RegistryError` type in `errors.go` already has `ErrTypeValidation` and the `Suggestion()` method. A new schema-version error fits cleanly here -- either as `ErrTypeValidation` with a schema-specific suggestion, or as a new `ErrTypeSchemaVersion` error type with a fixed suggestion like "Run 'tsuku upgrade' or download the latest version from tsuku.dev".

### Build info availability

`internal/buildinfo/version.go` provides `Version()` for the CLI's own version. The deprecation warning could include this in its message: "Your tsuku version is dev-abc123; the registry recommends at least v0.5.0."

## Strengths

- **Proven pattern in this codebase.** The discovery registry already uses integer `schema_version` with validation. Extending to range acceptance is incremental, not novel. Developers already understand the pattern.

- **Eliminates silent failures today.** The current manifest has zero validation on `schema_version`. Adding a range check to `parseManifest` closes the gap with minimal code. Even before any migration happens, the CLI gains protection against incompatible manifests.

- **Cache safety for free.** Because `parseManifest` is the single entry point for both `FetchManifest` and `GetCachedManifest`, the version check automatically protects against stale cached manifests with incompatible schemas. No separate cache-invalidation logic needed.

- **Deprecation signals enable graceful migration.** The `deprecation` object is purely additive -- old CLIs ignore unknown JSON fields (Go's `json.Unmarshal` behavior). New CLIs that understand it can warn users before the breaking change arrives. This satisfies the migration lifecycle requirement.

- **Simple mental model.** An integer version with a range is easier to reason about than semver compatibility rules, capability negotiation, or content-type headers. Registry operators bump the integer. CLI authors bump MaxSchemaVersion and add parsing logic. Done.

- **Registry independence.** Each registry carries its own `schema_version`. Distributed registries can evolve at their own pace. The CLI's range check is per-manifest, not global.

## Weaknesses

- **Integer-to-string migration for the manifest.** The current manifest `SchemaVersion` is a string (`"1.2.0"`). Changing to an integer requires a coordinated rollout: the generator must emit an integer, and existing CLIs that expect a string won't break (they'll just get `""` or `"0"` from an integer field, which they ignore anyway since there's no validation). But this is a one-time cost, and the fact that the field is currently unvalidated actually makes it safer -- no existing code depends on the string value.

- **No partial compatibility.** An integer version is all-or-nothing per version. If schema version 2 adds three new fields, a CLI supporting only version 1 can't use the two fields it does understand. However, additive field changes don't need a version bump at all (Go's JSON unmarshalling ignores unknown fields), so the integer only needs to increment for genuinely breaking changes. This weakness is more theoretical than practical.

- **Deprecation object has no enforcement.** The `deprecation` object is advisory. Users can ignore warnings indefinitely. The actual enforcement happens when the registry bumps `schema_version`, which is the registry operator's decision. This is arguably a feature (registries control their timeline), but it means the CLI can't force upgrades during the deprecation window.

- **Range gap for very old CLIs.** If a CLI's `MaxSchemaVersion` is 1 and the registry jumps to version 3, the error message says "upgrade tsuku" but doesn't say which version to target. This is solvable by including a `min_cli_version` field in the manifest alongside `schema_version`.

## Deal-Breaker Risks

None identified. The approach is conservative and additive. The main risk (string-to-integer type change) is mitigated by the fact that the current field is completely unvalidated -- no code path reads `Manifest.SchemaVersion` after parsing. The transition can be staged: first release validates the string "1" (or ignores the field), then switch the type to integer with the next schema version.

## Implementation Complexity

### Files to modify

1. **`internal/registry/manifest.go`** -- Change `SchemaVersion` from `string` to `int`, add version range constants, add validation in `parseManifest`, add `Deprecation` struct and warning logic.

2. **`internal/registry/errors.go`** -- Optionally add `ErrTypeSchemaVersion` error type with upgrade suggestion.

3. **`internal/registry/manifest_test.go`** -- Add tests for: version in range, version above range, version below range, missing version, deprecation warning parsing.

4. **`scripts/generate-registry.py`** -- Change `SCHEMA_VERSION` from `"1.2.0"` to `1` (integer).

5. **`internal/discover/registry.go`** -- Optionally refactor strict equality check to range check for consistency.

### New infrastructure

- Two constants: `MinManifestSchemaVersion`, `MaxManifestSchemaVersion`
- One struct: `ManifestDeprecation` with `MinCLIVersion`, `SunsetDate`, `Message` fields
- One function or method for emitting deprecation warnings to stderr

### Estimated scope

Small. Core change is ~50 lines of production code in `manifest.go` plus ~100 lines of tests. The generator change is a one-line edit. Total effort: a few hours, including tests.

## Summary

Integer version with range acceptance is the lowest-risk, lowest-complexity option that directly addresses all the decision drivers. It extends a pattern already proven in this codebase (the discovery registry's integer `schema_version`), closes the current validation gap with minimal code, and gets cache safety for free through the existing `parseManifest` chokepoint. The deprecation object adds graceful migration signaling without breaking old CLIs, since Go's JSON unmarshalling silently ignores unknown fields. The main cost -- migrating `SchemaVersion` from string to integer -- is safe precisely because the field is currently unvalidated.
