# Security Review: Sandbox Implicit Dependencies Feature

## Executive Summary

This review assesses the security posture of the sandbox implicit dependencies feature, which downloads build tools (cmake, make, zig, pkg-config) to the host and mounts them read-only into containers. The analysis identifies **three critical attack vectors not addressed in the original analysis**, evaluates mitigation sufficiency, and recommends additional controls.

**Key Findings:**
- ✅ Download verification approach is sound (SHA256 checksums)
- ⚠️ **CRITICAL**: Time-of-check-time-of-use (TOCTOU) vulnerability in host installation
- ⚠️ **HIGH**: Unvalidated tool execution creates arbitrary code execution risk
- ⚠️ **MEDIUM**: Container escape potential via mounted binary exploitation
- ⚠️ **MEDIUM**: "Not applicable" justifications miss telemetry/logging exposure risks

---

## 1. Attack Vectors Not Considered

### 1.1 Time-of-Check-Time-of-Use (TOCTOU) Race Condition

**Attack Vector**: Between checksum verification and container mounting, an attacker with host filesystem access could swap the verified binary with a malicious one.

**Attack Scenario**:
```
1. Tsuku downloads cmake-3.28.tar.gz, verifies SHA256 ✓
2. Tsuku extracts to ~/.tsuku/tools/cmake-3.28/bin/cmake
3. [ATTACKER] Replaces ~/.tsuku/tools/cmake-3.28/bin/cmake with malicious binary
4. Container starts with --volume ~/.tsuku/tools:/workspace/tsuku/tools:ro
5. Malicious cmake executes in container with workspace access
```

**Impact**: Complete compromise of sandbox isolation. Attacker gains arbitrary code execution in container context with access to:
- Recipe source code (potential IP theft)
- Build artifacts (malware injection into final binary)
- Container network access (data exfiltration)

**Likelihood**: Medium (requires local host access or prior compromise)

**Original Analysis Miss**: Not mentioned in any security section.

**Recommended Mitigation**:
1. **Re-verify checksums at mount time**: Before starting container, re-hash all binaries in `~/.tsuku/tools/` that will be mounted
2. **Immutable tool storage**: Use SquashFS or similar to create read-only filesystem images for tools after installation
3. **File integrity monitoring**: Maintain a manifest of expected hashes, check on every sandbox invocation

**Implementation Priority**: HIGH - This is a critical vulnerability in multi-user systems or compromised hosts.

---

### 1.2 Malicious Tool Execution (Post-Installation)

**Attack Vector**: Implicit dependencies execute arbitrary code during build process. If a tool is compromised (either at install time or via TOCTOU), it has unfettered execution access during the build.

**Attack Scenario**:
```
1. User installs neovim via tsuku (requires cmake implicit dep)
2. Compromised cmake binary executes during build:
   - Scans workspace for credentials/secrets (SSH keys, API tokens in config files)
   - Injects backdoor into neovim binary
   - Exfiltrates data via DNS tunneling (bypasses network monitoring)
```

**Impact**:
- **Code injection**: Compromised compiler/build tool can inject malware into target binary
- **Data theft**: Build tools can read all files in workspace (source code, configs, secrets)
- **Persistence**: Backdoored binary installed to `~/.tsuku/bin/`, executed by user repeatedly

**Likelihood**: Low-Medium (depends on upstream compromise success)

**Original Analysis Coverage**: Partially covered under "Supply Chain Risks" but doesn't emphasize **execution** risk vs. installation risk.

**Gap**: Original analysis focuses on preventing malicious *installation* but assumes tools execute benignly once installed. This is insufficient - even correctly-installed tools could be malicious.

**Recommended Mitigation**:
1. **Seccomp/AppArmor profiles**: Restrict syscalls available to build tools (block network, limit file access to workspace only)
2. **Network isolation**: Disable network access for containers during build (except explicit allowlist for downloads)
3. **Workspace scanning**: Pre-execution scan for secrets (API keys, private keys) and warn user
4. **Audit logging**: Log all executions of implicit dependencies with arguments for forensic analysis

**Implementation Priority**: MEDIUM - Adds defense-in-depth against compromised tools.

---

### 1.3 Container Escape via Binary Exploitation

**Attack Vector**: Mounted binaries contain exploitable vulnerabilities (buffer overflows, use-after-free) that could enable container escape.

**Attack Scenario**:
```
1. cmake-3.28 has unpatched CVE allowing arbitrary code execution
2. Malicious recipe crafted with CMakeLists.txt that triggers vulnerability
3. Exploit achieves container escape, gains host root privileges
4. Attacker can modify ~/.tsuku/tools/ to persist access
```

**Impact**: Complete host compromise from unprivileged container.

**Likelihood**: Low (requires known vulnerability in mounted binary + exploit development)

**Original Analysis Miss**: "Execution Isolation" section mentions container compromise but assumes attacker is limited to reading tools directory. It doesn't consider **container escape** via exploiting the mounted binaries themselves.

**Recommended Mitigation**:
1. **Tool version pinning with CVE monitoring**: Track known vulnerabilities in implicit dependencies, auto-update recipes
2. **Minimal container capabilities**: Drop all capabilities except CAP_CHOWN, CAP_DAC_OVERRIDE (required for build tools)
3. **User namespaces**: Run container as non-root user mapped to unprivileged UID on host
4. **gVisor/Kata Containers**: Use stronger isolation runtime for high-security environments

**Implementation Priority**: LOW-MEDIUM - Defense-in-depth measure; requires CVE database integration.

---

### 1.4 Shared Tools Directory Privilege Escalation

**Attack Vector**: Multiple users on shared system install tools to same `~/.tsuku/tools/` directory, allowing one user to compromise another's builds.

**Attack Scenario** (Multi-user system):
```
1. UserA installs cmake-3.28 to ~/.tsuku/tools/ (shared directory)
2. UserB runs tsuku install neovim --sandbox (uses UserA's cmake)
3. UserA replaces cmake with malicious binary
4. UserB's container executes malicious cmake with UserB's workspace access
```

**Impact**: Cross-user compromise in shared environments (CI/CD servers, shared development machines).

**Likelihood**: Low (uncommon for ~/.tsuku to be shared, but possible misconfiguration)

**Original Analysis Miss**: Assumes single-user installation model. Multi-user scenarios not evaluated.

**Recommended Mitigation**:
1. **Documentation warning**: Explicitly document that `~/.tsuku` must NOT be shared between users
2. **Permission validation**: On startup, verify `~/.tsuku/tools/` is owned by current user and not world-writable
3. **User-isolated tool storage**: Force per-user tool directories even on shared systems

**Implementation Priority**: LOW - Document as unsupported configuration, validate permissions on startup.

---

## 2. Mitigation Sufficiency Evaluation

### 2.1 Download Verification (SHA256 Checksums)

**Assessment**: ✅ **Sufficient** for detecting tampered downloads, but insufficient alone.

**Strengths**:
- Industry-standard cryptographic hash function (SHA256)
- Prevents man-in-the-middle attacks during download
- Same mechanism as normal tsuku installs (consistency)

**Weaknesses**:
- **Checksum rotation lag**: Users with stale registries use outdated checksums (acknowledged as residual risk)
- **Single point of failure**: Compromising recipe repository allows checksum replacement
- **No signature verification**: Cannot prove checksums came from trusted source

**Recommendation**:
- ✅ Keep current mitigation (SHA256 verification)
- ➕ Add GPG signature verification for recipes (future work)
- ➕ Implement checksum pinning (warn if checksum changes unexpectedly)

**Priority**: MEDIUM - Current mitigation adequate for initial release, improve incrementally.

---

### 2.2 Read-Only Container Mounts

**Assessment**: ✅ **Sufficient** for preventing container modification of tools, but doesn't prevent all risks.

**Strengths**:
- Prevents container from tampering with tools on host
- Standard Docker isolation mechanism
- Minimal performance overhead

**Weaknesses**:
- **Doesn't prevent execution**: Malicious binaries can still execute even if read-only
- **Information disclosure**: Container can enumerate all installed tools (acknowledged)
- **No integrity checking**: Read-only mount doesn't verify binary hasn't been modified since installation

**Recommendation**:
- ✅ Keep current mitigation (read-only mounts)
- ➕ Add integrity checking before mount (see TOCTOU mitigation above)
- ➕ Consider per-recipe tool mounts instead of entire `~/.tsuku/tools/` (principle of least privilege)

**Priority**: MEDIUM - Read-only mounts are good baseline, add integrity checks for completeness.

---

### 2.3 Recipe PR Review Process

**Assessment**: ⚠️ **Insufficient** as sole supply chain security control.

**Strengths**:
- Human review can catch obvious malicious changes
- Version control provides audit trail
- GitHub PR model is standard practice

**Weaknesses**:
- **Doesn't scale**: Manual review of every checksum update is impractical
- **Trust model unclear**: Who reviews? What credentials required? How are compromised maintainers detected?
- **No automated verification**: Reviewers must manually verify checksums against upstream (tedious, error-prone)
- **Single repo compromise**: If tsuku-registry GitHub account compromised, attacker can merge malicious PRs

**Recommendation**:
- ✅ Keep PR review as first line of defense
- ➕ Implement automated checksum verification in CI (fetch upstream, compare checksums)
- ➕ Require multiple approvals for implicit dependency recipes
- ➕ Add CODEOWNERS file restricting who can approve dependency changes
- ➕ Implement reproducible builds verification where possible

**Priority**: HIGH - Supply chain security is critical; current process has gaps.

---

### 2.4 Sandbox Container Isolation

**Assessment**: ✅ **Mostly sufficient**, but could be strengthened.

**Strengths**:
- Docker/Podman provide battle-tested isolation
- Prevents most container escape scenarios
- Standard industry practice

**Weaknesses**:
- **Assumes secure container runtime**: Docker/Podman vulnerabilities could allow escape
- **Default capabilities**: Containers run with more capabilities than necessary
- **No seccomp profile**: Build tools have full syscall access
- **Network access**: Containers can exfiltrate data via network (if enabled)

**Recommendation**:
- ✅ Keep container isolation as core mitigation
- ➕ Add seccomp profile restricting build tool syscalls
- ➕ Disable network by default for sandbox builds (unless recipe declares network_required)
- ➕ Drop unnecessary container capabilities (CAP_NET_RAW, CAP_SYS_ADMIN, etc.)

**Priority**: MEDIUM - Container isolation is adequate baseline, hardening adds defense-in-depth.

---

## 3. Residual Risk Assessment

### 3.1 Acknowledged Residual Risks

**Risk 1: Stale Registry Attacks**
- **Severity**: MEDIUM
- **Likelihood**: LOW (requires user neglect + attacker timing)
- **Escalation Needed**: ❌ No - Document user best practices (regular `tsuku update-registry`)

**Risk 2: Tool Enumeration (Information Disclosure)**
- **Severity**: LOW
- **Likelihood**: HIGH (any compromised container can enumerate)
- **Escalation Needed**: ❌ No - Minimal value to attacker, acceptable trade-off for functionality

**Risk 3: Sophisticated Dual-Compromise Attack**
- **Severity**: CRITICAL
- **Likelihood**: VERY LOW (requires compromising both upstream and recipe repo)
- **Escalation Needed**: ⚠️ **YES** - Document as known limitation, prioritize signature verification work

---

### 3.2 Unacknowledged Residual Risks

**Risk 4: TOCTOU Window (New Finding)**
- **Severity**: HIGH
- **Likelihood**: MEDIUM (trivial exploit if host compromised)
- **Escalation Needed**: ✅ **YES** - Implement integrity checking before mount (see section 1.1)

**Risk 5: Malicious Tool Execution (New Finding)**
- **Severity**: HIGH
- **Likelihood**: LOW (depends on supply chain compromise)
- **Escalation Needed**: ✅ **YES** - Add network isolation and seccomp profiles (see section 1.2)

**Risk 6: Container Escape via Binary Exploitation (New Finding)**
- **Severity**: CRITICAL
- **Likelihood**: LOW (requires exploitable vulnerability)
- **Escalation Needed**: ⚠️ **MAYBE** - Document as known risk, implement CVE monitoring for future versions

**Risk 7: Multi-User Compromise (New Finding)**
- **Severity**: MEDIUM
- **Likelihood**: VERY LOW (uncommon configuration)
- **Escalation Needed**: ❌ No - Document as unsupported, validate permissions on startup

---

## 4. "Not Applicable" Justification Review

### 4.1 Telemetry/Logging Claims

**Original Claim**: "Not applicable considerations: This feature does not introduce telemetry, logging, or external API calls beyond existing tsuku behavior."

**Review**: ⚠️ **PARTIALLY INCORRECT**

**Gap 1: Implicit Dependency Downloads ARE New Telemetry**
- Original tsuku: User explicitly runs `tsuku install cmake` → telemetry event for cmake
- New behavior: User runs `tsuku install neovim --sandbox` → tsuku silently installs cmake → telemetry event for cmake
- **Privacy implication**: User's implicit dependency graph is exposed without explicit consent
- **Mitigation needed**: Document telemetry behavior, allow opt-out for implicit deps

**Gap 2: Container Execution Logging**
- New feature executes tools in containers → new execution patterns to log
- Build tool failures inside container need logging for debugging
- **Security implication**: Logs may contain sensitive data (file paths, environment variables)
- **Mitigation needed**: Document what's logged, sanitize sensitive data from logs

**Recommendation**:
- ❌ Remove "not applicable" claim for telemetry/logging
- ➕ Add section: "Telemetry Changes" documenting implicit dependency tracking
- ➕ Add section: "Logging Considerations" documenting container execution logs and data sensitivity

**Priority**: MEDIUM - Transparency issue; should document actual behavior.

---

### 4.2 Data Access Claims

**Original Claim**: "No new data exposure - tools already had access to workspace in normal install mode"

**Review**: ⚠️ **MISLEADING**

**Gap 1: Implicit Deps Have Broader Access Than Explicit**
- Normal install: User installs `cmake` → cmake only runs when user explicitly invokes it
- Sandbox install: User installs `neovim` → cmake auto-installed and auto-executed
- **Difference**: Implicit execution means tools access workspaces user didn't intend
- **Example**: User installs 10 recipes requiring cmake → cmake sees 10 different workspaces, not just one

**Gap 2: Persistent Tools See Multiple Workspaces**
- Tools installed to `~/.tsuku/tools/` are reused across multiple sandbox builds
- Same cmake binary builds multiple unrelated projects → cross-project correlation risk
- **Privacy implication**: Compromised cmake could build profile of user's development activity

**Recommendation**:
- ❌ Revise claim to acknowledge broader access scope
- ➕ Document that implicit dependencies execute across multiple workspaces
- ➕ Consider per-recipe tool isolation for high-security use cases

**Priority**: LOW - Disclosure issue, doesn't change security posture but should be documented accurately.

---

### 4.3 Network Access Claims

**Original Claim**: "No new network access during container execution (tools mounted from host)"

**Review**: ✅ **CORRECT**, but incomplete.

**Missing Context**:
- Claim is accurate for mounted tools (no downloads during build)
- BUT: Doesn't mention whether containers have network access for *other* purposes
- Build tools could exfiltrate data via network if container has internet access
- **Uncertainty**: Does current sandbox implementation disable network? If not, claim is misleading.

**Recommendation**:
- ✅ Keep claim (accurate as stated)
- ➕ Clarify container network policy: "Containers have network access disabled by default except for recipes declaring `network_required = true`" (or whatever the actual policy is)

**Priority**: LOW - Clarification needed but no security gap if network is already disabled.

---

## 5. Recommended Security Enhancements

### 5.1 Immediate (Block Release)

| Enhancement | Priority | Effort | Impact |
|-------------|----------|--------|--------|
| Add integrity re-verification before mount (TOCTOU mitigation) | HIGH | Medium | Prevents race condition attacks |
| Document multi-user unsupported, validate permissions | HIGH | Low | Prevents cross-user compromise |
| Add telemetry/logging disclosure section | MEDIUM | Low | Transparency/compliance |

### 5.2 Short-Term (Next Release)

| Enhancement | Priority | Effort | Impact |
|-------------|----------|--------|--------|
| Implement seccomp profile for build tools | MEDIUM | Medium | Restricts syscall access |
| Add automated checksum verification in CI | HIGH | Medium | Improves supply chain security |
| Disable container network by default | MEDIUM | Low | Prevents data exfiltration |
| Drop unnecessary container capabilities | MEDIUM | Low | Reduces attack surface |

### 5.3 Long-Term (Future Roadmap)

| Enhancement | Priority | Effort | Impact |
|-------------|----------|--------|--------|
| GPG signature verification for recipes | HIGH | High | Proves recipe authenticity |
| CVE monitoring for implicit dependencies | MEDIUM | High | Detects vulnerable tools |
| Reproducible builds verification | HIGH | Very High | Eliminates trust in binaries |
| Per-recipe tool isolation (separate mounts) | LOW | Medium | Reduces information disclosure |

---

## 6. Threat Model Summary

### 6.1 Threat Actors

| Actor | Capability | Motivation | Likelihood |
|-------|------------|------------|------------|
| **Nation-state** | Compromise upstream providers, develop exploits | Espionage, supply chain attack | Low |
| **Cybercriminal** | Compromise mirrors, inject malware | Cryptocurrency mining, ransomware | Medium |
| **Insider threat** | Malicious recipe submissions, registry compromise | Sabotage, data theft | Low |
| **Local attacker** | Host filesystem access (TOCTOU attacks) | Privilege escalation, persistence | Medium |
| **Opportunist** | Exploit public CVEs in build tools | Botnet recruitment, crypto mining | Medium |

### 6.2 Attack Trees

**Attack Goal: Execute Malicious Code in Sandbox**

```
Execute Malicious Code in Sandbox
├── Compromise Tool at Installation
│   ├── Upstream Provider Compromise
│   │   └── Mitigated by: Checksum verification (partial)
│   ├── Recipe Repository Compromise
│   │   └── Mitigated by: PR review (weak), CODEOWNERS (future)
│   └── Man-in-the-Middle Attack
│       └── Mitigated by: HTTPS + checksum verification
├── Compromise Tool Post-Installation (TOCTOU)
│   └── Replace Binary Between Verification and Mount
│       └── NOT MITIGATED ⚠️ (Critical gap)
└── Exploit Vulnerability in Legitimate Tool
    └── Trigger CVE in cmake/zig/make
        └── NOT MITIGATED ⚠️ (No CVE monitoring)
```

**Attack Goal: Escape Container to Compromise Host**

```
Escape Container to Compromise Host
├── Exploit Container Runtime Vulnerability
│   └── Mitigated by: Use up-to-date Docker/Podman (user responsibility)
├── Exploit Build Tool Vulnerability for Escape
│   └── NOT MITIGATED ⚠️ (No seccomp, full syscall access)
└── Exploit Kernel Vulnerability via Syscalls
    └── Partially mitigated by: Container isolation (could improve with seccomp)
```

---

## 7. Compliance and Regulatory Considerations

### 7.1 SLSA Framework Alignment

**Current State**: SLSA Level 0 (no provenance, no verification beyond checksums)

**Gap Analysis**:
- ❌ SLSA L1: No build provenance for implicit dependencies
- ❌ SLSA L2: No signed provenance, no tamper protection
- ❌ SLSA L3: No hardened build platform, no non-falsifiable provenance

**Recommendation**: Document SLSA compliance roadmap, prioritize provenance generation.

### 7.2 GDPR/Privacy Implications

**Data Processing**:
- Implicit dependency installations generate telemetry (installation events)
- User's development activity graph exposed via implicit dependency patterns
- Container logs may contain personal data (file paths, usernames)

**Compliance Requirements**:
- ✅ Document data collection in privacy policy
- ✅ Provide opt-out mechanism for telemetry
- ✅ Implement log data retention limits

**Recommendation**: Legal review of telemetry changes, update privacy policy before release.

---

## 8. Conclusion and Risk Rating

### 8.1 Overall Security Posture

**Rating**: ⚠️ **MEDIUM RISK** with critical gaps requiring mitigation before release.

**Rationale**:
- Core security mechanisms (checksums, container isolation, read-only mounts) are sound
- **CRITICAL GAP**: TOCTOU vulnerability is exploitable and unmitigated
- **HIGH GAP**: No defense-in-depth against compromised tool execution
- **MEDIUM GAP**: Supply chain security relies too heavily on manual review

### 8.2 Release Recommendation

**CONDITIONAL APPROVAL** - Address critical findings before release:

**Blockers (Must Fix)**:
1. Implement TOCTOU mitigation (integrity re-verification before mount)
2. Document multi-user scenario as unsupported, add permission validation
3. Add telemetry/logging disclosure section (transparency)

**Strongly Recommended (Should Fix)**:
4. Add automated checksum verification in CI
5. Implement seccomp profile for build tools
6. Disable container network access by default

**Future Work (May Defer)**:
7. GPG signature verification for recipes
8. CVE monitoring for implicit dependencies
9. Reproducible builds verification

### 8.3 Comparison to Alternatives

**vs. Normal Install Mode (No Sandbox)**:
- ✅ Better isolation (container protects host)
- ⚠️ New attack surface (TOCTOU, mounted binaries)
- ✅ Better privacy (container limits tool access to workspace only)

**vs. Fully Isolated Container Build (Nix-style)**:
- ❌ Weaker isolation (host tools mounted vs. hermetic container)
- ✅ Better performance (no duplicate downloads/builds)
- ⚠️ Same supply chain risks (nix packages also require trust)

**Recommendation**: Sandbox mode with mitigations is acceptable risk for most users. High-security users should wait for GPG signatures + reproducible builds.

---

## 9. Security Review Checklist

### 9.1 Attack Vector Coverage

- ✅ Download interception (MitM) - Covered
- ✅ Upstream compromise - Covered
- ✅ Recipe repository compromise - Covered
- ⚠️ TOCTOU race condition - **MISSED (Critical)**
- ⚠️ Malicious tool execution - Partially covered
- ⚠️ Container escape via exploitation - **MISSED (High)**
- ⚠️ Multi-user privilege escalation - **MISSED (Medium)**
- ✅ Information disclosure - Covered

**Gap Summary**: 3 critical attack vectors not adequately addressed.

### 9.2 Mitigation Effectiveness

- ✅ SHA256 checksums - Effective for download verification
- ✅ Read-only mounts - Effective for preventing modification
- ⚠️ PR review - Weak without automation
- ⚠️ Container isolation - Adequate but could be hardened

**Gap Summary**: Supply chain security relies too heavily on manual review; container hardening needed.

### 9.3 "Not Applicable" Justifications

- ❌ Telemetry claim - **INCORRECT** (implicit deps change telemetry behavior)
- ⚠️ Data access claim - **MISLEADING** (broader scope than acknowledged)
- ✅ Network access claim - Correct but incomplete

**Gap Summary**: 2 of 3 "not applicable" claims need revision.

---

## 10. Final Recommendations

### 10.1 Immediate Actions (Before Release)

1. **Fix TOCTOU vulnerability**: Implement integrity re-verification before mounting tools to containers
2. **Validate installation permissions**: Add startup check that `~/.tsuku/tools/` is owned by current user
3. **Revise security documentation**: Remove incorrect "not applicable" claims, add telemetry disclosure

### 10.2 Short-Term Actions (Next Release)

4. **Automate checksum verification**: Add CI job that fetches upstream binaries and verifies recipe checksums
5. **Harden container isolation**: Add seccomp profile, drop capabilities, disable network by default
6. **Implement audit logging**: Log all implicit dependency executions with arguments for forensic analysis

### 10.3 Long-Term Actions (Roadmap)

7. **Add signature verification**: Implement GPG signatures for recipes, verify signatures on registry updates
8. **CVE monitoring**: Integrate vulnerability database, warn users about outdated implicit dependencies
9. **Reproducible builds**: Implement reproducible build verification where possible (zig, go)

### 10.4 Process Improvements

10. **Security review process**: Require security review for all PRs modifying implicit dependency recipes
11. **Threat modeling**: Conduct formal threat modeling session for each new feature
12. **Penetration testing**: Commission external pentest of sandbox implementation before 1.0 release

---

## Appendix A: Reference Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                         Host System                          │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ Tsuku CLI (Install neovim --sandbox)                   │ │
│  │                                                         │ │
│  │ 1. ensurePackageManagersForRecipe()                    │ │
│  │    ├─> Download cmake-3.28.tar.gz                      │ │
│  │    ├─> Verify SHA256 ✓                                 │ │
│  │    ├─> Extract to ~/.tsuku/tools/cmake-3.28/           │ │
│  │    └─> [TOCTOU WINDOW] ⚠️                              │ │
│  │                                                         │ │
│  │ 2. Start Container                                     │ │
│  │    └─> docker run \                                    │ │
│  │         --volume ~/.tsuku/tools:/workspace/tsuku/tools:ro │
│  └──────────────────┬─────────────────────────────────────┘ │
│                     │                                        │
│  ┌──────────────────▼─────────────────────────────────────┐ │
│  │ ~/.tsuku/tools/  (Persistent, multi-workspace)         │ │
│  │ ├── cmake-3.28/bin/cmake                               │ │
│  │ ├── zig-0.11.0/bin/zig                                 │ │
│  │ └── make-4.3/bin/make                                  │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                      │
                      │ Read-only mount
                      ▼
┌─────────────────────────────────────────────────────────────┐
│                    Container (Isolated)                      │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ /workspace/tsuku/tools/  (Read-only)                   │ │
│  │ ├── cmake-3.28/bin/cmake                               │ │
│  │ ├── zig-0.11.0/bin/zig                                 │ │
│  │ └── make-4.3/bin/make                                  │ │
│  └────────────────────────────────────────────────────────┘ │
│                     │                                        │
│                     │ Execute during build                   │
│                     ▼                                        │
│  ┌────────────────────────────────────────────────────────┐ │
│  │ Build Process (neovim compilation)                     │ │
│  │                                                         │ │
│  │ Risks:                                                 │ │
│  │ • Compromised cmake reads workspace source ⚠️          │ │
│  │ • Injected backdoor into neovim binary ⚠️              │ │
│  │ • Data exfiltration via network ⚠️                     │ │
│  │ • Container escape via cmake CVE ⚠️                    │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## Appendix B: Exploit Proof-of-Concept (TOCTOU)

**Scenario**: Attacker with host filesystem access (malware, compromised user account) exploits TOCTOU window.

```bash
#!/bin/bash
# TOCTOU exploit PoC for tsuku sandbox implicit deps

# 1. Monitor for tsuku installations
inotifywait -m ~/.tsuku/tools/ -e create -e moved_to |
while read path action file; do
    # 2. Wait for extraction to complete
    sleep 1

    # 3. Identify binaries
    if [[ "$file" == "cmake-"* ]] && [[ -d "$path$file/bin/" ]]; then
        binary="$path$file/bin/cmake"

        # 4. Replace with malicious binary
        if [[ -f "$binary" ]]; then
            echo "[*] Replacing $binary with malicious version"
            cp /tmp/malicious_cmake "$binary"
            chmod +x "$binary"
            echo "[+] Exploit successful - malicious cmake will execute in next sandbox run"
        fi
    fi
done
```

**Impact**: Next `tsuku install <any-recipe-requiring-cmake> --sandbox` will execute attacker's binary in container with full workspace access.

**Mitigation**: Re-verify checksums immediately before mount, fail if mismatch detected.

## Appendix C: CVE Research (Implicit Dependencies)

**Known vulnerabilities in common build tools** (as of January 2025):

| Tool | Recent CVEs | Severity | Exploitability |
|------|-------------|----------|----------------|
| CMake | CVE-2024-XXXX (hypothetical) | High | Code execution via malicious CMakeLists.txt |
| GNU Make | CVE-2023-XXXX (hypothetical) | Medium | Command injection via crafted Makefile |
| Zig | No known CVEs | N/A | Young project, less audited |
| pkg-config | CVE-2021-XXXX (hypothetical) | Low | Information disclosure |

**Recommendation**: Implement CVE monitoring for all implicit dependencies, auto-update recipes when patches available.

---

**Document Version**: 1.0
**Review Date**: 2026-01-04
**Reviewer**: Claude (Security Analysis Agent)
**Next Review**: Before feature release
