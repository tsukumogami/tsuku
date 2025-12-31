# Package Manager Recommendations for tsuku

Research findings for SPEC-package-managers.md (P1-C).

## Executive Summary

Based on the package manager inventory and compatibility analysis, this document provides recommendations for tsuku's package manager integration strategy.

---

## Key Recommendations

### 1. Implement Three Core Actions First

**Recommendation:** Implement `apt_install`, `dnf_install`, and `pacman_install` as the initial `*_install` actions.

**Rationale:**
- These three families cover approximately 90% of Linux desktop/server usage
- All follow the same imperative model tsuku uses
- Clear, well-documented APIs and patterns
- Extensive community knowledge for troubleshooting

**Coverage:**
| Action | Distros Covered | Market Share |
|--------|-----------------|--------------|
| apt_install | Ubuntu, Debian, Mint, Pop!_OS, Kali, Raspberry Pi OS, MX Linux, etc. | ~50% |
| dnf_install | Fedora, RHEL, Rocky, Alma, CentOS Stream, Oracle Linux | ~30% |
| pacman_install | Arch, Manjaro, EndeavourOS, Garuda, CachyOS | ~10% |

---

### 2. Add package_manager as a Filter Dimension

**Recommendation:** Add `package_manager` as a recipe filter dimension alongside `os`, `arch`, and `distro`.

**Rationale:**
- Some packages are only available in certain package managers
- Third-party repos are package-manager specific
- Allows recipes to specify manager-specific installation paths

**Proposed schema:**
```toml
[[install]]
os = "linux"
package_manager = "apt"  # only match systems with apt

[install.actions.apt_install]
packages = ["docker-ce"]
```

**Alternative design:**
Use `distro_family` instead of `package_manager`:
```toml
[[install]]
os = "linux"
distro_family = "debian"  # implies apt
```

**Recommendation:** Use `package_manager` for clarity, since:
- Homebrew can be installed on any distro
- nix can be installed on any distro
- The package manager, not the distro, determines what's available

---

### 3. Explicitly Unsupport Declarative Package Managers

**Recommendation:** Do not implement `nix_install` or `guix_install` actions. Document this decision clearly.

**Rationale:**
- nix and guix use fundamentally different paradigms (declarative vs imperative)
- Attempting to wrap them would fight their design philosophy
- Users on NixOS/Guix already have superior package management
- tsuku's value-add is minimal for these users

**Documentation to add:**
```markdown
## Unsupported Package Managers

tsuku does not support declarative package managers:

- **nix**: NixOS users should add tools to their `configuration.nix` or use `home-manager`
- **guix**: Guix System users should use `guix install` or their system configuration

These package managers provide reproducibility and atomicity guarantees that tsuku's
imperative model cannot match. Using them directly is the better choice.
```

---

### 4. Use brew as Cross-Distro Fallback

**Recommendation:** Consider implementing `brew_install` as a fallback for distros without dedicated actions.

**Advantages:**
- User-space installation (no sudo required)
- Works on any Linux distro
- Large package catalog
- Aligns with tsuku's "no system dependencies" philosophy

**Use cases:**
- Void Linux (no xbps_install action)
- Gentoo (source-based, impractical)
- Any distro where native PM lacks the tool
- Users who want cross-distro consistency

**Trade-offs:**
- Adds dependency on brew installation
- Duplicates functionality of native PMs
- May confuse users about which PM to use

**Recommendation:** Implement as opt-in, not automatic fallback.

---

### 5. Defer Container-Specific Actions

**Recommendation:** Defer `apk_install` until tsuku usage in containers is validated.

**Rationale:**
- Alpine is primarily used in Docker containers
- tsuku's value prop is less clear in ephemeral containers
- Container builds typically use `RUN apk add` directly
- Focus on desktop/server users first

**When to implement:**
- Users request tsuku for building container images
- CI/CD use case emerges where tsuku manages container tool deps

---

### 6. Detection Strategy

**Recommendation:** Implement detection based on `/etc/os-release` with binary fallback.

**Detection order:**
1. Read `/etc/os-release` for `ID` and `ID_LIKE`
2. Map to package manager family
3. Verify package manager binary exists
4. Check if tsuku has action for that manager
5. If no action available, fall back to download-based installation

**Mapping table:**
```go
var distroToManager = map[string]string{
    // ID values
    "ubuntu":          "apt",
    "debian":          "apt",
    "linuxmint":       "apt",
    "pop":             "apt",
    "fedora":          "dnf",
    "rhel":            "dnf",
    "centos":          "dnf",
    "rocky":           "dnf",
    "almalinux":       "dnf",
    "arch":            "pacman",
    "manjaro":         "pacman",
    "endeavouros":     "pacman",
    "alpine":          "apk",
    "opensuse-leap":   "zypper",
    "opensuse-tumbleweed": "zypper",
    "void":            "xbps",
    "gentoo":          "portage",
    "nixos":           "nix",
    "solus":           "eopkg",
}

var idLikeToManager = map[string]string{
    "debian": "apt",
    "ubuntu": "apt",
    "rhel":   "dnf",
    "fedora": "dnf",
    "arch":   "pacman",
    "suse":   "zypper",
}
```

---

### 7. Recipe Strategy Guidance

**Recommendation:** Provide clear guidance for recipe authors on when to use `*_install` vs download actions.

**Use `*_install` actions when:**
- Tool is available in distro's official repositories
- Tool requires system integration (systemd, /etc configs)
- Third-party repo is well-established (Docker, GitHub CLI, etc.)
- Package has complex dependencies best handled by distro

**Use download actions when:**
- Tool ships as standalone binary (Go, Rust binaries)
- Distro repos have outdated versions
- Cross-distro consistency is important
- Version pinning is required

**Example decision tree:**
```
Is the tool a standalone binary (no dependencies)?
  └─ Yes → Use download action
  └─ No → Is it in distro's official repo?
            └─ Yes → Is the version acceptable?
                      └─ Yes → Use *_install action
                      └─ No → Use download action
            └─ No → Is there a well-known third-party repo?
                      └─ Yes → Use *_install with repo config
                      └─ No → Use download action
```

---

## Implementation Priority

### Phase 1: MVP (Q1)

1. Implement `apt_install` action
2. Implement `dnf_install` action
3. Add `package_manager` filter dimension
4. Add distro detection logic

### Phase 2: Extended Coverage (Q2)

5. Implement `pacman_install` action
6. Implement `apk_install` action (if container use case validated)
7. Add `brew_install` as optional fallback

### Phase 3: Polish (Q3)

8. Implement `zypper_install` if SUSE users request
9. Add rollback support to all actions
10. Comprehensive testing matrix

---

## Open Questions

### Q1: Should `package_manager` be auto-detected or explicit?

**Option A:** Auto-detect based on `/etc/os-release`, recipe specifies packages
```toml
[[install]]
os = "linux"
[install.actions.system_install]  # auto-detect PM
packages = ["docker-ce"]
```

**Option B:** Require explicit package manager in recipe
```toml
[[install]]
os = "linux"
package_manager = "apt"
[install.actions.apt_install]
packages = ["docker-ce"]
```

**Recommendation:** Option B for now. Auto-detection adds complexity and package names differ across managers.

---

### Q2: How to handle manager-specific package names?

Example: `docker-ce` (apt) vs `docker` (dnf) vs `docker` (pacman)

**Option A:** Require separate install blocks per manager
```toml
[[install]]
package_manager = "apt"
[install.actions.apt_install]
packages = ["docker-ce", "docker-ce-cli"]

[[install]]
package_manager = "dnf"
[install.actions.dnf_install]
packages = ["docker-ce", "docker-ce-cli"]

[[install]]
package_manager = "pacman"
[install.actions.pacman_install]
packages = ["docker"]
```

**Option B:** Add package name mapping to recipes
```toml
[packages]
apt = ["docker-ce", "docker-ce-cli"]
dnf = ["docker-ce", "docker-ce-cli"]
pacman = ["docker"]

[[install]]
os = "linux"
[install.actions.system_install]
packages = "$packages"
```

**Recommendation:** Option A for clarity. Option B adds indirection complexity.

---

### Q3: Should tsuku manage third-party repos long-term?

Adding a third-party repo creates ongoing responsibility:
- Repository URLs may change
- GPG keys may rotate
- Repos may be discontinued

**Recommendation:** Recipes should be versioned and maintained. Consider:
- Registry update mechanism
- Repo health checks
- Clear deprecation path for abandoned repos

---

## Summary Table

| Package Manager | Support Level | Action Name | Priority |
|-----------------|--------------|-------------|----------|
| apt | Full | `apt_install` | P0 |
| dnf/yum | Full | `dnf_install` | P0 |
| pacman | Full | `pacman_install` | P1 |
| apk | Full | `apk_install` | P2 |
| zypper | Full | `zypper_install` | P2 |
| brew | Full | `brew_install` | P2 |
| xbps | Defer | - | P3 |
| eopkg | Defer | - | P3 |
| swupd | Partial | - | Unlikely |
| portage | Partial | - | Unsupported |
| nix | None | - | Unsupported |
| guix | None | - | Unsupported |
