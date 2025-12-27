# Security Review: PGP Signature Verification for Tsuku

**Reviewer**: Security Analysis Agent
**Date**: 2025-12-27
**Feature**: PGP Signature Verification (Issue #682)
**Design Document**: docs/DESIGN-pgp-verification.md

## Executive Summary

The proposed PGP signature verification design for tsuku is **well-conceived and security-conscious**. The fingerprint-based key verification (Option 2E) provides meaningful security improvement over both checksums and TOFU-style key URL fetching. The design correctly identifies the threat model (compromised download servers) and provides an appropriate mitigation.

**Overall Assessment**: The security analysis provided in the design document is thorough and accurate. There are a few additional considerations and minor gaps identified below, but no critical flaws that would require redesign.

---

## 1. Attack Vector Analysis

### 1.1 Attack Vectors Considered (Complete and Accurate)

The design document correctly identifies:

| Vector | Mitigation | Assessment |
|--------|------------|------------|
| Compromised download server | Signature cannot be forged without private key | Correct |
| Modified checksums on server | Signature is independent of checksum | Correct |
| MITM attacks | Signature verification is independent of TLS | Correct |
| Malicious key at URL | Fingerprint validation | Correct |
| Cache poisoning | Keys cached by fingerprint, re-validated on load | Correct |
| Signature stripping | Recipe requires signature | Correct |

### 1.2 Additional Attack Vectors to Consider

**1.2.1 Fingerprint Collision Attacks (Very Low Risk)**

SHA-1 fingerprints (used by OpenPGP) are theoretically vulnerable to collision attacks. However:
- This requires finding a collision, not a preimage attack
- An attacker would need to create a malicious key that hashes to the SAME fingerprint as the legitimate key
- SHA-1 collision cost is estimated at ~$100K+ in compute resources
- The attacker would ALSO need recipe write access to exploit this

**Mitigation Status**: Adequate. The attack cost far exceeds the value for most targets. If PGP moves to SHA-256 fingerprints (RFC 9580), the design will benefit automatically.

**1.2.2 Key Server Timing Attacks (Low Risk)**

If `signature_key_url` points to a third-party key server, an attacker who can observe network traffic could learn which tools are being installed. This is informational leakage, not an integrity threat.

**Mitigation Status**: Acceptable. The design correctly notes keys are cached, minimizing repeated fetches. This is not meaningfully different from the existing download URL leakage.

**1.2.3 Subkey vs Primary Key Confusion (Low Risk)**

OpenPGP keys can have multiple subkeys with different capabilities. A malicious key could be constructed with a signing subkey whose fingerprint matches the primary key fingerprint in the recipe, but the signature was made by a different subkey.

**Mitigation Status**: Requires implementation verification. The gopenpgp library should validate that the signature was made by the key corresponding to the fingerprint. Recommend adding a test case for this scenario.

**1.2.4 Signature Algorithm Downgrade (Very Low Risk)**

An attacker could potentially create signatures using deprecated algorithms (MD5, SHA-1) that gopenpgp might accept. Modern signing keys typically use SHA-256 or better.

**Mitigation Status**: Acceptable. gopenpgp v2 defaults to modern algorithms and rejects known-weak configurations. The curl maintainer's key uses modern crypto.

**1.2.5 Revoked Key Handling (Low Risk)**

If a signing key is revoked (e.g., after compromise), the design has no mechanism to learn of the revocation. Old releases signed with the revoked key would still verify.

**Mitigation Status**: Acceptable trade-off. Revocation checking would require keyserver queries, adding complexity and a network dependency. The recipe fingerprint can be updated during code review if a key is compromised. Document this in the recipe authoring guide.

**1.2.6 Time-of-Check vs Time-of-Use (TOCTOU) on Key Cache (Very Low Risk)**

Between reading the cached key and using it for verification, a local attacker with file system access could replace the key file.

**Mitigation Status**: Adequate. A local attacker with write access to `$TSUKU_HOME/cache/keys/` would already have access to modify installed binaries. The fingerprint re-validation on cache load prevents cache poisoning by remote attackers.

---

## 2. Mitigation Sufficiency Analysis

### 2.1 Mitigations Assessed as Sufficient

| Risk | Mitigation | Analysis |
|------|------------|----------|
| Malicious key at URL | Fingerprint validation | **Sufficient**. Fingerprint is SHA-1 hash of key, cryptographically binding. Attacker cannot produce valid key without finding preimage. |
| Cache poisoning | Keys cached by fingerprint; re-validated | **Sufficient**. Key filename is fingerprint, content is re-validated on load. |
| Signature stripping | Recipe requires signature | **Sufficient**. No code path to bypass when `signature_url` is specified. |
| Key URL unavailable | Cache keys after first fetch | **Sufficient**. Degradation is graceful (fails closed, not open). |

### 2.2 Mitigations Requiring Enhancement

**2.2.1 Expired Signing Key**

**Current Mitigation**: "Verify signature math only, not key expiration"

**Analysis**: This is the correct approach for verifying old releases, but the design should clarify:
- gopenpgp's default behavior regarding expired keys
- Whether `VerifyTime` parameter should be set to signature creation time (for historical releases) or current time
- Recommendation: Use signature creation time for verification (matches `--ignore-time-conflict` in gpg)

**Residual Risk**: Old keys may use weaker algorithms. Acceptable given the fingerprint constraint.

**2.2.2 gopenpgp Vulnerability**

**Current Mitigation**: "Use stable v2, monitor for security advisories"

**Enhancement Recommendation**:
- Add gopenpgp to security monitoring (Dependabot or similar)
- Consider adding a CI job that checks for known CVEs in dependencies
- Document upgrade path if critical vulnerability is found

---

## 3. Residual Risk Assessment

### 3.1 Residual Risks Requiring Escalation

**None identified.** All residual risks are acceptable given tsuku's threat model.

### 3.2 Residual Risks Accepted (Documented)

| Risk | Severity | Justification |
|------|----------|---------------|
| Attacker with recipe write access | Medium | Repository access controls and code review are assumed. This is the trust anchor. |
| SHA-1 fingerprint collision | Very Low | Attack cost exceeds value; future PGP versions will use SHA-256. |
| Zero-day in gopenpgp | Low | Using well-maintained library from security-focused organization. |
| Key revocation not checked | Low | Revocations require recipe update; documented trade-off. |
| Legacy keys with weak crypto | Very Low | Fingerprint constraint limits scope; unlikely for active projects. |

---

## 4. "Not Applicable" Justification Review

The security considerations section marks the following as "Not Applicable":

### 4.1 Privilege Escalation

**Stated**: "None. Signature verification runs with the same permissions as the existing download action."

**Review**: **Accurate**. Signature verification:
- Does not execute downloaded content
- Does not change file permissions beyond what download action already does
- Does not request sudo or elevated permissions
- Key cache uses 0600 permissions (user-only)

### 4.2 User Data Exposure

**Stated**: "None. This feature accesses only files and URLs explicitly specified in the recipe."

**Review**: **Accurate**. The feature:
- Does not access user credentials or environment variables (beyond standard Go runtime)
- Does not transmit user-identifying information
- Network requests are to recipe-specified URLs only
- Cache directory is user-controlled

---

## 5. Additional Security Recommendations

### 5.1 Implementation Recommendations

1. **Fingerprint Validation**: Ensure fingerprints are validated as exactly 40 hex characters (case-insensitive) before use. Reject malformed fingerprints early.

2. **Key Cache Permissions**: The design states 0600. Verify this is enforced in implementation and that the cache directory is created with 0700.

3. **Error Messages**: Avoid leaking key material or file paths in error messages that could aid attackers. Use sanitized paths and fingerprint prefixes.

4. **Timeout for Key Fetch**: Ensure key URL fetches have reasonable timeouts to prevent slowloris-style attacks.

5. **Size Limit on Keys**: Consider implementing a size limit on fetched keys (e.g., 100KB) to prevent resource exhaustion from maliciously large responses.

### 5.2 Testing Recommendations

1. **Test with Real Signatures**: Include integration tests using actual curl release signatures to verify compatibility with real-world key formats.

2. **Test Fingerprint Mismatch**: Verify that a key with incorrect fingerprint is rejected, even if it's a valid PGP key.

3. **Test Signature for Wrong File**: Verify that a valid signature for a different file is rejected.

4. **Test Expired Key Handling**: Verify behavior with an expired signing key.

5. **Test Malformed Inputs**: Test with:
   - Truncated signatures
   - Non-armored signatures
   - Binary signatures (if planning to support)
   - Keys with multiple user IDs
   - Keys with expired subkeys

### 5.3 Documentation Recommendations

1. **Recipe Authoring Guide**: Add section on obtaining and verifying key fingerprints:
   - How to download a key
   - How to verify the key out-of-band (e.g., comparing fingerprint on project website)
   - How to extract the fingerprint
   - Example: "gpg --show-keys --fingerprint path/to/key.asc"

2. **Key Rotation Documentation**: Document the process for updating a recipe when a project rotates their signing key.

3. **Error Troubleshooting**: Document common PGP verification errors and their causes.

---

## 6. Comparison with Industry Practices

### 6.1 Similar Tools

| Tool | Signature Verification | Key Management |
|------|------------------------|----------------|
| Homebrew | Checksums only | N/A |
| apt | GPG signatures | Keyring packages, trusted.gpg.d |
| pacman | GPG signatures | Master keys, web of trust |
| nix | Signatures | Pre-shared public keys |
| asdf | None (plugins may verify) | N/A |

**Assessment**: Tsuku's fingerprint-based approach is more secure than Homebrew (checksums only) and simpler than apt/pacman (no keyring management). It's comparable to nix but with per-recipe key specification.

### 6.2 SLSA Comparison

SLSA Level 2+ requires verified provenance. PGP signatures provide author verification but not build provenance. The design correctly scopes this as out-of-scope, noting SLSA as a future enhancement.

---

## 7. Conclusion

The PGP signature verification design is **security-sound** and appropriate for tsuku's threat model. The fingerprint-based verification provides meaningful protection against compromised download servers without introducing complex key management.

**Key Strengths**:
- Fingerprint in recipe provides cryptographic binding without embedded keys
- Cache-after-fetch minimizes network dependency
- Fail-closed on verification failure
- Uses well-maintained cryptographic library

**Areas for Implementation Attention**:
- Fingerprint format validation
- Key cache permissions (0600/0700)
- Size limits on fetched keys
- Expired key handling strategy
- Subkey vs primary key validation in tests

**No blocking security concerns identified.** The design can proceed to implementation with the minor enhancements noted above.

---

## Appendix: Security Checklist

- [x] Threat model clearly defined
- [x] Attack vectors enumerated
- [x] Mitigations documented
- [x] Residual risks identified
- [x] Trust anchor established (fingerprint in version-controlled recipe)
- [x] Fail-closed behavior (verification failure rejects download)
- [x] No privilege escalation
- [x] No user data exposure
- [x] HTTPS enforced (via existing infrastructure)
- [x] Library choice justified (gopenpgp)
- [x] File permissions addressed (0600 cache)
- [ ] Implementation testing recommendations (added in this review)
- [ ] Size limits on external data (recommended in this review)
- [ ] Expired key strategy clarification (recommended in this review)
