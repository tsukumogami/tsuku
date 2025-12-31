# Package Manager Inventory

Research findings for SPEC-package-managers.md (P1-C).

## Complete Inventory Table

| Name | Command | Family | Distros (Primary) | Model | Root Required | Repo Format | GPG Key Management |
|------|---------|--------|-------------------|-------|---------------|-------------|-------------------|
| **dpkg** | `dpkg` | Debian | Debian, Ubuntu, Linux Mint, Pop!_OS | Imperative (low-level) | Yes | N/A (single packages) | N/A |
| **apt/apt-get** | `apt`, `apt-get` | Debian | Debian, Ubuntu, Linux Mint, Pop!_OS, Kali, Raspberry Pi OS | Imperative | Yes | `/etc/apt/sources.list.d/*.{list,sources}` | `/etc/apt/keyrings/` (modern), signed-by directive |
| **rpm** | `rpm` | RHEL | Fedora, RHEL, CentOS, Rocky, Alma, openSUSE | Imperative (low-level) | Yes | N/A (single packages) | `rpm --import <key-url>` |
| **dnf** | `dnf`, `dnf5` | RHEL | Fedora (22+), RHEL 8+, CentOS Stream, Rocky, Alma | Imperative | Yes | `/etc/yum.repos.d/*.repo` | `gpgkey=` in .repo file, `/etc/pki/rpm-gpg/` |
| **yum** | `yum` | RHEL | RHEL 7, CentOS 7, older Fedora | Imperative (legacy) | Yes | `/etc/yum.repos.d/*.repo` | Same as dnf |
| **pacman** | `pacman` | Arch | Arch Linux, Manjaro, EndeavourOS, Garuda, CachyOS | Imperative | Yes | `/etc/pacman.conf` | `pacman-key --recv-keys`, `/usr/share/pacman/keyrings/` |
| **yay** | `yay` | Arch (AUR) | Arch-based distros | Imperative (wrapper) | No (builds as user) | AUR (PKGBUILD) | Inherits pacman keyring |
| **paru** | `paru` | Arch (AUR) | Arch-based distros | Imperative (wrapper) | No (builds as user) | AUR (PKGBUILD) | Inherits pacman keyring |
| **zypper** | `zypper` | SUSE | openSUSE Leap, openSUSE Tumbleweed, SLES | Imperative | Yes | `/etc/zypp/repos.d/*.repo` | `--gpg-auto-import-keys`, `rpm --import` |
| **apk** | `apk` | Alpine | Alpine Linux, postmarketOS | Imperative | Yes | `/etc/apk/repositories` | Public key in `/etc/apk/keys/` |
| **xbps** | `xbps-install`, `xbps-query`, `xbps-remove` | Void | Void Linux | Imperative | Yes | `/etc/xbps.d/*.conf` | Built-in signature verification |
| **portage/emerge** | `emerge` | Gentoo | Gentoo, Funtoo, Calculate Linux | Source-based | Yes | `/etc/portage/repos.conf/` | `emerge-webrsync` (GPG verified sync) |
| **nix** | `nix`, `nix-env` | NixOS | NixOS, any distro (standalone) | Declarative/Functional | No (user mode) | `/etc/nix/nix.conf`, flakes | Binary cache signatures |
| **guix** | `guix` | Guix | Guix System, any distro (standalone) | Declarative/Functional | No (user mode) | `channels.scm` | Substitute server signatures |
| **eopkg** | `eopkg` | Solus | Solus | Imperative | Yes | `/etc/eopkg/repos.xml` | Key embedded in repo metadata |
| **swupd** | `swupd` | Clear Linux | Clear Linux | Bundle-based | Yes | `swupd 3rd-party` commands | Certificate verification |
| **brew** | `brew` | Homebrew | Any distro (standalone install) | Hybrid (bottles + source) | No | Taps (git repos) | Bottle signatures via attestation |

## Detailed Package Manager Profiles

### Debian Family

#### dpkg
- **Role**: Low-level package installer (backend for apt)
- **Format**: `.deb` packages
- **Key commands**:
  - `dpkg -i package.deb` - Install package
  - `dpkg -r package` - Remove package
  - `dpkg -l` - List installed packages
  - `dpkg -S /path/to/file` - Find package owning file
- **Limitations**: No dependency resolution, no repository support

#### apt / apt-get
- **Role**: High-level package manager, frontend to dpkg
- **Key commands**:
  - `apt update` - Refresh package index
  - `apt install <package>` - Install package
  - `apt remove <package>` - Remove package
  - `apt upgrade` - Upgrade all packages
  - `apt search <query>` - Search packages
- **Third-party repos**: Create file in `/etc/apt/sources.list.d/`
  - Modern format (deb822): `.sources` files with `Signed-By:` directive
  - Legacy format: `.list` files with `[signed-by=/path/to/key.gpg]`
- **GPG keys**: Store in `/etc/apt/keyrings/` (local) or `/usr/share/keyrings/` (package-managed)
- **Note**: `apt-key` is deprecated; use `signed-by` directive

### RHEL Family

#### rpm
- **Role**: Low-level package installer (backend for dnf/yum)
- **Format**: `.rpm` packages
- **Key commands**:
  - `rpm -i package.rpm` - Install package
  - `rpm -e package` - Remove package
  - `rpm -qa` - List installed packages
  - `rpm --import <key-url>` - Import GPG key
- **Limitations**: No dependency resolution, no repository support

#### dnf (and yum)
- **Role**: High-level package manager, successor to yum
- **DNF5**: Next-generation rewrite with improved performance
- **Key commands**:
  - `dnf check-update` - Check for updates
  - `dnf install <package>` - Install package
  - `dnf remove <package>` - Remove package
  - `dnf upgrade` - Upgrade all packages
  - `dnf search <query>` - Search packages
- **Third-party repos**: Create `.repo` file in `/etc/yum.repos.d/`
  ```ini
  [reponame]
  name=Repository Name
  baseurl=https://example.com/repo/
  enabled=1
  gpgcheck=1
  gpgkey=https://example.com/key.gpg
  ```
- **GPG keys**: Specified in repo file, stored in `/etc/pki/rpm-gpg/`

### Arch Family

#### pacman
- **Role**: Primary package manager for Arch
- **Format**: `.pkg.tar.zst` packages
- **Key commands**:
  - `pacman -Syu` - Full system upgrade
  - `pacman -S <package>` - Install package
  - `pacman -R <package>` - Remove package
  - `pacman -Ss <query>` - Search packages
  - `pacman -U /path/to/package.pkg.tar.zst` - Install local package
- **Third-party repos**: Add section to `/etc/pacman.conf`
  ```ini
  [reponame]
  SigLevel = Required DatabaseOptional
  Server = https://example.com/$repo/$arch
  ```
- **GPG keys**: `pacman-key --recv-keys <keyid>` then `pacman-key --lsign-key <keyid>`

#### yay / paru (AUR Helpers)
- **Role**: Wrappers around pacman that add AUR support
- **Model**: Download PKGBUILD, build from source, install with pacman
- **Key difference**: Run as user, only need sudo for final pacman install
- **Security consideration**: AUR packages are user-submitted; review PKGBUILDs

### SUSE Family

#### zypper
- **Role**: Primary package manager for openSUSE/SLES
- **Key commands**:
  - `zypper refresh` - Refresh repositories
  - `zypper install <package>` - Install package
  - `zypper remove <package>` - Remove package
  - `zypper update` - Update all packages
  - `zypper search <query>` - Search packages
  - `zypper addrepo <url> <name>` - Add repository
- **Third-party repos**: Use `zypper addrepo` or create file in `/etc/zypp/repos.d/`
- **GPG keys**: `--gpg-auto-import-keys` or `rpm --import <key-url>`

### Independent Distributions

#### apk (Alpine)
- **Role**: Lightweight package manager for Alpine
- **Key commands**:
  - `apk update` - Update package index
  - `apk add <package>` - Install package
  - `apk del <package>` - Remove package
  - `apk search <query>` - Search packages
  - `apk upgrade` - Upgrade all packages
- **Repositories**: Line-based entries in `/etc/apk/repositories`
- **GPG keys**: Public keys in `/etc/apk/keys/`

#### xbps (Void)
- **Role**: Package manager designed for Void Linux
- **Key commands**:
  - `xbps-install -S` - Sync repositories
  - `xbps-install <package>` - Install package
  - `xbps-remove <package>` - Remove package
  - `xbps-query -Rs <query>` - Search remote packages
  - `xbps-install -Su` - Full system update
- **Repositories**: Files in `/etc/xbps.d/` override `/usr/share/xbps.d/`
- **Note**: Third-party repos officially unsupported by Void project

#### portage/emerge (Gentoo)
- **Role**: Source-based package manager
- **Model**: Downloads source, applies USE flags, compiles locally
- **Key commands**:
  - `emerge --sync` - Sync repository
  - `emerge <package>` - Install/compile package
  - `emerge --unmerge <package>` - Remove package
  - `emerge -uDN @world` - Update entire system
- **USE flags**: Enable/disable features at compile time
- **Binary packages**: Optional via `--getbinpkg`

#### eopkg (Solus)
- **Role**: Fork of PiSi from Pardus Linux
- **Key commands**:
  - `eopkg update-repo` - Update repository
  - `eopkg install <package>` - Install package
  - `eopkg remove <package>` - Remove package
  - `eopkg search <query>` - Search packages
  - `eopkg upgrade` - Upgrade all packages
- **Note**: Planned replacement with "sol" package manager

#### swupd (Clear Linux)
- **Role**: Bundle-based package manager from Intel
- **Model**: Installs bundles (groups of packages), not individual packages
- **Key commands**:
  - `swupd update` - Update system
  - `swupd bundle-add <bundle>` - Install bundle
  - `swupd bundle-remove <bundle>` - Remove bundle
  - `swupd bundle-list` - List installed bundles
  - `swupd search <term>` - Search for bundles
- **Third-party**: `swupd 3rd-party` subcommands, installs to `/opt/3rd-party/`

### Functional/Declarative Package Managers

#### nix
- **Role**: Purely functional package manager
- **Model**: Declarative configuration, immutable store
- **Key concepts**:
  - Packages installed to `/nix/store/` with hash-based paths
  - Generations allow atomic upgrades and rollbacks
  - `configuration.nix` defines system state (NixOS)
  - Flakes provide reproducible dependency management
- **Key commands**:
  - `nix-env -iA nixpkgs.<package>` - Install package (imperative)
  - `nix profile install nixpkgs#<package>` - Install (modern)
  - `nix-channel --update` - Update channels
  - `nix-collect-garbage -d` - Clean old generations
- **User-space**: Can install to `~/.nix-profile` without root
- **Note**: Fundamentally different model from traditional package managers

#### guix
- **Role**: GNU's functional package manager (inspired by Nix)
- **Model**: Declarative, uses Guile Scheme for definitions
- **Key concepts**:
  - Similar to Nix but uses Scheme instead of Nix language
  - Transactional upgrades and rollbacks
  - Reproducible builds
- **Key commands**:
  - `guix install <package>` - Install package
  - `guix remove <package>` - Remove package
  - `guix upgrade` - Upgrade all packages
  - `guix pull` - Update Guix itself
- **User-space**: Designed for unprivileged use

### Cross-Platform

#### brew (Homebrew/Linuxbrew)
- **Role**: User-space package manager from macOS, ported to Linux
- **Install location**: `/home/linuxbrew/.linuxbrew/`
- **Model**: Hybrid - prebuilt bottles when available, source compile otherwise
- **Key commands**:
  - `brew install <package>` - Install package
  - `brew uninstall <package>` - Remove package
  - `brew upgrade` - Upgrade all packages
  - `brew search <query>` - Search packages
  - `brew tap <user/repo>` - Add third-party repository
- **No root required**: Installs to user-writable location
- **Bottles**: Prebuilt binaries for common configurations
