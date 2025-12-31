# Distro-to-Package-Manager Mapping

Research findings for SPEC-package-managers.md (P1-C).

## Package Manager to Distro Mapping

### apt/dpkg (Debian Family)

| Distro | Base | Market Share | Notes |
|--------|------|--------------|-------|
| Debian | - | 16% (servers) | The upstream for all apt-based distros |
| Ubuntu | Debian | 33.9% (overall) | Most popular Linux distro globally |
| Linux Mint | Ubuntu | Top 5 | User-friendly desktop focus |
| Pop!_OS | Ubuntu | Popular | System76's developer-focused distro |
| Kali Linux | Debian | Niche | Security/penetration testing |
| Raspberry Pi OS | Debian | Embedded | ARM-focused for Raspberry Pi |
| elementary OS | Ubuntu | Niche | macOS-like desktop |
| Zorin OS | Ubuntu | Growing | Windows-like for newcomers |
| MX Linux | Debian | Top 5 | Lightweight, DistroWatch popular |
| antiX | Debian | Niche | Ultra-lightweight |
| Deepin | Debian | Regional | China-focused, custom DE |
| Parrot OS | Debian | Niche | Security-focused |
| LMDE | Debian | Alternative | Mint's Debian Edition |
| Devuan | Debian | Niche | systemd-free fork |

### dnf/yum/rpm (RHEL Family)

| Distro | Base | Market Share | Notes |
|--------|------|--------------|-------|
| Fedora | - | Developer popular | Upstream for RHEL |
| RHEL | Fedora | 43.1% (enterprise) | Enterprise standard |
| CentOS Stream | RHEL | Enterprise | RHEL development branch |
| Rocky Linux | RHEL | Growing | CentOS replacement |
| AlmaLinux | RHEL | Growing | CentOS replacement |
| Oracle Linux | RHEL | Enterprise | Oracle's RHEL rebuild |
| Amazon Linux 2023 | Fedora | Cloud | AWS-native |
| CentOS 7 | RHEL 7 | Legacy (yum) | End of life 2024 |
| Scientific Linux | RHEL | Legacy | Discontinued |
| Nobara | Fedora | Niche | Gaming-focused Fedora |

### pacman (Arch Family)

| Distro | Base | Market Share | Notes |
|--------|------|--------------|-------|
| Arch Linux | - | Developer popular | Rolling release, DIY |
| Manjaro | Arch | Top 10 | User-friendly Arch |
| EndeavourOS | Arch | Growing | Antergos successor |
| Garuda Linux | Arch | Growing | Gaming-focused |
| CachyOS | Arch | Top (DistroWatch 2025) | Performance-optimized |
| ArcoLinux | Arch | Niche | Learning-focused |
| Artix Linux | Arch | Niche | systemd-free Arch |
| BlackArch | Arch | Niche | Security testing |
| ArchBang | Arch | Niche | Lightweight |

### zypper (SUSE Family)

| Distro | Base | Market Share | Notes |
|--------|------|--------------|-------|
| openSUSE Leap | SLES | Moderate | Stable, point releases |
| openSUSE Tumbleweed | - | Moderate | Rolling release |
| SUSE Linux Enterprise | - | Enterprise | Commercial enterprise |
| GeckoLinux | openSUSE | Niche | Preconfigured openSUSE |

### apk (Alpine)

| Distro | Base | Market Share | Notes |
|--------|------|--------------|-------|
| Alpine Linux | - | High (containers) | Minimal, musl-based |
| postmarketOS | Alpine | Niche | Mobile Linux |
| Chimera Linux | Alpine-influenced | New | Experimental |

### xbps (Void)

| Distro | Base | Notes |
|--------|------|-------|
| Void Linux | - | Independent, rolling release |

### portage/emerge (Gentoo)

| Distro | Base | Notes |
|--------|------|-------|
| Gentoo | - | Source-based, highly customizable |
| Funtoo | Gentoo | Gentoo variant |
| Calculate Linux | Gentoo | Desktop-focused Gentoo |
| ChromeOS/ChromiumOS | Gentoo | Uses portage for build system |

### nix

| Distro | Base | Notes |
|--------|------|-------|
| NixOS | - | Declarative Linux distribution |
| Any distro | - | nix can be installed standalone |

### guix

| Distro | Base | Notes |
|--------|------|-------|
| Guix System | - | GNU's declarative distribution |
| Any distro | - | guix can be installed standalone |

### eopkg (Solus)

| Distro | Base | Notes |
|--------|------|-------|
| Solus | - | Independent, Budgie desktop origin |

### swupd (Clear Linux)

| Distro | Base | Notes |
|--------|------|-------|
| Clear Linux | - | Intel-optimized |

### brew (Homebrew)

| Distro | Base | Notes |
|--------|------|-------|
| Any distro | - | Cross-platform, installs to user space |

---

## Distro to Package Manager Mapping

### Tier 1: High Usage (Combined Market Share > 60%)

| Distro | Primary PM | Secondary PM(s) | Family |
|--------|------------|-----------------|--------|
| Ubuntu | apt | snap, flatpak | Debian |
| Debian | apt | - | Debian |
| RHEL/CentOS/Rocky/Alma | dnf | - | RHEL |
| Fedora | dnf | flatpak | RHEL |
| Arch Linux | pacman | yay/paru (AUR) | Arch |
| Linux Mint | apt | flatpak | Debian |
| Alpine Linux | apk | - | Alpine |

### Tier 2: Moderate Usage

| Distro | Primary PM | Secondary PM(s) | Family |
|--------|------------|-----------------|--------|
| openSUSE | zypper | flatpak | SUSE |
| Manjaro | pacman | yay/paru (AUR), snap, flatpak | Arch |
| Pop!_OS | apt | flatpak | Debian |
| EndeavourOS | pacman | yay (AUR) | Arch |
| CachyOS | pacman | paru (AUR) | Arch |
| MX Linux | apt | flatpak | Debian |
| Kali Linux | apt | - | Debian |

### Tier 3: Niche/Specialized

| Distro | Primary PM | Secondary PM(s) | Family |
|--------|------------|-----------------|--------|
| Void Linux | xbps | - | Independent |
| Gentoo | portage | - | Independent |
| NixOS | nix | - | Independent |
| Guix System | guix | - | Independent |
| Solus | eopkg | - | Independent |
| Clear Linux | swupd | - | Independent |
| Raspberry Pi OS | apt | - | Debian |
| elementary OS | apt | flatpak | Debian |

---

## Universal Package Managers (Cross-Distro)

These can be installed on any distribution:

| Manager | Install Method | Use Case |
|---------|---------------|----------|
| Homebrew/Linuxbrew | Shell script | Developer tools |
| nix | Shell script | Reproducible environments |
| guix | Shell script | GNU ecosystem |
| Flatpak | System PM | Desktop applications |
| Snap | System PM | Desktop/server applications |
| AppImage | Download | Portable applications |

---

## Detection Strategy for tsuku

### Primary Detection (File-Based)

| Check | Indicates |
|-------|-----------|
| `/etc/debian_version` exists | Debian family (apt) |
| `/etc/redhat-release` exists | RHEL family (dnf/yum) |
| `/etc/arch-release` exists | Arch family (pacman) |
| `/etc/alpine-release` exists | Alpine (apk) |
| `/etc/SuSE-release` or `/etc/SUSE-brand` exists | SUSE family (zypper) |
| `/etc/void-release` exists | Void (xbps) |
| `/etc/gentoo-release` exists | Gentoo (portage) |
| `/etc/solus-release` exists | Solus (eopkg) |

### Secondary Detection (Command-Based)

| Command | Indicates |
|---------|-----------|
| `which apt` | Debian family |
| `which dnf` | RHEL family (modern) |
| `which yum` (no dnf) | RHEL family (legacy) |
| `which pacman` | Arch family |
| `which zypper` | SUSE family |
| `which apk` | Alpine |
| `which xbps-install` | Void |
| `which emerge` | Gentoo |
| `which nix-env` | NixOS or nix standalone |
| `which guix` | Guix or guix standalone |
| `which eopkg` | Solus |
| `which swupd` | Clear Linux |

### `/etc/os-release` Fields

Standard detection using `/etc/os-release`:

| Field | Example Values |
|-------|----------------|
| `ID` | `ubuntu`, `debian`, `fedora`, `rhel`, `arch`, `alpine`, `opensuse-leap`, etc. |
| `ID_LIKE` | `debian`, `rhel fedora`, `arch`, `suse opensuse` |
| `VERSION_ID` | `22.04`, `9`, `40`, etc. |

---

## Market Share Summary (2025)

### By Package Manager Family

| Family | Approximate Share | Distros |
|--------|------------------|---------|
| apt/dpkg | ~50% | Ubuntu, Debian, Mint, Pop!_OS, etc. |
| dnf/yum/rpm | ~30% | RHEL, Fedora, Rocky, Alma, etc. |
| pacman | ~10% | Arch, Manjaro, EndeavourOS, etc. |
| Other | ~10% | Alpine, Void, Gentoo, NixOS, etc. |

### Key Insights

1. **apt dominates** - Debian-based distros cover roughly half of all Linux usage
2. **dnf/yum critical for enterprise** - RHEL family holds 43% of enterprise market
3. **pacman growing** - Arch-based distros popular with developers and gamers
4. **Alpine important for containers** - De facto standard for Docker base images
5. **Nix/Guix rising** - Developer interest in declarative approaches growing
