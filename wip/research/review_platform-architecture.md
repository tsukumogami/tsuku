# Platform Architecture Review

**Reviewer:** Platform Architecture Specialist
**Document:** DESIGN-platform-compatibility-verification.md
**Date:** 2026-01-24

---

## Executive Summary

The design's "self-contained tools, system-managed dependencies" philosophy is technically sound and well-researched. The decision to use system packages instead of hermetic APK extraction is correct given Alpine's package retention policy and the lack of cross-musl-distro portability. However, the implementation plan underestimates the UX friction of requiring users to run system package manager commands and lacks contingency for environments where sudo is unavailable.

---

## 1. Technical Soundness

**Verdict: Sound, with minor concerns**

### 1.1 System Packages for Libraries - Correct Decision

The research correctly identifies that:

1. **Homebrew bottles are glibc-only** - There's no musl equivalent, and building one would be significant work
2. **Alpine doesn't retain old packages** - Hermetic APK extraction would provide false reproducibility
3. **APK is Alpine-only** - Void uses xbps, Chimera uses APKv3 - no universal musl format exists
4. **System packages solve the actual problem** - Getting libraries that work on the target libc

The four library dependencies (zlib, libyaml, openssl, gcc-libs) are foundational with stable APIs. Version differences between distros rarely cause compatibility issues because distros backport security fixes without breaking ABI.

### 1.2 Technical Concerns

**Concern A: Package installation detection is underspecified**

The design mentions checking if packages are "already installed" but doesn't specify how:

```go
if isInstalled(pkgName, pm) {
    return nil // Already satisfied
}
```

Detection methods vary by package manager:
- apt: `dpkg-query -W -f='${Status}' <pkg> 2>/dev/null | grep "install ok installed"`
- dnf: `rpm -q <pkg>`
- pacman: `pacman -Q <pkg>`
- apk: `apk info -e <pkg>`
- zypper: `rpm -q <pkg>`

These need testing across all families. Some package managers return success codes differently for "package exists but not installed" vs "package unknown."

**Concern B: Dev packages vs runtime packages**

The package mapping table shows dev packages (libssl-dev, openssl-devel):

```
| openssl | libssl-dev | openssl-devel | openssl | openssl-dev | openssl-devel |
```

But tsuku tools may need runtime libraries, not development headers. The distinction matters:
- Dev packages include headers (*.h) and static libraries (*.a)
- Runtime packages include shared objects (*.so)

For dlopen verification, runtime packages suffice. The design should clarify whether it's guiding users to install dev or runtime packages.

**Concern C: Arch uses split packages**

On Arch, `openssl` is a single package containing both runtime and dev files. But on Debian/RHEL, they're split. This asymmetry is handled correctly in the mapping table but worth noting for testing.

---

## 2. Platform Coverage

**Verdict: Excellent - actually solves the musl problem**

### 2.1 The Design Solves Alpine Support

This is the key insight that many reviewers might miss: **the design transforms Alpine from "broken" to "fully supported."**

Before this design:
- Homebrew bottles fail on Alpine with "Dynamic loading not supported"
- dlopen tests are skipped on Alpine
- Alpine users encounter silent failures

After this design:
- System packages (apk) provide musl-native libraries
- dlopen tests work on Alpine
- Clear guidance when packages are missing

The research finding that Alpine represents ~20% of container usage justifies this as a first-class citizen, not a niche target.

### 2.2 ARM64 Coverage

The hybrid testing approach correctly identifies:
- GitHub provides free native ARM64 Linux runners (`ubuntu-24.04-arm`)
- ARM64 macOS runners are available (`macos-latest`)
- No ARM64 containers needed for family-specific tests (family behavior is arch-independent)

This closes a real gap - ARM64 binaries are currently released but never integration-tested.

### 2.3 Family Coverage Matrix

The CI matrix correctly covers:

| Family | Real Environment | Test Type |
|--------|------------------|-----------|
| Debian | Container (debian:bookworm-slim) | dlopen, checksum |
| RHEL | Container (fedora:41) | dlopen, checksum |
| Arch | Container (archlinux:base) | dlopen, checksum |
| Alpine | Container (alpine:3.19) | dlopen (now works!) |
| SUSE | Container (opensuse/leap:15) | dlopen, checksum |

---

## 3. Implementation Feasibility

**Verdict: Achievable with some adjustments**

### 3.1 Phase Analysis

| Phase | Complexity | Risk | Notes |
|-------|------------|------|-------|
| 1. Platform Detection | Low | Low | Straightforward exec.LookPath |
| 2. System Dependency Action | Medium | Medium | Package detection logic varies |
| 3. Recipe Migration | Low | Medium | Risk of breaking existing users |
| 4. ARM64 Native Testing | Low | Low | Just adding runners |
| 5. Container Family Tests | Medium | Medium | Container setup complexity |
| 6. Documentation | Low | Low | Standard cleanup |

### 3.2 Feasibility Concerns

**Concern A: Recipe migration is breaking**

The design says recipes change from:
```toml
[[steps]]
action = "homebrew"
formula = "openssl@3"
```

To:
```toml
[[steps]]
action = "system_dependency"
name = "openssl"
when = { os = ["linux"] }
```

This is a breaking change for users with cached recipes. The design should address:
- How cached recipes are invalidated
- Whether old recipes continue to work during transition
- Communication to users about the change

**Concern B: The Execute() stub pattern isn't actionable**

Current implementation from `linux_pm_actions.go`:
```go
func (a *ApkInstallAction) Execute(...) error {
    fmt.Printf("   Would install via apk: %v\n", packages)
    fmt.Printf("   (Skipped - requires sudo and system modification)\n")
    return nil
}
```

The design proposes:
```go
return fmt.Errorf("missing dependency %s: run '%s'", libraryName, cmd)
```

Returning an error halts installation. This is correct for "missing dependency" but the transition from "stub that prints" to "action that fails" is a behavior change.

**Concern C: CI container setup complexity**

Building tsuku-dltest from source in each container requires:
- Rust toolchain in container
- Cross-compilation or native builds
- Caching strategy for build artifacts

The design mentions "Build tsuku-dltest from source in each container" (Phase 5) but doesn't detail how. Pre-built musl binaries would simplify this.

---

## 4. Gaps and Risks

### 4.1 Critical Gap: No-sudo environments

The design acknowledges "Package manager dependency: Users need access to their system package manager" but the mitigation is weak:

> "For locked-down environments, the system_dependency action provides clear install commands that users can request from IT"

Real-world scenarios where this fails:
- Rootless containers (common in CI)
- Kubernetes pods without package managers
- Multi-stage Docker builds where base image is immutable
- Corporate laptops without admin access

**Recommendation:** Add a fallback path for no-sudo environments:
1. Detect if sudo is available
2. If not, check if required libraries exist in standard paths
3. If libraries exist but aren't "installed" via package manager, use them anyway

### 4.2 Risk: Package name mapping maintenance

The design says "changes infrequently" but:
- Alpine edge updates packages frequently
- New distros appear (NixOS popularity growing, immutable distros like Fedora Silverblue)
- Package renames happen (e.g., openssl 1.x to 3.x transitions)

**Recommendation:** Store package mapping in a separate data file (JSON/TOML) that can be updated without code changes. Consider a `system_deps.toml` embedded via `//go:embed`.

### 4.3 Risk: dlopen verification may pass with wrong library

If a user has system openssl 3.x but a tool was built against 1.x, dlopen may succeed but the tool may crash later with symbol errors.

**Recommendation:** Phase 2 should include version range checking where applicable (especially openssl which has significant ABI breaks).

### 4.4 Gap: macOS asymmetry is undersold

The design says "macOS still uses Homebrew for library deps since there's no system package manager" but this means:
- Two completely different code paths
- Homebrew's relocation logic remains necessary for macOS
- Testing matrices differ between platforms

The design should explicitly state: "On macOS, no changes to current Homebrew bottle approach."

---

## 5. Alternative Approaches

### 5.1 Considered and Correctly Rejected

The design correctly rejected:

1. **Hermetic APK extraction** - Alpine removes old packages, defeating reproducibility
2. **Musl-specific binaries from Homebrew** - Would require Homebrew to support musl (they explicitly don't)
3. **Static linking** - Not feasible for openssl (licensing) or libstdc++ (size)

### 5.2 Alternative Not Fully Explored: Nix as Backend

The research mentions "Users who truly need hermetic builds on Alpine already use Nix" but doesn't consider:

- tsuku already has nix-portable support (`internal/actions/nix_portable.go`)
- Nix works on both glibc and musl
- Nix provides true reproducibility with version pinning

**Consideration:** For users who need hermetic builds, document that recipes can use `nix_install` as an alternative to `system_dependency`. This is mentioned in passing ("Nix backend for complex deps") but could be elevated to a first-class alternative path.

### 5.3 Alternative Not Explored: Container-based recipes

Instead of system packages, tools with complex dependencies could use container recipes:

```toml
[[steps]]
action = "container_run"
image = "ruby:3.2-alpine"
```

This would provide hermetic isolation without system dependencies. Not necessarily better, but worth mentioning why it wasn't chosen (likely: performance overhead, container runtime dependency).

---

## 6. Specific Technical Recommendations

### 6.1 Detection Logic

Replace vague "isInstalled()" with concrete implementation per package manager:

```go
type PackageChecker interface {
    IsInstalled(pkg string) (bool, error)
    InstallCommand(pkg string) string
}

type AptChecker struct{}
func (c *AptChecker) IsInstalled(pkg string) (bool, error) {
    cmd := exec.Command("dpkg-query", "-W", "-f=${Status}", pkg)
    out, err := cmd.Output()
    if err != nil {
        return false, nil // Package not found
    }
    return strings.Contains(string(out), "install ok installed"), nil
}
```

### 6.2 Graceful Degradation

Instead of hard-failing when system packages are missing:

```go
// Try to find library in standard paths before failing
func findLibrary(name string) (string, bool) {
    paths := []string{
        "/usr/lib/lib" + name + ".so",
        "/lib/lib" + name + ".so",
        "/usr/lib64/lib" + name + ".so",
    }
    for _, p := range paths {
        if _, err := os.Stat(p); err == nil {
            return p, true
        }
    }
    return "", false
}
```

### 6.3 Test Matrix Simplification

For container family tests, pre-build tsuku-dltest binaries:
- `tsuku-dltest-linux-amd64-glibc`
- `tsuku-dltest-linux-amd64-musl`
- `tsuku-dltest-linux-arm64-glibc`
- `tsuku-dltest-linux-arm64-musl`

Include these in releases. This eliminates Rust toolchain setup in CI containers.

### 6.4 Recipe Migration Strategy

Add deprecation warnings before breaking changes:

```toml
# v1: Still works but warns
[[steps]]
action = "homebrew"
formula = "openssl@3"
# tsuku: WARNING: homebrew action for libraries is deprecated on Linux.
#        Please migrate to system_dependency. See https://...

# v2: New approach
[[steps]]
action = "system_dependency"
name = "openssl"
```

---

## 7. Overall Verdict

**Approve with Changes**

The design is technically sound and the core decision (system packages over hermetic APK) is correct. The research supporting this decision is thorough and convincing. However, the implementation details need refinement:

### Required Changes

1. **Specify package installation detection methods** for each package manager
2. **Add graceful degradation** for no-sudo environments (library path detection)
3. **Clarify recipe migration strategy** with deprecation warnings

### Recommended Changes

1. Extract package mapping to embedded data file for easier maintenance
2. Pre-build tsuku-dltest binaries for all libc variants
3. Document Nix as alternative path for users needing hermetic builds
4. Add version range checking for libraries with ABI breaks (openssl)

### Minor Suggestions

1. Clarify dev vs runtime package guidance
2. Explicitly document unchanged macOS behavior
3. Consider container-based recipes as future option (out of scope for this design)

---

## Appendix: Research Quality Assessment

The supporting research documents are well-structured and evidence-based:

| Document | Quality | Key Insight |
|----------|---------|-------------|
| explore_alpine-market.md | High | Quantified 20% container market share |
| explore_musl-landscape.md | High | Confirmed Alpine dominance (95%+) |
| explore_hermetic-value.md | High | Identified Alpine's package retention gap |
| explore_apk-synthesis.md | High | Consolidated findings effectively |

The research appropriately killed the hermetic APK extraction option with data rather than assumptions. This is good engineering practice.
