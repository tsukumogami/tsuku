# Container Base Image Strategy for target = (platform, distro)

**Context**: Issue S2 requires mapping target distros to container base images.
**Date**: 2024-12-30
**Related Issues**: S1 (minimal base container), S2 (container spec derivation), S4 (executor integration)

---

## Executive Summary

The proposed mapping is sound with one critical exception: **Arch Linux lacks official ARM64 support**. Recommended adjustments:

1. Use debian:bookworm-slim as the default and for Debian targets (smallest, most secure)
2. Use ubuntu:24.04 for Ubuntu targets (excellent multi-arch, recent packages)
3. Use fedora:41 (not 40) for Fedora targets (fedora:40 EOL May 2025)
4. Drop archlinux:base for now (no official ARM64) or use with x86_64 only
5. Do NOT use Alpine unless specifically required (musl/glibc incompatibility)

---

## 1. Image Size and Pull Times

### Compressed Download Sizes (amd64)

| Base Image | Compressed Size | Uncompressed Size | Pull Time (100Mbps) |
|------------|-----------------|-------------------|---------------------|
| alpine:3.19 | ~3 MB | ~7 MB | <1s |
| debian:bookworm-slim | ~25 MB | ~75 MB | ~2s |
| ubuntu:24.04 | ~28 MB | ~76 MB | ~2s |
| fedora:40/41 | ~60 MB | ~187 MB | ~5s |
| archlinux:base | ~140 MB | ~450 MB | ~12s |

### Analysis

- **debian:bookworm-slim** and **ubuntu:24.04** are nearly identical in size (~25-28MB compressed)
- **fedora** is 2-3x larger than Debian/Ubuntu but still reasonable
- **archlinux** is significantly larger (5x Debian) due to rolling release package freshness
- All images except Alpine are well within the acceptable range for CI builds

### Recommendation

Prefer **debian:bookworm-slim** as the default base (smallest glibc-based image, excellent security track record). Ubuntu is comparable in size but has more frequent updates.

---

## 2. Package Manager Differences

### Package Manager Matrix

| Distro | Package Manager | Update Command | Install Command | Notes |
|--------|-----------------|----------------|-----------------|-------|
| debian/ubuntu | apt | `apt-get update` | `apt-get install -y --no-install-recommends` | Best practice: `--no-install-recommends` |
| fedora | dnf | `dnf check-update \|\| true` | `dnf install -y` | Note: check-update exits 100 if updates available |
| archlinux | pacman | `pacman -Sy` | `pacman -S --noconfirm` | Rolling release, always latest |
| alpine | apk | `apk update` | `apk add --no-cache` | Uses musl, NOT glibc |

### Dockerfile Generation Implications

Container spec derivation (S2) must generate distro-appropriate commands:

```go
// Debian/Ubuntu
"RUN apt-get update && apt-get install -y --no-install-recommends " + strings.Join(packages, " ") + " && rm -rf /var/lib/apt/lists/*"

// Fedora
"RUN dnf install -y " + strings.Join(packages, " ") + " && dnf clean all"

// Arch
"RUN pacman -Syu --noconfirm " + strings.Join(packages, " ") + " && pacman -Scc --noconfirm"
```

### Key Differences

1. **apt** requires explicit `apt-get update` before install; cleanup via `rm -rf /var/lib/apt/lists/*`
2. **dnf** auto-updates metadata; cleanup via `dnf clean all`
3. **pacman** requires `-Syu` for sync+update before install (rolling release)
4. **apt repo setup** requires GPG key import, sources.list file
5. **dnf repo setup** simpler (RPM repos self-contained)

---

## 3. Multi-Arch Availability (amd64 + arm64)

### Official Support Matrix

| Image | amd64 | arm64 | armv7 | ppc64le | s390x | riscv64 |
|-------|-------|-------|-------|---------|-------|---------|
| ubuntu:24.04 | Yes | Yes | Yes | Yes | Yes | Yes |
| debian:bookworm-slim | Yes | Yes | Yes | Yes | Yes | No |
| fedora:40/41 | Yes | Yes | No | Yes | No | No |
| archlinux:base | Yes | **NO** | No | No | No | No |
| alpine:3.19 | Yes | Yes | Yes | Yes | Yes | Yes |

### Critical Finding: Arch Linux ARM64

**The official Arch Linux Docker image does NOT support ARM64.**

From the Arch Linux project: "ArchLinux doesn't support any other architecture than AMD64." ARM support is delegated to the separate Arch Linux ARM project, which maintains unofficial Docker images.

**Options for Arch Linux:**
1. **Drop Arch Linux support** from multi-arch builds
2. **Use unofficial image** (e.g., `menci/docker-archlinuxarm` or `agners/archlinuxarm`)
3. **Support x86_64 only** for Arch Linux containers

**Recommendation**: Support Arch Linux for x86_64 only in initial release. Document the ARM64 limitation. Consider adding unofficial ARM support in a future milestone if user demand exists.

---

## 4. Version Pinning Strategy

### Pinning Options

| Strategy | Example | Reproducibility | Security Updates | Maintenance |
|----------|---------|-----------------|------------------|-------------|
| No pin (latest) | `FROM ubuntu` | None | Automatic | None |
| Major version | `FROM ubuntu:24` | Low | Automatic | Low |
| Minor/patch | `FROM ubuntu:24.04` | Medium | Semi-auto | Medium |
| Digest pin | `FROM ubuntu:24.04@sha256:...` | Perfect | Manual | High |

### Recommended Strategy: **Minor Version Pin with Digest Tracking**

```dockerfile
# Pin to minor version for human readability
# Use digest for reproducibility in CI
FROM ubuntu:24.04@sha256:abc123...
```

**Implementation:**
1. Store version tag in code for readability: `ubuntu:24.04`
2. Use tools like Renovate or dockpin to track and update digests
3. Document base image versions in a single source of truth
4. Update digests on schedule (monthly) or when CVEs are announced

### Fedora Special Case

Fedora's short lifecycle (13 months) requires proactive version updates:
- Fedora 40: EOL May 13, 2025 (already EOL!)
- Fedora 41: EOL ~November 2025
- Fedora 42: EOL ~May 2026

**Recommendation**: Use `fedora:41` now (not 40). Schedule Fedora version updates every 6 months aligned with Fedora releases.

---

## 5. Security Scanning and Update Cadence

### CVE Comparison (Typical Container)

| Base Image | CVEs After 3 Months | Time to Patch Critical | Notes |
|------------|---------------------|------------------------|-------|
| Alpine | ~8 CVEs | <7 days | Fewer CVEs due to smaller package set |
| Debian Slim | ~38 CVEs | <10 days | Good patch cadence |
| Ubuntu | ~50 CVEs | <7 days | Canonical Security response |
| Fedora | ~40 CVEs | <14 days | Community-driven |
| Arch | Varies | <3 days (rolling) | Bleeding edge, frequent updates |

### Key Insights

1. **Alpine has fewest CVEs** but musl incompatibility limits usefulness
2. **Debian/Ubuntu have comparable security** with ~10 day patch cadence
3. **Updating OS packages reduces CVEs by only ~6%** (most are in application layers)
4. **Minimal base images (distroless, slim) significantly reduce attack surface**

### Recommendation

Use **debian:bookworm-slim** as default (good CVE response, small attack surface). Implement automated scanning in CI (Trivy, Grype) with blocking threshold for critical CVEs.

---

## Question Analysis

### Q1: Should we build our own minimal base images per distro?

**Recommendation: No (for now)**

Reasons against:
- Maintenance overhead (need to rebuild weekly for security updates)
- Limited value over official slim images (~20MB savings)
- Complexity in multi-arch builds (need to maintain build infrastructure)
- Official images have security scanning and support

When to reconsider:
- If image sizes become a measurable bottleneck in CI
- If we need distro-specific customizations not possible with RUN commands
- If we need to support distros without official images (e.g., ARM64 Arch)

### Q2: How do we handle base image updates (e.g., ubuntu:24.04 to ubuntu:26.04)?

**Recommendation: Version mapping config + scheduled updates**

```go
// internal/sandbox/images.go
var DistroImageMap = map[string]ImageSpec{
    "ubuntu":  {Image: "ubuntu", Tag: "24.04", Digest: "sha256:..."},
    "debian":  {Image: "debian", Tag: "bookworm-slim", Digest: "sha256:..."},
    "fedora":  {Image: "fedora", Tag: "41", Digest: "sha256:..."},
    "arch":    {Image: "archlinux", Tag: "base", Digest: "sha256:...", Arch: []string{"amd64"}},
}
```

Update process:
1. **Minor updates (24.04.1 to 24.04.2)**: Automated via Renovate/dockpin (weekly)
2. **Major updates (24.04 to 26.04)**: Manual review, scheduled 1 month after LTS release
3. **Fedora updates**: Schedule every 6 months aligned with release cycle
4. **Arch updates**: Rolling, pin to weekly snapshot for reproducibility

### Q3: What about Alpine (musl vs glibc issues)?

**Recommendation: Do NOT use Alpine as a general-purpose base**

Why Alpine is problematic:
1. **musl/glibc incompatibility**: Many pre-built binaries assume glibc
2. **Error messages are cryptic**: "no such file or directory" when binary can't find libc
3. **DNS resolution issues**: musl has different resolver behavior
4. **Python/Node native extensions**: Often fail to build or run

When Alpine IS appropriate:
- Go binaries with `CGO_ENABLED=0` (static linking)
- Rust binaries (can target musl)
- Recipes that explicitly support Alpine

**Implementation**: Do not include Alpine in the standard distro mapping. Add Alpine as an opt-in target for specific recipes that declare musl compatibility.

---

## Final Recommendations

### Distro-to-Base-Image Mapping (Updated from S2)

```go
var DistroImageMapping = map[string]string{
    "ubuntu":  "ubuntu:24.04",
    "debian":  "debian:bookworm-slim",
    "fedora":  "fedora:41",         // Updated from 40 (EOL)
    "arch":    "archlinux:base",    // x86_64 only!
    "":        "debian:bookworm-slim", // Default
}
```

### Action Items for S2 Implementation

1. **Use fedora:41** instead of fedora:40 (EOL May 2025)
2. **Document Arch Linux x86_64-only** limitation
3. **Implement multi-arch builds** for ubuntu, debian, fedora (skip archlinux for arm64)
4. **Add digest pinning** infrastructure for reproducibility
5. **Do not support Alpine** in the general distro mapping

### Version Update Schedule

| Distro | Update Trigger | Owner |
|--------|----------------|-------|
| Ubuntu | LTS+1 month, or CVE | Monthly digest check |
| Debian | Stable release+1 month, or CVE | Monthly digest check |
| Fedora | N+1 release | Every 6 months |
| Arch | Weekly snapshot (if used) | Weekly |

---

## References

- Docker multi-platform builds: https://docs.docker.com/build/building/multi-platform/
- Fedora lifecycle: https://docs.fedoraproject.org/en-US/releases/lifecycle/
- Arch Linux ARM: https://archlinuxarm.org/
- Alpine musl compatibility: https://docs.docker.com/dhi/core-concepts/glibc-musl/
- Base image security comparison: https://www.chainguard.dev/unchained/the-zero-cve-challenge-can-official-docker-hub-images-pass-the-test
