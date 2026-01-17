# System Dependencies in Tsuku Recipes

**Date:** 2026-01-17
**Purpose:** Document how tsuku handles system dependencies for design discussion

## Summary

Tsuku recipes declare system dependencies using **package manager-specific actions**, NOT via a `requires` field. These actions generate instructions rather than executing directly.

## Mechanism

### Package Manager Actions

Each system package manager has a corresponding action:

| Action | Platform | Example |
|--------|----------|---------|
| `apt_install` | Debian/Ubuntu | `packages = ["ca-certificates"]` |
| `dnf_install` | RHEL/Fedora | `packages = ["ca-certificates"]` |
| `pacman_install` | Arch | `packages = ["ca-certificates"]` |
| `apk_install` | Alpine | `packages = ["ca-certificates"]` |
| `brew_install` | macOS | `packages = ["ca-certificates"]` |

### Implicit Constraints

Actions carry built-in platform constraints (from DESIGN-system-dependency-actions.md):

```go
// From internal/actions/apt_actions.go
var debianConstraint = &Constraint{OS: "linux", LinuxFamily: "debian"}

func (a *AptInstallAction) ImplicitConstraint() *Constraint {
    return debianConstraint
}
```

### Example Recipe (ca-certs-system.toml)

```toml
[metadata]
name = "ca-certs-system"

# Debian/Ubuntu
[[steps]]
action = "apt_install"
packages = ["ca-certificates"]

# RHEL/Fedora
[[steps]]
action = "dnf_install"
packages = ["ca-certificates"]

# macOS
[[steps]]
action = "brew_install"
packages = ["ca-certificates"]
```

## Key Behavior

1. **Does NOT execute** system package commands on user's host
2. **Generates instructions** for manual installation
3. **In sandbox mode**: Extracts packages for container builds
4. **No automatic verification** that system packages are installed

## Recipe-Level Dependencies

The `dependencies` field references **other tsuku recipes**, not system packages:

```toml
[metadata]
name = "git-source"
dependencies = ["libcurl", "openssl", "zlib", "expat"]  # tsuku recipe names
```

## Implications for Verification

1. **System deps are not automatically validated** - no check that apt packages exist
2. **Recipe deps are tsuku recipe names** - not library sonames
3. **No mapping exists** from system package → tsuku recipe
4. **`require_command` action** is the only way to verify a command exists post-install:
   ```toml
   [[steps]]
   action = "require_command"
   command = "docker"
   ```

## Open Question

Should tsuku require system dependencies to be expressed as tsuku recipes?

Example: Instead of:
```toml
[[steps]]
action = "apt_install"
packages = ["libssl-dev"]
```

Use:
```toml
[metadata]
dependencies = ["openssl-system"]  # tsuku recipe that wraps apt_install
```

This would enable:
- Unified dependency graph (all deps are tsuku recipes)
- Recursive validation can walk the entire tree
- System dep recipes act as "terminal nodes" that delegate to apt/brew
