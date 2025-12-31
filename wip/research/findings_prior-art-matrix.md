# Prior Art Comparison Matrix

This document compares how various tools handle Linux distribution targeting across key dimensions.

## Summary Matrix

| Aspect | Ansible | Puppet | Chef | Nix | Homebrew | asdf/mise | rustup |
|--------|---------|--------|------|-----|----------|-----------|--------|
| **Detection method** | Python `distro` library, reads `/etc/os-release` | Ruby Facter reads `/etc/os-release`, `lsb_release` | Ruby Ohai reads `/etc/os-release` | LLVM target triple, `config.guess` | Host glibc version check | uname -s/-m in shell scripts | Compile-time target triple |
| **Hierarchy model** | Two-level (distribution + family) | Two-level (name + family) | Two-level (platform + platform_family) | Flat (target triples) | Flat + glibc version constraint | Flat (os/arch only) | Flat (target triples) |
| **Family concept** | Yes (`os_family`: RedHat, Debian, etc.) | Yes (`osfamily`: RedHat, Debian, etc.) | Yes (`platform_family`: rhel, debian, etc.) | No (uses triples) | No (uses glibc compatibility) | No | No (uses ABI field in triple) |
| **Version constraints** | Per-distribution version checks | Via `os.release.major/minor` | Via `node['platform_version']` | Pin to Nixpkgs commit | glibc minimum version | Plugin-specific | Channel (stable/beta/nightly) |
| **Package abstraction** | `package` module auto-selects apt/yum/dnf | `package` type auto-selects provider | `package` resource auto-detects | Full abstraction (Nix expressions) | Full abstraction (formulae) | Plugins fetch binaries | N/A (toolchain only) |
| **Edge case handling** | Fallback heuristics, may fail on minimal containers | Requires `lsb_release` or falls back | Prefers `/etc/os-release`, has fallbacks | No distro detection needed | Builds from source if no bottle | Plugin-dependent | Falls back to building from source |

## Detailed Breakdown by Tool

### Ansible

**Detection Model:**
- Uses Python's `distro` library (since 2.8) as the backend
- Reads from `/etc/os-release` primarily
- Exposes facts: `ansible_distribution`, `ansible_distribution_version`, `ansible_distribution_major_version`, `ansible_os_family`
- Legacy facts accessible via `ansible_facts['os_family']` or shorthand `ansible_os_family`

**Hierarchy Model:**
- Two-level: specific distribution + family abstraction
- Family mappings are defined in `module_utils/facts/system/distribution.py`
- Example families: RedHat (includes AlmaLinux, Fedora, CentOS, Oracle), Debian (includes Ubuntu, Mint)

**Targeting Model:**
- Conditional execution via `when: ansible_os_family == "RedHat"`
- Can target specific distributions or entire families
- Version constraints expressed inline: `when: ansible_distribution_major_version | int >= 8`

**Package Abstraction:**
- `ansible.builtin.package` module auto-selects apt/yum/dnf based on detected package manager
- Falls back to `ansible_pkg_mgr` fact for package manager selection
- Package name differences across distros NOT abstracted (must use variables or conditionals)

**Edge Cases:**
- Minimal containers often lack `/etc/os-release` fields
- `lsb_release` absence can cause detection failures
- Amazon Linux detection has broken multiple times
- Debian version detection differs between `/etc/debian_version` and `/etc/os-release`

---

### Puppet

**Detection Model:**
- Facter collects facts at each run
- Reads `/etc/os-release` and translates `id_like` field to known families
- Falls back to `lsb_release` for additional details if available
- Structured facts: `os.name`, `os.family`, `os.release.major`, `os.release.minor`, `os.release.full`

**Hierarchy Model:**
- Two-level: platform name + family
- Ubuntu maps to Debian family, CentOS maps to RedHat family
- Family is determined by base distribution mapping

**Targeting Model:**
- Conditionals: `if $facts['os']['family'] == 'RedHat' { ... }`
- Version checks via `$facts['os']['release']['major']`
- Provider selection via `defaultfor` class method in provider code

**Package Abstraction:**
- `package` resource type auto-selects provider based on platform
- Provider inheritance (e.g., apt inherits from dpkg)
- Source attribute behavior varies by provider

**Edge Cases:**
- Facter 3 required `lsb_release` for some facts (fixed in Facter 4)
- Oracle Linux incorrectly reported as RedHat in Facter 3.0.0
- Linux Mint incorrectly reported as Debian
- macOS version facts had issues with Big Sur

---

### Chef

**Detection Model:**
- Ohai collects attributes at each Chef run
- Reads `/etc/os-release` as primary source (standardized in Ohai 15)
- Provides: `node['platform']`, `node['platform_version']`, `node['platform_family']`

**Hierarchy Model:**
- Two-level: platform + platform_family
- platform_family groups: debian (debian, ubuntu, mint), rhel (redhat, centos, oracle, almalinux, rocky), fedora, suse, gentoo, slackware, arch
- "rhel" family reserved for recompiled RHEL variants with version compatibility

**Targeting Model:**
- `value_for_platform_family` helper method
- `platform?(:ubuntu, :debian)` for platform checks
- `platform_family?(:rhel)` for family checks

**Package Abstraction:**
- `package` resource auto-detects correct package provider
- Platform-specific resources available: `apt_package`, `yum_package`, `dnf_package`
- Recommended to use generic `package` for cross-platform cookbooks

**Edge Cases:**
- openSUSE Leap/SLES detection failed in Chef 14 due to `/usr/lib/os-release` handling
- Amazon Linux 2 platform_version returned null when format changed
- Clear Linux required special handling

---

### Nix/Nixpkgs

**Detection Model:**
- Uses LLVM-style target triples (e.g., `x86_64-unknown-linux-gnu`)
- `config.guess` script detects host platform
- `builtins.currentSystem` provides Nix host double (e.g., `x86_64-linux`)
- No distro detection needed - packages are self-contained

**Hierarchy Model:**
- Flat: target triples only
- Platform tiers (1-7) determine support level, not hierarchy
- Examples stored in `lib/systems/examples.nix`

**Targeting Model:**
- `localSystem` vs `crossSystem` for build vs deploy
- `pkgsCross.raspberryPi.openssl` for cross-compilation
- `buildPlatform`, `hostPlatform`, `targetPlatform` concepts

**Package Abstraction:**
- Full abstraction via Nix expressions
- Binary cache (substituters) provides pre-built packages
- Same package definition works across all distros
- Cross-compilation uses same packages with different target

**Edge Cases:**
- No distro-specific issues (distro-agnostic by design)
- Binary cache may not have packages for Tier 3+ platforms
- Cross-compilation requires proper target specification

---

### Homebrew (Linuxbrew)

**Detection Model:**
- Checks host glibc version (minimum 2.19 for no rebuild)
- Platform detection: macOS vs Linux
- Architecture detection: x86_64, arm64, arm (32-bit)
- No distribution-specific detection

**Hierarchy Model:**
- Flat with glibc compatibility layer
- Bottles built on Ubuntu 16.04 LTS (glibc 2.23) for portability
- Portable bottles built on CentOS 6 (glibc 2.12) for oldest compat

**Targeting Model:**
- Single Linux target (glibc-based)
- If host glibc too old, Homebrew builds its own glibc
- Prefix path `/home/linuxbrew/.linuxbrew` enables bottle usage

**Package Abstraction:**
- Full abstraction via formulae
- Same formula works on macOS and Linux
- Binary bottles for common platforms, source build for others

**Edge Cases:**
- 32-bit ARM lacks bottles (Tier 3)
- WSL 1 has known issues
- No sandbox on Linux (unlike macOS)
- Upgrades can break if critical packages disappear mid-upgrade

---

### asdf/mise

**Detection Model:**
- Simple uname-based detection in plugin shell scripts
- `uname -s` for OS (Darwin, Linux)
- `uname -m` for architecture (x86_64, arm64)
- No distribution detection

**Hierarchy Model:**
- Flat: OS + architecture only
- No family concept
- Plugins handle platform-specific logic

**Targeting Model:**
- `.tool-versions` file specifies tool versions
- Plugins determine download URL based on platform
- No distribution-specific targeting

**Package Abstraction:**
- Plugins abstract tool installation
- Each plugin independently handles platform differences
- mise compatible with asdf `.tool-versions` format

**Edge Cases:**
- Plugin quality varies significantly
- asdf 0.16.0 rewrite in Go caused breaking changes
- Scripts without shebang fail in Go version
- Plugin updates may break existing setups

---

### rustup

**Detection Model:**
- Uses LLVM target triples
- Host triple detected at installation
- Common triples: `x86_64-unknown-linux-gnu`, `x86_64-unknown-linux-musl`
- Format: `<arch>-<vendor>-<os>-<abi>`

**Hierarchy Model:**
- Flat: target triples only
- ABI field distinguishes glibc vs musl: `-gnu` vs `-musl`
- No family concept

**Targeting Model:**
- `rustup target add <triple>` adds compilation targets
- Single toolchain supports multiple targets
- `rust-toolchain.toml` for project-specific configuration

**Package Abstraction:**
- Only manages Rust toolchain, not general packages
- musl target enables fully static binaries
- Cross-compilation requires additional system tools (linker, etc.)

**Edge Cases:**
- `rustup target add` only installs std library, not linker
- Users often confused about additional requirements for cross-compilation
- NixOS has issues with toolchain paths after garbage collection
- `/tmp` mounted noexec causes failures
