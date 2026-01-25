# Security Review: Platform Compatibility Verification

**Reviewer:** Security Specialist
**Date:** 2026-01-24
**Document:** DESIGN-platform-compatibility-verification.md
**Status:** Proposed

---

## Executive Summary

The shift from Homebrew bottles to system package managers for library dependencies is a net security improvement. Distro packages receive institutional security review and faster CVE response than Homebrew bottles. The `system_dependency` action correctly maintains tsuku's no-sudo principle by displaying commands rather than executing them. One gap requires attention: the design should explicitly address package manager detection spoofing in hostile environments.

---

## 1. Supply Chain Security Analysis

### 1.1 Homebrew vs Distro Packages: A Comparison

| Aspect | Homebrew Bottles | Distro Packages |
|--------|------------------|-----------------|
| **Binary provenance** | Homebrew GHCR | Official distro mirrors |
| **Signing mechanism** | Bottle checksums (SHA256) | GPG-signed packages |
| **Security review** | Homebrew maintainers (volunteer) | Distro security teams (institutional) |
| **CVE response** | Homebrew rebuild schedule | Rapid backport (6-24 hours for critical) |
| **Auditability** | Recipe git history | Distro changelogs + CVE tracking |

**Finding: Security improvement.** Distribution security teams have institutional processes that individual Homebrew maintainers cannot match. Debian Security Team, Fedora Security Response Team, and Alpine's security infrastructure all have documented CVE response SLAs. Homebrew has no published SLA for security updates.

### 1.2 Trust Model Shift

The design correctly identifies this shift:

> "By using system packages instead of Homebrew bottles, we shift supply chain trust from Homebrew to distribution maintainers."

This is appropriate for library dependencies because:

1. Users already trust their distro for the base system
2. Library dependencies are implementation details, not user-facing choices
3. Distros patch libraries faster than Homebrew rebuilds bottles

**Residual risk:** Users on outdated distro releases (e.g., Ubuntu 20.04 LTS) may have older library versions. However, distros backport security fixes without changing version numbers, so "older" does not mean "less secure."

### 1.3 macOS Asymmetry

The design acknowledges macOS continues using Homebrew:

> "macOS still uses Homebrew for library deps since there's no system package manager"

This is unavoidable. macOS has a single libc (libSystem), so the glibc/musl incompatibility doesn't apply. Homebrew bottles work consistently on macOS. No additional risk is introduced.

---

## 2. Privilege Model Analysis

### 2.1 The system_dependency Action

The design document describes the action as:

```go
func (a *SystemDependencyAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    // 1. Detect system package manager
    // 2. Look up package name for this library
    // 3. Check if already installed
    // 4. Show install command (tsuku doesn't run sudo)
    cmd := getInstallCommand(pkgName, pm)
    return fmt.Errorf("missing dependency %s: run '%s'", libraryName, cmd)
}
```

**Finding: Correctly maintains no-sudo principle.** The action:
- Performs read-only checks (package manager detection, installed check)
- Returns an error with the command to run
- Never invokes `sudo` or privileged commands

This matches the existing pattern in `linux_pm_actions.go`:

```go
func (a *ApkInstallAction) Describe(params map[string]interface{}) string {
    packages, ok := GetStringSlice(params, "packages")
    return fmt.Sprintf("sudo apk add %s", strings.Join(packages, " "))
}
```

The user decides whether to run the displayed command.

### 2.2 Installed Package Check

The design mentions checking if a package is installed:

> "Check if already installed; if not, shows the install command"

This requires querying the package manager without privileges:
- `apt`: `dpkg -s <package>` or `dpkg-query -W`
- `dnf`: `rpm -q <package>`
- `apk`: `apk info -e <package>`
- `pacman`: `pacman -Q <package>`
- `zypper`: `rpm -q <package>`

All of these are read-only operations that work without sudo. No privilege escalation risk.

---

## 3. Attack Surface Analysis

### 3.1 Reduction in Attack Surface

**Before (Homebrew bottles):**
- tsuku downloads binaries from GHCR
- tsuku verifies checksums
- tsuku relocates binaries (patchelf RPATH)
- tsuku manages library versions

**After (system packages):**
- User installs packages via their package manager (GPG-verified)
- Packages are managed by the distro
- No relocation needed
- tsuku only verifies presence

**Finding: Reduced attack surface.** tsuku no longer:
- Downloads library binaries
- Performs RPATH manipulation
- Maintains version state for libraries

The binary download and relocation code paths for libraries are eliminated on Linux, reducing the codebase that handles untrusted input.

### 3.2 New Attack Vectors Introduced

**Package manager detection spoofing:**

The design proposes detecting package managers via:

```go
func DetectPackageManager() PackageManager {
    if _, err := exec.LookPath("apt"); err == nil {
        return Apt
    }
    // ...
}
```

In a hostile environment, an attacker could:
1. Place a fake `apt` binary in PATH
2. Cause tsuku to display incorrect install commands
3. User follows incorrect commands, potentially installing malicious packages

**Severity: Low.** This requires:
- Attacker has write access to user's PATH directories
- User blindly follows displayed commands
- The displayed command would need to point to a malicious repository

**Recommendation:** The design should note that package manager detection trusts the PATH. This is acceptable because an attacker with PATH control can already execute arbitrary code. Document this as an accepted risk.

### 3.3 CI Container Security

The design proposes using official Docker Hub images for testing:

> "Using official distribution images introduces a supply chain dependency on Docker Hub."

**Finding: Acceptable for CI.**

- Container compromise affects CI results, not user installations
- Official images have higher trust than arbitrary images
- Image digest pinning (mentioned in the previous security review) would improve reproducibility

The previous review recommended:

> "Consider image digest pinning for CI container tests once the matrix stabilizes."

This remains a good medium-term improvement but is not a blocker.

---

## 4. CI Security Analysis

### 4.1 Container-Based Testing

The hybrid approach uses containers for family-specific testing:

| Family | Base Image | Runner |
|--------|------------|--------|
| debian | debian:bookworm-slim | ubuntu-latest |
| rhel | fedora:41 | ubuntu-latest |
| arch | archlinux:base | ubuntu-latest |
| alpine | alpine:3.19 | ubuntu-latest |
| suse | opensuse/leap:15 | ubuntu-latest |

**Security considerations:**

1. **Image provenance:** All listed images are official (from Docker Library or distro-maintained namespaces)
2. **Container isolation:** GitHub Actions containers have limited host access
3. **No secrets in containers:** CI tests don't require credentials; no secrets exposure risk
4. **Ephemeral execution:** Containers are destroyed after each workflow

**Finding: Adequate isolation.** Container tests introduce no new security risks beyond standard CI practices.

### 4.2 ARM64 Testing

The design adds ARM64 native runners:

> "GitHub Actions provides free native ARM64 Linux runners (ubuntu-24.04-arm) for public repos."

GitHub-hosted runners have equivalent security isolation regardless of architecture. No new risks.

### 4.3 tsuku-dltest in Containers

The dltest helper needs to be built from source in each container:

> "Build tsuku-dltest from source in each container"

This is correct for musl containers (Alpine) since the existing binary is glibc-only. Building from source in CI is a standard practice and introduces no new supply chain risks - the source code is already trusted.

---

## 5. Gap Analysis: Missing Security Considerations

### 5.1 Package Manager Detection in Hostile Environments

**Gap:** The design doesn't address what happens when package manager detection is unreliable.

**Scenario:** A user runs tsuku in a minimal container or chroot where `/etc/os-release` is missing or incomplete.

**Current behavior (from DESIGN-system-dependency-actions.md):**

> "If /etc/os-release is missing or family cannot be determined, steps with linux_family conditions are skipped."

**Recommendation:** Ensure the `system_dependency` action fails gracefully with an informative message:

```
Unable to detect Linux distribution family.
If you're on an Alpine-based system, run: apk add <package>
If you're on a Debian-based system, run: apt install <package>
...
```

### 5.2 Package Name Mapping Integrity

**Gap:** The design introduces a hardcoded mapping of library names to distro package names:

```go
var systemPackageNames = map[string]string{
    "openssl": {
        "debian": "libssl-dev",
        "alpine": "openssl-dev",
        // ...
    },
}
```

If this mapping contains errors, users could:
- Install the wrong package
- Get incorrect dependency resolution
- Potentially install unrelated packages with similar names

**Recommendation:** Add CI tests that verify:
1. All package names in the mapping exist in their respective distro repositories
2. The package provides the expected library files

This is mentioned in the design:

> "CI tests verify the mapping works"

This should be explicitly documented as a security control.

### 5.3 Version Pinning Removed

**Trade-off acknowledged:** The design accepts that library versions are now controlled by distros, not tsuku.

From the hermetic-value research:

> "Alpine doesn't retain old package versions... pinning fails within days/weeks"

**Finding: Acceptable.** The design correctly identifies this as a trade-off, not a gap. Distros backport security fixes without version bumps, so "older version" does not mean "less secure." The inability to pin versions is offset by automatic security updates.

### 5.4 No New Data Collection

**Verified:** The design explicitly states:

> "No new data collection. Package manager detection runs entirely locally."

The previous security review confirmed this. No telemetry or data exfiltration risks.

---

## 6. Comparison with Previous Security Review

The previous review (explore_phase8_security-review.md) covered dltest infrastructure. This review covers the broader platform compatibility changes. The reviews are complementary:

| Aspect | Previous Review | This Review |
|--------|-----------------|-------------|
| dltest checksum verification | Verified adequate | N/A |
| Environment sanitization | Verified adequate | N/A |
| Path validation | Verified adequate | N/A |
| System dependency action | N/A | Verified adequate |
| Supply chain shift | N/A | Security improvement |
| CI containers | Acceptable | Acceptable |

The previous review's recommendation for code signing remains valid but is outside the scope of this design.

---

## 7. Recommendations

### 7.1 Required Changes (Before Implementation)

None. The design is security-sound.

### 7.2 Recommended Improvements (During Implementation)

1. **Document PATH trust assumption:** Add a note that package manager detection trusts the PATH, which is acceptable since PATH modification requires existing code execution capability.

2. **Graceful detection failure:** Ensure `system_dependency` provides fallback instructions for all families when detection fails, not just an error.

3. **Package name validation in CI:** Implement the "verify mapping works" tests as explicit CI jobs that query distro repositories.

### 7.3 Future Considerations

1. **Image digest pinning:** Once the test matrix stabilizes, pin container images by digest for reproducibility.

2. **Signed packages on macOS:** When Apple's notarization requirements evolve, evaluate whether Homebrew bottles need additional verification.

---

## 8. Verdict

**Approve.**

The design improves tsuku's security posture by:
- Shifting library supply chain trust to institutional security teams
- Reducing binary download and relocation attack surface
- Maintaining the no-sudo principle
- Adding comprehensive platform testing

No blocking security concerns were identified. The recommended improvements are enhancements, not requirements.

---

## References

- Design document: `docs/designs/DESIGN-platform-compatibility-verification.md`
- Previous security review: `wip/research/explore_phase8_security-review.md`
- APK rejection research: `wip/research/explore_apk-synthesis.md`
- Hermetic value analysis: `wip/research/explore_hermetic-value.md`
- System dependency actions: `docs/designs/current/DESIGN-system-dependency-actions.md`
- Current implementation: `internal/actions/linux_pm_actions.go`, `internal/verify/dltest.go`
