# Security Analysis: Validation Centralization Design

**Date**: 2025-12-14
**Reviewer**: Security Analysis Agent
**Design Document**: /home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/docs/DESIGN-centralize-validation.md

## Executive Summary

The validation centralization design improves security posture by making network requirements explicit and consolidating validation logic. However, several attack vectors require additional consideration, and some mitigations need strengthening. Most critically, the design introduces a new attack surface through `tsuku validate` that allows execution of arbitrary recipes, and the metadata registry creates a single point of failure for supply chain security.

**Verdict**: Design is generally sound but requires additional security controls before implementation.

## Critical Findings

### 1. Arbitrary Recipe Execution via `tsuku validate`

**Risk Level**: HIGH
**Status**: ACKNOWLEDGED BUT INSUFFICIENTLY MITIGATED

The design introduces a new user-facing command `tsuku validate <recipe.toml>` that executes arbitrary recipes in containers. While container isolation provides some protection, this creates significant attack surface:

**Attack Vectors Not Considered**:

1. **Resource exhaustion attacks**:
   - Malicious recipe could include actions that consume maximum allowed resources (2GB memory, 2 CPUs)
   - Multiple parallel validations could exhaust host resources
   - Example: Recipe with `go_build` that fetches massive dependency trees
   - Example: Recipe with infinite loops or fork bombs within process limits

2. **Container escape attempts**:
   - While rare, container escapes are documented (CVE-2019-5736 for runc, CVE-2022-0492 for cgroups)
   - Design assumes container isolation is perfect but provides no defense-in-depth
   - Rootless containers (Podman, Docker rootless) provide better isolation than Docker with group membership
   - No verification that rootless mode is being used

3. **Container image vulnerabilities**:
   - Design uses `debian:bookworm-slim` and `ubuntu:22.04` as base images
   - No mechanism to verify image checksums or use specific image digests
   - Vulnerable packages in base images could be exploited
   - No regular image update policy specified

4. **Side-channel attacks**:
   - Timing attacks: Malicious recipe could measure execution time to infer host information
   - Resource contention: Recipe could probe for other containers/processes via resource availability
   - /proc leakage: Even with read-only root, /proc may expose host information

5. **Social engineering vectors**:
   - User downloads malicious "install firefox" recipe from untrusted source
   - Recipe appears normal but includes network-enabled actions that exfiltrate data
   - No warning system for recipes from untrusted sources
   - No sandboxing or approval workflow for first-time recipe validation

**Mitigations Missing**:

1. **No rate limiting**: User could spam `tsuku validate` to DoS their own system
2. **No network egress filtering**: When `RequiresNetwork=true`, container has full network access
3. **No capability dropping**: Containers run with default capabilities (could be reduced)
4. **No seccomp/AppArmor profiles**: No syscall filtering beyond container defaults
5. **No user confirmation**: `tsuku validate malicious.toml` runs immediately without warning

**Recommended Additions**:

```markdown
## Enhanced Security Controls

1. **Container Hardening**:
   - Use specific image digests instead of tags (debian:bookworm-slim@sha256:...)
   - Add seccomp profile to restrict syscalls (blocking mount, reboot, etc.)
   - Drop unnecessary capabilities (--cap-drop=ALL, selectively add back)
   - Enforce rootless runtime where available

2. **Network Isolation**:
   - For RequiresNetwork=true, use firewall rules to restrict egress
   - Block access to private IP ranges (RFC 1918, link-local)
   - Consider DNS-only network mode for ecosystem builds

3. **User Interaction**:
   - Display warning for recipes from untrusted sources
   - Show network requirements before validation
   - Add --yes flag to skip confirmation (for automation)
   - Implement recipe source allowlist (default: official registry only)

4. **Resource Management**:
   - Add global concurrency limit for parallel validations
   - Implement exponential backoff for repeated validation failures
   - Add resource usage reporting post-validation
```

### 2. ActionValidationMetadata Registry as Supply Chain Target

**Risk Level**: MEDIUM
**Status**: INSUFFICIENTLY MITIGATED

The design centralizes all build tool requirements into a single Go map in `internal/actions/validation_metadata.go`. This creates a high-value target for supply chain attacks.

**Attack Vectors Not Considered**:

1. **Malicious package injection**:
   - Attacker with commit access adds typosquatted package name
   - Example: `autoconf` -> `autoconF` (capital F)
   - Example: Adding seemingly-legitimate `build-tools-common` package that doesn't exist
   - Debian/Ubuntu will fail installation, but error might be ignored in verbose output

2. **Excessive dependency addition**:
   - Attacker adds unnecessary but real packages to increase attack surface
   - Example: Adding `openssh-server` to `cargo_build` requirements
   - Each additional package increases probability of vulnerable code being present

3. **Version-specific vulnerabilities**:
   - Design doesn't specify package versions
   - `apt-get install autoconf` gets whatever version is in the repo
   - Could get vulnerable version if base image is stale

4. **Compromised package repositories**:
   - Containers fetch packages from Debian/Ubuntu mirrors
   - No signature verification beyond apt defaults
   - Man-in-the-middle on network could substitute packages (mitigated if using HTTPS mirrors)

5. **Insider threat**:
   - Maintainer with malicious intent could add compromised packages
   - Code review may not catch subtle package name issues
   - No automated verification of package legitimacy

**Current Mitigation Analysis**:

The design states:
> "Mitigation: Code-reviewed, well-known package names, low typosquatting risk."

This is **insufficient** because:
- Code review catches obvious issues but not subtle typosquatting
- "Well-known package names" is subjective and not enforced
- No automated verification that packages are legitimate
- No baseline or diff checking for package list changes

**Recommended Additions**:

```markdown
## Metadata Registry Security

1. **Automated Verification**:
   - CI check that all packages in ActionValidationMetadata exist in Debian/Ubuntu repos
   - Allowlist of permitted package names (denies any package not on list)
   - Alert on package list changes (require manual approval for additions)

2. **Package Pinning**:
   - Consider pinning package versions for deterministic builds
   - Use apt package snapshots (snapshot.debian.org) for reproducibility
   - Document minimum package versions required

3. **Review Requirements**:
   - CODEOWNERS file requiring security team review for metadata changes
   - Documented process for adding new packages to registry
   - Rationale required for each package (comment in code)

4. **Runtime Verification**:
   - Log all packages installed during validation
   - Checksum installed binaries and compare to known-good values
   - Alert if unexpected packages are installed
```

### 3. Network-Enabled Validation Attack Surface

**Risk Level**: MEDIUM
**Status**: ACKNOWLEDGED BUT UNDERSPECIFIED

When `RequiresNetwork=true`, containers run with `--network=host`, granting full network access. The design acknowledges this but doesn't specify sufficient constraints.

**Attack Vectors Not Considered**:

1. **Data exfiltration**:
   - Malicious recipe with `cargo_build` that includes dependency on attacker-controlled crate
   - Build script in crate exfiltrates $TSUKU_HOME contents, environment variables, /etc/passwd
   - No egress filtering prevents this

2. **C2 beaconing**:
   - Recipe build process establishes reverse shell or C2 channel
   - Could be used to pivot into internal network if user is on corporate network
   - Persists beyond container lifetime via network side-effects

3. **Internal network scanning**:
   - Container has access to host network stack
   - Can scan internal IP ranges, probe internal services
   - Could identify vulnerable services on local network

4. **DNS exfiltration**:
   - Even with firewall rules, DNS is often permitted
   - Build process can encode data in DNS queries to attacker-controlled domain

5. **Supply chain compromise**:
   - Ecosystem builds fetch dependencies from public registries (crates.io, npmjs.com, etc.)
   - No verification that fetched dependencies are legitimate
   - Vulnerable to registry compromise or account takeover

**Current Mitigation Analysis**:

The design states:
> "Network enabled only when RequiresNetwork=true"

This is **true but insufficient** because:
- No specification of what "network access" means (full host network vs bridge vs custom)
- No egress filtering or allowlisting
- No documentation of why `--network=host` is necessary vs `--network=bridge`

**Why `--network=host` May Not Be Necessary**:

The design uses `--network=host` for ecosystem builds, presumably for:
1. Access to package registries (crates.io, npmjs.com, etc.)
2. Git clone over HTTPS
3. apt-get for build tool installation

**However**: All of these can work with `--network=bridge` (default NAT networking):
- Outbound connections work fine from bridge networks
- DNS resolution works
- No need for host network stack access

**`--network=host` grants additional capabilities**:
- Access to localhost services on host (Redis, PostgreSQL, etc.)
- Access to internal IP ranges without NAT
- Ability to bind to host ports

**Recommended Changes**:

```markdown
## Network Isolation Improvements

1. **Use Bridge Networking**:
   - Change from `--network=host` to `--network=bridge`
   - Verify ecosystem builds work with bridge networking
   - Only use host networking if absolutely necessary (document why)

2. **Egress Filtering**:
   - Implement iptables/nftables rules for validation containers
   - Allowlist: Package registries, Git hosting, public mirrors
   - Blocklist: Private IP ranges (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
   - Blocklist: Link-local (169.254.0.0/16), localhost

3. **DNS Security**:
   - Use public DNS servers (1.1.1.1, 8.8.8.8) instead of host resolver
   - Consider DNS filtering service (blocks malware domains)
   - Log DNS queries for forensic analysis

4. **Transparency**:
   - Log all network connections during validation
   - Display network access warning to user before validation
   - Provide --offline flag to reject recipes with RequiresNetwork=true
```

### 4. Build Tool Trust Model Unclear

**Risk Level**: LOW
**Status**: NOT ADDRESSED

The design doesn't specify the trust model for build tools installed via apt.

**Questions Not Answered**:

1. **Package authentication**: Does apt verify signatures? (Yes by default, but should be explicit)
2. **Repository trust**: Which repositories are used? (Default Debian/Ubuntu, but could be overridden)
3. **HTTPS enforcement**: Are packages fetched over HTTPS? (Depends on mirror configuration)
4. **Stale package vulnerability**: How often are base images updated? (Not specified)

**Attack Scenarios**:

1. **Stale base image**: `ubuntu:22.04` tag could point to months-old image with vulnerable packages
2. **Mirror compromise**: If apt is configured to use HTTP mirrors, MITM could inject malicious packages
3. **Repository manipulation**: If container has writable `/etc/apt/sources.list`, malicious recipe could add attacker's repository

**Recommended Additions**:

```markdown
## Build Tool Security

1. **Base Image Management**:
   - Use specific image digests instead of tags
   - Automated monthly rebuilds with latest security updates
   - Document minimum base image versions

2. **Package Installation Verification**:
   - Enforce HTTPS for package mirrors (configured in Dockerfile or apt config)
   - Verify apt signature checking is enabled
   - Log package versions installed during validation

3. **Repository Immutability**:
   - Mount /etc/apt read-only to prevent repository manipulation
   - Pre-configure trusted repositories in base image
```

## Medium Priority Findings

### 5. Download Cache as Attack Vector

**Risk Level**: LOW-MEDIUM
**Status**: PARTIALLY MITIGATED

The design mounts the download cache read-only, which is good. However, there are edge cases:

**Attack Vectors**:

1. **Cache poisoning** (pre-validation):
   - If attacker can manipulate download cache before validation, they can substitute malicious binaries
   - Plan specifies checksums, but if plan itself is malicious, checksums match malicious content

2. **Cache timing side-channel**:
   - Malicious recipe could probe cache to determine what tools user has validated
   - Could reveal usage patterns or identify specific user

3. **Symlink attacks**:
   - If cache contains symlinks, read-only mount may not prevent following symlinks outside cache
   - Could expose host filesystem to container

**Current Mitigations**:

- Download cache is mounted read-only ✓
- Checksums verified during plan generation ✓
- Checksums verified during installation ✓

**Gaps**:

- No verification that cache directory doesn't contain symlinks
- No documentation of cache directory permissions/ownership

**Recommended Additions**:

```markdown
## Download Cache Security

1. **Cache Integrity**:
   - Verify no symlinks in cache directory before mounting
   - Set strict permissions on cache directory (0700, owned by user)
   - Consider mounting with nosymfollow option if available

2. **Cache Isolation**:
   - Use separate cache directory per validation to prevent cross-contamination
   - Clean up validation-specific caches after completion
```

### 6. Workspace Isolation Incomplete

**Risk Level**: LOW
**Status**: PARTIALLY ADDRESSED

The design mounts workspace at `/workspace` but doesn't specify workspace creation or cleanup.

**Attack Vectors**:

1. **Workspace persistence**:
   - If workspace is reused across validations, secrets could leak between validations
   - Example: First validation writes SSH key to /workspace/.ssh, second validation reads it

2. **Workspace escape via symlinks**:
   - Malicious recipe could create symlinks in workspace pointing to host filesystem
   - If workspace is mounted writable (it is for source builds), container can create symlinks
   - Symlinks persist after container exits, could be exploited later

3. **Predictable workspace paths**:
   - If workspace is at predictable path like /tmp/tsuku-validate-<tool>, other processes could race to create it
   - Symlink race condition (TOCTOU)

**Current Mitigations**:

- Workspace is container-specific (scoped to validation run)
- No access to user home directory ✓

**Gaps**:

- Workspace creation security not specified
- Workspace cleanup not specified
- Workspace reuse policy not specified

**Recommended Additions**:

```markdown
## Workspace Security

1. **Secure Workspace Creation**:
   - Use mkdtemp() for unpredictable workspace paths
   - Set strict permissions (0700, owned by user)
   - Verify workspace doesn't exist before creation (prevent races)

2. **Workspace Cleanup**:
   - Always clean up workspace after validation (defer removal)
   - Securely delete workspace contents (prevent forensic recovery if sensitive)
   - Handle cleanup failures gracefully (log error, continue)

3. **Workspace Isolation**:
   - Never reuse workspace across validations
   - Verify no symlinks in workspace before mounting
   - Consider mounting workspace with nosymfollow
```

### 7. tsuku Binary Injection Risk

**Risk Level**: LOW
**Status**: ACKNOWLEDGED

The design mounts the host's tsuku binary into containers at `/usr/local/bin/tsuku`. This creates potential for binary substitution.

**Attack Vectors**:

1. **Malicious binary substitution**:
   - If attacker controls user's filesystem, they could replace tsuku binary
   - Validation would execute attacker's binary in container
   - Binary has full container access

2. **Version mismatch vulnerabilities**:
   - No verification that tsuku binary version matches expected version
   - Older versions might have known vulnerabilities
   - Could exploit container runtime or host

**Current Mitigations**:

- Binary is mounted read-only ✓
- Binary is from host, not downloaded ✓

**Gaps**:

- No checksum verification of binary
- No version checking

**Recommended Additions**:

```markdown
## Binary Integrity

1. **Binary Verification**:
   - Compute checksum of tsuku binary before mounting
   - Compare to known-good checksum (embedded in binary or separate file)
   - Warn if checksum doesn't match

2. **Version Checking**:
   - Verify binary version is compatible with validation requirements
   - Reject if version is too old (known vulnerabilities)
```

## Low Priority Findings

### 8. Process Limits May Be Insufficient

**Risk Level**: LOW
**Status**: PARTIALLY ADDRESSED

The design specifies `PidsMax=100` for binary validation and `PidsMax=500` for source builds.

**Analysis**:

- Fork bomb protection: 100 processes is reasonable for preventing simple fork bombs ✓
- Legitimate use: Some builds may spawn many processes (parallel make)
- Bypass: 100 cooperating processes can still consume significant resources

**Recommended Monitoring**:

```markdown
## Resource Monitoring

1. **Process Limit Tuning**:
   - Monitor actual process counts during validation
   - Adjust limits based on real-world usage
   - Consider separate limits for build vs install phases

2. **Additional Limits**:
   - Add file descriptor limit (ulimit -n)
   - Add thread limit (separate from process limit)
   - Add disk I/O limits (--device-write-bps, --device-read-bps)
```

### 9. Metadata Registry Completeness Not Enforced

**Risk Level**: LOW
**Status**: NOT ADDRESSED

The design mentions a test to verify all actions have metadata entries, but doesn't specify enforcement.

**Gaps**:

1. **Missing entry handling**: What happens if action has no metadata entry?
   - Current: Returns zero value (no network, no build tools)
   - Could silently fail to install required tools

2. **Validation**: No validation that metadata is sensible
   - Could have RequiresNetwork=true for "chmod" (nonsensical)

3. **Completeness**: No verification that BuildTools list is complete
   - Could be missing critical tool

**Recommended Additions**:

```markdown
## Metadata Validation

1. **Completeness Enforcement**:
   - CI test that all registered actions have explicit metadata entries
   - Fail build if any action is missing from registry
   - Require comment explaining "no requirements" if metadata is empty

2. **Sanity Checking**:
   - Validate that primitive actions (chmod, extract) don't require network
   - Validate that ecosystem actions (cargo_build, go_build) do require network
   - Flag suspicious combinations for manual review

3. **Documentation**:
   - Require comment for each metadata entry explaining why packages are needed
   - Document testing methodology to verify tools are actually required
```

## Residual Risks Assessment

### Risks Appropriately Accepted

1. **Container runtime trust**: Design assumes container runtime (Podman/Docker) is secure
   - **Assessment**: Reasonable. Container runtimes are widely used and regularly audited.
   - **Recommendation**: Document minimum runtime versions with known vulnerabilities patched.

2. **Base image trust**: Design assumes Debian/Ubuntu images are not malicious
   - **Assessment**: Reasonable. Official images are generally trustworthy.
   - **Recommendation**: Use specific digests and document verification process.

3. **Ecosystem registry trust**: Design assumes crates.io, npmjs.com, etc. are trustworthy
   - **Assessment**: Reasonable for this stage. Full supply chain verification is out of scope.
   - **Recommendation**: Document this assumption and note as future enhancement.

### Risks Requiring Escalation

1. **Arbitrary recipe execution with minimal user confirmation**
   - **Current mitigation**: Container isolation
   - **Residual risk**: Container escapes, resource exhaustion, social engineering
   - **Recommendation**: Add user confirmation and warnings before executing untrusted recipes

2. **Full network access for ecosystem builds**
   - **Current mitigation**: None specified
   - **Residual risk**: Data exfiltration, internal network scanning, C2 beaconing
   - **Recommendation**: Switch to bridge networking + egress filtering

3. **Metadata registry as single point of failure**
   - **Current mitigation**: Code review
   - **Residual risk**: Typosquatting, insider threat, excessive dependencies
   - **Recommendation**: Automated verification, package allowlist, enhanced review

## Comparison to Similar Systems

### Homebrew

- **Trust model**: Bottles from GitHub releases (HTTPS), formulas are Ruby code (requires trust)
- **Isolation**: No container isolation, runs on host
- **Security**: Relies on code review, HTTPS, and user trust
- **Verdict**: tsuku's container isolation is significantly more secure

### Nix

- **Trust model**: Derivations specify exact inputs, binary cache with signatures
- **Isolation**: Build sandbox with restricted network, filesystem
- **Security**: Strong isolation, cryptographic verification
- **Verdict**: Nix is more secure but more complex; tsuku is reasonable middle ground

### Docker Official Images

- **Trust model**: Images from trusted publishers, Dockerfile review
- **Isolation**: Not applicable (Docker is the isolation mechanism)
- **Security**: Multi-stage builds, minimal base images, regular rebuilds
- **Verdict**: tsuku should adopt similar practices (image digests, minimal bases)

## Recommendations Summary

### Must Have (Block Implementation)

1. **Add user confirmation for untrusted recipes** in `tsuku validate`
   - Display network requirements before validation
   - Require --yes flag or interactive confirmation

2. **Switch from `--network=host` to `--network=bridge`**
   - Test that ecosystem builds work
   - Document if host network is truly required

3. **Add automated verification for ActionValidationMetadata**
   - CI check that all packages exist in repos
   - Alert on metadata changes

### Should Have (Implement Soon)

4. **Container hardening**
   - Use specific image digests
   - Drop unnecessary capabilities
   - Add seccomp profile

5. **Egress filtering for network-enabled builds**
   - Block private IP ranges
   - Log network connections

6. **Workspace security**
   - Secure creation (mkdtemp)
   - Guaranteed cleanup
   - Symlink prevention

### Nice to Have (Future Enhancements)

7. **Binary integrity checking**
   - Checksum verification for tsuku binary
   - Version compatibility checks

8. **Enhanced monitoring**
   - Resource usage reporting
   - Network connection logging
   - Validation audit trail

9. **Supply chain verification**
   - Package version pinning
   - Checksum verification for installed tools

## Conclusion

The validation centralization design improves security by making network requirements explicit and consolidating validation logic. However, the design introduces new attack surface through `tsuku validate` that requires additional security controls.

**Key strengths**:
- Container isolation provides strong baseline security
- Read-only mounts for tsuku binary and download cache
- Explicit network requirements improve transparency
- Resource limits prevent basic DoS

**Key weaknesses**:
- `tsuku validate` allows arbitrary recipe execution without sufficient warnings
- `--network=host` grants excessive network access
- Metadata registry is single point of failure with insufficient verification
- Missing defense-in-depth controls (seccomp, capabilities, egress filtering)

**Verdict**: The design is **acceptable with modifications**. Implement the "Must Have" recommendations before proceeding with implementation, and plan for "Should Have" items in the same milestone.

The container isolation provides sufficient protection for the primary use case (validating official registry recipes), but executing untrusted recipes requires additional safeguards. The design correctly identifies the risks but underestimates the likelihood and impact of certain attack vectors, particularly around social engineering and supply chain compromise.
