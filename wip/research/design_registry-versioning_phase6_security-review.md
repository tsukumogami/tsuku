# Security Review: Registry Schema Versioning and Deprecation Signaling

**Design:** `docs/designs/DESIGN-registry-versioning.md`
**Reviewer:** architect-reviewer (security focus)
**Date:** 2026-03-05

---

## 1. Attack Surface Analysis

### 1.1 Deprecation message as phishing vector (IDENTIFIED IN DESIGN)

The design correctly identifies that `deprecation.message` and `upgrade_url` are registry-authored free text that a malicious distributed registry could use to direct users to fake upgrade pages.

**Design's mitigation:** CLI labels the source ("Registry at X reports: ..."), never auto-opens URLs.

**Assessment: Mitigation is adequate for the current trust model.** Users who add a distributed registry have already extended trust to that source. The CLI correctly treats the URL as display-only text. The source labeling is the right approach.

**Gap:** The design says "Registry at tsuku.dev reports: ..." but doesn't specify what happens when `TSUKU_REGISTRY_URL` or `TSUKU_MANIFEST_URL` is set to an arbitrary URL. The source label should show the actual URL being used, not a hardcoded string. If the label says "tsuku.dev" but the manifest came from an env-var override, the attribution is misleading.

### 1.2 Deprecation-driven version downgrade attack (NOT CONSIDERED)

**Severity: Moderate. Needs design-level attention.**

A compromised registry could add a `deprecation` block with `min_cli_version` set to an old, vulnerable CLI version. The warning message would say something like "Downgrade to v0.2.0 for compatibility." The design has no constraint that `min_cli_version` must be greater than the current version. The field name implies "minimum version that supports the new schema," but nothing enforces this semantically.

The practical risk is low because:
- The deprecation block is informational only (no enforcement)
- Users must manually act on the message
- The central registry is maintainer-controlled

But for distributed registries, this becomes a social engineering vector. The message could include a convincing reason to downgrade.

**Recommendation:** Add a validation rule: if `min_cli_version` is older than the currently running CLI version, the deprecation message should say "Your CLI (vX.Y.Z) already supports the new format" (the design already describes this path). Ensure this comparison always runs, not just as an optional display variation. Never display an instruction to downgrade.

### 1.3 Schema version downgrade via cached manifest poisoning (NOT CONSIDERED)

**Severity: Low but worth documenting.**

The design states "incompatible manifests are never cached." But consider this sequence:
1. Attacker briefly controls the registry (or DNS)
2. Serves a valid manifest with `schema_version: 0` (legacy format) and malicious recipe entries
3. This is within the supported range `[0, 1]`, so it passes validation and gets cached
4. Attacker stops serving; the CLI uses the cached malicious manifest

This is not unique to this design -- it exists today. But the version negotiation creates a false sense of security: the version check protects against *future* incompatible formats, not against rollback to older valid formats.

**Recommendation:** Document this as a known limitation. A future enhancement could track the highest schema version seen and refuse to cache a lower version (ratchet mechanism). Not blocking for this design.

### 1.4 Manifest size/complexity bomb (NOT CONSIDERED)

**Severity: Low.**

The `deprecation.message` field is unbounded free text. A malicious registry could set it to a very large string. The current `FetchManifest` uses `io.ReadAll`, and the HTTP client already has `DisableCompression: true` and timeouts. JSON parsing will consume the full message into memory.

**Recommendation:** Consider a length limit on `deprecation.message` (e.g., 1024 characters) during parsing. Truncate with "..." if exceeded. This is minor but prevents degenerate cases.

### 1.5 Integer overflow on schema_version (NOT CONSIDERED)

**Severity: Negligible.**

The custom `UnmarshalJSON` will parse `schema_version` as an `int`. A malicious manifest could send `"schema_version": 999999999999999999999` which would cause a JSON parsing error (integer overflow). This is handled correctly by the error path -- the manifest won't parse and won't be cached.

No action needed.

## 2. "Not Applicable" Justification Review

### 2.1 Download verification: "Not applicable"

**Verdict: Correctly marked.** The design modifies manifest parsing only. Download checksums and binary verification are separate code paths in the action system. No interaction.

### 2.2 Execution isolation: "Not applicable"

**Verdict: Correctly marked.** Version checking and deprecation warnings are pure data operations (JSON parse, integer compare, string format). No new exec, no new file writes beyond existing cache paths.

### 2.3 User data exposure: "Not applicable"

**Verdict: Correctly marked.** The deprecation check is purely local. No telemetry events are added for deprecation detection. The `min_cli_version` comparison uses `buildinfo.Version()` which is a compiled-in value, not user data.

## 3. Trust Model for Distributed Registries

### 3.1 Current trust model

The current codebase uses a single registry (`DefaultRegistryURL` or env override). There's no multi-registry support yet -- #2073 is future work. The design correctly designs for it by making deprecation per-manifest.

### 3.2 Trust escalation via deprecation

**Concern:** When multiple registries exist, a less-trusted registry could use the deprecation mechanism to influence user behavior regarding the central registry. For example, a distributed registry's deprecation message could say "The central tsuku registry is migrating. Update at [malicious-url]."

**Current mitigation:** Source labeling ("Registry at X reports: ...") disambiguates the source.

**Assessment: Adequate for now.** The design doesn't implement multi-registry, so cross-registry confusion is theoretical. When #2073 lands, the deprecation display should clearly scope each warning to its source registry. The design already describes this approach.

### 3.3 No signature verification on deprecation block

The deprecation block is unsigned. For the central registry (served over HTTPS from tsuku.dev), TLS provides authenticity. For future distributed registries, there's no mechanism to verify that the deprecation notice was authored by the registry's legitimate maintainer vs. a MITM or compromised mirror.

**Assessment: Acceptable.** This matches the trust model for the rest of the manifest (recipes are also unsigned). Adding signing to just the deprecation block would be inconsistent. If manifest signing is added later, it should cover the entire manifest including deprecation.

## 4. Stale Cache Interaction

### 4.1 Version-aware stale fallback (PARTIALLY ADDRESSED)

The design states: "Stale cached manifests that are version-incompatible are treated as unusable." This is correct for the forward case (cached v1, server upgraded to v2, CLI only supports v1 -- cache v1 remains usable).

**Potential issue:** The stale-if-error fallback in `CachedRegistry.handleStaleFallback` currently returns cached content without re-validating. After this design lands, `parseManifest()` adds version validation, but `GetCachedManifest()` also calls `parseManifest()`, so cached manifests are always validated on read. This means a cached manifest that was valid when cached but is now out-of-range (because `MinManifestSchemaVersion` was raised in a CLI upgrade) will correctly fail validation.

**Assessment: The design's `parseManifest()` chokepoint handles this correctly.** No gap.

### 4.2 Race condition during two-phase rollout

During Phase 1-3, some users will have CLIs that map `"1.2.0"` to version 0, while the generation script still emits the string. If a user downgrades their CLI to a pre-design version during this window, the cached integer-format manifest would fail to parse (the old CLI expects a string). The old `parseManifest()` would return an error, and the old `FetchManifest()` would not cache the result.

**Assessment: This is the correct behavior.** The old CLI would fetch the old-format manifest from the server (which still serves the string format during phase 1-2) and work fine. No gap.

## 5. Residual Risks

| Risk | Severity | Escalation Needed? |
|------|----------|-------------------|
| Deprecation message used for phishing in distributed registries | Low | No -- mitigated by source labeling and user trust model |
| `min_cli_version` used to suggest downgrade | Moderate | No -- add validation rule (never suggest downgrade) |
| Schema version rollback via brief registry compromise | Low | No -- document as known limitation |
| Unbounded `deprecation.message` length | Low | No -- add optional length cap |
| No manifest signing | Low | No -- consistent with current trust model; future concern |

## 6. Summary

The design's security analysis correctly identifies the primary supply chain risk (phishing via deprecation message) and provides an adequate mitigation (source labeling, no auto-open). The "not applicable" markings for download verification, execution isolation, and user data exposure are all accurate.

**Two findings that should be addressed before implementation:**

1. **Source attribution accuracy (Section 1.1):** The deprecation warning must display the actual registry URL being used, not a static string. When `TSUKU_MANIFEST_URL` is overridden, the label must reflect the real source.

2. **Downgrade suggestion prevention (Section 1.2):** Add a hard rule: if the running CLI version meets or exceeds `min_cli_version`, never display the upgrade instruction -- only display "your CLI already supports the new format." If the running CLI version is *below* `min_cli_version`, display the upgrade path. Never suggest downgrading.

**One finding to document but not block on:**

3. **Schema version rollback (Section 1.3):** A future ratchet mechanism (refuse to cache lower schema versions than previously seen) would add defense-in-depth. Not urgent because the overall manifest is equally vulnerable to rollback today.
