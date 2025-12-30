# Security Assessment: System Dependency Actions

## Executive Summary

The proposed system dependency actions design introduces privileged operations (apt_install, apt_repo, dnf_install, group_add, service_enable) that execute with elevated permissions. This assessment evaluates privilege escalation risks, attack surface, audit requirements, content-addressing gaps, and sandbox vs host execution differences.

**Overall Assessment**: The design demonstrates security-conscious thinking with its "no shell primitive" constraint, content-addressing for external resources, and explicit user consent flow. However, several areas require additional safeguards before production deployment.

## 1. Privilege Escalation Risks

### Direct Escalation Vectors

**group_add primitive**: Adding users to privileged groups (docker, wheel, sudo) provides effective root access. The docker group is explicitly mentioned, which grants container root equivalent to host root. Recommendation: Implement a group allowlist and treat docker/wheel/sudo additions as high-severity operations requiring additional confirmation.

**service_enable/service_start primitives**: Enabling arbitrary systemd services can start attacker-controlled code at boot time with root privileges. If a recipe first installs a malicious package and then enables its service, the escalation path is complete. Recommendation: Consider validating that enabled services are from packages installed in the same recipe.

**apt_repo/dnf_repo primitives**: Adding third-party repositories shifts trust to external maintainers. A compromised repository can serve malicious package updates indefinitely. The content-addressed GPG key mitigates initial installation but not future package updates. Recommendation: Document that adding repositories creates long-term trust relationships and consider repository allowlisting for official sources.

### Indirect Escalation Paths

The design correctly identifies that `group_add`, `file_write` (future), and `service_enable` create escalation paths (see Future Work section). However, the current design does not implement mitigations. These should be addressed before Phase 4 (Host Execution).

## 2. Attack Surface Analysis

### apt_install / dnf_install

**Attack Surface**: Medium. Relies on package manager integrity (apt/dnf GPG verification) and package repository trust.

**Risks**:
- Typosquatting attacks (package name confusion)
- Dependency confusion (malicious packages with popular names)
- Time-of-check-time-of-use (TOCTOU) between review and installation if package version not pinned

**Mitigation**: Package managers provide strong verification. Consider adding optional version pinning (`apt = ["docker-ce=5:24.0.7-1~ubuntu.22.04~jammy"]`).

### apt_repo / dnf_repo

**Attack Surface**: High. Introduces external trust roots and fetches GPG keys from the network.

**Risks**:
- GPG key URL could serve different content (mitigated by content-addressing)
- Repository URL could redirect (HTTP to HTTPS upgrade is noted, but malicious redirects are possible)
- Repository content changes after initial addition

**Current Mitigations**:
- key_sha256 required for all external keys (good)
- Preflight validation rejects missing hashes (good)

**Gaps**:
- Repository URL itself is not content-addressed
- No TLS certificate pinning
- No repository scope restriction (could add PPAs with broader package coverage)

### group_add

**Attack Surface**: High. Direct privilege escalation vector.

**Risks**:
- Adding to docker/wheel/sudo grants effective root
- No validation of group legitimacy
- Silent privilege accumulation

**Recommendation**: Implement tiered group categories:
- Safe (unprivileged): dialout, cdrom, floppy
- Elevated (capability-granting): docker, libvirt, kvm
- Dangerous (root-equivalent): wheel, sudo, root

### service_enable / service_start

**Attack Surface**: Medium-High. Enables persistent execution with privileges.

**Risks**:
- Services run as root by default
- No validation that service is from installed packages
- Enables persistence mechanism for malicious code

**Recommendation**: Validate service unit file exists and was installed by a package from the same recipe.

## 3. Audit Trail and User Consent Requirements

### Current Design

The design specifies a user consent flow displaying primitives before execution and mentions audit logging. This is appropriate but needs strengthening.

### Recommendations

**Audit Log Requirements**:
1. Log all privileged operations with timestamp, primitive type, parameters, and outcome
2. Log the recipe name, version, and content hash that triggered the operation
3. Store logs outside `$TSUKU_HOME` to prevent tampering (e.g., syslog or `/var/log/tsuku/`)
4. Include cryptographic binding to the recipe content that authorized the operation

**Consent Flow Enhancements**:
1. Categorize operations by risk level (informational for brew, warning for apt, critical for group_add to docker)
2. Require typing "yes" for high-risk operations rather than just "y"
3. Provide a `--system-deps-review` mode that shows full details including URLs, hashes, and equivalent shell commands
4. Consider requiring `--allow-privileged` flag explicitly (not just `--system-deps`)

## 4. Content-Addressing Gaps

### Current Coverage

- GPG key URLs for apt_repo/dnf_repo: SHA256 required (good)
- Package names: Not addressed (relies on package manager)

### Missing Content-Addressing

**Repository URLs**: The repository URL itself (e.g., `https://download.docker.com/linux/ubuntu`) is not hashed. An attacker controlling DNS or network could redirect to a malicious mirror after review.

**Recommendation**: For apt_repo/dnf_repo, consider:
1. Repository URL allowlist (official distribution and major vendor repositories)
2. Optional repository content hash (Release file signature verification)
3. TLS certificate fingerprint pinning for high-security installations

**Service Unit Files**: The service_enable primitive does not verify the service unit content. A malicious package could install a legitimate-looking service that executes attacker code.

**Recommendation**: Consider optional service unit content verification for sensitive installations.

**Package Versions**: Package names are specified but versions are optional. A recipe reviewed with docker-ce 24.0 could install a compromised 25.0.

**Recommendation**: Add optional version pinning with content-addressable package checksums for high-security recipes.

## 5. Sandbox vs Host Execution Security Differences

### Sandbox Context (Container)

**Security Properties**:
- Ephemeral: Container destroyed after test
- Isolated: No host filesystem access (except explicit mounts)
- Resource-limited: Memory, CPU, process caps
- Network-isolated: No host network by default

**Remaining Risks**:
- Container escapes (mitigated by not using privileged containers)
- Denial of service (resource limits help)
- Information disclosure via network (if network enabled)

**Assessment**: The sandbox design is sound. Running as root inside the container is acceptable for testing purposes.

### Host Context (User Machine)

**Security Properties**:
- Persistent: Changes survive execution
- Privileged: sudo access to system
- Audited: Operations logged
- Consented: User confirms before execution

**Remaining Risks**:
- Privilege persistence (group changes, services)
- System configuration modification
- Difficult-to-reverse changes (removing packages may not undo all effects)

**Assessment**: Host execution requires stronger safeguards than sandbox. The consent flow is necessary but not sufficient.

### Key Difference: Reversibility

Sandbox operations are inherently reversible (destroy container). Host operations are not. The design should emphasize this distinction more strongly:

1. Provide `tsuku rollback` mechanism for system changes
2. Record baseline state before modifications
3. Warn users that some operations (group_add, apt_repo) are difficult to fully reverse

## 6. Recommendations for Security Constraints

### High Priority (Before Phase 4 - Host Execution)

1. **Group Allowlist**: Implement categorized group lists. Require explicit `--allow-privileged-groups` for docker/wheel/sudo.

2. **Repository Allowlist**: Maintain a list of known-safe repository domains. Warn or block unknown repositories.

3. **Audit Log Implementation**: Implement tamper-resistant logging before enabling host execution. Log to syslog, not just local files.

4. **Risk-Tiered Consent**: Different confirmation flows for different risk levels:
   - Low risk (brew install): y/n prompt
   - Medium risk (apt install): y/n with package list
   - High risk (apt_repo, group_add): Require typing "yes" and display warnings

### Medium Priority (Before GA)

5. **Package Version Pinning**: Optional but recommended for security-sensitive recipes.

6. **Service Validation**: Verify service_enable targets are from installed packages.

7. **Rollback Mechanism**: Track system modifications and provide undo capability.

8. **Repository Content Verification**: For apt_repo, verify Release file signature matches expected key.

### Low Priority (Future Hardening)

9. **TLS Certificate Pinning**: For official repository URLs.

10. **Package Checksum Verification**: Content-address specific package versions.

11. **SELinux/AppArmor Integration**: Confine primitive execution further.

## Conclusion

The design makes sound architectural decisions: no shell primitives, content-addressed external resources, explicit user consent, and clear sandbox isolation. The primary gaps are in the host execution context where group_add and apt_repo create privilege escalation and trust extension vectors without adequate safeguards.

The recommendation is to implement group allowlisting and repository allowlisting before enabling Phase 4 (Host Execution). These constraints can be relaxed later if they prove too restrictive, but starting strict establishes a secure baseline.

The "no shell primitive" constraint is the design's strongest security property. This should be maintained indefinitely - any future requests to add shell execution should be rejected in favor of purpose-built primitives with narrow, auditable behavior.
