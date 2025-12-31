# Package Manager Compatibility Classification

Research findings for SPEC-package-managers.md (P1-C).

## Compatibility with tsuku's Imperative Action Model

tsuku's `*_install` actions follow an imperative model:
1. Optionally add a repository
2. Optionally import GPG keys
3. Run install command
4. Package is installed to system paths

This document classifies each package manager's compatibility with this model.

---

## Classification Summary

| Manager | Compatibility | Model | Root Required | Notes |
|---------|--------------|-------|---------------|-------|
| **apt** | Full | Imperative | Yes | Standard model, well-understood |
| **dnf** | Full | Imperative | Yes | Standard model, compatible with yum syntax |
| **yum** | Full | Imperative | Yes | Legacy, alias for dnf on modern systems |
| **pacman** | Full | Imperative | Yes | Standard model |
| **zypper** | Full | Imperative | Yes | Standard model |
| **apk** | Full | Imperative | Yes | Standard model, minimal syntax |
| **xbps** | Full | Imperative | Yes | Standard model |
| **eopkg** | Full | Imperative | Yes | Standard model |
| **brew** | Full | Imperative | No | User-space, no sudo required |
| **swupd** | Partial | Bundle-based | Yes | Bundles not packages, different granularity |
| **portage** | Partial | Source-based | Yes | Compiles from source, very slow |
| **nix** | None | Declarative | No | Fundamentally different paradigm |
| **guix** | None | Declarative | No | Fundamentally different paradigm |

---

## Full Compatibility

### apt (Debian/Ubuntu)

**Why compatible:**
- Clear imperative commands: `apt install <package>`
- Well-defined repo addition process
- GPG key management follows standard patterns
- Immediate effect on system state

**Integration pattern:**
```go
type AptInstallAction struct {
    Packages     []string
    AddRepos     []AptRepository  // optional
    ImportKeys   []string         // optional
    UpdateFirst  bool             // run apt update before install
}

type AptRepository struct {
    Name     string  // e.g., "docker"
    Content  string  // deb822 or sources.list format
    KeyURL   string  // GPG key URL
    KeyPath  string  // where to save key (/etc/apt/keyrings/)
}
```

**Example recipe action:**
```toml
[actions.apt_install]
packages = ["docker-ce", "docker-ce-cli"]
update_first = true

[[actions.apt_install.repos]]
name = "docker"
key_url = "https://download.docker.com/linux/ubuntu/gpg"
key_path = "/etc/apt/keyrings/docker.gpg"
content = """
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: jammy
Components: stable
Signed-By: /etc/apt/keyrings/docker.gpg
"""
```

---

### dnf/yum (Fedora/RHEL)

**Why compatible:**
- Clear imperative commands: `dnf install <package>`
- Repo files are simple INI format
- GPG key URL can be specified in repo file
- Immediate effect on system state

**Integration pattern:**
```go
type DnfInstallAction struct {
    Packages    []string
    AddRepos    []DnfRepository  // optional
    ImportKeys  []string         // optional, usually in repo file
}

type DnfRepository struct {
    Name       string
    BaseURL    string
    GPGKey     string  // URL to GPG key
    GPGCheck   bool
    Enabled    bool
}
```

**Example recipe action:**
```toml
[actions.dnf_install]
packages = ["docker-ce", "docker-ce-cli"]

[[actions.dnf_install.repos]]
name = "docker-ce-stable"
baseurl = "https://download.docker.com/linux/fedora/$releasever/$basearch/stable"
gpgkey = "https://download.docker.com/linux/fedora/gpg"
gpgcheck = true
enabled = true
```

---

### pacman (Arch)

**Why compatible:**
- Clear imperative commands: `pacman -S <package>`
- Repo configuration in `/etc/pacman.conf`
- GPG key management via `pacman-key`
- Immediate effect on system state

**Integration pattern:**
```go
type PacmanInstallAction struct {
    Packages   []string
    AddRepos   []PacmanRepository  // optional
    ImportKeys []PacmanKey         // optional
    Sync       bool                // run -Sy before install
}

type PacmanRepository struct {
    Name     string
    Server   string
    SigLevel string
}

type PacmanKey struct {
    KeyID       string
    KeyServer   string  // optional, default keyserver used
    LocalSign   bool    // whether to locally sign the key
}
```

**Example recipe action:**
```toml
[actions.pacman_install]
packages = ["docker"]
sync = true

[[actions.pacman_install.repos]]
name = "docker"
server = "https://example.com/docker/$arch"
siglevel = "Required DatabaseOptional"

[[actions.pacman_install.keys]]
keyid = "ABCD1234"
localsign = true
```

---

### zypper (openSUSE)

**Why compatible:**
- Clear imperative commands: `zypper install <package>`
- Built-in repo management: `zypper addrepo`
- GPG key auto-import option
- Immediate effect on system state

**Integration pattern:**
```go
type ZypperInstallAction struct {
    Packages       []string
    AddRepos       []ZypperRepository
    AutoImportKeys bool  // use --gpg-auto-import-keys
}

type ZypperRepository struct {
    Name    string
    URL     string
    Refresh bool
}
```

---

### apk (Alpine)

**Why compatible:**
- Clear imperative commands: `apk add <package>`
- Simple line-based repo configuration
- Key management via `/etc/apk/keys/`
- Immediate effect on system state

**Integration pattern:**
```go
type ApkInstallAction struct {
    Packages   []string
    AddRepos   []string  // repository URLs to add
    ImportKeys []ApkKey  // optional
    Update     bool      // run apk update first
}

type ApkKey struct {
    Name    string  // key filename
    Content string  // public key content
}
```

---

### xbps (Void)

**Why compatible:**
- Clear imperative commands: `xbps-install <package>`
- Configuration in `/etc/xbps.d/`
- Signature verification built-in
- Immediate effect on system state

**Note:** Third-party repos officially unsupported by Void project.

---

### eopkg (Solus)

**Why compatible:**
- Clear imperative commands: `eopkg install <package>`
- XML-based repository configuration
- Immediate effect on system state

**Note:** Small distro, may not warrant dedicated action.

---

### brew (Homebrew)

**Why compatible:**
- Clear imperative commands: `brew install <package>`
- No root required
- Taps for third-party repos: `brew tap <repo>`
- Immediate effect on system state

**Unique advantages for tsuku:**
- User-space installation aligns with tsuku philosophy
- Works across distros
- Could be used as fallback when distro PM unavailable

---

## Partial Compatibility

### swupd (Clear Linux)

**Why partial:**
- Uses bundles instead of packages (different granularity)
- Bundle = collection of packages, not individual package
- `swupd bundle-add <bundle>` installs a capability, not a specific tool

**Challenges:**
- Cannot install specific package versions
- Cannot install individual packages, only bundles
- Bundle contents controlled by Intel

**Possible approach:**
- Map tool names to bundle names
- Accept that granularity is different
- May need fallback to Homebrew for specific tools

---

### portage/emerge (Gentoo)

**Why partial:**
- Imperative command structure: `emerge <package>`
- But compiles from source, taking potentially hours
- USE flags add complexity
- Binary packages available but not default

**Challenges:**
- Install times unpredictable
- Compilation may fail due to missing dependencies
- USE flag configuration affects build
- Not practical for quick tool installation

**Possible approach:**
- Use `--getbinpkg` for binary packages where available
- Warn users about expected compile times
- Consider unsupported due to model mismatch

---

## No Compatibility

### nix

**Why incompatible:**
- Declarative model: "describe what you want, not how to get it"
- Packages installed to `/nix/store/` with cryptographic hashes
- System state derived from configuration, not commands
- Imperative mode exists but goes against nix philosophy

**Fundamental mismatch:**
- nix: "My system should have X, Y, Z packages"
- tsuku: "Install package X now"

**How nix users handle tools:**
- Add to `configuration.nix` (NixOS)
- Add to `home.nix` (home-manager)
- Use `nix-shell` for temporary environments
- Use `nix profile install` (imperative, but discouraged)

**Recommendation:**
- Do not support nix with `*_install` actions
- nix users already have superior package management
- Attempting to integrate would fight the paradigm

---

### guix

**Why incompatible:**
- Same declarative/functional model as nix
- System state derived from Scheme configuration
- Transactional updates, atomic rollbacks
- Different philosophy from imperative package managers

**Same fundamental mismatch as nix.**

**Recommendation:**
- Do not support guix with `*_install` actions
- guix users should use guix directly

---

## Compatibility Matrix Summary

| Manager | tsuku Compatible | Repo Support | GPG Support | Notes |
|---------|-----------------|--------------|-------------|-------|
| apt | Full | deb822/.list + keyrings | Yes | Primary target |
| dnf | Full | .repo files | Yes | Primary target |
| yum | Full | .repo files | Yes | Legacy, alias for dnf |
| pacman | Full | pacman.conf sections | Yes | Primary target |
| zypper | Full | zypper addrepo | Yes | Secondary target |
| apk | Full | /etc/apk/repositories | Yes | Secondary target |
| xbps | Full | /etc/xbps.d/ | Built-in | Niche |
| eopkg | Full | repos.xml | Built-in | Niche |
| brew | Full | brew tap | N/A | Cross-platform fallback |
| swupd | Partial | 3rd-party subcommands | Certs | Different granularity |
| portage | Partial | repos.conf | emerge-webrsync | Source-based, slow |
| nix | None | channels/flakes | Binary cache | Declarative paradigm |
| guix | None | channels.scm | Substitutes | Declarative paradigm |

---

## Decision Framework

### When to use system package manager actions

Use `apt_install`, `dnf_install`, etc. when:
1. Tool is available in distro repos or well-known third-party repo
2. Tool needs system integration (systemd units, etc.)
3. Tool has complex dependencies best handled by distro
4. User explicitly requests distro package

### When to use tsuku's download-based approach

Use `download` + `install_binaries` when:
1. Tool ships as standalone binary
2. Version in distro repos is outdated
3. User wants specific version control
4. Cross-distro consistency needed

### When to recommend external package manager

Recommend direct use of nix/guix when:
1. User is on NixOS or Guix System
2. User wants declarative configuration
3. User needs reproducible environments
