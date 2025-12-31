# Linux Distro Detection Challenges for `target = (platform, distro)` Model

## Executive Summary

This research analyzes challenges with Linux distribution detection and matching when extending tsuku's platform model from `(os, arch)` to `(os, arch, distro)`. The analysis covers `/etc/os-release` format variations, derivative chain handling, version constraints, container edge cases, and architectural recommendations.

**Key findings:**
1. `/etc/os-release` is well-standardized but has practical variations requiring defensive parsing
2. `ID_LIKE` chains are inconsistently maintained across derivatives; implicit matching is pragmatic but imperfect
3. Version handling adds significant complexity; defer to feature detection where possible
4. Container and WSL environments generally have valid `/etc/os-release`; scratch images are the exception
5. Recommend: structured type for distro with parsed fields, canonical list of "first-class" distros, explicit matching with ID_LIKE fallback

---

## 1. `/etc/os-release` Format Specification

### Standard Location and Format

The [freedesktop.org specification](https://www.freedesktop.org/software/systemd/man/latest/os-release.html) defines `/etc/os-release` as the canonical source. Key points:

- **Primary path**: `/etc/os-release` (preferred)
- **Fallback path**: `/usr/lib/os-release` (vendor trees)
- **Format**: Shell-compatible variable assignments, one per line
- **Encoding**: UTF-8, no shell variable expansion

```ini
ID=ubuntu
ID_LIKE=debian
VERSION_ID="22.04"
VERSION_CODENAME=jammy
PRETTY_NAME="Ubuntu 22.04.3 LTS"
```

### Key Fields for Detection

| Field | Format | Purpose | Detection Use |
|-------|--------|---------|---------------|
| `ID` | lowercase a-z, 0-9, ., _, - | Canonical distro identifier | Primary match target |
| `ID_LIKE` | space-separated IDs | Parent/similar distros | Fallback matching |
| `VERSION_ID` | lowercase, mostly numeric | Script-friendly version | Version filtering |
| `VERSION_CODENAME` | lowercase identifier | Release codename | Alternative version ref |
| `VARIANT_ID` | lowercase identifier | Distro variant (server, workstation) | Optional refinement |

### Format Variations Observed

**Quoting styles:**
```ini
# Single quotes (rare but valid)
NAME='Alpine Linux'

# Double quotes (standard)
NAME="Ubuntu"

# Unquoted (valid for simple values)
ID=debian
```

**Missing fields:**
- Arch Linux: No `VERSION_ID` (rolling release, uses `BUILD_ID=rolling`)
- Debian testing/unstable: No `VERSION_ID` (stable has it)
- Custom/minimal containers: May have partial information

---

## 2. `ID_LIKE` Chains: Derivative Handling

### The Inheritance Problem

Distributions form inheritance trees:
```
debian
  ubuntu
    pop
    linuxmint
    elementary
  linuxmint (debian edition - different branch!)
fedora
  rhel
    centos
    rocky
    alma
arch
  manjaro
  endeavouros
```

### Inconsistent `ID_LIKE` Values

Based on research from the [which-distro/os-release](https://github.com/which-distro/os-release) collection:

| Distro | `ID` | `ID_LIKE` | Issue |
|--------|------|-----------|-------|
| Ubuntu | ubuntu | debian | Correct single-level |
| Pop!_OS | pop | ubuntu | **Missing debian** |
| Linux Mint | linuxmint | ubuntu debian | Correct full chain |
| Elementary | elementary | ubuntu | **Missing debian** |
| Manjaro | manjaro | arch | Correct |
| Rocky Linux | rocky | rhel centos fedora | Full chain |

**Problem**: Pop!_OS declares only `ID_LIKE=ubuntu`, not `ID_LIKE="ubuntu debian"`. This means:
- Matching `distro = ["debian"]` won't catch Pop!_OS without transitive resolution
- Different distros at the same family level have inconsistent declarations

### Matching Strategies

**Strategy A: Direct `ID` and `ID_LIKE` matching (no transitivity)**
```go
func matches(target string, id string, idLike []string) bool {
    if target == id { return true }
    for _, like := range idLike {
        if target == like { return true }
    }
    return false
}
```
- Pro: Simple, predictable, honors what distro declares
- Con: Pop!_OS won't match `debian` even though it's debian-based

**Strategy B: Transitive resolution (build full chain)**
```go
// Maintain known hierarchy
var knownParents = map[string][]string{
    "ubuntu": {"debian"},
    "pop": {"ubuntu"},
    // ...
}

func expandChain(id string, idLike []string) []string {
    chain := append([]string{id}, idLike...)
    for _, like := range idLike {
        if parents, ok := knownParents[like]; ok {
            chain = append(chain, parents...)
        }
    }
    return dedupe(chain)
}
```
- Pro: Handles incomplete `ID_LIKE` declarations
- Con: Requires maintaining distro hierarchy knowledge; can become stale

**Strategy C: Hybrid (ID_LIKE first, known fallbacks for specific distros)**
- Use `ID_LIKE` as declared
- Only apply transitivity for known problem cases (pop, elementary)
- Log warning when applying transitive resolution

### Recommendation

**Use Strategy A (direct matching) with explicit recipe authoring guidance.**

Rationale:
1. Simpler implementation, fewer surprises
2. Recipe authors explicitly list supported distros: `distro = ["ubuntu", "debian", "pop", "mint"]`
3. If a derivative doesn't work, recipe update is straightforward
4. Avoids maintaining a parallel distro hierarchy database

The `ID_LIKE` fallback still catches most cases. For the edge cases (Pop!_OS matching debian), recipes should list both `ubuntu` and `debian` if they need debian-family compatibility.

---

## 3. Version Handling

### The Version Constraint Question

Should tsuku support `distro = ["ubuntu>=22.04"]`?

### Versioning Scheme Variations

| Distro | VERSION_ID Format | Example | Notes |
|--------|-------------------|---------|-------|
| Ubuntu | YY.MM | 22.04 | Point releases add patch (22.04.3) |
| Debian | integer | 12 | Codenames preferred (bookworm) |
| Fedora | integer | 39 | Increments with each release |
| RHEL | X.Y | 9.3 | Major.minor |
| Arch | *empty* | - | Rolling release |
| Alpine | X.Y.Z | 3.18.4 | Semantic-ish |
| NixOS | YY.MM | 24.05 | But fundamentally different model |

### Comparison Challenges

```go
// Naive string comparison fails
"22.04" > "21.10"  // true (correct)
"22.04" > "9.3"    // false (string comparison: "2" < "9")
"3.18.4" > "3.9"   // false (string comparison: "1" < "9")
```

Proper version comparison requires:
1. Parsing into components
2. Numeric comparison of each component
3. Handling variable-length versions

### Recommendation: No Version Constraints (Initially)

**Defer version-specific logic to feature detection:**

Instead of:
```toml
[[steps]]
action = "apt_install"
packages = ["docker-ce"]
when = { distro = ["ubuntu>=22.04"] }  # Complex
```

Use:
```toml
[[steps]]
action = "require_command"
command = "apt"
version_flag = "--version"

[[steps]]
action = "apt_install"
packages = ["docker-ce"]
when = { distro = ["ubuntu", "debian"] }
```

If version constraints become essential:
1. Introduce `distro_version_min` and `distro_version_max` as separate fields
2. Parse versions using semantic versioning library with distro-specific adapters
3. Add to Phase 2 after core detection is proven

---

## 4. Container and Special Environments

### Docker Containers

**Standard containers (debian, ubuntu, alpine, fedora images):**
- `/etc/os-release` is present and accurate
- Reflects the container's distro, not the host
- This is correct behavior for package management

**Scratch images:**
- [No filesystem at all](https://hub.docker.com/_/scratch), no `/etc/os-release`
- Detection returns "unknown"
- Not a problem: scratch images don't need package management

**Distroless images (Google):**
- May or may not have `/etc/os-release`
- Base is typically debian; may need fallback heuristics

### Docker Desktop's LinuxKit VM

When running Docker Desktop on macOS/Windows:
- The host VM is [LinuxKit-based](https://www.mirantis.com/blog/ok-i-give-up-is-docker-now-moby-and-what-is-linuxkit/) (Alpine derivative)
- Container `/etc/os-release` is still container-specific
- No special handling needed

### WSL2

- [WSL distros have valid `/etc/os-release`](https://learn.microsoft.com/en-us/windows/wsl/basic-commands)
- Reflects the installed Linux distro (Ubuntu, Debian, etc.)
- No WSL-specific detection needed
- Package management works normally

### Minimal/Custom Containers

Some minimal containers strip `/etc/os-release`:
```dockerfile
FROM scratch
COPY myapp /app
```

Handling:
- Return `id = "unknown"`, `idLike = []`
- Steps with `when = { distro = [...] }` are skipped
- Fallback `manual` action can provide guidance

---

## 5. Edge Cases Catalog

### Missing `/etc/os-release`

**When it happens:**
- Scratch-based containers
- Very old distros (pre-2012)
- Highly customized/stripped images

**Handling:**
```go
func DetectDistro() (*Distro, error) {
    data, err := os.ReadFile("/etc/os-release")
    if os.IsNotExist(err) {
        return &Distro{ID: "unknown", IDLike: nil}, nil
    }
    // ... parse
}
```

### Unknown Distribution

**When it happens:**
- Niche distros (Void, Solus, Clear Linux)
- Non-Linux systems returning unexpected values
- Custom enterprise distros

**Handling:**
- Return the actual `ID` from the file
- Recipe matching may fail (expected)
- Recipe can use `package_manager` detection as fallback

### NixOS Special Case

NixOS has `/etc/os-release` with `ID=nixos` but:
- No `ID_LIKE` (not derived from another distro)
- Package management is fundamentally different (Nix, not apt/dnf)
- Recipes using `apt_install` or `dnf_install` won't work

**Handling:**
- Detect as `nixos`, no fallback chain
- Recipe must explicitly support NixOS with nix-specific actions
- Or use `require_command` verification with `manual` fallback

### Alpine in Docker

Alpine is common in containers:
- Has valid `/etc/os-release` with `ID=alpine`
- Uses `apk` not `apt` or `dnf`
- No `ID_LIKE` (independent lineage)

**Handling:**
- First-class support with `apk_install` action (if needed)
- Or treat as separate distro requiring explicit recipe support

---

## 6. Architectural Recommendations

### Distro Type Design

**Option A: Simple string**
```go
type Distro string

func DetectDistro() (Distro, []Distro, error)  // id, idLike, err
```
- Minimal overhead
- Requires passing multiple values around

**Option B: Structured type (Recommended)**
```go
type Distro struct {
    ID            string   // "ubuntu"
    IDLike        []string // ["debian"]
    VersionID     string   // "22.04" (may be empty)
    VersionName   string   // "jammy" (may be empty)
    PrettyName    string   // "Ubuntu 22.04.3 LTS"
}

func DetectDistro() (*Distro, error)
func (d *Distro) Matches(target string) bool
func (d *Distro) MatchesAny(targets []string) bool
```

Benefits of structured type:
- Single return value with all parsed fields
- Methods encapsulate matching logic
- Easy to extend with version comparison later
- Can cache parsed result

### Canonical Distro List

**First-class support (tested, documented):**
- `ubuntu` (all LTS versions: 20.04, 22.04, 24.04)
- `debian` (stable: bookworm, bullseye)
- `fedora` (current and previous: 39, 40)
- `arch` (rolling)
- `alpine` (for container use)

**Known but not tested:**
- `rhel`, `centos`, `rocky`, `alma` (RHEL family)
- `opensuse-leap`, `opensuse-tumbleweed`
- `manjaro`, `endeavouros` (Arch derivatives)
- `linuxmint`, `pop`, `elementary` (Ubuntu derivatives)

**Explicitly unsupported (different paradigm):**
- `nixos` - Different package model
- `gentoo` - Source-based

### Matching Algorithm

```go
func (d *Distro) MatchesAny(targets []string) bool {
    for _, target := range targets {
        // Exact ID match first
        if strings.EqualFold(target, d.ID) {
            return true
        }
        // Then check ID_LIKE chain
        for _, like := range d.IDLike {
            if strings.EqualFold(target, like) {
                return true
            }
        }
    }
    return false
}
```

### Caching

Detection should be cached per execution context:
```go
var (
    detectedDistro     *Distro
    detectDistroOnce   sync.Once
    detectDistroErr    error
)

func GetDistro() (*Distro, error) {
    detectDistroOnce.Do(func() {
        detectedDistro, detectDistroErr = parseOSRelease("/etc/os-release")
    })
    return detectedDistro, detectDistroErr
}
```

---

## 7. Design Questions and Answers

### Q1: Should distro be a simple string or a structured type?

**Answer: Structured type.**

A `Distro` struct with `ID`, `IDLike`, `VersionID`, and optional fields provides:
- Clean encapsulation of parsing logic
- Single cached value with all necessary data
- Methods for matching without external functions
- Extensibility for future version constraints

### Q2: How do we handle distro families (debian-based, rhel-based)?

**Answer: Use ID_LIKE as declared, with explicit recipe listing.**

- Don't implement transitive resolution (complexity, maintenance burden)
- `ID_LIKE` provides direct parent declarations
- Recipe authors list all intended distros: `["ubuntu", "debian", "pop", "mint"]`
- Documentation guides authors on common families

### Q3: What's the canonical list of supported distros?

**Answer: Tier-based support model.**

| Tier | Distros | Commitment |
|------|---------|------------|
| 1 (Tested) | ubuntu, debian, fedora, arch, alpine | CI validation, golden files |
| 2 (Known) | rhel, rocky, centos, manjaro, mint | Listed in docs, community-tested |
| 3 (Unknown) | Everything else | Detection works, recipes may not |

---

## 8. Implementation Checklist

Based on this analysis, the implementation should:

1. **Parse `/etc/os-release` only** (no legacy fallbacks)
2. **Return structured `Distro` type** with ID, IDLike, version fields
3. **Cache detection result** per execution context
4. **Match against ID first, then ID_LIKE chain** (no transitivity)
5. **Return "unknown" for missing/unparseable** (not an error)
6. **Skip distro-specific steps gracefully** when distro doesn't match
7. **Provide clear documentation** on distro families for recipe authors
8. **Defer version constraints** to Phase 2 or feature detection

---

## References

- [freedesktop.org os-release specification](https://www.freedesktop.org/software/systemd/man/latest/os-release.html)
- [os-release collection repository](https://github.com/which-distro/os-release)
- [Linux Mint ID_LIKE discussion](https://forums.linuxmint.com/viewtopic.php?t=376664)
- [Docker scratch image documentation](https://hub.docker.com/_/scratch)
- [WSL documentation](https://learn.microsoft.com/en-us/windows/wsl/basic-commands)
- [LinuxKit and Moby overview](https://www.mirantis.com/blog/ok-i-give-up-is-docker-now-moby-and-what-is-linuxkit/)
- [Arch Linux os-release discussion](https://bbs.archlinux.org/viewtopic.php?id=290496)
