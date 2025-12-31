# Package Manager Action Coverage Analysis

Research findings for SPEC-package-managers.md (P1-C).

## Which Package Managers Need Dedicated tsuku Actions?

This document analyzes which package managers warrant dedicated `*_install` actions based on usage, compatibility, and implementation complexity.

---

## Tiered Approach

### Tier 1: Dedicated Actions (High Priority)

**Criteria:**
- Combined distro market share > 10%
- Full compatibility with imperative model
- Well-documented, stable API
- Clear third-party repo/GPG patterns

| Manager | Market Share | Action Name | Justification |
|---------|-------------|-------------|---------------|
| **apt** | ~50% | `apt_install` | Largest user base (Ubuntu, Debian, Mint) |
| **dnf** | ~30% | `dnf_install` | Enterprise critical (RHEL, Fedora, Rocky) |
| **pacman** | ~10% | `pacman_install` | Developer/power user base (Arch, Manjaro) |

**Implementation Priority:** apt > dnf > pacman

---

### Tier 2: Add If Needed (Medium Priority)

**Criteria:**
- Moderate usage (containers, specific use cases)
- Full compatibility with imperative model
- May be added based on user demand

| Manager | Use Case | Action Name | Justification |
|---------|----------|-------------|---------------|
| **apk** | Containers | `apk_install` | Docker base image standard |
| **zypper** | Enterprise | `zypper_install` | SUSE enterprise deployments |
| **brew** | Cross-platform | `brew_install` | User-space fallback option |

**When to implement:**
- `apk_install`: When tsuku is used inside containers
- `zypper_install`: When SUSE enterprise users request it
- `brew_install`: As cross-distro fallback or for macOS support

---

### Tier 3: Consider Later (Low Priority)

**Criteria:**
- Niche distribution usage
- Full compatibility but small user base
- May never warrant dedicated action

| Manager | Distro | Notes |
|---------|--------|-------|
| **xbps** | Void Linux | Small but dedicated user base |
| **eopkg** | Solus | Small distro, PM being replaced |
| **swupd** | Clear Linux | Bundle model, different granularity |

**Recommendation:** Monitor user requests; implement if demand materializes.

---

### Tier 4: Explicitly Unsupported

**Criteria:**
- Fundamentally incompatible model
- Supporting would fight the paradigm
- Users have better native solutions

| Manager | Reason | Alternative |
|---------|--------|-------------|
| **nix** | Declarative model incompatible | Users should use nix directly |
| **guix** | Declarative model incompatible | Users should use guix directly |
| **portage** | Source-based, impractically slow | Gentoo users compile from source |

**Recommendation:** Document that these are unsupported and why.

---

## Action Interface Design

### Common Interface

All `*_install` actions should implement a common interface:

```go
type PackageInstallAction interface {
    // Validate checks if the action can be executed
    Validate(ctx context.Context) error

    // Execute runs the package installation
    Execute(ctx context.Context) error

    // Rollback undoes the installation (best effort)
    Rollback(ctx context.Context) error

    // RequiresRoot returns whether root privileges are needed
    RequiresRoot() bool

    // Manager returns the package manager name
    Manager() string
}
```

### Shared Configuration Fields

```toml
# Common fields across all *_install actions
packages = ["package1", "package2"]  # Required: packages to install
update_index = true                   # Optional: update package index first

# Repo configuration (structure varies by manager)
[[repos]]
name = "repo-name"
# ... manager-specific fields
```

---

## Tier 1 Action Specifications

### apt_install

```toml
[actions.apt_install]
packages = ["package1", "package2"]
update_first = true  # run apt update before install

# Modern deb822 format (recommended)
[[actions.apt_install.repos]]
name = "example"
types = "deb"
uris = "https://example.com/apt"
suites = "stable"
components = "main"
signed_by = "/etc/apt/keyrings/example.gpg"
key_url = "https://example.com/key.gpg"  # tsuku downloads and installs key

# Legacy sources.list format (alternative)
[[actions.apt_install.repos]]
name = "example-legacy"
line = "deb [signed-by=/etc/apt/keyrings/example.gpg] https://example.com/apt stable main"
key_url = "https://example.com/key.gpg"
```

**Implementation notes:**
- Detect Ubuntu/Debian version for `suites` field
- Use `/etc/apt/keyrings/` for GPG keys (modern best practice)
- Generate both deb822 `.sources` files (modern) or `.list` files (legacy)
- Always use `signed-by` directive for security

---

### dnf_install

```toml
[actions.dnf_install]
packages = ["package1", "package2"]

[[actions.dnf_install.repos]]
name = "example"
baseurl = "https://example.com/fedora/$releasever/$basearch"
gpgkey = "https://example.com/key.gpg"
gpgcheck = true
enabled = true
```

**Implementation notes:**
- Generate `.repo` files in `/etc/yum.repos.d/`
- GPG key URL in repo file, dnf handles import
- Use `$releasever` and `$basearch` variables
- yum is alias for dnf on modern systems

---

### pacman_install

```toml
[actions.pacman_install]
packages = ["package1", "package2"]
sync = true  # run pacman -Sy before install

[[actions.pacman_install.repos]]
name = "example"
server = "https://example.com/$arch"
siglevel = "Required DatabaseOptional"

[[actions.pacman_install.keys]]
keyid = "ABCDEF123456"
keyserver = "keyserver.ubuntu.com"  # optional
localsign = true
```

**Implementation notes:**
- Modify `/etc/pacman.conf` to add repos
- Use `pacman-key --recv-keys` and `--lsign-key` for GPG
- Place repos before `[core]` section
- Use appropriate SigLevel

---

## Tier 2 Action Specifications

### apk_install

```toml
[actions.apk_install]
packages = ["package1", "package2"]
update = true  # run apk update first

repos = [
    "https://example.com/alpine/v3.18/main",
    "@testing https://dl-cdn.alpinelinux.org/alpine/edge/testing"
]

[[actions.apk_install.keys]]
name = "example@example.com.rsa.pub"
content = "-----BEGIN PUBLIC KEY-----\n..."
```

**Implementation notes:**
- Append to `/etc/apk/repositories`
- Keys go in `/etc/apk/keys/`
- Support tagged repos (`@testing`)

---

### zypper_install

```toml
[actions.zypper_install]
packages = ["package1", "package2"]
auto_import_keys = true  # use --gpg-auto-import-keys

[[actions.zypper_install.repos]]
name = "example"
url = "https://example.com/opensuse/"
refresh = true
```

**Implementation notes:**
- Use `zypper addrepo` for adding repos
- `--gpg-auto-import-keys` or explicit `rpm --import`

---

### brew_install

```toml
[actions.brew_install]
packages = ["package1", "package2"]
casks = ["cask1", "cask2"]  # GUI applications (optional)

taps = [
    "user/repo",
    "organization/tap"
]
```

**Implementation notes:**
- No root required
- `brew tap` for third-party repos
- Can be used as cross-distro fallback
- Check if brew is installed first

---

## Detection and Fallback Strategy

### Detection Order

1. Check `/etc/os-release` for `ID` and `ID_LIKE`
2. Verify package manager binary exists
3. Check if tsuku has action for that manager
4. Fall back to download-based installation

### Fallback Chain

```
apt_install available?
  └─ Yes → Use apt_install
  └─ No → dnf_install available?
            └─ Yes → Use dnf_install
            └─ No → pacman_install available?
                      └─ Yes → Use pacman_install
                      └─ No → brew_install available?
                                └─ Yes → Use brew_install
                                └─ No → Use download action
```

---

## Implementation Roadmap

### Phase 1: Core Actions (Covers ~90% of users)

1. **apt_install** - Ubuntu, Debian, Mint, Pop!_OS
2. **dnf_install** - Fedora, RHEL, Rocky, Alma
3. **pacman_install** - Arch, Manjaro, EndeavourOS

### Phase 2: Extended Support

4. **apk_install** - Alpine (containers)
5. **zypper_install** - openSUSE, SLES
6. **brew_install** - Cross-platform fallback

### Phase 3: Niche Support (If Requested)

7. **xbps_install** - Void Linux
8. **eopkg_install** - Solus

### Never Implement

- nix_install (declarative, fundamentally incompatible)
- guix_install (declarative, fundamentally incompatible)
- emerge_install (source-based, impractically slow)

---

## Complexity Estimates

| Action | Complexity | Reason |
|--------|------------|--------|
| apt_install | Medium | Repo formats, key management, deb822 vs legacy |
| dnf_install | Low | Simple .repo format, straightforward GPG |
| pacman_install | Medium | pacman.conf editing, pacman-key management |
| apk_install | Low | Simple format |
| zypper_install | Low | Built-in repo management |
| brew_install | Low | No root, simple tap system |
| xbps_install | Low | Simple format |
| eopkg_install | Medium | XML format |

---

## Testing Strategy

### Per-Action Tests

Each action should be tested in:
1. Docker container with target distro
2. Clean install (no prior repos)
3. Idempotent re-run
4. Rollback verification

### Test Matrix

| Action | Test Container |
|--------|---------------|
| apt_install | `ubuntu:24.04`, `debian:12` |
| dnf_install | `fedora:40`, `rockylinux:9` |
| pacman_install | `archlinux:latest` |
| apk_install | `alpine:3.19` |
| zypper_install | `opensuse/leap:15.5` |
| brew_install | Any Linux (install brew first) |
