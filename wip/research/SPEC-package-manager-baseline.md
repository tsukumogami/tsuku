# Research Spec P2-A: Package Manager as Commonality Boundary

## Insight

The natural boundary for system dependency handling is **package manager**, not distro or family:

- We cannot install a package manager on a system that lacks one
- Package manager presence IS the capability we're targeting
- "Debian family" is really just "systems with apt available"

Therefore, the targeting model should be:
```toml
[[steps]]
action = "apt_install"
packages = ["curl", "ca-certificates"]
when = { package_manager = "apt" }

[[steps]]
action = "dnf_install"
packages = ["curl", "ca-certificates"]
when = { package_manager = "dnf" }
```

## Objective

Establish what tsuku can assume at each layer:

1. **Universal baseline**: What exists on ANY Linux system (no package manager needed)
2. **Package manager baseline**: What can be installed via each package manager
3. **Package name mapping**: How common dependencies map across package managers

## Research Questions

### 1. Universal Linux Baseline

What can tsuku assume exists on ANY Linux system, regardless of distro or package manager?

| Category | Question |
|----------|----------|
| Shell | Is `/bin/sh` always POSIX-compliant? Is it always present? |
| Coreutils | Are `cat`, `cp`, `mv`, `rm`, `mkdir`, `chmod` always present? |
| Filesystem | Is `/tmp` always writable? Is `$HOME` always defined? |
| Networking | Is there ANY way to download a file without curl/wget? |
| Certificates | Are CA certificates always present for HTTPS? |

**Methodology**: Test minimal base images for each package manager ecosystem:
- `debian:bookworm-slim`
- `ubuntu:24.04`
- `fedora:41`
- `alpine:3.19`
- `archlinux:base`
- `nixos/nix` (or similar minimal NixOS)
- `gentoo/stage3`

For each, document what's present out of the box.

**Deliverable**: `findings_universal-baseline.md` - The absolute minimum tsuku can assume.

### 2. Package Manager Detection

How does tsuku reliably detect which package manager is available?

| Method | Question |
|--------|----------|
| Binary check | Is `which apt` / `command -v apt` reliable? |
| File check | Is `/etc/apt/sources.list` presence reliable? |
| os-release | Does `ID` or `ID_LIKE` reliably predict package manager? |
| Multiple PMs | What if both `apt` and `snap` are present? Priority? |

**Methodology**:
- Test detection methods on 10+ distro containers
- Document edge cases (WSL, Docker-in-Docker, minimal images)

**Deliverable**: `findings_pm-detection.md` - Reliable detection algorithm.

### 3. Package Manager Baselines

For each major package manager, what's the baseline state before any packages are installed?

| Package Manager | Base Image | Test |
|-----------------|------------|------|
| apt | `debian:bookworm-slim` | What commands exist? |
| apt | `ubuntu:24.04` | What differs from Debian? |
| dnf | `fedora:41` | What commands exist? |
| pacman | `archlinux:base` | What commands exist? |
| apk | `alpine:3.19` | What commands exist? (musl, busybox) |

For each baseline, document:
- Shell available (`/bin/sh`, `/bin/bash`, `/bin/ash`)
- Download tools (`curl`, `wget`, neither)
- SSL/TLS capability (certificates present?)
- Compression tools (`tar`, `gzip`, `unzip`, `xz`)
- Common utilities (`git`, `make`, `gcc` - likely absent)

**Deliverable**: `findings_pm-baselines.md` - What each package manager ecosystem provides by default.

### 4. Package Name Mapping

How do common dependency names map across package managers?

| Dependency | apt | dnf | pacman | apk |
|------------|-----|-----|--------|-----|
| curl | `curl` | `curl` | `curl` | `curl` |
| wget | `wget` | `wget` | `wget` | `wget` |
| CA certs | `ca-certificates` | `ca-certificates` | `ca-certificates` | `ca-certificates` |
| git | `git` | `git` | `git` | `git` |
| build essentials | `build-essential` | `@development-tools` | `base-devel` | `build-base` |
| Python 3 | `python3` | `python3` | `python` | `python3` |
| OpenSSL dev | `libssl-dev` | `openssl-devel` | `openssl` | `openssl-dev` |

**Methodology**:
- Identify 20-30 common dependencies that recipes might need
- Find the correct package name for each package manager
- Note any that don't exist or have significantly different behavior

**Deliverable**: `findings_package-name-mapping.md` - Cross-PM package name table.

### 5. Minimum Bootstrap Requirements

What does tsuku itself need to function, and how can it bootstrap on a minimal system?

| Requirement | Why | How to ensure |
|-------------|-----|---------------|
| Download capability | Fetch binaries | curl/wget, or tsuku bundles a downloader |
| HTTPS/TLS | Secure downloads | CA certificates must exist |
| Extraction | Unpack archives | tar/gzip, or tsuku bundles extraction |
| Filesystem write | Install binaries | $HOME must be writable |

**Question**: Should tsuku be able to bootstrap its own dependencies, or declare minimum requirements?

**Deliverable**: `findings_bootstrap-requirements.md` - What tsuku needs and how to get it.

### 6. The "No Package Manager" Case

What happens when tsuku runs on a system with no supported package manager?

- NixOS (has `nix`, but declarative)
- Gentoo (has `emerge`, but source-based)
- Minimal containers (distroless, scratch)
- Embedded systems

**Options**:
1. Refuse to install recipes with system deps
2. Show manual instructions only
3. Use a fallback (Homebrew/Linuxbrew everywhere?)
4. Declare explicitly unsupported

**Deliverable**: `findings_no-pm-strategy.md` - How to handle unsupported systems.

## Synthesis Question

After research, answer:

**What is the correct targeting model for tsuku?**

| Model | Description |
|-------|-------------|
| A | `target = (platform)` - Distro doesn't matter for most recipes |
| B | `target = (platform, package_manager)` - PM is the capability boundary |
| C | `target = (platform, package_manager, distro)` - Need distro for package names |
| D | Something else? |

## Deliverables Summary

| File | Content |
|------|---------|
| `findings_universal-baseline.md` | What exists on any Linux |
| `findings_pm-detection.md` | How to detect package manager |
| `findings_pm-baselines.md` | What each PM ecosystem provides |
| `findings_package-name-mapping.md` | Package names across PMs |
| `findings_bootstrap-requirements.md` | What tsuku needs to function |
| `findings_no-pm-strategy.md` | Handling unsupported systems |
| `findings_targeting-model-recommendation.md` | Final model recommendation |

## Methodology

This research requires **empirical testing**, not just documentation review:

```bash
# Test universal baseline across ecosystems
for img in debian:bookworm-slim ubuntu:24.04 fedora:41 alpine:3.19 archlinux:base; do
  echo "=== $img ==="
  docker run --rm $img sh -c '
    echo "Shell: $(readlink -f /bin/sh)"
    echo "Curl: $(which curl 2>/dev/null || echo MISSING)"
    echo "Wget: $(which wget 2>/dev/null || echo MISSING)"
    echo "Tar: $(which tar 2>/dev/null || echo MISSING)"
    echo "CA certs: $(ls /etc/ssl/certs 2>/dev/null | wc -l) files"
  '
done
```

## Output Location

All deliverables go in: `wip/research/`

## Dependencies

Builds on P1-C (Package Manager Inventory) findings.

## Handoff

Findings feed into:
- Final targeting model decision
- `when` clause design (platform vs package_manager vs distro)
- Bootstrap and minimum requirements documentation
- Unsupported system handling strategy
