# Prior Art: Anti-Patterns and Failures

This document catalogs patterns that have failed, caused breaking changes, or created maintenance burdens in existing tools.

## Anti-Pattern 1: Distribution Enumeration Without Fallback

**Seen in:** Early Ansible, early Chef, many custom scripts

**Description:**
Explicitly listing every supported distribution in conditionals without a catch-all fallback.

**Example (problematic):**
```yaml
- name: Install package
  apt:
    name: nginx
  when: ansible_distribution == "Ubuntu" or ansible_distribution == "Debian"

- name: Install package
  yum:
    name: nginx
  when: ansible_distribution == "CentOS" or ansible_distribution == "RedHat"
# What about AlmaLinux? Rocky? Pop!_OS? Mint?
```

**Why it fails:**
- New distributions break the conditional
- Derivatives are not recognized
- Maintenance burden grows with each new distro
- Users on unlisted distributions get no installation

**Evidence:**
- Ansible GitHub issues show repeated "distribution not detected" problems
- Ubuntu 20.04 release broke playbooks that didn't anticipate it
- AlmaLinux/Rocky Linux required updates to many conditionals

**Better approach:**
- Use family-based matching with fallback to generic behavior

---

## Anti-Pattern 2: Hardcoded Detection File Paths

**Seen in:** Older versions of Ansible, Puppet, Chef

**Description:**
Relying on specific files like `/etc/redhat-release` or `/etc/debian_version` for detection without considering that the standard has moved to `/etc/os-release`.

**Example (problematic):**
```python
if os.path.exists('/etc/redhat-release'):
    distro = 'rhel'
elif os.path.exists('/etc/debian_version'):
    distro = 'debian'
# Fails on many modern systems that only have /etc/os-release
```

**Why it fails:**
- `/etc/os-release` is the systemd standard, not legacy files
- Minimal containers often only have `/etc/os-release`
- Legacy files have inconsistent formats
- Derivatives may not have parent's release file

**Evidence:**
- Ansible issue #70304: Debian Bullseye detection fails without `lsb_release`
- Puppet Facter had issues detecting Ubuntu without `lsb_release` package
- Docker containers frequently trigger these failures

**Better approach:**
- Use `/etc/os-release` as primary, legacy files as fallback

---

## Anti-Pattern 3: Changing Family Membership

**Seen in:** Chef (Amazon Linux), Ansible (various)

**Description:**
Changing which family a distribution belongs to, breaking existing code that relied on the previous classification.

**Case study - Amazon Linux:**
- Originally classified as `platform_family: rhel` in Chef
- Later changed to `platform_family: amazon`
- Broke cookbooks that assumed `rhel` family membership
- Required users to update conditionals

**Why it fails:**
- Existing code relies on family classification
- Users write `if platform_family?(:rhel)` expecting it to be stable
- No deprecation path for family changes

**Evidence:**
- Chef ohai changelog: "Amazon was updated to use platform_family of 'amazon' instead of RHEL"
- Multiple GitHub issues about Amazon Linux detection changes

**Better approach:**
- Family membership should be considered immutable once established
- If change is necessary, provide multi-version deprecation window
- Consider "secondary families" or traits that can change without breaking

---

## Anti-Pattern 4: Backend Library Swap with Different Semantics

**Seen in:** Ansible 2.8 (distro library change)

**Description:**
Replacing the underlying detection library with one that produces different results for edge cases, without a compatibility layer.

**Case study - Ansible 2.8:**
- Switched from custom detection to `nir0s/distro` Python library
- `ansible_distribution_version` format changed
- `ansible_distribution_release` values changed
- Debian version information differed from before

**Why it fails:**
- Existing playbooks break on subtle value changes
- Version comparison logic may fail
- String matching against release names fails
- Users don't expect detection changes in minor updates

**Evidence:**
- Ansible issue #57463: `ansible_distribution_version` missing minor version on Debian
- Ansible porting guide 2.8 explicitly documents this breaking change
- Many user reports of broken playbooks after upgrade

**Better approach:**
- If changing backends, add compatibility layer for common values
- Document changes clearly in upgrade guides
- Consider semantic versioning for fact value formats

---

## Anti-Pattern 5: Ignoring Package Name Differences

**Seen in:** Ansible `package` module misconception, naive cross-platform code

**Description:**
Providing a "generic package" abstraction without acknowledging that package names differ between distributions.

**Example (problematic):**
```yaml
# Works on Debian, fails on RHEL
- package:
    name: libc-dev
# RHEL name is glibc-devel
```

**Why it fails:**
- Package abstraction only abstracts the package manager, not package names
- Users expect "write once, run anywhere" but that's not reality
- Same software has different package names on different distros

**Evidence:**
- Common complaint in Ansible issues: "package module doesn't help with different names"
- Documentation explicitly notes this limitation
- ceph-ansible issue #520 discusses whether to use `package` vs specific modules

**Better approach:**
- Clearly document that package names must still vary
- Provide package name mapping feature or recommend variable-based approach
- Don't oversell the abstraction

---

## Anti-Pattern 6: Major Rewrite Breaking Plugin Ecosystem

**Seen in:** asdf 0.16.0 (Go rewrite)

**Description:**
Completely rewriting a tool in a new language, causing subtle incompatibilities with the plugin ecosystem.

**Case study - asdf 0.16.0:**
- Rewritten from Bash to Go
- Shell scripts without shebangs no longer work (Bash would execute them, Go's `syscall.Exec` cannot)
- `asdf update` command couldn't upgrade to new version
- Plugin manager compatibility table needed

**Why it fails:**
- Plugins written to work with original implementation details
- Language change affects execution semantics
- No clear migration path
- Community plugins require updates

**Evidence:**
- asdf upgrade guide explicitly warns about shebang requirement
- Plugin manager has compatibility table for pre/post 0.16.0
- Multiple GitHub issues about plugin breakage

**Better approach:**
- Maintain compatibility layer for plugin execution
- Test extensively with community plugins before release
- Provide automated migration tooling

---

## Anti-Pattern 7: Silent Detection Failures

**Seen in:** Ansible, Puppet, early Chef

**Description:**
When distribution detection fails or returns unexpected values, failing silently or with confusing downstream errors rather than clear warnings.

**Example (problematic):**
- Ansible on Ubuntu 20.04: `ansible_distribution` empty
- No clear error about detection failure
- Playbook proceeds with wrong conditionals
- Failures happen in unrelated package installation steps

**Why it fails:**
- Root cause hidden from user
- Debugging requires deep investigation
- Users don't know to check detection facts
- May proceed with wrong distro assumptions

**Evidence:**
- Ansible issue #78782: `pkg_mgr` detected as `dnf` on Ubuntu
- Blog post "how to handle no ansible_distribution for Ubuntu 20.04"
- Multiple GitHub issues about "wrong distro detected"

**Better approach:**
- Warn explicitly when detection is incomplete
- Fail early with clear message when required facts missing
- Provide `--check-facts` style verification mode

---

## Anti-Pattern 8: Assuming lsb_release Is Available

**Seen in:** Puppet Facter 3, older Ansible

**Description:**
Requiring the `lsb_release` command or `lsb-release` package for distribution detection, which is often not installed in minimal systems.

**Example (problematic):**
```python
# This fails in containers and minimal installations
result = subprocess.run(['lsb_release', '-a'], capture_output=True)
distro = parse_lsb_output(result.stdout)
```

**Why it fails:**
- Containers often don't have lsb_release
- Minimal server installs may not include it
- Cloud images frequently omit it
- `/etc/os-release` is always present on systemd systems

**Evidence:**
- Facter 3 known issues: "cannot resolve facts if lsb_release not installed"
- Puppet issues about missing `lsbdistcodename` on Debian
- Facter 4 specifically fixed this issue

**Better approach:**
- Use `/etc/os-release` as primary source
- Only use `lsb_release` as supplementary source for extra details

---

## Anti-Pattern 9: Per-Distribution Binary Bottles

**Seen in:** Considered and rejected by Homebrew/Linuxbrew

**Description:**
Building separate binary packages for each Linux distribution rather than using a compatibility approach.

**Why Homebrew rejected it:**
- Too many distributions to maintain
- Each distro needs its own CI/CD pipeline
- Storage and bandwidth multiply with each distro
- Still doesn't handle distro derivatives

**Quote from Homebrew issue #380:**
> "The Homebrew team would rather not make a separate bottle for each distribution since there are simply too many."

**Better approach (what Homebrew does):**
- Build against old glibc (2.12-2.23) for maximum compatibility
- Bring own glibc if host is too old
- Single bottle works on virtually any modern Linux

---

## Anti-Pattern 10: Breaking Changes in Upgrade Path

**Seen in:** asdf 0.15 -> 0.16, Facter 2 -> 3

**Description:**
Having an upgrade mechanism that cannot handle major version transitions, requiring manual intervention.

**Case study - asdf:**
- `asdf update` command in 0.15.x cannot upgrade to 0.16.x
- Users must manually uninstall and reinstall
- Shell configuration files must be edited manually
- Different installation paths between versions

**Why it fails:**
- Users expect `update` to just work
- Manual intervention is error-prone
- May break existing installations
- Discourages adoption of new versions

**Evidence:**
- asdf upgrade guide: "The asdf update command... cannot upgrade to version 0.16.0 because the install process has changed"
- Documentation provides rollback instructions

**Better approach:**
- Design upgrade mechanism to handle major transitions
- Or clearly communicate that manual upgrade is required before release
- Provide migration scripts

---

## Anti-Pattern 11: Inconsistent Version Number Formats

**Seen in:** Ansible, Puppet, Chef across different platforms

**Description:**
Exposing version numbers in inconsistent formats that make comparison difficult.

**Examples of inconsistency:**
```
# Debian
ansible_distribution_version: "10"  # Only major (unlike other distros)
ansible_distribution_major_version: "10"  # Same as above

# Ubuntu
ansible_distribution_version: "22.04"
ansible_distribution_major_version: "22"

# Amazon Linux
platform_version returned as null in some Chef versions
```

**Why it fails:**
- Version comparisons require per-distro logic
- `version > "20"` doesn't work when format varies
- Users write fragile comparison code
- Numeric vs string comparison confusion

**Evidence:**
- Ansible issue #57463: Debian version missing minor
- Chef Amazon Linux platform_version null issue #1207
- Multiple tools have version detection edge cases documented

**Better approach:**
- Normalize to semantic version format (major.minor.patch)
- Provide helper functions for version comparison
- Document expected format per distribution
