# Security Review: Channel-aware Version Resolution

**Design**: DESIGN-channel-aware-resolution.md
**Date**: 2026-03-29
**Scope**: Cache poisoning, version string injection, pin-level derivation bypass, residual risks

---

## 1. Attack Surface Summary

The design introduces three new code paths:

1. **PinLevelFromRequested()** -- derives pin level from the `Requested` string in state.json
2. **ResolveWithinBoundary()** -- routes version resolution through cached list or fuzzy prefix matching
3. **CachedVersionLister.ResolveLatest()** change -- derives latest from cached ListVersions instead of network call

All three operate on data from two trust boundaries: (a) the local `state.json` file and (b) the `$TSUKU_HOME/cache/versions/` cache files, both of which are user-writable files at standard permissions (0644).

---

## 2. Cache Poisoning of the ListVersions Cache

### 2.1 Current State

Cache files are JSON in `$TSUKU_HOME/cache/versions/<hash>.json` written with 0644 permissions. The cache directory is created with 0755. Any process running as the same user can modify these files.

### 2.2 Attack with This Design

**Pre-design**: A poisoned ListVersions cache only affects `tsuku versions` display. `ResolveLatest` bypasses the cache entirely (line 87-89 of cache.go: direct delegation to underlying provider).

**Post-design**: `CachedVersionLister.ResolveLatest()` will derive its result from the cached list. `ResolveWithinBoundary()` will filter this same cached list for pin-constrained queries. A poisoned cache now directly controls what version gets installed during `tsuku update`.

This is a meaningful escalation. Before the design, cache poisoning was cosmetic. After, it becomes an installation vector.

### 2.3 Concrete Scenario

1. Attacker writes a malicious version to the cache file (e.g., replaces "18.20.4" with "18.20.4-backdoored" or inserts a version "18.999.0" that points to a compromised release)
2. User runs `tsuku update node`
3. `ResolveWithinBoundary` reads the poisoned cache, filters to "18.*", and returns the attacker's version
4. Download proceeds to the version URL constructed from the poisoned version string

### 2.4 Mitigation Assessment

The design doc claims: "mitigated by the existing checksum verification during download." This is partially correct but incomplete:

- **Recipes with checksums per version**: If the recipe has static checksums for each version in a `[checksums]` table, a fake version string won't match any checksum and the install fails safely. This is the happy path.
- **Recipes with dynamic URLs but no per-version checksums**: Many recipes use URL templates like `https://github.com/{{.repo}}/releases/download/v{{.version}}/...` with no checksum table. The version string is interpolated into the URL. If the attacker-controlled version string happens to resolve to a real (but unintended) release, download succeeds with no checksum failure.
- **GitHub provider specifics**: GitHub releases are real artifacts. An attacker who can write to the local cache cannot create fake GitHub releases. But they can reorder the list to make `ResolveWithinBoundary` pick an older, vulnerable version instead of the latest patch.

**Verdict**: The existing checksum mitigation is insufficient for the expanded attack surface. The design should acknowledge that cache poisoning now affects installation (not just display) and note that the risk is bounded by same-user-privilege (an attacker with write access to `$TSUKU_HOME` could also modify binaries directly).

### 2.5 Recommendations

- **R1**: Add an integrity check to cache entries. A lightweight approach: include an HMAC of the version list using a per-installation random key stored in `$TSUKU_HOME/state.json` (or a separate key file). This prevents casual tampering without adding cryptographic complexity. Lower priority since same-user access is already a compromised boundary.
- **R2**: At minimum, document in the Security Considerations section that cache poisoning now influences installation decisions, not just display. The current text understates the impact ("This design doesn't worsen the attack surface" -- it does).
- **R3**: Consider tightening cache file permissions to 0600 to reduce exposure to group-readable attacks.

---

## 3. Version String Injection via the Requested Field

### 3.1 Input Path Analysis

The `Requested` field flows from user input at install time:

```
tsuku install node@18  -->  SplitN("@", 2)  -->  versionConstraint = "18"
  -->  installWithDependencies(... versionConstraint ...)
  -->  opts.RequestedVersion = versionConstraint
  -->  state.json: {"requested": "18"}
```

At update time, the design reads this back:
```
state.json Requested="18"  -->  PinLevelFromRequested("18")  -->  ResolveWithinBoundary(ctx, provider, "18")
```

### 3.2 What Validates the Requested String?

**At install time**: The `versionConstraint` is passed to `runInstallWithTelemetry` which calls `installWithDependencies`. The version goes through `version.ValidateVersionString()` in `manager.go:232` before being used in directory paths. This validates against path traversal (`..`, `/`, `\`) and enforces an allowlist regex `[a-zA-Z0-9._+\-@/]` with a 128-char max length.

**At update time (proposed)**: The `Requested` field is read from `state.json` and passed to `PinLevelFromRequested()` and then to `ResolveWithinBoundary()`. The design does not describe any re-validation of the `Requested` string before use.

### 3.3 Attack Scenario: Tampered state.json

If an attacker can modify `state.json`, they could set `Requested` to a crafted value:

- `Requested: "../../malicious"` -- path traversal in any path construction using the requested string
- `Requested: ""` -- forces PinLatest, removing the pin constraint the user intended
- `Requested: "1"` -- widens a PinMinor ("1.29") to PinMajor ("1"), allowing major version jumps

However, the design uses `Requested` only for:
1. Pin level derivation (component counting -- safe, it's just string splitting)
2. Prefix matching against a version list (string comparison -- safe against injection)
3. Passing to `provider.ResolveVersion(ctx, requested)` for VersionResolver-only providers

For path (3), the `ResolveVersion` implementations send the version string in HTTP requests to external APIs (GitHub, PyPI, etc.). A crafted `Requested` value could cause unexpected API queries, but the responses are validated by the provider's parsing logic.

### 3.4 Verdict

The version string injection risk is **low**. The `Requested` field is not used in path construction by the proposed code, and its use in API queries is bounded by provider-side validation. However:

- **R4**: `ResolveWithinBoundary()` should validate its `requested` parameter with `version.ValidateVersionString()` (or the state.go equivalent) as a defense-in-depth measure. This catches the case where state.json is hand-edited or tampered with. A one-line guard at the function entry point.

---

## 4. Pin-Level Derivation Manipulation

### 4.1 The Derivation Logic

```
""        -> PinLatest    (0 components)
"20"      -> PinMajor     (1 component)
"1.29"    -> PinMinor     (2 components)
"1.29.3"  -> PinExact     (3 components)
"@lts"    -> PinChannel   (starts with @)
```

### 4.2 Can Pin Level Be Manipulated to Bypass Constraints?

**Widening attack**: Change `Requested` from "1.29" (PinMinor) to "1" (PinMajor). This requires write access to state.json, which is the same privilege level as modifying binaries directly. Not a meaningful escalation.

**Narrowing attack**: Change `Requested` from "" (PinLatest) to "1.29.3" (PinExact). This is a denial-of-update, not a privilege escalation. Low impact.

**CalVer confusion**: A CalVer version like "2024.01" maps to PinMinor. A user expecting PinExact behavior for this would get updates within "2024.01.x". The design acknowledges this trade-off. It's a usability issue, not a security one.

**PinChannel bypass**: If `Requested` starts with "@", the design says it's PinChannel (resolved dynamically by provider). The design explicitly scopes named channels out of this implementation. If the code simply treats "@anything" as PinChannel and delegates to a provider that doesn't understand channels, the behavior depends on the provider's `ResolveVersion` implementation. Most providers would return an error for "@lts" as a version string, which is a safe failure mode.

### 4.3 Prefix Matching Ambiguity

The existing `GitHubProvider.ResolveVersion()` uses `strings.HasPrefix(v, version)` for fuzzy matching. This means:

- `Requested: "1"` matches "1.0.0", "10.0.0", "11.0.0", "100.0.0"
- `Requested: "18"` matches "18.0.0" but also "180.0.0" if such a version exists

The proposed `VersionMatchesPin()` and `ResolveWithinBoundary()` filter must use a dot-delimited prefix match, not a raw string prefix. For example, "18" should match "18.x.y" but not "180.x.y". The design mentions the function is ~20 lines but doesn't specify the matching algorithm.

### 4.4 Recommendations

- **R5 (Critical)**: `VersionMatchesPin()` must use dot-boundary-aware prefix matching. The input "18" should match versions starting with "18." (or exactly "18"), never "180" or "189". This is a correctness issue that has security implications -- a user who pins to major version 1 should never get version 10, 11, or 100. The implementation must append a "." separator when checking prefixes: `version == requested || strings.HasPrefix(version, requested+".")`.
- **R6**: Document that the existing `GitHubProvider.ResolveVersion()` has this same prefix ambiguity bug. The fallback path for VersionResolver-only providers inherits it. Consider fixing the underlying providers or documenting it as a known limitation.

---

## 5. "Not Applicable" Justification Review

### 5.1 "No new trust boundaries or attack surfaces"

**Assessment: Partially incorrect.**

The design does not introduce new trust boundaries (correct). But it does expand the attack surface of an existing trust boundary: the cache files. Before this design, cache poisoning affected display only. After, it affects what version gets installed. The Security Considerations section should be updated to reflect this.

### 5.2 "Version data still comes from the same upstream providers through the same HTTPS channels"

**Assessment: Correct but misleading.**

For VersionLister providers (the majority), version data now comes from the local cache file, not directly from HTTPS. The HTTPS fetch populates the cache, but a stale or tampered cache is read without re-verification. The statement is technically true about the origin but elides the caching intermediary.

### 5.3 "Mitigated by the existing checksum verification during download"

**Assessment: Partially correct.**

As analyzed in Section 2.4, checksum verification only helps when recipes include per-version checksums. Many recipes use dynamic URL templates without static checksums. The mitigation is real but not universal.

---

## 6. Residual Risks

### 6.1 Should Be Escalated

- **Cache poisoning now influences installation** (Section 2). Severity: Medium. The same-user-privilege boundary limits real-world exploitability, but the design doc's claim that the attack surface is unchanged is incorrect. **Action**: Update the Security Considerations section to acknowledge the expanded impact and document the same-user-privilege boundary as the primary mitigation.

### 6.2 Should Be Fixed in Implementation

- **R5 (dot-boundary prefix matching)**: Without this, pin-level enforcement is subtly broken for single-digit major versions. This is a functional correctness issue with security implications.
- **R4 (validate Requested on read)**: Low-cost defense-in-depth measure.

### 6.3 Acceptable Residual Risk

- CalVer pin-level ambiguity (documented trade-off)
- VersionResolver-only providers uncached (2 of 20+ providers)
- state.json tampering by same-user processes (equivalent to direct binary modification)

---

## 7. Summary of Recommendations

| ID | Priority | Description |
|----|----------|-------------|
| R1 | Low | Add cache integrity check (HMAC or similar) |
| R2 | Medium | Update Security Considerations to reflect cache poisoning impact escalation |
| R3 | Low | Tighten cache file permissions to 0600 |
| R4 | Medium | Validate `requested` parameter in `ResolveWithinBoundary()` |
| R5 | High | Use dot-boundary-aware prefix matching in `VersionMatchesPin()` |
| R6 | Low | Document or fix prefix ambiguity in existing `GitHubProvider.ResolveVersion()` |
