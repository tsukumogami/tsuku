# Findings: Package Name Mapping

## Summary

Package names are largely consistent across package managers, with a few notable exceptions. This mapping enables tsuku to translate system dependency requirements to the appropriate packages for each package manager.

## Test Methodology

For each package manager, tested:
1. Package existence via search/info commands
2. Actual installation to verify packages work

Tested package managers:
- apt (Debian, Ubuntu)
- dnf (Fedora, RHEL 8+, CentOS Stream)
- apk (Alpine)
- pacman (Arch)
- zypper (openSUSE)

## Package Name Mapping Table

### Download Tools

| Tool | apt | dnf | apk | pacman | zypper |
|------|-----|-----|-----|--------|--------|
| curl | `curl` | `curl` | `curl` | `curl` | `curl` |
| wget | `wget` | `wget2` | `wget` | `wget` | `wget` |

**Note**: Fedora 41 uses `wget2` as the default wget package. The package name `wget` is available as `wget1-wget` or `wget2-wget` shims.

### SSL/TLS

| Tool | apt | dnf | apk | pacman | zypper |
|------|-----|-----|-----|--------|--------|
| CA certificates | `ca-certificates` | `ca-certificates` | `ca-certificates` | `ca-certificates` | `ca-certificates` |
| OpenSSL | `openssl` | `openssl` | `openssl` | `openssl` | `openssl` |

### Archive Tools

| Tool | apt | dnf | apk | pacman | zypper |
|------|-----|-----|-----|--------|--------|
| unzip | `unzip` | `unzip` | `unzip` | `unzip` | `unzip` |
| xz | `xz-utils` | `xz` | `xz` | `xz` | `xz` |
| bzip2 | `bzip2` | `bzip2` | `bzip2` | `bzip2` | `bzip2` |
| zstd | `zstd` | `zstd` | `zstd` | `zstd` | `zstd` |

**Note**: apt uses `xz-utils` instead of just `xz`.

### Common Utilities

| Tool | apt | dnf | apk | pacman | zypper |
|------|-----|-----|-----|--------|--------|
| git | `git` | `git` | `git` | `git` | `git` |
| jq | `jq` | `jq` | `jq` | `jq` | `jq` |
| make | `make` | `make` | `make` | `make` | `make` |
| gcc | `gcc` | `gcc` | `gcc` | `gcc` | `gcc` |

### Development Headers

| Library | apt | dnf | apk | pacman | zypper |
|---------|-----|-----|-----|--------|--------|
| OpenSSL dev | `libssl-dev` | `openssl-devel` | `openssl-dev` | `openssl` | `libopenssl-devel` |
| zlib dev | `zlib1g-dev` | `zlib-devel` | `zlib-dev` | `zlib` | `zlib-devel` |

**Note**: Development header naming varies significantly:
- apt: `lib<name>-dev`
- dnf/zypper: `<name>-devel`
- apk: `<name>-dev`
- pacman: Usually just `<name>` (headers included in main package)

### Python

| Tool | apt | dnf | apk | pacman | zypper |
|------|-----|-----|-----|--------|--------|
| Python 3 | `python3` | `python3` | `python3` | `python` | `python3` |
| pip | `python3-pip` | `python3-pip` | `py3-pip` | `python-pip` | `python3-pip` |

**Note**: Arch uses `python` (always Python 3 now, no `python2`).

### Build Essentials (Meta-packages/Groups)

| apt | dnf | apk | pacman | zypper |
|-----|-----|-----|--------|--------|
| `build-essential` | `@development-tools` | `build-base` | `base-devel` | `@devel_basis` (pattern) |

**Note**:
- dnf uses `@` prefix for groups
- zypper uses patterns with `-t pattern` flag
- Alpine and Arch are meta-packages

## Verified Installation Commands

All of these commands were tested and confirmed working:

```bash
# apt (Debian/Ubuntu)
apt-get update && apt-get install -y curl ca-certificates jq

# dnf (Fedora)
dnf install -y jq unzip

# apk (Alpine)
apk add --no-cache curl jq

# pacman (Arch)
pacman -Sy --noconfirm jq unzip

# zypper (openSUSE)
zypper install -y curl jq
```

## Install Command Reference

| PM | Non-interactive Install | Update Cache |
|----|------------------------|--------------|
| apt | `apt-get install -y <pkg>` | `apt-get update` |
| dnf | `dnf install -y <pkg>` | (automatic) |
| apk | `apk add --no-cache <pkg>` | `apk update` |
| pacman | `pacman -S --noconfirm <pkg>` | `pacman -Sy` |
| zypper | `zypper install -y <pkg>` | `zypper refresh` |

## Inconsistencies to Handle

### 1. xz package name (apt)
```go
if pm == "apt" && pkg == "xz" {
    return "xz-utils"
}
```

### 2. wget on Fedora
```go
if pm == "dnf" && pkg == "wget" {
    return "wget2" // or wget2-wget for compatibility shim
}
```

### 3. Python package name (Arch)
```go
if pm == "pacman" && pkg == "python3" {
    return "python"
}
```

### 4. Development headers
Different naming patterns require explicit mapping:
```go
var devHeaderMap = map[string]map[string]string{
    "openssl-dev": {
        "apt":    "libssl-dev",
        "dnf":    "openssl-devel",
        "apk":    "openssl-dev",
        "pacman": "openssl",
        "zypper": "libopenssl-devel",
    },
}
```

## Recommendations

1. **Maintain explicit mapping**: Don't try to auto-translate package names. Maintain an explicit mapping table.

2. **Test before release**: Any new package mapping should be verified with actual container tests.

3. **Handle groups/meta-packages specially**: Build essentials are different across all PMs.

4. **Document common packages**: For recipes requiring system deps, document the canonical name and let tsuku translate.

5. **Consider virtual packages**: apt has virtual packages (e.g., `awk` is provided by `mawk` or `gawk`). Handle accordingly.

## Implementation Suggestion

```go
type PackageMapping struct {
    Apt    string
    Dnf    string
    Apk    string
    Pacman string
    Zypper string
}

var CommonPackages = map[string]PackageMapping{
    "curl":            {"curl", "curl", "curl", "curl", "curl"},
    "wget":            {"wget", "wget2", "wget", "wget", "wget"},
    "ca-certificates": {"ca-certificates", "ca-certificates", "ca-certificates", "ca-certificates", "ca-certificates"},
    "git":             {"git", "git", "git", "git", "git"},
    "jq":              {"jq", "jq", "jq", "jq", "jq"},
    "unzip":           {"unzip", "unzip", "unzip", "unzip", "unzip"},
    "xz":              {"xz-utils", "xz", "xz", "xz", "xz"},
    "python3":         {"python3", "python3", "python3", "python", "python3"},
    "openssl-dev":     {"libssl-dev", "openssl-devel", "openssl-dev", "openssl", "libopenssl-devel"},
    "build-essential": {"build-essential", "@development-tools", "build-base", "base-devel", "@devel_basis"},
}

func (m PackageMapping) ForPM(pm string) string {
    switch pm {
    case "apt":
        return m.Apt
    case "dnf", "microdnf", "yum":
        return m.Dnf
    case "apk":
        return m.Apk
    case "pacman":
        return m.Pacman
    case "zypper":
        return m.Zypper
    default:
        return ""
    }
}
```
