# Platform Detection Assessment: Distro Detection for `when` Clause

## Executive Summary

This assessment evaluates how tsuku should implement Linux distribution detection for the `when` clause, focusing on practical reliability, version constraints, derivative handling, and failure modes.

## Current State

The `WhenClause` in `internal/recipe/types.go` currently supports:
- `Platform []string` - exact tuples like `["linux/amd64"]`
- `OS []string` - operating system matching like `["linux", "darwin"]`
- `PackageManager string` - runtime check for available package managers

The design doc proposes adding `Distro []string` for matching Linux distributions.

## Detection Mechanism: `/etc/os-release`

The `/etc/os-release` file is the standard detection mechanism (defined by systemd/freedesktop.org). Key fields:

```ini
ID=ubuntu                          # Canonical distro identifier
ID_LIKE="debian"                   # Parent/similar distros
VERSION_ID="22.04"                 # Numeric version
VERSION_CODENAME=jammy             # Release codename
PRETTY_NAME="Ubuntu 22.04.3 LTS"   # Human-readable name
```

**Reliability**: This file exists on virtually all modern Linux distributions (systemd adoption is near-universal). Fallback to `/etc/lsb-release` or distro-specific files (e.g., `/etc/debian_version`) covers edge cases but adds complexity with diminishing returns.

**Recommendation**: Parse `/etc/os-release` as the primary mechanism. Return "unknown" if missing rather than attempting legacy fallbacks.

## Version Constraint Syntax

**Question**: Should we support `distro = ["ubuntu>=22.04"]`?

### Analysis

Version constraints add significant complexity:
1. **Non-uniform versioning**: Ubuntu uses `YY.MM`, Fedora uses integers, Debian uses codenames alongside numbers, Arch is rolling (no version).
2. **Semantic ambiguity**: Is `22.04` greater than `21.10`? Naive string comparison fails; we need version-aware parsing.
3. **Maintenance burden**: Version constraints in recipes become stale as distros release updates.

### Recommendation: No Version Constraints (Initially)

For the initial implementation, support only distro ID matching without version constraints:

```toml
when = { distro = ["ubuntu", "debian"] }
```

If version constraints become necessary (e.g., a package only exists in Ubuntu 22.04+), consider:
- Separate `distro_version` field with explicit semantics
- Or defer to `require_command` to check if the needed package/feature exists

This keeps the `when` clause declarative and avoids recipe authors encoding assumptions about distro versioning schemes.

## Derivative Distro Handling

**Question**: How to handle Linux Mint, Pop!_OS, Elementary OS, etc.?

### The `ID_LIKE` Approach

`/etc/os-release` includes `ID_LIKE` which declares compatibility:
- Linux Mint: `ID=linuxmint`, `ID_LIKE=ubuntu`
- Pop!_OS: `ID=pop`, `ID_LIKE="ubuntu debian"`
- Manjaro: `ID=manjaro`, `ID_LIKE=arch`

### Matching Semantics

Two strategies:

**Option A: Explicit matching only**
- `distro = ["ubuntu"]` matches only `ID=ubuntu`
- Recipe authors must list all supported derivatives explicitly
- Pro: Predictable, no surprises
- Con: Recipes need updating for new derivatives

**Option B: Implicit ID_LIKE fallback**
- `distro = ["ubuntu"]` matches `ID=ubuntu` OR any distro with `ubuntu` in `ID_LIKE`
- Pro: Broader compatibility without maintenance
- Con: False positives if derivatives diverge (e.g., package versions differ)

### Recommendation: Option B with Explicit Override

Default to matching `ID` first, then `ID_LIKE` chain. This covers the 90% case where derivatives are compatible with their parent.

For the 10% case where a derivative is incompatible, recipe authors can:
1. Use negative matching: `distro = ["ubuntu", "!linuxmint"]` (requires syntax extension)
2. Or use more specific package manager detection: `when = { package_manager = "apt" }` already handles this

Simpler alternative: Start with Option B (implicit ID_LIKE) and add explicit exclude syntax only if real-world recipes need it.

## WhenClause Extension

Proposed structure:

```go
type WhenClause struct {
    Platform       []string `toml:"platform,omitempty"`
    OS             []string `toml:"os,omitempty"`
    Distro         []string `toml:"distro,omitempty"`         // NEW
    PackageManager string   `toml:"package_manager,omitempty"`
}
```

Validation rules:
- `Distro` and `OS` should not both be set (mutual exclusivity like `Platform`/`OS`)
- If `Distro` is set, implicitly requires `OS=linux` (distro detection only makes sense on Linux)

Matching logic:
```go
func (w *WhenClause) matchesDistro(distroID string, idLike []string) bool {
    for _, d := range w.Distro {
        if d == distroID {
            return true
        }
        for _, like := range idLike {
            if d == like {
                return true
            }
        }
    }
    return false
}
```

## Edge Cases and Failure Modes

### 1. Missing `/etc/os-release`
- **Scenario**: Minimal containers, very old distros
- **Handling**: Return `distroID = "unknown"`, `idLike = []`
- **Recipe impact**: Steps with `when = { distro = [...] }` will be skipped; fallback `manual` action can guide users

### 2. Unknown Distribution
- **Scenario**: Niche distro (NixOS, Void, Alpine)
- **Handling**: Return actual ID; recipe must explicitly support or use `package_manager` detection
- **Note**: NixOS and Alpine are special cases (different package paradigms); may need explicit support

### 3. Container/WSL Environments
- **Scenario**: Running in Docker, WSL, or other virtualized environments
- **Handling**: `/etc/os-release` reflects the container's distro, which is correct behavior
- **WSL specific**: May show Ubuntu even on Windows host; this is fine since package management is Linux-based

### 4. Multiple ID_LIKE Values
- **Scenario**: Pop!_OS has `ID_LIKE="ubuntu debian"`
- **Handling**: Parse as space-separated list, match against any

## Implementation Guidance

### Phase 1: Core Detection
1. Add `osrelease` package under `internal/` with:
   - `Parse(path string) (*OSRelease, error)` - parse `/etc/os-release`
   - `Detect() (*OSRelease, error)` - convenience wrapper
2. `OSRelease` struct with `ID`, `IDLike []string`, `VersionID`, `VersionCodename`

### Phase 2: WhenClause Integration
1. Add `Distro []string` field to `WhenClause`
2. Extend `Matches()` to call `matchesDistro()` when on Linux
3. Cache detection result at recipe execution start (avoid repeated file reads)

### Phase 3: Validation
1. Add CI check that `distro` and `os` are mutually exclusive
2. Validate distro values against known set (warn on unknown, don't fail)

## Summary

For distro detection, I recommend:
1. **Detection**: Parse `/etc/os-release` only, return "unknown" on failure
2. **Version constraints**: Not in initial implementation; use feature detection instead
3. **Derivative handling**: Match both `ID` and `ID_LIKE` chain by default
4. **Failure mode**: Skip distro-specific steps gracefully; rely on verification step to catch missing dependencies

This approach balances practical coverage with implementation simplicity, and aligns with tsuku's philosophy of explicit, composable actions.
