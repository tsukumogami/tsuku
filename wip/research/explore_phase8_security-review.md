# Security Review: Registry Recipe Cache Policy

**Reviewer:** Claude (phase 8 security review)
**Design:** DESIGN-registry-cache-policy.md
**Date:** 2026-01-26

## Executive Summary

The registry recipe cache policy design is **sound for its stated scope** (TOML metadata caching). The security considerations section correctly identifies the primary risks and proposes reasonable mitigations. However, I identified **three areas warranting closer attention** and **two minor gaps** to address.

**Risk Level:** Low to Medium (appropriate for the scope)

The design maintains tsuku's existing trust model while adding bounded staleness risk. The 7-day maximum stale window is a reasonable trade-off between reliability and security.

---

## Detailed Analysis

### 1. Attack Vectors Reviewed

#### 1.1 Identified in Design (Correct Assessment)

| Attack Vector | Design Assessment | Review Verdict |
|--------------|-------------------|----------------|
| Stale malicious recipe persists | 7-day bound | **Appropriate** - bounds risk window |
| Cache poisoning via local write | 0644 permissions | **Appropriate** - no worse than current |
| Recipe tampered in transit | HTTPS to GitHub | **Appropriate** - relies on TLS |
| Cache content corruption | SHA256 hash | **Appropriate** - cryptographically sound |
| Unwanted stale cache use | stderr warning | **Appropriate** - user awareness |

#### 1.2 Additional Attack Vectors Considered

**1.2.1 Symlink Attack in Cache Directory (NOT COVERED)**

**Risk:** An attacker with local write access to `$TSUKU_HOME` could replace a recipe file with a symlink pointing outside the cache directory, potentially to:
- `/etc/passwd` (information disclosure during recipe parsing)
- A attacker-controlled file in `/tmp` (recipe injection)

**Assessment:** Low risk. Requires local file system access, which already gives the attacker significant capabilities. The current implementation does not follow symlinks when reading cached recipes (`os.ReadFile` does not dereference).

**Recommendation:** Consider adding symlink detection in `GetCached()` as defense-in-depth. Not blocking.

**1.2.2 Race Condition in Metadata Write (NOT COVERED)**

**Risk:** A TOCTOU (time-of-check-time-of-use) race between writing the recipe file and writing the metadata sidecar. If the process crashes after writing the recipe but before metadata:
- Recipe exists without metadata
- Next read treats it as "never cached" or uses stale fallback logic incorrectly

**Assessment:** Very low risk. The design's migration logic (create metadata on first read) handles this gracefully. The atomic write pattern for metadata (temp + rename) prevents partial writes.

**Recommendation:** Document this behavior as intentional. No action needed.

**1.2.3 GitHub Account Compromise (CORRECTLY NOT IN SCOPE)**

**Risk:** If a tsuku maintainer's GitHub account is compromised, malicious recipes could be pushed.

**Assessment:** This is correctly out of scope for this design. This is a supply chain risk at the registry level, not the cache level. The cache design doesn't make this worse.

**Recommendation:** Future work: Consider recipe signing (mentioned in design as out of scope). This design explicitly states "Recipe content validation/signing (separate future work)" which is appropriate.

**1.2.4 Denial of Service via Cache Exhaustion (PARTIALLY COVERED)**

**Risk:** An attacker could cause the cache to fill with many recipes, exhausting the LRU limit and evicting legitimate recipes.

**Assessment:** Design addresses this with 50MB limit and LRU eviction. However, the 80%/60% thresholds mean an attacker could keep the cache churning if they can trigger many recipe fetches.

**Recommendation:** The design is sufficient. Recipe fetches require user commands (install/info), limiting attacker-controlled cache population. No action needed.

**1.2.5 Downgrade Attack via TSUKU_REGISTRY_URL (CORRECTLY HANDLED)**

**Risk:** If an attacker can set `TSUKU_REGISTRY_URL` environment variable, they could redirect to a malicious registry.

**Assessment:** This is an existing risk, not introduced by this design. The current implementation in `registry.go` uses this env var. The cache design doesn't change this attack surface.

**Recommendation:** Out of scope for this design, but worth noting: environment variables require same-user access, which already grants significant capabilities.

---

### 2. Mitigation Sufficiency Analysis

#### 2.1 7-Day Maximum Staleness

**Sufficient?** Yes, with caveats.

The 7-day window is a reasonable balance. Analysis:
- **Comparison:** apt uses 7-day expiry, npm uses 5-minute cache + stale fallback
- **Recipe change frequency:** Most recipe changes are version updates or bug fixes, not security patches
- **Detection capability:** Users running `tsuku install` will see the stale warning on stderr

**Residual risk accepted:** A compromised recipe discovered and fixed on day 1 could still be used until day 7 during network issues. This is acceptable because:
1. Network issues persisting 7 days is unusual
2. Users can force refresh with `tsuku update-registry`
3. The design allows `TSUKU_RECIPE_CACHE_MAX_STALE=0` for strict environments

#### 2.2 SHA256 Content Hash

**Sufficient?** Yes.

SHA256 is cryptographically strong for integrity verification. The design correctly notes hash collision is "cryptographically infeasible."

One improvement: The design should specify **what happens if hash verification fails**. Current behavior from code review: re-fetch from network. This is correct.

#### 2.3 Warning on Stale Use

**Sufficient?** Yes, with usability consideration.

Stderr warning is appropriate. The design specifies the exact message format:
```
Warning: Using cached recipe '{name}' (last updated {X} hours ago). Run 'tsuku update-registry' to refresh.
```

**Minor gap:** No log-level option to elevate warnings to errors for CI/security-conscious environments. Consider adding `TSUKU_RECIPE_CACHE_WARN_AS_ERROR` or similar in future work.

#### 2.4 File Permissions (0644)

**Sufficient?** Yes.

Standard permissions for cache files. World-readable is acceptable since recipes are public data. World-readable is actually desirable for multi-tool scenarios where different users might share a tsuku installation.

---

### 3. "Not Applicable" Justifications Review

#### 3.1 Execution Isolation: "Not directly applicable"

**Assessment:** Correct.

The design explicitly notes: "This design does not execute code. It manages TOML files that describe how to install software."

This is accurate. The recipe cache design handles TOML metadata. Execution happens in the recipe execution layer (`internal/actions/`), which has its own security controls:
- HTTPS enforcement for downloads
- Checksum verification for binaries
- PGP signature support
- SSRF protection in HTTP client

#### 3.2 User Data Exposure: "No change from current behavior"

**Assessment:** Correct.

The design adds local cache statistics display but doesn't transmit any new data. Recipe fetch requests go to GitHub (public), same as current implementation.

---

### 4. Residual Risks Summary

| Risk | Severity | Escalation Required? | Notes |
|------|----------|---------------------|-------|
| 7-day stale window | Medium | No | Acceptable trade-off, configurable |
| Symlink attack in cache | Low | No | Requires local access, defense-in-depth improvement optional |
| No recipe signing | Medium | No | Explicitly out of scope, mentioned as future work |
| CI environments using stale cache silently | Low | No | Consider warn-as-error option in future |

**Escalation Recommendation:** No risks require immediate escalation. All are acceptable for the stated scope.

---

### 5. Comparison with Version Cache Implementation

The design explicitly follows `internal/version/cache.go` patterns. Review confirmed consistency:

| Aspect | Version Cache | Proposed Recipe Cache | Assessment |
|--------|--------------|----------------------|------------|
| Metadata format | JSON sidecar | JSON sidecar | Consistent |
| TTL handling | `ExpiresAt` field | `expires_at` field | Consistent (case matches Go/JSON convention) |
| Stale fallback | No | Yes (new feature) | Appropriate addition |
| Size limits | No | Yes (new feature) | Appropriate addition |
| Atomic writes | temp + rename | temp + rename | Consistent |

The proposed design extends the version cache pattern appropriately.

---

### 6. Security Properties Matrix

| Property | Before (Current) | After (This Design) | Change |
|----------|-----------------|---------------------|--------|
| Confidentiality | N/A (public recipes) | N/A (public recipes) | No change |
| Integrity | HTTPS only | HTTPS + SHA256 hash | **Improved** |
| Availability | Fail on network error | Stale fallback | **Improved** (with bounded risk) |
| Auditability | No cache metadata | Timestamps + staleness tracking | **Improved** |
| Attack surface | Registry fetch | Registry fetch + local cache | Slightly larger, mitigated |

---

## Recommendations

### Top 3 Recommendations (Priority Order)

1. **Document hash verification failure behavior explicitly.** The design should state: "If content hash verification fails on read, the cached entry is discarded and a fresh fetch is attempted." This is the current behavior but should be documented.

2. **Consider symlink detection in GetCached() as defense-in-depth.** Add a check like:
   ```go
   info, _ := os.Lstat(path)
   if info.Mode()&os.ModeSymlink != 0 {
       return nil, fmt.Errorf("symlink detected in cache")
   }
   ```
   This prevents cache directory symlink attacks. Low priority, but trivial to implement.

3. **Add configurable strict mode for security-sensitive environments.** Consider:
   - `TSUKU_RECIPE_CACHE_STRICT=true` - treat stale warnings as errors
   - Useful for CI pipelines that want to ensure fresh recipes

### Non-Blocking Observations

- The 50MB default cache size is generous for ~150 recipes at ~3KB each (450KB total). The design acknowledges this. No action needed, but could reduce default if disk space becomes a user concern.

- The design correctly separates binary verification (checksums in recipes) from recipe caching. This separation of concerns is good security design.

- Error message templates are well-defined and provide actionable guidance. This is good for security UX.

---

## Conclusion

The security considerations section is **comprehensive and appropriate for the design scope**. The identified risks are real but bounded, and the mitigations are reasonable. The design does not introduce significant new attack surface.

The key insight is that this design is about **metadata caching**, not binary download caching. The trust model (HTTPS + GitHub PR review) is inherited from the existing implementation, not changed by this design.

**Recommendation:** Proceed with implementation. Address documentation gap (recommendation 1) in the design doc. Consider symlink detection and strict mode as optional enhancements.

---

## Appendix: Code Review Notes

Files reviewed for security context:
- `/internal/registry/registry.go` - Current fetch/cache implementation
- `/internal/registry/errors.go` - Error classification
- `/internal/recipe/loader.go` - Recipe loading priority chain
- `/internal/version/cache.go` - Existing cache pattern (precedent)
- `/internal/actions/download.go` - Binary download with checksums
- `/internal/install/checksum.go` - Checksum verification
- `/internal/httputil/client.go` - SSRF protection
- `/internal/httputil/ssrf.go` - IP validation

Relevant security controls observed:
- HTTPS enforcement in downloads
- SSRF protection via IP validation on redirects
- Decompression bomb prevention (DisableCompression)
- Symlink attack prevention in checksum verification (isWithinDir check)
- Atomic file writes (temp + rename pattern)
