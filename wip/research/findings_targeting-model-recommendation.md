# Findings: Targeting Model Recommendation

## Executive Summary

Based on empirical testing across 11 Linux distributions, the recommended targeting model is:

**Model B: `target = (platform, package_manager)`**

The package manager IS the capability boundary. Distro-level targeting adds complexity without benefit.

## Research Summary

### Universal Baseline (All Linux)

| Capability | Universal | Notes |
|------------|-----------|-------|
| /bin/sh (POSIX shell) | YES | Always present (dash/bash/busybox) |
| Coreutils (cat, cp, mv, etc.) | YES | All tested distros |
| tar + gzip | YES | All tested distros |
| $HOME, $PATH | YES | Always defined |
| /tmp writable | YES | Always writable |
| curl or wget | NO | Varies by distro |
| CA certificates | NO | Missing on Debian/Ubuntu |
| bash | NO | Missing on Alpine |
| xz, unzip | NO | Not universal |

### Package Manager Detection

| Package Manager | Detection Method | Distributions |
|-----------------|-----------------|---------------|
| apt | `type apt-get` | Debian, Ubuntu, derivatives |
| dnf | `type dnf` | Fedora, RHEL 8+, CentOS Stream |
| microdnf | `type microdnf` | RHEL/CentOS minimal images |
| yum | `type yum` | Legacy RHEL/CentOS |
| apk | `type apk` | Alpine |
| pacman | `type pacman` | Arch, Manjaro |
| zypper | `type zypper` | openSUSE, SLES |

### Package Name Consistency

Most package names are consistent across PMs. Notable exceptions:

| Canonical | apt | dnf | apk | pacman | zypper |
|-----------|-----|-----|-----|--------|--------|
| xz | xz-utils | xz | xz | xz | xz |
| wget | wget | wget2 | wget | wget | wget |
| python3 | python3 | python3 | python3 | python | python3 |
| openssl-dev | libssl-dev | openssl-devel | openssl-dev | openssl | libopenssl-devel |
| build-tools | build-essential | @development-tools | build-base | base-devel | @devel_basis |

## Model Analysis

### Model A: `target = (platform)` only

```toml
[[steps]]
action = "download"
url = "https://example.com/tool-{version}-linux-amd64.tar.gz"
when = { os = "linux", arch = "amd64" }
```

**Pros**:
- Simplest model
- Works for most recipes (pure binaries)
- No PM complexity

**Cons**:
- Cannot handle recipes requiring system deps
- No way to express "install curl if missing"

**Verdict**: Insufficient for recipes with system dependencies.

### Model B: `target = (platform, package_manager)` (RECOMMENDED)

```toml
[[steps]]
action = "apt_install"
packages = ["curl", "ca-certificates"]
when = { package_manager = "apt" }

[[steps]]
action = "dnf_install"
packages = ["curl"]
when = { package_manager = "dnf" }
```

**Pros**:
- PM is the actual capability boundary
- Package names can differ by PM (handled in mapping)
- "Debian family" IS "systems with apt"
- No need to enumerate distros

**Cons**:
- Need package name mapping table
- Slightly more complex than Model A

**Verdict**: Right level of abstraction. Package manager determines what can be installed.

### Model C: `target = (platform, package_manager, distro)`

```toml
[[steps]]
action = "apt_install"
packages = ["curl"]
when = { package_manager = "apt", distro = "debian" }

[[steps]]
action = "apt_install"
packages = ["curl"]
when = { package_manager = "apt", distro = "ubuntu" }
```

**Pros**:
- Maximum specificity
- Could handle distro-specific quirks

**Cons**:
- Debian and Ubuntu are identical for apt purposes
- Huge maintenance burden (enumerate every distro)
- No practical benefit over Model B

**Verdict**: Over-engineered. Distro adds no useful information beyond PM.

### Model D: Family-based targeting

```toml
[[steps]]
action = "install_dep"
package = "curl"
when = { family = "debian" }
```

**Pros**:
- Higher-level abstraction
- "Family" maps to PM anyway

**Cons**:
- "Family" is just PM with extra steps
- Still need to define families

**Verdict**: Just use PM directly. Family = PM in practice.

## The Insight

> **The package manager IS the commonality boundary.**

We cannot install a package manager on a system that doesn't have one. Package manager presence defines what operations are possible. "Debian family" is really just "systems where apt-get works."

Therefore:

```
target = platform + package_manager
```

Where:
- **platform** = `{os, arch}` (linux-amd64, darwin-arm64, etc.)
- **package_manager** = `{apt, dnf, apk, pacman, zypper, none}`

## Recommended Implementation

### Detection Priority

```go
func DetectPackageManager() string {
    // Check in priority order
    checks := []struct {
        cmd string
        pm  string
    }{
        {"apt-get", "apt"},
        {"dnf", "dnf"},
        {"microdnf", "dnf"},  // Treat as dnf
        {"yum", "yum"},
        {"zypper", "zypper"},
        {"pacman", "pacman"},
        {"apk", "apk"},
    }

    for _, c := range checks {
        if commandExists(c.cmd) {
            return c.pm
        }
    }
    return "none"
}
```

### Recipe Format

```toml
[recipe]
name = "example-tool"
version = "1.2.3"

# Platform targeting (always required)
[[steps]]
action = "download"
url = "https://example.com/tool-linux-amd64.tar.gz"
when = { os = "linux", arch = "amd64" }

# Package manager targeting (for system deps)
[[steps]]
action = "pm_install"
packages = ["libssl-dev"]
when = { package_manager = "apt" }

[[steps]]
action = "pm_install"
packages = ["openssl-devel"]
when = { package_manager = "dnf" }

# When no supported PM
[[steps]]
action = "warn"
message = "Please install OpenSSL development headers manually"
when = { package_manager = "none" }
```

### Package Name Translation

```go
var PackageMap = map[string]map[string]string{
    "openssl-dev": {
        "apt":    "libssl-dev",
        "dnf":    "openssl-devel",
        "apk":    "openssl-dev",
        "pacman": "openssl",
        "zypper": "libopenssl-devel",
    },
    // ... more packages
}

// Recipe can use canonical name
// [[steps]]
// action = "pm_install"
// packages = ["openssl-dev"]  # Translated per PM
```

### Runtime Context

```go
type Context struct {
    OS             string  // "linux", "darwin", "windows"
    Arch           string  // "amd64", "arm64"
    PackageManager string  // "apt", "dnf", "none", etc.
    // NO distro field - it's redundant
}
```

## Edge Cases

### 1. No Package Manager (NixOS, Gentoo)

```toml
[[steps]]
action = "warn"
message = """
This recipe requires system dependencies: openssl-dev
Your system doesn't have a supported package manager.
Please install dependencies manually.
"""
when = { package_manager = "none" }
```

### 2. Bootstrap (Debian/Ubuntu without CA certs)

Handled at tsuku startup, not per-recipe:

```go
func checkBootstrap() error {
    if !canMakeHTTPS() {
        return errors.New(`
CA certificates not found. Please install:
  sudo apt-get update && sudo apt-get install -y ca-certificates
`)
    }
    return nil
}
```

### 3. Alpine (musl libc)

May need separate binary. Handle at platform level:

```toml
[[steps]]
action = "download"
url = "https://example.com/tool-linux-musl-amd64.tar.gz"
when = { os = "linux", arch = "amd64", libc = "musl" }
```

Or add `libc` to platform context if needed.

## Final Recommendation

### Use Model B: `target = (platform, package_manager)`

**Why**:
1. Package manager IS the capability boundary
2. No benefit to distro-level targeting
3. Package names vary by PM, not by distro
4. Debian/Ubuntu are identical for our purposes
5. Clean abstraction that matches reality

### Implementation Checklist

1. [ ] Add `package_manager` detection to tsuku
2. [ ] Implement `when = { package_manager = "..." }` in recipe parser
3. [ ] Create package name mapping table
4. [ ] Add `pm_install` action type
5. [ ] Handle `package_manager = "none"` gracefully
6. [ ] Document supported package managers

### Not Recommended

- Do NOT add distro-level targeting
- Do NOT try to support NixOS/Gentoo package managers
- Do NOT enumerate distros in recipes
