# Findings: 80/20 Analysis - Linux Distribution Coverage

## Executive Summary

**Two distribution families cover ~90% of tsuku's target users:**

1. **Debian family (Ubuntu, Debian):** ~70% coverage
2. **Alpine Linux:** ~15% coverage

The remaining ~15% is a long tail of RHEL derivatives, Fedora, Amazon Linux, Arch, and others.

## Coverage Matrix

| Tier | Distributions | Est. Coverage | Rationale |
|------|--------------|---------------|-----------|
| **Tier 1** | Ubuntu LTS | ~50% | CI default everywhere, cloud tutorials, developer familiarity |
| **Tier 1** | Debian | ~20% | Docker base images, Ubuntu parent, server deployments |
| **Tier 2** | Alpine | ~15% | Container optimization, CI speed, production containers |
| **Tier 2** | Fedora/RHEL family | ~10% | Enterprise, Amazon Linux base, cutting edge |
| **Tier 3** | Others | ~5% | Arch, openSUSE, NixOS, Gentoo, etc. |

## Evidence Summary

### Ubuntu Dominance (~50%)

| Domain | Ubuntu Presence |
|--------|-----------------|
| CI Providers | Default on GitHub Actions, Azure Pipelines, CircleCI, Travis CI |
| Cloud Tutorials | Azure quickstart, GCP examples, DigitalOcean/Linode docs |
| Developer Tooling | Homebrew CI, Bitrise stacks, version manager examples |
| Container Base | 5M+ pulls/week, language runtime alternative |

**Why Ubuntu wins:**
- Developers already know it (desktop familiarity)
- Most tutorials target Ubuntu
- LTS releases provide stability
- apt ecosystem is well-understood

### Debian Importance (~20%)

| Domain | Debian Presence |
|--------|-----------------|
| Docker Official Images | Most popular base OS for official images |
| Language Runtimes | python:3.x, node:xx, ruby:x.x all Debian-based |
| Slim Variants | debian-slim provides compatibility + smaller size |
| GitLab CI | Default ruby:3.1 is Debian-based |

**Why Debian matters:**
- Parent of Ubuntu (binary compatible)
- Stability focus appeals to servers
- Slim variants compete with Alpine
- Official Docker image foundation

### Alpine Significance (~15%)

| Domain | Alpine Presence |
|--------|-----------------|
| Production Containers | Preferred for minimal attack surface |
| CI Speed | 5MB base vs 80MB Ubuntu |
| Language Runtimes | node:xx-alpine, python:3.x-alpine variants |
| Kubernetes | Common for final-stage multi-stage builds |

**Why Alpine is critical:**
- Only mainstream musl-based distribution
- Size matters in container registries
- Security-conscious deployments prefer it
- Cannot be ignored for container support

### RHEL Family (~10%)

| Distribution | Context |
|-------------|---------|
| Amazon Linux 2023 | AWS default, Fedora-based |
| Fedora | Cutting edge, AL2023 upstream |
| Rocky/AlmaLinux | RHEL alternatives post-CentOS |
| RHEL | Enterprise paid support |

**Why RHEL family matters:**
- AWS-centric teams use Amazon Linux
- Enterprise deployments require RHEL compatibility
- Fedora previews future Ubuntu/Debian features

### Long Tail (~5%)

| Distribution | Niche |
|-------------|-------|
| Arch Linux | Developer enthusiasts, AUR |
| openSUSE | European enterprise, SUSE family |
| NixOS | Reproducibility purists |
| Gentoo | Source-based, customization |
| Void Linux | Alternative init, musl option |
| Clear Linux | Intel optimization |

**Characteristics:**
- Passionate user bases
- Often technically sophisticated
- Willing to work around issues
- May contribute fixes upstream

## Developer Tool Ecosystem Alignment

| Tool | Primary Targets | Secondary | Unsupported Notes |
|------|-----------------|-----------|-------------------|
| Homebrew | Ubuntu, Debian | Fedora, RHEL | glibc 2.13+ required |
| rustup | Ubuntu (CI) | All major distros | Snap/package manager available |
| pyenv | Ubuntu, Debian | All with build-essential | Distro-specific deps |
| nvm | Ubuntu, Debian, RHEL | Alpine (source build) | FreeBSD has issues |
| asdf | Ubuntu, Debian | All with git | Language plugin deps vary |
| mise | All (Rust binary) | Alpine (native package) | Excellent musl support |

**Key Observation:** Most tools implicitly target Ubuntu via their CI and documentation, even when broadly compatible.

## The 80/20 Point

### 80% Coverage Achieved With:

1. **Ubuntu 22.04 LTS** - Current stable, CI default
2. **Ubuntu 24.04 LTS** - Latest, CI transitioning
3. **Alpine 3.x** - Container optimization

### 90% Coverage With Addition Of:

4. **Debian 12 (Bookworm)** - Docker base, Ubuntu parent

### 95% Coverage With:

5. **Fedora latest** - Cutting edge, Amazon Linux proxy
6. **RHEL 8/9 or Rocky/Alma** - Enterprise deployments

## Binary Compatibility Insights

### glibc Distributions (Mutually Compatible)

If a binary works on one, it generally works on all:
- Ubuntu
- Debian
- Fedora
- RHEL/Rocky/Alma
- Amazon Linux
- openSUSE

**Constraint:** Minimum glibc version (typically 2.17-2.31 range)

### musl Distributions (Separate Binaries)

Require separate builds:
- Alpine Linux
- Void Linux (musl variant)

**Recommendation:** Provide both glibc and musl binaries, or build static binaries.

## Strategic Implications

### For Tier 1 (Full Support)

- Ubuntu 22.04 and 24.04 LTS
- Debian 12 (Bookworm)
- Alpine 3.18+

**Actions:**
- CI tests on all Tier 1
- Golden files for each
- Installation docs target these
- Pre-built binaries guaranteed

### For Tier 2 (Supported)

- Fedora latest (N and N-1)
- Amazon Linux 2023
- RHEL 9 / Rocky 9 / Alma 9

**Actions:**
- Periodic testing
- Community bug reports accepted
- Binaries should work (glibc compatible)
- Documentation may be sparse

### For Tier 3 (Community)

- Arch Linux
- openSUSE
- NixOS
- Older LTS versions

**Actions:**
- No CI coverage
- Community contributions welcome
- "Should work" but no guarantees
- Issues deprioritized

## Conclusion

**The 80/20 insight:** Testing on Ubuntu 22.04/24.04, Debian 12, and Alpine 3.x covers the vast majority of tsuku's target users. The glibc/musl split is the primary compatibility concern, not distribution-specific quirks.

**The key decision:** Whether to provide:
1. Static binaries (simplest, covers everything)
2. glibc + musl dynamic binaries (two builds)
3. Distribution-specific packages (complex, maximum integration)

For a tool installer like tsuku, static binaries provide the best coverage with minimal maintenance burden.

## Sources

See individual findings documents:
- [findings_ci-providers.md](./findings_ci-providers.md)
- [findings_container-ecosystem.md](./findings_container-ecosystem.md)
- [findings_cloud-defaults.md](./findings_cloud-defaults.md)
