# Architecture Review: Registry Schema Versioning and Deprecation Signaling

## Summary

The design is architecturally sound. It extends an existing pattern (discovery registry integer versioning), places validation at the correct chokepoint (`parseManifest`), and avoids introducing parallel patterns. The phased rollout is correctly sequenced. Below are findings organized by severity.

## Findings

### Blocking: None

The design introduces no structural violations. It follows the established patterns for error handling (`RegistryError` + `Suggester`), version validation (integer check, matching `internal/discover/registry.go:52-53`), and places all logic in the correct package (`internal/registry`).

### Advisory

#### 1. Deprecation warning display crosses a package boundary

**Location:** Design section "Warning display" -- `cmd/tsuku/helpers.go`

The design proposes `printWarning()` in `cmd/tsuku/helpers.go` with `sync.Once` to deduplicate warnings. However, `parseManifest()` lives in `internal/registry/manifest.go` -- an internal package that should not print to stderr. The design says the deprecation notice "triggers a warning" during manifest parse, but who calls `printWarning()`?

Two options exist, both architecturally clean:
1. `parseManifest()` returns the `DeprecationNotice` as data on the `Manifest` struct (as designed). The **caller** in `cmd/tsuku/` checks `manifest.Deprecation != nil` and calls `printWarning()`. This keeps `internal/registry` free of UI concerns.
2. `parseManifest()` returns both the manifest and a warning struct that the caller can choose to display.

The design's struct-level approach (storing `Deprecation *DeprecationNotice` on `Manifest`) already enables option 1. The implementation just needs to be explicit that `internal/registry` never writes to stderr -- the `cmd/tsuku/` layer does. The existing `fmt.Fprintf(os.Stderr, ...)` at `manifest.go:133` for cache warnings is a minor precedent, but it's a different concern (operational logging vs. user-facing deprecation notices).

**Recommendation:** Clarify that `parseManifest()` only stores the deprecation data. The `sync.Once` warning trigger belongs in `cmd/tsuku/` code that calls `FetchManifest()` / `GetCachedManifest()`, not in the parse function.

#### 2. `sync.Once` placement needs care for multiple registries

**Location:** Design section "Warning display"

The design says "warn once per CLI invocation via `sync.Once`." With distributed registries (#2073), a single CLI invocation might parse manifests from multiple registries, each with different deprecation notices. A single `sync.Once` would suppress all but the first.

**Recommendation:** Use per-registry warning dedup (keyed by registry URL) rather than a global `sync.Once`. Or document that global dedup is intentional and only the first deprecation warning surfaces. This isn't blocking because distributed registries aren't shipped yet, but the design should state the intent.

#### 3. Semver comparison in `internal/buildinfo/` may be better as a standalone utility

**Location:** Design section "Semver comparison"

The design proposes adding semver comparison to `internal/buildinfo/`. This package currently has one responsibility: returning the version string. Adding comparison logic makes it a version utility package. The ~20 lines of code are fine, but `internal/buildinfo/` is imported by `cmd/tsuku/main.go` and changing its scope could invite more functionality to accumulate there.

**Recommendation:** Either keep the comparison in `internal/buildinfo/` (acceptable given the small scope) or put it adjacent to the `DeprecationNotice` struct in `internal/registry/manifest.go` as a method like `notice.CLIUpdateNeeded(currentVersion string) bool`. The latter keeps the concern collocated with its only consumer.

#### 4. `RegistryError.Suggestion()` for `ErrTypeValidation` will change meaning

**Location:** `internal/registry/errors.go:82-83`

Currently, `ErrTypeValidation` returns: "The request was rejected by the registry. Check that the tool name is valid." The design proposes reusing `ErrTypeValidation` for schema version errors with a different suggestion ("Update tsuku to the latest version"). These are semantically different errors sharing one type.

The design mentions `ErrTypeSchemaVersion` as an alternative (Phase 1 deliverables), which would be cleaner. A dedicated error type avoids overloading the suggestion for existing validation errors.

**Recommendation:** Use a new `ErrTypeSchemaVersion` constant. It's one line to add and avoids changing the meaning of existing error types.

## Architecture Fit Assessment

### Correct decisions

1. **`parseManifest()` as the chokepoint.** This is the right place. Both `FetchManifest()` and `GetCachedManifest()` flow through it. The cache-before-validate ordering in `FetchManifest()` (validate at line 125, cache at line 131) means incompatible manifests never reach disk. No new code paths needed.

2. **Custom `UnmarshalJSON` for string/int migration.** Go's JSON unmarshaler is the idiomatic way to handle this. The `json.Number` or type-switch approach keeps the transition invisible to all callers of `parseManifest()`.

3. **Version 0 for legacy manifests.** Mapping `"1.2.0"` to version 0 is pragmatic. It means the range `[0, 1]` covers both legacy and new formats without special-casing. When version 0 is eventually dropped, only `MinManifestSchemaVersion` changes.

4. **Additive deprecation block.** Old CLIs ignore unknown JSON fields, so adding `deprecation` to the manifest is safe. No coordination needed with existing deployments.

5. **Phase sequencing.** Phase 1 (CLI change) before Phase 3 (generation script change) is correct. Any CLI released before Phase 1 will see the integer `schema_version` as `0` (Go zero value for int), which would be in range `[0, 1]` -- wait, this is only true if those old CLIs have the custom unmarshaler. Old CLIs without the change will try to unmarshal an integer into a `string` field and get an empty string. This is safe because old CLIs never read `SchemaVersion` anyway. The two-phase rollout is sound.

### Missing pieces (minor)

1. **No mention of `GetCachedManifest` behavior on version error.** The design covers `FetchManifest` clearly (don't cache incompatible data). But `GetCachedManifest` reads from an existing cache file. If a user downgrades their CLI, the cached manifest might have a higher schema version than the old CLI supports. `parseManifest()` would reject it, and `GetCachedManifest` would return an error. This is correct behavior, but the design doesn't call it out. The user would need to run `tsuku update-registry` or delete the cache manually. Consider whether the error message should suggest this.

2. **No test strategy for the string-to-integer transition window.** The design lists test deliverables but doesn't mention testing the specific scenario where the generation script has already switched to integer but the user has an old cached manifest with a string version. The custom `UnmarshalJSON` handles this, but an explicit test for "cached string manifest + fresh integer manifest in same session" would catch regressions.

3. **`upgrade_url` validation.** The design notes the phishing risk but doesn't specify whether `upgrade_url` should be validated as a URL (e.g., must start with `https://`). A malicious registry could put `javascript:` or other scheme URLs. Since the URL is displayed as text only, the risk is low, but URL scheme validation is trivial and worth adding.

## Questions from the review prompt

### 1. Is the architecture clear enough to implement?

Yes. The component list, data flow diagram, and phased deliverables give an implementer enough to start. The one ambiguity is the warning display ownership (advisory #1 above).

### 2. Are there missing components or interfaces?

No missing components. The design correctly reuses `RegistryError`, `Suggester`, and existing error infrastructure. The `DeprecationNotice` struct and version constants are the only new types needed.

### 3. Are the implementation phases correctly sequenced?

Yes. Phase 1 (validation infrastructure) must ship before Phase 3 (generation script change). Phase 2 (deprecation support) is independent of Phase 3 and can be done in parallel or after. Phase 4 (cleanup) is correctly marked optional.

### 4. Are there simpler alternatives we overlooked?

No. The design already considered and rejected HTTP negotiation and dual-manifest endpoints. The chosen approach (integer version in manifest, range check in `parseManifest()`) is the simplest option that solves the problem. It mirrors the discovery registry pattern, which is proven.

### 5. Does the data flow make sense?

Yes. The flow is: fetch raw bytes -> `parseManifest()` (unmarshal + version check) -> cache only if valid -> return manifest with optional deprecation data -> caller displays warning. Each step has a single responsibility.

### 6. Are edge cases covered (version 0 manifests and deprecation)?

Mostly. The string-to-0 mapping covers legacy manifests. The design doesn't explicitly address:
- CLI downgrade (cached manifest has higher version than CLI supports) -- handled correctly by `parseManifest()` rejection, but error messaging could be more specific
- Multiple deprecation notices from different registries in one invocation (advisory #2)
- Malformed `deprecation` object (e.g., missing required fields) -- should this be a hard error or silently ignored? Given it's advisory metadata, silently ignoring malformed deprecation seems right.
