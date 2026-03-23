---
document: Security Review — DESIGN-binary-index-full-registry.md
phase: 6
role: security-review
date: 2026-03-23
---

# Security Review: Binary Index — Full Registry Coverage

## Scope

Review of the Security Considerations section in `docs/designs/DESIGN-binary-index-full-registry.md`.
Analysis verified against current code in `internal/registry/registry.go` and `internal/registry/manifest.go`.

---

## 1. Attack vectors not considered

### 1a. Manifest name injection (not addressed in design)

`ListAll()` reads recipe names from the cached manifest and passes each name to `recipeURL()`, which constructs a URL path:

```go
firstLetter := strings.ToLower(string(name[0]))
return fmt.Sprintf("%s/recipes/%s/%s.toml", r.BaseURL, firstLetter, name)
```

A manifest entry with a name like `../../etc/passwd` or `../secrets` would produce a path traversal in the local-registry case (`fetchLocalRecipe` calls `os.ReadFile(path)` where `path` is the composed local filesystem path). A name like `evil/../../secret` similarly produces a traversal.

The default registry is `raw.githubusercontent.com`, where this manifests as a URL path injection rather than a filesystem path traversal. Against a `TSUKU_REGISTRY_URL` local-path deployment, it is a direct filesystem read outside the registry directory.

The design does not mention manifest name validation. The upstream design (`DESIGN-binary-index.md`) is not reviewed here, but `ListAll()` is a new code path that trusts manifest content more broadly than the existing `GetCached()` path did (which only read names from the local filesystem, already constrained to be valid filenames by the OS).

**Severity: Blocking.** The design claims "no new attack surface" but this is incorrect for local-registry deployments. Manifest name sanitization (rejecting names containing `/`, `..`, or null bytes before they reach `recipeURL()`) must be an acceptance criterion alongside the response size cap.

### 1b. Unbounded manifest size (gap in scope)

`FetchManifest()` in `manifest.go` line 129 reads the manifest body with `io.ReadAll(resp.Body)` without a size limit. At ~1,400 entries, a legitimate manifest is small (tens of KB), but a malicious or misconfigured `TSUKU_MANIFEST_URL` server can return an arbitrarily large body. The design explicitly calls out the per-recipe response size cap but does not mention the manifest fetch, which is in the same trust boundary.

This is an existing gap, not introduced by this design. However, the design's own framing ("same trust boundary as the existing update-registry network operations") invites reading it as already safe, which is misleading. The design should acknowledge this existing gap rather than implying the manifest fetch is already hardened.

**Severity: Advisory** (pre-existing, not introduced here). Should be noted in the design's Security Considerations to avoid a false "already handled" conclusion.

---

## 2. Mitigations assessed as sufficient

### Response size cap (acceptance criterion)

The design identifies memory exhaustion from unbounded recipe bodies (10 concurrent `io.ReadAll` calls) and calls for `io.LimitReader(resp.Body, 1<<20)` as an acceptance criterion. The current `fetchRemoteRecipe` implementation (registry.go:187) uses raw `io.ReadAll` without this limit — confirming the design correctly flags it as not yet implemented. Treating it as an acceptance criterion (rather than deferring) is the right call.

### TLS verification

The design's claim that TLS verification is inherited from the existing HTTP client is accurate. `NewRegistryHTTPClient()` uses Go's default TLS stack with `DisableCompression: true` and explicit dial/handshake/header timeouts. No new client is introduced for the `FetchRecipe()` bulk-fetch path.

### TOML content trust

The claim that `extractBinariesFromMinimal` only reads specific TOML keys and does not execute recipe logic is structurally sound — the binary index pipeline is read-only at the TOML layer. No action dispatch occurs during rebuild. This is correct.

### Rate-limit handling (acknowledged gap)

The 429 / skip-and-warn behavior is consistent with the existing error-handling pattern and the design correctly documents it as a known gap. No escalation needed here.

---

## 3. "Not applicable" justifications reviewed

The design does not use explicit "not applicable" markers, but implies several risks are out of scope by framing them as pre-existing:

- **`TSUKU_REGISTRY_URL` trust**: correctly flagged as an existing concern with amplified volume. The amplification point (1,400 fetches vs. one) is material and the design calls it out. Adequate.
- **Redirect behavior**: the advisory about HTTP-to-HTTPS downgrade is accurate for `TSUKU_REGISTRY_URL` deployments. Adequate for a design-level note.
- **"No new attack surface" claim**: this claim is incorrect as analyzed in §1a. The implicit "not applicable" for manifest name injection is not justified.

---

## 4. Residual risk requiring escalation

No finding rises to escalation beyond the design team. The path traversal risk in §1a is blocking for local-registry deployments but is a contained fix (input sanitization on names from `ListAll()` before they reach `recipeURL()`). It should be added as an acceptance criterion before implementation begins.

---

## Summary

| Finding | Type | Severity |
|---------|------|----------|
| Manifest name injection → path traversal in local-registry mode; URL injection in remote mode | Attack vector not considered | Blocking — add name sanitization as acceptance criterion |
| `FetchManifest` has no response size cap (pre-existing) | Scope omission in design text | Advisory — acknowledge in Security Considerations to prevent false confidence |
| Response size cap for per-recipe fetch | Correctly identified, acceptance criterion | Sufficient |
| TLS verification claim | Verified against code | Accurate |
| TOML content trust claim | Verified against architecture | Accurate |
