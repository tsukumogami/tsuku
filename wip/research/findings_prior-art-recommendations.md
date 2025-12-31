# Prior Art: Recommendations for tsuku

This document synthesizes findings from prior art research into actionable recommendations for tsuku's Linux distribution targeting model.

## Executive Summary

Based on analysis of Ansible, Puppet, Chef, Nix, Homebrew, asdf/mise, and rustup, we recommend tsuku adopt a **hybrid model** that combines:

1. **Homebrew-style binary compatibility** as the primary approach (download static/portable binaries)
2. **Chef-style platform_family hierarchy** as fallback for distro-specific recipes
3. **rustup-style target triples** for precise platform specification in downloads

This minimizes distribution detection complexity while providing escape hatches when needed.

## Closest Model to tsuku's Needs: Homebrew + rustup

**Why Homebrew:**
- tsuku's goal is installing developer tools, not system configuration
- Tools are typically downloaded as pre-built binaries (like Homebrew bottles)
- tsuku doesn't need to integrate with system package managers
- The "bring your own runtime" approach (portable binaries) sidesteps distro complexity

**Why rustup:**
- Clear target triple syntax for specifying binary variants
- Explicit musl vs glibc distinction when needed
- Users understand `linux-gnu` vs `linux-musl` distinction

**Why NOT full Ansible/Puppet/Chef:**
- CM tools need deep distro integration for system configuration
- tsuku only needs to download and extract binaries
- Over-engineering distro detection adds maintenance burden without benefit

## Recommendation 1: Minimize Distribution Detection

**Recommendation:** Default to architecture-only detection; add distro detection only when recipes require it.

**Rationale:**
- Most developer tool binaries are glibc-portable or statically linked
- Detection complexity causes many of the bugs seen in prior art
- Only ~10% of tools need distro-specific handling

**Implementation:**
```go
// Primary detection: just arch + OS + libc
type Platform struct {
    OS       string // "linux", "darwin", "windows"
    Arch     string // "amd64", "arm64", "arm"
    Libc     string // "gnu", "musl", "" (auto-detect from /lib)
}

// Secondary detection: only when recipe requires it
type DistroInfo struct {
    ID       string // "ubuntu", "fedora", "arch"
    Family   string // "debian", "rhel", "arch"
    Version  string // "22.04", "40", "rolling"
}
```

**What to avoid:**
- Don't detect distro by default
- Don't maintain per-distro conditionals in core
- Don't try to abstract package managers (not tsuku's job)

---

## Recommendation 2: Use /etc/os-release with Minimal Parsing

**Recommendation:** When distro detection is needed, use only `/etc/os-release` with a fallback to "unknown-linux".

**Rationale:**
- It's the systemd standard, present on all modern Linux
- `ID_LIKE` field provides family information
- Don't fall back to legacy files (adds complexity, breaks in containers)

**Implementation:**
```go
// Parse only essential fields
type OSRelease struct {
    ID      string   // "ubuntu"
    IDLike  []string // ["debian"]
    Version string   // "22.04"
}

func DetectDistro() (*OSRelease, error) {
    f, err := os.Open("/etc/os-release")
    if err != nil {
        return &OSRelease{ID: "unknown"}, nil  // Don't fail
    }
    // Parse ID, ID_LIKE, VERSION_ID only
}
```

**What to avoid:**
- Don't require `lsb_release` command
- Don't parse `/etc/redhat-release`, `/etc/debian_version` etc.
- Don't try to detect "codename" - version is enough

---

## Recommendation 3: Adopt Three-Level Targeting Hierarchy

**Recommendation:** Support targeting at three levels: generic, family, specific.

**Rationale:**
- Chef's `platform_family` concept is proven and intuitive
- Allows recipe authors to choose appropriate specificity
- New distros automatically inherit family behavior

**Hierarchy:**
```
Level 1 (Generic):  linux
Level 2 (Family):   debian, rhel, arch, suse, alpine
Level 3 (Specific): ubuntu-22.04, fedora-40, arch (rolling)
```

**Recipe syntax:**
```toml
# Most tools - no targeting needed (works everywhere)
[actions.install_binary]
url = "https://example.com/tool-{{.Version}}-linux-{{.Arch}}.tar.gz"

# When family matters (e.g., different dependency names)
[actions.install_binary]
url.linux = "https://example.com/tool-linux.tar.gz"
url.linux-musl = "https://example.com/tool-linux-musl.tar.gz"

# Rare case: distro-specific override
[actions.install_binary]
url.ubuntu = "https://example.com/tool.deb"
url.fedora = "https://example.com/tool.rpm"
url.default = "https://example.com/tool.tar.gz"
```

**Family mappings (keep minimal):**
```go
var familyMap = map[string]string{
    "ubuntu": "debian", "debian": "debian", "pop": "debian", "mint": "debian",
    "fedora": "rhel", "rhel": "rhel", "centos": "rhel", "rocky": "rhel", "alma": "rhel",
    "arch": "arch", "manjaro": "arch", "endeavouros": "arch",
    "alpine": "alpine",
    "opensuse": "suse", "sles": "suse",
}
```

---

## Recommendation 4: Prefer musl/Static Binaries When Available

**Recommendation:** When a tool offers musl-linked or static binary, prefer it over glibc.

**Rationale:**
- musl binaries work on any Linux (including Alpine)
- No glibc version compatibility concerns
- Smaller binary size in many cases
- This is how rustup handles cross-distro

**Implementation:**
```toml
# Recipe can specify preference
[download]
prefer_static = true

# Templates resolve to musl first
url = "https://example.com/tool-{{.Version}}-{{.TargetTriple}}.tar.gz"
# Tries: x86_64-unknown-linux-musl, then x86_64-unknown-linux-gnu
```

**What to avoid:**
- Don't require distro-specific glibc versions
- Don't assume gnu libc is available

---

## Recommendation 5: Use Target Triples for Binary Selection

**Recommendation:** Adopt rustup-style target triples internally for binary selection.

**Rationale:**
- Standard format understood by developers
- Encodes all relevant information (arch, OS, libc)
- Many upstream projects already use this naming convention

**Target triple vocabulary for tsuku:**
```
x86_64-unknown-linux-gnu      # Standard 64-bit Linux
x86_64-unknown-linux-musl     # Static/Alpine-compatible
aarch64-unknown-linux-gnu     # ARM64 Linux
aarch64-unknown-linux-musl    # ARM64 static
armv7-unknown-linux-gnueabihf # 32-bit ARM with hard float
x86_64-apple-darwin           # Intel Mac
aarch64-apple-darwin          # Apple Silicon
x86_64-pc-windows-msvc        # Windows
```

**Template variable:**
```toml
url = "https://github.com/user/repo/releases/download/{{.Version}}/tool-{{.TargetTriple}}.tar.gz"
```

---

## Recommendation 6: Fail Gracefully with Clear Messages

**Recommendation:** When platform is unsupported, provide actionable error with platform details.

**Rationale:**
- Prior art shows silent failures cause user confusion
- Users need to know what was detected and what's missing
- Good error messages reduce support burden

**Implementation:**
```
Error: Recipe "tool-name" does not support your platform.

Detected platform:
  OS:           linux
  Architecture: amd64
  Distribution: fedora
  Family:       rhel
  Version:      40

Recipe supports:
  - darwin (amd64, arm64)
  - linux-debian

Consider:
  - Opening an issue at https://github.com/tsuku/recipes
  - Checking if a portable binary exists upstream
```

---

## Recommendation 7: Don't Change Family Membership

**Recommendation:** Once a distribution is assigned to a family, never change it.

**Rationale:**
- Chef's Amazon Linux family change broke many cookbooks
- Family membership should be stable for recipe authors
- Add new families rather than restructure existing ones

**Commitment:**
```go
// Document these mappings as stable API
// Once published, a distro's family NEVER changes
const (
    FamilyDebian = "debian"  // ubuntu, debian, pop, mint, ...
    FamilyRHEL   = "rhel"    // fedora, rhel, centos, rocky, alma, ...
    FamilyArch   = "arch"    // arch, manjaro, endeavouros, ...
    FamilyAlpine = "alpine"  // alpine
    FamilySUSE   = "suse"    // opensuse, sles
    FamilyAmazon = "amazon"  // amazon linux (NOT rhel!)
)
```

---

## Recommendation 8: Document Platform Detection Behavior

**Recommendation:** Maintain clear documentation on what is detected and how.

**Rationale:**
- Prior art shows detection changes cause user confusion
- Recipe authors need to know what values to expect
- Users debugging issues need to understand the model

**Documentation structure:**
```markdown
## Platform Detection

tsuku detects the following platform attributes:

| Attribute | Source | Example Values |
|-----------|--------|----------------|
| os | Go runtime | linux, darwin, windows |
| arch | Go runtime | amd64, arm64, arm |
| libc | /lib/ld-* probe | gnu, musl |
| distro.id | /etc/os-release ID | ubuntu, fedora, arch |
| distro.family | Mapped from ID | debian, rhel, arch |
| distro.version | /etc/os-release VERSION_ID | 22.04, 40, rolling |

### Family Mappings

| Distribution ID | Family |
|-----------------|--------|
| ubuntu, debian, pop, mint, ... | debian |
| fedora, rhel, centos, rocky, alma, ... | rhel |
| ...
```

---

## Summary: What to Adopt vs Avoid

### ADOPT

| Pattern | From | Why |
|---------|------|-----|
| Portable binaries first | Homebrew | Avoids distro complexity |
| Target triples | rustup | Standard, expressive format |
| Family hierarchy | Chef | Proven, maintainable |
| /etc/os-release only | All modern tools | Standard, always present |
| Clear failure messages | Best practice | User experience |
| Immutable family membership | Lesson learned | Stability for recipe authors |

### AVOID

| Anti-pattern | Why |
|--------------|-----|
| Per-distro enumeration | Maintenance nightmare, breaks on new distros |
| Multiple detection sources | Complexity, inconsistency |
| Changing family membership | Breaks existing recipes |
| Silent detection failures | User confusion |
| Package manager abstraction | Not tsuku's job |
| Requiring lsb_release | Unavailable in containers |
| Deep distro integration | Over-engineering for tool installation |

---

## Next Steps

1. **Phase 2:** Design tsuku's hierarchy model based on these recommendations
2. **Prototype:** Implement minimal detection in Go
3. **Validate:** Test against sample recipes with distro-specific needs
4. **Document:** Write user-facing platform targeting documentation
