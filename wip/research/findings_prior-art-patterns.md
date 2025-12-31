# Prior Art: Patterns That Work Well

This document catalogs proven patterns from existing tools that have stood the test of time for Linux distribution targeting.

## Pattern 1: Family-Based Hierarchy

**Used by:** Ansible, Puppet, Chef

**Description:**
Group distributions into families based on common ancestry and package management. Rather than targeting individual distributions, target the family level when possible.

**Implementation:**
```
Family: RedHat
  - RHEL, CentOS, AlmaLinux, Rocky Linux, Fedora, Oracle Linux, Amazon Linux

Family: Debian
  - Debian, Ubuntu, Linux Mint, Pop!_OS, elementary OS

Family: SUSE
  - SLES, openSUSE Leap, openSUSE Tumbleweed
```

**Why it works:**
- Reduces maintenance burden: one rule covers many distros
- New distributions automatically inherit parent behavior
- Package manager semantics are consistent within families
- Users intuitively understand family relationships

**Evidence:**
- Ansible's `ansible_os_family` has been stable since 2.x
- Chef's `platform_family?(:rhel)` is the recommended approach
- Puppet's `osfamily` is a core facter fact

**Caveats:**
- Some distributions don't fit neatly (Amazon Linux moved from "rhel" to "amazon")
- Family membership can be disputed (is Fedora its own family or part of RedHat?)

---

## Pattern 2: Standard Detection Source (/etc/os-release)

**Used by:** All modern tools (Ansible, Puppet, Chef, systemd-based detection)

**Description:**
Use `/etc/os-release` as the primary source for distribution detection, as standardized by systemd and freedesktop.org.

**Key fields:**
```
ID=ubuntu
ID_LIKE=debian
VERSION_ID=22.04
VERSION_CODENAME=jammy
NAME="Ubuntu"
```

**Why it works:**
- Standardized format across all systemd-based distributions
- `ID_LIKE` field explicitly declares family relationships
- Machine-readable with clear field semantics
- Present on vast majority of modern Linux systems

**Evidence:**
- Ohai 15 made `/etc/os-release` the default linux platform identifier
- Ansible 2.8 switched to Python `distro` library which uses os-release
- Puppet Facter translates `id_like` field to known families

**Caveats:**
- Not present on very old systems (RHEL 6, Debian 7)
- Minimal containers may have incomplete files
- Version information format varies (Debian only has major version)

---

## Pattern 3: Graceful Fallback Chain

**Used by:** Ansible, Puppet, Chef

**Description:**
When primary detection fails, fall back through a chain of increasingly generic methods rather than failing immediately.

**Typical chain:**
```
1. /etc/os-release (preferred)
2. lsb_release -a command
3. Distribution-specific files (/etc/redhat-release, /etc/debian_version)
4. Kernel information (uname)
5. "Unknown Linux" with generic behavior
```

**Why it works:**
- Handles containers with minimal filesystems
- Supports legacy systems
- Prevents hard failures on unknown distributions
- Allows functionality with reduced features

**Evidence:**
- Ansible issue discussions about detection failures led to fallback improvements
- Chef Ohai handles Clear Linux by checking both `/etc/os-release` and `/usr/lib/os-release`
- Facter 4 improved detection without requiring `lsb_release`

---

## Pattern 4: Abstract Package Interface with Concrete Implementations

**Used by:** Ansible, Puppet, Chef, Nix, Homebrew

**Description:**
Provide a single package abstraction that automatically delegates to the correct platform-specific implementation.

**Examples:**
```yaml
# Ansible - same syntax works everywhere
package:
  name: nginx
  state: present

# Chef - auto-detects apt/yum/dnf
package 'nginx' do
  action :install
end

# Puppet - auto-selects provider
package { 'nginx':
  ensure => installed,
}
```

**Why it works:**
- Users write platform-agnostic code
- Implementation complexity hidden from users
- Easy to add new platforms without changing user code
- Consistent mental model across platforms

**Evidence:**
- Ansible's `package` module has been stable for years
- Chef documentation recommends generic `package` over specific resources
- Homebrew formulae work on both macOS and Linux

**Caveats:**
- Package names differ across distributions (requires separate abstraction)
- Some features only available in platform-specific modules

---

## Pattern 5: Escape Hatch to Platform-Specific

**Used by:** Ansible, Puppet, Chef

**Description:**
While providing abstractions, also expose platform-specific modules/resources when users need fine-grained control.

**Examples:**
```yaml
# Ansible - when you need yum-specific features
yum:
  name: nginx
  enablerepo: epel

# Ansible - when you need apt-specific features
apt:
  name: nginx
  update_cache: yes
```

**Why it works:**
- Abstractions can't cover every use case
- Power users need platform-specific features
- Gradual migration path from platform-specific to generic

**Evidence:**
- All CM tools provide both generic and specific package resources
- Users mix both approaches in real playbooks/manifests

---

## Pattern 6: Target Triple / Platform Identifier

**Used by:** rustup, Nix, LLVM toolchains

**Description:**
Use standardized target identifiers that encode architecture, vendor, OS, and ABI in a single string.

**Format:**
```
<arch>-<vendor>-<os>-<abi>

Examples:
x86_64-unknown-linux-gnu      # Standard Linux with glibc
x86_64-unknown-linux-musl     # Linux with musl (static linking)
aarch64-apple-darwin          # Apple Silicon macOS
armv7-unknown-linux-gnueabihf # 32-bit ARM with hard float
```

**Why it works:**
- Precise specification of target platform
- Standardized across LLVM ecosystem
- Captures ABI differences (gnu vs musl)
- Composable and extensible
- Good for cross-compilation scenarios

**Evidence:**
- rustup uses this model successfully
- Nix cross-compilation system built on target triples
- Go uses similar `GOOS/GOARCH` model

**Caveats:**
- Not user-friendly for configuration
- May be overkill for package management (vs toolchain management)

---

## Pattern 7: Compatibility Layer via Binary Runtime

**Used by:** Homebrew (glibc), Nix (Nix store)

**Description:**
Instead of adapting to host distribution, bring a known-compatible runtime environment.

**How Homebrew does it:**
- Builds bottles against old glibc (2.23 on Ubuntu 16.04)
- If host glibc is older, builds its own glibc/gcc
- All dependencies self-contained in `/home/linuxbrew/.linuxbrew`

**How Nix does it:**
- All packages in `/nix/store` with fixed paths
- No dependency on host system except kernel
- Binary cache provides pre-built packages for common platforms

**Why it works:**
- Eliminates distribution detection complexity
- Same binaries work on any distribution
- No need to handle distribution-specific package names
- Truly reproducible builds

**Evidence:**
- Homebrew runs on virtually any modern Linux
- Nix packages work identically on Ubuntu, Fedora, Arch, etc.

**Caveats:**
- Larger disk footprint (own copies of libc, etc.)
- Initial installation more complex
- May conflict with system packages

---

## Pattern 8: Version File Convention

**Used by:** asdf, mise, rbenv, nvm, pyenv

**Description:**
Use a simple file (`.tool-versions`, `.node-version`, `.ruby-version`) to declare required tool versions, checked into version control.

**Format:**
```
# .tool-versions
nodejs 20.10.0
python 3.12.0
terraform 1.6.0
```

**Why it works:**
- Declarative and reproducible
- Easy to understand and edit
- Checked into version control with project
- Tool automatically uses correct version in directory

**Evidence:**
- `.tool-versions` is a de facto standard
- mise maintains compatibility with existing formats
- Teams use this for consistent development environments

---

## Pattern 9: Plugin Architecture for Extensibility

**Used by:** asdf, mise, Puppet providers, Chef resources

**Description:**
Allow community to extend tool with plugins that follow a defined interface, rather than trying to support everything in core.

**asdf plugin interface:**
```
bin/
  list-all        # List all available versions
  download        # Download specific version
  install         # Install version
  bin-path        # Return path to binaries
```

**Why it works:**
- Core stays simple and maintainable
- Community can add support for any tool
- Plugins can be updated independently
- Clear separation of concerns

**Evidence:**
- asdf has 600+ community plugins
- Puppet Forge has thousands of modules
- Chef Supermarket hosts community cookbooks

**Caveats:**
- Plugin quality varies widely
- Breaking changes in core affect all plugins
- Security concerns with community code

---

## Pattern 10: Structured Facts with Multiple Granularities

**Used by:** Ansible, Puppet, Chef

**Description:**
Expose platform information at multiple levels of detail in a structured format.

**Puppet example:**
```ruby
$facts['os']['name']           # "Ubuntu"
$facts['os']['family']         # "Debian"
$facts['os']['release']['full'] # "22.04"
$facts['os']['release']['major'] # "22"
$facts['os']['release']['minor'] # "04"
$facts['os']['distro']['codename'] # "jammy"
```

**Why it works:**
- Users choose appropriate level of specificity
- Structured access via hash/dictionary
- Version comparisons straightforward with numeric values
- All information available for complex conditionals

**Evidence:**
- All major CM tools provide structured facts
- User code migrating from flat to structured facts over time
