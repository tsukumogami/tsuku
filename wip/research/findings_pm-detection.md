# Findings: Package Manager Detection

## Summary

Package manager detection is best done via **binary presence check** using `type cmd >/dev/null 2>&1`. The `/etc/os-release` file provides supplementary information but should not be the primary detection method.

## Test Methodology

Tested 9 distribution images:
- Debian-based: `debian:bookworm-slim`, `ubuntu:24.04`
- Red Hat-based: `fedora:41`, `almalinux:9-minimal`, `rockylinux:9-minimal`, `amazonlinux:2023`
- SUSE-based: `opensuse/leap:15.6`
- Alpine: `alpine:3.19`
- Arch: `archlinux:base`

## Detection Results

### Binary Detection Matrix

| Image | apt | dnf | yum | microdnf | pacman | apk | zypper |
|-------|-----|-----|-----|----------|--------|-----|--------|
| debian:bookworm-slim | Y | - | - | - | - | - | - |
| ubuntu:24.04 | Y | - | - | - | - | - | - |
| fedora:41 | - | Y | Y* | - | - | - | - |
| almalinux:9-minimal | - | - | - | Y | - | - | - |
| rockylinux:9-minimal | - | - | - | Y | - | - | - |
| amazonlinux:2023 | - | Y | Y* | - | - | - | - |
| opensuse/leap:15.6 | - | - | - | - | - | - | Y |
| alpine:3.19 | - | - | - | - | - | Y | - |
| archlinux:base | - | - | - | - | Y | - | - |

*Note: `yum` on Fedora/Amazon Linux is typically a symlink to `dnf`.

### Key Finding: microdnf on Minimal Images

Enterprise Linux minimal images (AlmaLinux, Rocky Linux) use `microdnf` instead of `dnf`:
- `microdnf` is a lightweight C-based DNF implementation
- Same package format (RPM) and repository structure
- Package names are identical to DNF
- Should be treated as equivalent for tsuku's purposes

### /etc/os-release Analysis

| Image | ID | ID_LIKE |
|-------|-----|---------|
| debian:bookworm-slim | debian | (not set) |
| ubuntu:24.04 | ubuntu | debian |
| fedora:41 | fedora | (not set) |
| almalinux:9-minimal | almalinux | rhel centos fedora |
| rockylinux:9-minimal | rocky | rhel centos fedora |
| amazonlinux:2023 | amzn | fedora |
| opensuse/leap:15.6 | opensuse-leap | suse opensuse |
| alpine:3.19 | alpine | (not set) |
| archlinux:base | arch | (not set) |

**Finding**: `ID_LIKE` is useful for fallback but not reliable for direct package manager detection.

### Alternative Detection Files

| File | Indicates |
|------|-----------|
| `/etc/debian_version` | Debian or derivative (use apt) |
| `/etc/redhat-release` | RHEL or derivative (use dnf/yum/microdnf) |
| `/etc/alpine-release` | Alpine (use apk) |
| `/etc/arch-release` | Arch (use pacman) |
| `/etc/SuSE-release` | SUSE (use zypper) - deprecated |

**Note**: `/etc/debian_version` exists on Ubuntu (set to "trixie/sid" on 24.04), so it indicates apt availability, not specifically Debian.

## Recommended Detection Algorithm

```go
func DetectPackageManager() string {
    // Order matters: prefer primary over secondary

    // apt family (Debian, Ubuntu, etc.)
    if commandExists("apt-get") {
        return "apt"
    }

    // dnf family (Fedora, RHEL 8+, CentOS Stream)
    if commandExists("dnf") {
        return "dnf"
    }

    // microdnf (RHEL minimal, CentOS minimal)
    if commandExists("microdnf") {
        return "microdnf" // Treat separately or as "dnf" depending on needs
    }

    // yum (legacy RHEL/CentOS)
    if commandExists("yum") {
        return "yum"
    }

    // zypper (SUSE family)
    if commandExists("zypper") {
        return "zypper"
    }

    // pacman (Arch family)
    if commandExists("pacman") {
        return "pacman"
    }

    // apk (Alpine)
    if commandExists("apk") {
        return "apk"
    }

    return "unknown"
}

func commandExists(cmd string) bool {
    _, err := exec.LookPath(cmd)
    return err == nil
}
```

## Package Manager Families

For package name mapping, group by equivalent package ecosystems:

| Family | Package Managers | Package Format |
|--------|-----------------|----------------|
| apt | apt, apt-get | .deb |
| dnf | dnf, microdnf, yum | .rpm |
| zypper | zypper | .rpm |
| pacman | pacman, yay, paru | .pkg.tar.* |
| apk | apk | .apk |

**Note**: DNF and Zypper both use RPM packages but have different package names for many system libraries.

## Edge Cases

### 1. Multiple Package Managers Present

Some systems have multiple package managers:
- Ubuntu with Snap: Has both `apt` and `snap`
- Arch with AUR helpers: Has `pacman` plus `yay`/`paru`
- Any system with Homebrew: Has `brew` alongside native PM

**Recommendation**: Prefer native package manager. Snap/Flatpak/Homebrew are supplementary.

### 2. WSL (Windows Subsystem for Linux)

WSL Ubuntu has standard apt detection. Test confirmed identical to native Ubuntu.

### 3. Container Runtime Restrictions

Some containers run with restricted permissions:
- Distroless: No shell, no package manager
- scratch: Nothing at all

**Recommendation**: Detect absence gracefully and provide appropriate error.

### 4. Read-only Root Filesystems

Some systems (NixOS, immutable distros) have read-only roots:
- Package manager may exist but cannot install

**Recommendation**: Attempt a test install or check for specific markers.

## microdnf Considerations

For tsuku, `microdnf` should likely be treated as equivalent to `dnf`:
- Same package names
- Same repository format
- Subset of DNF commands (install, update, remove work)

```go
// Option 1: Treat as separate
if pm == "microdnf" || pm == "dnf" {
    // Use DNF package names
}

// Option 2: Normalize to "dnf"
if commandExists("dnf") || commandExists("microdnf") {
    return "dnf"
}
```

## Recommendations

1. **Use Binary Detection**: `type cmd >/dev/null 2>&1` or Go's `exec.LookPath()`

2. **Check in Priority Order**: apt > dnf > microdnf > yum > zypper > pacman > apk

3. **Normalize Variants**: Treat dnf/microdnf/yum as one family for package names

4. **Fallback to os-release**: Use `ID_LIKE` for family grouping if binary check is ambiguous

5. **Handle Missing PM**: Return clear error with instructions for unsupported systems
