# Findings: Ecosystem Recommendations

## Support Tier Recommendations

Based on the ecosystem analysis, the following support tiers are recommended for tsuku:

## Tier 1: Full Support

**Distributions:** Ubuntu 22.04 LTS, Ubuntu 24.04 LTS, Debian 12, Alpine 3.18+

### Criteria for Tier 1
- CI runs on every PR
- Golden files maintained for each
- Pre-built binaries provided
- Documentation explicitly targets these
- Bugs are high priority

### Implementation

| Distribution | CI Runner | Golden File | Binary Type |
|-------------|-----------|-------------|-------------|
| Ubuntu 22.04 | `ubuntu-22.04` | Yes | glibc |
| Ubuntu 24.04 | `ubuntu-latest` | Yes | glibc |
| Debian 12 | `debian:bookworm` container | Yes | glibc |
| Alpine 3.18+ | `alpine:3.18` container | Yes | musl/static |

### Rationale

- **Ubuntu 22.04/24.04:** Every CI provider defaults to Ubuntu. This is where developers will first encounter tsuku.
- **Debian 12:** Parent of Ubuntu, base for Docker official images. Binary compatible with Ubuntu.
- **Alpine 3.18+:** Critical for container users. Only musl distribution with mainstream adoption.

## Tier 2: Supported

**Distributions:** Fedora (latest, N-1), Amazon Linux 2023, RHEL 9, Rocky Linux 9, AlmaLinux 9

### Criteria for Tier 2
- Periodic CI runs (weekly or release-gated)
- No golden files (rely on Tier 1 testing)
- Binaries should work (glibc compatible with Tier 1)
- Documentation mentions support
- Bugs accepted, medium priority

### Implementation

| Distribution | Testing Strategy | Binary Type |
|-------------|-----------------|-------------|
| Fedora latest | Weekly CI job | glibc (same as Ubuntu) |
| Amazon Linux 2023 | Release testing | glibc (same as Ubuntu) |
| RHEL 9 / Rocky 9 | Release testing | glibc (same as Ubuntu) |

### Rationale

- **Fedora:** Upstream of Amazon Linux 2023, cutting-edge features preview
- **Amazon Linux 2023:** Important for AWS-centric users and Lambda
- **RHEL/Rocky/Alma:** Enterprise users, binary compatible with Fedora

## Tier 3: Community Supported

**Distributions:** Arch Linux, openSUSE, NixOS, older Ubuntu/Debian versions, other glibc distros

### Criteria for Tier 3
- No CI coverage
- Binaries may work (glibc compatible)
- Documentation may mention (wiki-style)
- Community contributions welcome
- Bugs accepted, low priority

### Implementation

- Rely on community bug reports
- Accept PRs for distribution-specific fixes
- Maintain a "Community Distributions" wiki page
- Link to community-maintained packages (AUR, nixpkgs, etc.)

### Rationale

- These distributions have passionate users who can self-support
- glibc compatibility means binaries usually "just work"
- Explicit support requires resources we can allocate to Tier 1/2

## Explicitly Unsupported

**Distributions:** 32-bit x86, pre-glibc-2.17 systems, non-Linux POSIX (FreeBSD, OpenBSD)

### Criteria for Unsupported
- Will not accept bug reports
- May actively reject PRs that complicate codebase
- Documentation clearly states unsupported

### Rationale

- 32-bit systems are increasingly rare in developer tooling
- Very old glibc versions require significant compatibility work
- BSD systems have different ecosystems (ports, pkg)

## Binary Distribution Strategy

### Recommended Approach: Static + musl

Provide two binary types:

1. **Static glibc binary** (or dynamically linked with low glibc minimum)
   - Works on: Ubuntu, Debian, Fedora, RHEL, Amazon Linux, openSUSE, Arch
   - Target glibc: 2.17+ (RHEL 7 era, broadly compatible)

2. **Static musl binary**
   - Works on: Alpine, Void Linux (musl)
   - Eliminates dynamic linking concerns

### Alternative: Fully Static Binary

If Go CGO is not required:
- Single static binary works everywhere
- Simplest distribution model
- May have limitations (DNS resolution, certain system calls)

### Package Manager Integration (Future)

Consider for future adoption:
- Homebrew formula (covers macOS + Linux)
- AUR package (Arch community)
- Nixpkgs derivation (NixOS community)
- Alpine APK (if significant Alpine demand)

## CI Configuration Recommendations

### PR Checks (Required to Pass)

```yaml
# GitHub Actions example
jobs:
  test-tier1:
    strategy:
      matrix:
        include:
          - os: ubuntu-24.04
            name: Ubuntu 24.04
          - os: ubuntu-22.04
            name: Ubuntu 22.04
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - name: Run tests
        run: go test ./...

  test-tier1-containers:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        container:
          - debian:bookworm
          - alpine:3.18
    container: ${{ matrix.container }}
    steps:
      - uses: actions/checkout@v4
      - name: Install Go
        run: # container-specific Go installation
      - name: Run tests
        run: go test ./...
```

### Weekly Tier 2 Checks

```yaml
# Scheduled workflow
on:
  schedule:
    - cron: '0 0 * * 0'  # Weekly on Sunday

jobs:
  test-tier2:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        container:
          - fedora:latest
          - amazonlinux:2023
          - rockylinux:9
    container: ${{ matrix.container }}
    # ... test steps
```

### Release Gate Checks

Before each release, run full matrix including:
- All Tier 1 distributions
- All Tier 2 distributions
- ARM64 variants of Tier 1

## Documentation Recommendations

### Installation Page Structure

```markdown
# Installation

## Quick Install (Recommended)

curl -fsSL https://tsuku.dev/install.sh | bash

Supports: Ubuntu, Debian, Alpine, Fedora, Amazon Linux, RHEL

## Manual Installation

### Ubuntu / Debian
# glibc binary instructions

### Alpine
# musl binary instructions

### Other Distributions
# Generic instructions + link to community wiki
```

### Community Wiki

Create a community-editable page for:
- Arch Linux (AUR)
- NixOS (nixpkgs)
- Gentoo (ebuild)
- openSUSE (OBS)
- Void Linux

## Success Metrics

### Tier 1 Health
- Zero open bugs specific to Tier 1 distros
- CI passing on all Tier 1 runners
- Installation tested in each release

### Tier 2 Health
- Weekly CI green
- Bug backlog < 5 per distro
- Installation tested quarterly

### Community Health
- Active wiki contributions
- Community package maintainers
- Forum/Discord activity for Tier 3 distros

## Summary Table

| Tier | Distributions | CI Frequency | Golden Files | Bug Priority | Documentation |
|------|--------------|--------------|--------------|--------------|---------------|
| **1** | Ubuntu 22/24, Debian 12, Alpine 3.18+ | Every PR | Yes | High | Primary target |
| **2** | Fedora, AL2023, RHEL/Rocky/Alma 9 | Weekly | No | Medium | Mentioned |
| **3** | Arch, openSUSE, NixOS, etc. | None | No | Low | Community wiki |
| **Unsupported** | 32-bit, old glibc, BSD | Never | No | Rejected | Explicitly stated |

## Next Steps

1. Implement CI matrix for Tier 1 distributions
2. Create musl-compatible build target
3. Document installation for each Tier 1 distro
4. Set up weekly Tier 2 CI schedule
5. Create community wiki infrastructure
