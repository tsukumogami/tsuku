# Research Spec P1-C: Package Manager Inventory

## Objective

Create a complete inventory of Linux package managers, their associated distros, and their compatibility with tsuku's imperative `*_install` action model.

## Scope

Document all significant Linux package managers and classify their compatibility with tsuku's approach.

## Research Questions

### 1. Package Manager Inventory

For each package manager, document:

| Field | Description |
|-------|-------------|
| Name | apt, dnf, pacman, etc. |
| Command | apt-get, dnf, pacman, etc. |
| Distros | Which distros use this as primary? |
| Family | Debian, RHEL, Arch, Independent |
| Model | Imperative, Declarative, Source-based, Hybrid |
| Privilege | Requires root? Can run as user? |
| Repo format | How are third-party repos configured? |
| Key management | How are GPG keys handled? |

### 2. Distro-to-Manager Mapping

Create reverse mapping:

| Distro | Primary PM | Secondary PM | Family |
|--------|------------|--------------|--------|
| Ubuntu | apt | snap, flatpak | Debian |
| Fedora | dnf | flatpak | RHEL |
| Arch | pacman | AUR (yay/paru) | Arch |
| Alpine | apk | - | Independent |
| NixOS | nix | - | Independent |
| Gentoo | portage | - | Independent |
| openSUSE | zypper | - | SUSE |
| Void | xbps | - | Independent |

### 3. Model Classification

Classify each package manager:

| Model | Description | Examples | Tsuku Compatible? |
|-------|-------------|----------|-------------------|
| **Imperative** | `install <pkg>` modifies system state | apt, dnf, pacman | Yes |
| **Declarative** | Config file defines desired state | nix, guix | No - different paradigm |
| **Source-based** | Compiles from source | portage, pkgsrc | Partial - slow |
| **Hybrid** | Mix of above | Homebrew (bottles + source) | Partial |

### 4. Privilege Requirements

| Manager | Root Required? | User-space Option? | Notes |
|---------|---------------|-------------------|-------|
| apt | Yes | No | System packages only |
| dnf | Yes | No | System packages only |
| pacman | Yes | No | AUR helpers can be user |
| nix | No | Yes (single-user mode) | Designed for user-space |
| brew | No | Yes | Installs to /home/linuxbrew |

### 5. Third-Party Repository Configuration

| Manager | Repo Config Method | Key Management |
|---------|-------------------|----------------|
| apt | `/etc/apt/sources.list.d/*.list` | `apt-key` (deprecated), `/etc/apt/keyrings/` |
| dnf | `/etc/yum.repos.d/*.repo` | `rpm --import` |
| pacman | `/etc/pacman.conf` | `pacman-key` |

## Methodology

1. **Documentation Review**: Official docs for each package manager
2. **Distrowatch Cross-Reference**: Verify distro-to-manager mappings
3. **Container Testing**: Spin up containers for each distro, verify package manager behavior
4. **Edge Case Exploration**: Test third-party repo addition on each

## Deliverables

### 1. Package Manager Inventory (`findings_package-manager-inventory.md`)

Complete table of all package managers with all fields filled.

### 2. Distro Mapping (`findings_distro-pm-mapping.md`)

Bidirectional mapping:
- Package manager → distros that use it
- Distro → package manager(s) available

### 3. Compatibility Matrix (`findings_pm-compatibility.md`)

| Manager | tsuku compatible | Notes |
|---------|-----------------|-------|
| apt | Full | Standard imperative model |
| dnf | Full | Standard imperative model |
| pacman | Full | Standard imperative model |
| apk | Full | Alpine's package manager |
| nix | None | Declarative model incompatible |
| portage | Partial | Source-based, very slow |
| zypper | Full | openSUSE's package manager |
| xbps | Full | Void's package manager |

### 4. Action Coverage Analysis (`findings_pm-action-coverage.md`)

Which package managers need dedicated actions?

| Tier | Managers | Rationale |
|------|----------|-----------|
| Tier 1 (dedicated action) | apt, dnf, pacman, brew | High usage, well-understood |
| Tier 2 (add if needed) | apk, zypper, xbps | Lower usage but compatible |
| Tier 3 (unsupported) | nix, portage, guix | Fundamentally different model |

### 5. Recommendations (`findings_pm-recommendations.md`)

- Which package managers should tsuku support initially?
- Which should be explicitly unsupported (with rationale)?
- Should `package_manager` be a filter dimension alongside `distro`?

## Output Location

All deliverables go in: `wip/research/`

## Time Box

- 1 hour: Build initial inventory from documentation
- 1 hour: Verify with container testing
- 30 mins: Classify compatibility
- 30 mins: Write recommendations

## Dependencies

None - this track runs independently.

## Handoff

Findings feed into:
- Phase 2 imperative vs declarative classification
- Decision on which `*_install` actions to implement
- Decision on `package_manager` as filter dimension
