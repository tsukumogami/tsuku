# Security Review: Platform Compatibility Verification

**Reviewer:** Claude (Security Analysis)
**Date:** 2026-01-24
**Scope:** tsuku-dltest recipe, dltest.go invocation logic, platform detection, CI infrastructure

## Executive Summary

The security considerations documented in DESIGN-platform-compatibility-verification.md are accurate and appropriately scoped. The mitigations are sufficient for the identified risks. Two additional attack vectors warrant consideration, both rated low severity. No residual risks require escalation.

**Verdict:** The design is security-sound. Proceed with implementation.

---

## 1. Analysis of Documented Security Considerations

### 1.1 Download Verification

**Claim:** "No new download paths. Existing Homebrew bottle mechanism with SHA256 checksums remains."

**Finding:** Verified correct. The tsuku-dltest recipe uses `github_file` action pointing to the tsuku repo itself (line 15-16 of tsuku-dltest.toml). This uses the existing download infrastructure with checksum verification.

The musl detection feature blocks downloads before they occur, which is a defense-in-depth improvement - it prevents wasted bandwidth and confusing error messages.

**Status:** Adequate

### 1.2 Execution Isolation

**Claim:** "musl detection reads `/lib/ld-musl-*.so.1` or `ldd --version` with normal user permissions. No privilege escalation."

**Finding:** The design document describes this approach. The current implementation in abi.go validates PT_INTERP (line 39-57) and checks if the interpreter exists (line 66-80). This is read-only file access requiring no special permissions.

The sanitizeEnvForHelper function (dltest.go lines 303-327) properly strips dangerous loader variables:
- Linux: LD_PRELOAD, LD_AUDIT, LD_DEBUG, LD_DEBUG_OUTPUT, LD_PROFILE, LD_PROFILE_OUTPUT
- macOS: DYLD_INSERT_LIBRARIES, DYLD_FORCE_FLAT_NAMESPACE, DYLD_PRINT_LIBRARIES, DYLD_PRINT_LIBRARIES_POST_LAUNCH

**Status:** Adequate

### 1.3 Supply Chain Risks

**Claim:** "No new binary sources. Homebrew GHCR checksummed. tsuku-dltest built from source."

**Finding:** Partially accurate with nuance.

For releases: tsuku-dltest is built via goreleaser as a Go binary with the pinned version injected via ldflags (line 20 of .goreleaser.yaml). The recipe downloads from GitHub Releases with checksums.

For development: When pinnedDltestVersion == "dev", any installed version is accepted (dltest.go line 101). This is intentional but means dev builds trust whatever tsuku-dltest is installed.

The version pinning mechanism (verify/version.go) provides supply chain protection for release builds.

**Status:** Adequate for releases. Dev mode intentionally relaxed.

### 1.4 User Data Exposure

**Claim:** "No new data collection. musl detection is local only."

**Finding:** Verified. The detection reads:
- File system paths (/lib/ld-musl-*.so.1)
- Binary ELF headers
- ldd --version output

None of this data is transmitted. CI tests use fixtures.

**Status:** Adequate

---

## 2. Unconsidered Attack Vectors

### 2.1 Library Path Race Condition (Low Severity)

**Vector:** Between validateLibraryPaths() and the actual dlopen call by the helper, an attacker with write access to $TSUKU_HOME/libs could replace a validated library with a malicious one.

**Analysis:**
- Requires write access to $TSUKU_HOME, which means attacker already has equivalent access to user's binaries
- Time window is milliseconds (validation to subprocess spawn)
- The attacker could more easily modify any installed binary directly

**Recommendation:** Accept risk. The attacker must already have write access to the tsuku home directory, making this a non-privileged attack that could be achieved through simpler means. No mitigation needed.

### 2.2 tsuku-dltest Recipe Hijacking (Low Severity)

**Vector:** If an attacker gains write access to the recipe registry (GitHub repo), they could modify tsuku-dltest.toml to point to a malicious binary.

**Analysis:**
- Requires compromising the tsuku GitHub repository
- SHA256 checksums would need to be updated to match malicious binary
- This is equivalent to compromising any other recipe
- GitHub branch protection and code review provide mitigation

**Recommendation:** Accept risk. This is standard supply chain risk for all recipes. Existing controls (branch protection, checksums, signed releases) apply equally to tsuku-dltest.

### 2.3 Batch Processing Denial of Service (Informational)

**Vector:** Maliciously crafted libraries could cause the helper to hang or crash repeatedly, triggering exponential retry splitting.

**Analysis:**
- BatchTimeout (5 seconds) bounds each invocation (dltest.go line 37)
- Retry halving eventually isolates problematic libraries
- Maximum impact is verification timeout, not code execution
- Libraries must already be in $TSUKU_HOME/libs (user-installed)

**Recommendation:** No action needed. The timeout and retry logic adequately bounds the impact.

---

## 3. "Not Applicable" Justification Review

The design document does not explicitly mark items as "not applicable." However, several implicit assumptions deserve validation:

### 3.1 ARM64 Runner Security (Implicitly Same Trust Model)

**Claim:** "Same trust model as amd64"

**Finding:** Correct. GitHub-hosted ARM64 runners (ubuntu-24.04-arm) have equivalent isolation to amd64 runners. They're ephemeral VMs with the same security posture.

**Status:** Valid assumption

### 3.2 Container Image Trust (CI Only)

**Claim:** "CI containers pulled from official Docker Hub repos (test infrastructure only)"

**Finding:** The container-build.yml workflow uses GHCR (ghcr.io/tsukumogami/tsuku-sandbox) for tsuku's own images, not Docker Hub. The integration-tests.yml doesn't specify container images directly - it runs on ubuntu-latest runners.

The design document mentions fedora:41, archlinux:base, alpine:3.19, opensuse/leap:15 as targets for container-based family tests. These would be pulled from Docker Hub.

**Risks:**
- Compromised official image could affect CI results
- No impact on user-installed binaries (CI only)
- Image digest pinning could improve reproducibility but adds maintenance burden

**Status:** Acceptable for CI. User-facing code does not pull container images.

---

## 4. Mitigation Adequacy Assessment

| Mitigation | Risk Addressed | Adequacy |
|------------|----------------|----------|
| SHA256 checksum verification | Tampered downloads | Sufficient |
| pinnedDltestVersion in ldflags | Version mismatch attack | Sufficient |
| ErrChecksumMismatch hard failure | Bypass of verification | Sufficient |
| sanitizeEnvForHelper | LD_PRELOAD injection | Sufficient |
| validateLibraryPaths with EvalSymlinks | Path traversal | Sufficient |
| BatchTimeout (5s) | Hang during dlopen | Sufficient |
| Official container images only | Malicious CI environment | Acceptable |

All documented mitigations are sufficient for their intended purpose.

---

## 5. Residual Risks

### 5.1 Accepted Risks (No Escalation Needed)

1. **Alpine/musl limitation** - Users on musl systems cannot use embedded libraries. Addressed by runtime detection with clear messaging.

2. **Container test fidelity** - Container tests may not catch all bare-metal issues. Acceptable because primary purpose is package manager integration testing.

3. **Dev mode trust** - Development builds accept any installed tsuku-dltest version. Acceptable for development workflow.

### 5.2 Escalation Candidates (None)

No risks identified that require escalation to project maintainers or security review board.

---

## 6. Recommendations

### 6.1 Short Term (No blockers)

None. The design is ready for implementation.

### 6.2 Medium Term (Post-implementation)

1. **Consider image digest pinning** for CI container tests once the matrix stabilizes. This improves reproducibility without changing the security model.

2. **Document helper binary trust model** in security documentation. Users should understand that tsuku-dltest has the same trust level as tsuku itself.

### 6.3 Long Term (Future consideration)

1. **Code signing for releases** would strengthen supply chain security for both tsuku and tsuku-dltest binaries. This is a general improvement not specific to this design.

---

## 7. Conclusion

The security analysis in DESIGN-platform-compatibility-verification.md is thorough and accurate. The identified attack vectors (library path race, recipe hijacking, batch DoS) are either mitigated by existing controls or represent acceptable residual risk given the threat model.

Key strengths:
- Checksum verification provides integrity guarantee for helper binary
- Environment sanitization blocks loader injection attacks
- Path validation prevents directory traversal
- Timeout and retry logic bounds DoS impact
- musl detection provides fail-fast behavior with clear guidance

The design appropriately balances security with usability. No changes required before implementation.
