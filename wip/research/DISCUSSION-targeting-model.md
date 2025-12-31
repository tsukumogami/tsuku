# Design Discussion: Platform Targeting Model

This document captures the design discussion around how tsuku should handle Linux distribution targeting for system dependencies.

## Problem Statement

Tsuku needs to support recipes with system dependencies (e.g., `curl`, `ca-certificates`) that must be installed via the system package manager. The challenge is:

1. Linux has many distributions with different package managers
2. We need to generate testable, reproducible plans
3. We want to avoid recipe noise (repeating the same action for every distro)
4. We want to avoid plan proliferation (near-identical plans that only differ by distro)

## Initial Proposal

The initial design proposed:

```
target = (platform, distro)
```

With explicit actions per package manager:
```toml
[[steps]]
action = "apt_install"
packages = ["curl"]
when = { distro = ["ubuntu", "debian"] }

[[steps]]
action = "dnf_install"
packages = ["curl"]
when = { distro = ["fedora"] }
```

## Concerns Raised

### Concern 1: Oversimplification of Linux Ecosystem

The Linux ecosystem is hierarchical, not flat:

```
Family (debian, rhel, arch)
    └── Distro (ubuntu, fedora, manjaro)
            └── Version (22.04, 40, rolling)
```

Additionally:
- Binary compatibility varies (glibc vs musl)
- Package managers have different models (imperative vs declarative)
- Some distros (NixOS, Gentoo) are fundamentally different

### Concern 2: Plan Validity Across Distros

If we test a plan on Ubuntu, will it work on Debian? The plan might assume:
- `curl` is present (Ubuntu: yes, Debian minimal: no)
- Certain shell features exist
- Specific filesystem layouts

### Concern 3: Recipe Noise

Writing the same action multiple times with different `when` clauses:
```toml
when = { distro = ["ubuntu"] }  # apt_install curl
when = { distro = ["debian"] }  # apt_install curl (same!)
when = { distro = ["mint"] }    # apt_install curl (same!)
```

This offers no value and creates duplicated plans.

## Research Conducted

### Phase 1 Research (Parallel)

| Track | Focus | Key Finding |
|-------|-------|-------------|
| P1-A | Prior Art | Family-based hierarchy works well (Ansible, Chef) |
| P1-B | Binary Survey | 56% static binaries, 41% ship musl - most binaries are portable |
| P1-C | Package Managers | apt + dnf + pacman cover 90% of users |
| P1-D | Ecosystem | Ubuntu + Debian cover 70%, add Alpine for 85% |

### Phase 2 Research (Empirical)

Tested 11 Linux distributions in containers to find the universal baseline.

**Critical Discovery: Debian/Ubuntu are the outliers**

| Distro | curl | wget | CA certs |
|--------|------|------|----------|
| debian:bookworm-slim | MISSING | MISSING | MISSING |
| ubuntu:24.04 | MISSING | MISSING | MISSING |
| fedora:41 | present | present | present |
| alpine:3.19 | present | present | present |
| archlinux:base | present | MISSING | present |

**Universal Baseline**: Only `/bin/sh`, coreutils, `tar`, and `gzip` are truly universal. No download tool or CA certificates guaranteed.

## Key Insight: Package Manager as Commonality Boundary

The discussion led to a key insight:

> We cannot install a package manager on a system that lacks one. Package manager presence IS the capability we're targeting. "Debian family" is essentially "systems with apt."

This means:
- The natural boundary is **package manager**, not distro
- Family → PM is reliable (debian → apt, rhel → dnf)
- Distro-level targeting adds noise without benefit

## Evolution of the Model

### Model A: `target = (platform, distro)`
- ❌ Too granular
- ❌ Causes recipe noise and plan proliferation
- ❌ ubuntu/debian/mint all use apt - why distinguish?

### Model B: `target = (platform, package_manager)`
- ✓ PM is the capability boundary
- ❌ But PM is derived from distro/family, not specified directly
- ❌ "package_manager" is implementation detail, not user-facing concept

### Model C: `target = (platform, linux_family)` ✓ CHOSEN
- ✓ Family maps 1:1 to package manager
- ✓ More intuitive than "package_manager"
- ✓ Avoids distro-level proliferation
- ✓ Captures the meaningful difference (which PM to use)

## Refined Model

### Target Structure

```
target = (platform, linux_family?)

where:
  platform     = os/arch (linux/amd64, darwin/arm64)
  linux_family = debian | rhel | arch | alpine | suse
```

- `linux_family` is Linux-only
- `linux_family` is derived from distro at detection time
- `linux_family` maps 1:1 to package manager

### Family → Package Manager Mapping

| linux_family | Package Manager |
|--------------|-----------------|
| debian | apt |
| rhel | dnf |
| arch | pacman |
| alpine | apk |
| suse | zypper |

### Action Structure

Each `*_install` action has a **hardcoded, immutable** `when` clause:

```go
// apt_install always has: when = { linux_family = "debian" }
// dnf_install always has: when = { linux_family = "rhel" }
// pacman_install always has: when = { linux_family = "arch" }
```

Recipe authors don't specify the `when` clause - it's built into the action definition. This prevents mistakes (can't put `apt_install` with `when = { linux_family = "rhel" }`).

### Recipe Patterns

**Pattern 1: Composite action for common case**

When package names are the same across PMs (research found ~95% consistency):

```toml
[[steps]]
action = "system_install"  # Composite action
packages = ["curl", "ca-certificates", "git"]
# Automatically expands to correct PM based on target linux_family
```

**Pattern 2: Explicit actions for exceptions**

When package names differ:

```toml
[[steps]]
action = "apt_install"
packages = ["build-essential"]

[[steps]]
action = "dnf_install"
packages = ["@development-tools"]

[[steps]]
action = "pacman_install"
packages = ["base-devel"]
```

The composite action could include a mapping for common exceptions, but explicit actions remain available for edge cases.

### Plan Generation

| Platform | linux_family | Plan Contents |
|----------|--------------|---------------|
| linux/amd64 | debian | download, extract, apt_install, install_binaries |
| linux/amd64 | rhel | download, extract, dnf_install, install_binaries |
| linux/amd64 | arch | download, extract, pacman_install, install_binaries |
| linux/arm64 | debian | download, extract, apt_install, install_binaries |
| darwin/arm64 | (n/a) | download, extract, (homebrew?), install_binaries |

One plan per (platform, linux_family) - not per distro.

### Conditional Distro Dimension

Most recipes don't need `linux_family` at all (no system deps). The dimension is **conditional**:

- Recipe has no `*_install` actions → `target = (platform)` only
- Recipe has `*_install` actions → `target = (platform, linux_family)`

This keeps simple recipes simple.

## Open Questions

### 1. Darwin/macOS Handling

macOS doesn't have a "family" in the Linux sense. Options:
- Always require Homebrew
- Treat as separate case with its own rules
- Research needed to determine best approach

### 2. Unsupported Family (`linux_family = none`)

What happens when tsuku runs on NixOS, Gentoo, or other unsupported systems?

Current decision: **Skip for now, flag as future solution.**

Note: Tsuku supports Nix - could this be a universal fallback? (Install system deps via nix-portable on any Linux?)

### 3. Package Name Mapping

Research showed ~95% of packages have the same name across PMs. The 5% exceptions (e.g., `build-essential` vs `base-devel`) need handling.

Current decision:
- Composite `system_install` action handles common case
- Action can include internal mapping for known exceptions
- Recipe authors can use explicit `*_install` actions for edge cases
- Existing `system_dependency` proposal in docs covers this (includes Darwin)

## Summary

| Aspect | Decision |
|--------|----------|
| Targeting dimension | `linux_family` (not distro) |
| Family → PM | Derived (debian → apt, rhel → dnf) |
| Action `when` clauses | Hardcoded into action definition |
| Package names | Composite action + explicit fallback |
| Plan granularity | One per (platform, linux_family) |
| Darwin | Separate handling (TBD) |
| Unsupported systems | Skip for now (Nix as future fallback) |

## References

### Research Artifacts

All in `wip/research/`:
- `RESEARCH-HANDOFF.md` - Master research document
- `SPEC-*.md` - Research specifications
- `findings_*.md` - Research findings
- `investigation_*.md` - Investigation paths

### Design Documents

- `docs/DESIGN-system-dependency-actions.md` - Action vocabulary
- `docs/DESIGN-structured-install-guide.md` - Sandbox container building
- `wip/plan-outline.md` - Implementation issue plan

### Key Findings Documents

- `findings_universal-baseline.md` - What exists on any Linux
- `findings_pm-baselines.md` - What each PM ecosystem provides
- `findings_package-name-mapping.md` - Package names across PMs
- `findings_targeting-model-recommendation.md` - Final model recommendation
