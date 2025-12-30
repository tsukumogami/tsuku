# DESIGN: System Dependency Actions

## Status

Draft (Exploratory)

## Context and Problem Statement

The current `require_system` action conflates multiple concerns:

1. **Checking** if a command exists
2. **Installing** system packages if it doesn't
3. **Platform filtering** via embedded platform keys
4. **Post-install configuration** (groups, services)

The result is a polymorphic mess where platform-specific content (`apt`, `brew`) lives inside a generic container (`install_guide` or `packages`), and the action tries to handle every possible installation scenario.

**Current state:**

```toml
[[steps]]
action = "require_system"
command = "docker"

[steps.install_guide]
darwin = "brew install --cask docker"
linux = "See https://docs.docker.com/engine/install/"
```

**Problems identified:**

1. **Platform keys in parameters**: Platform filtering should use `when` clause, not keys inside `install_guide`

2. **Free-form text**: Cannot be machine-executed or validated

3. **Generic container with polymorphic content**: `packages = { apt = [...] }` puts platform-specific content in a generic wrapper

4. **Implicit platform assumptions**: Actions like `require_apt` with implicit `when = { os = ["linux"] }` are wrong - Linux has many distros with different package managers

5. **Rigid ordering**: Baking `repo → packages → group → service` into a single action assumes one workflow fits all

## Design Goals

1. **Composable**: Recipe authors control the sequence of operations
2. **Explicit**: No implicit platform assumptions - use `when` clause
3. **Typed**: Each action has its own well-defined schema
4. **Auditable**: Operations can be statically analyzed (no shell commands)
5. **Extensible**: New package managers can be added as new actions

## Open Questions

### Q1: What's the right granularity for actions?

**Option A: One action per package manager operation**

```toml
[[steps]]
action = "apt_install"
packages = ["docker.io"]
when = { distro = ["ubuntu", "debian"] }

[[steps]]
action = "apt_repo"
url = "https://download.docker.com/linux/ubuntu"
key_url = "..."
key_sha256 = "..."
when = { distro = ["ubuntu", "debian"] }
```

**Option B: One action per package manager (with sub-operations as fields)**

```toml
[[steps]]
action = "apt"
install = ["docker.io"]
when = { distro = ["ubuntu", "debian"] }

[[steps]]
action = "apt"
repo = { url = "...", key_url = "...", key_sha256 = "..." }
install = ["docker-ce"]
when = { distro = ["ubuntu", "debian"] }
```

**Option C: Unified action with typed variants**

```toml
[[steps]]
action = "pkg_install"
manager = "apt"
packages = ["docker.io"]
when = { distro = ["ubuntu", "debian"] }
```

### Q2: How should `when` support distro detection?

Current `when` supports:
- `platform`: exact tuple like `["linux/amd64"]`
- `os`: operating system like `["linux", "darwin"]`

Proposed addition:
- `distro`: Linux distribution like `["ubuntu", "debian", "fedora"]`

**Detection mechanism**: Read `/etc/os-release` on Linux.

**Questions:**
- Should we support version constraints? `distro = ["ubuntu>=22.04"]`
- What about derivative distros? (Linux Mint → Ubuntu → Debian)
- What's the fallback if detection fails?

### Q3: What about the "require" semantics?

The original `require_system` had two behaviors:
1. **Check**: Is the command available?
2. **Fail/Guide**: If not, fail with installation guidance

With composable actions, how do we express "install if needed, then verify"?

**Option A: Separate verify step**

```toml
[[steps]]
action = "apt_install"
packages = ["docker.io"]
when = { distro = ["ubuntu", "debian"] }

[[steps]]
action = "require_command"
command = "docker"
```

**Option B: Conditional execution**

```toml
[[steps]]
action = "apt_install"
packages = ["docker.io"]
unless_command = "docker"
when = { distro = ["ubuntu", "debian"] }
```

**Option C: Package managers are idempotent, just verify at end**

Most package managers handle "already installed" gracefully. Just run install and verify:

```toml
[[steps]]
action = "apt_install"
packages = ["docker.io"]
when = { distro = ["ubuntu", "debian"] }

[[steps]]
action = "brew_cask"
packages = ["docker"]
when = { os = ["darwin"] }

[[steps]]
action = "require_command"
command = "docker"
```

### Q4: How to handle post-install configuration?

Some installations need post-install steps:
- Add user to group (`docker` group for rootless access)
- Enable/start systemd service
- Set environment variables

**Option A: Separate actions for each**

```toml
[[steps]]
action = "apt_install"
packages = ["docker-ce"]
when = { distro = ["ubuntu"] }

[[steps]]
action = "group_add"
group = "docker"
when = { os = ["linux"] }

[[steps]]
action = "service_enable"
service = "docker"
when = { os = ["linux"] }
```

**Option B: Post-install hooks in package action**

```toml
[[steps]]
action = "apt_install"
packages = ["docker-ce"]
post_install = [
  { group_add = "docker" },
  { service_enable = "docker" },
]
when = { distro = ["ubuntu"] }
```

### Q5: What about manual/fallback instructions?

Not everything can be automated. Some cases need human intervention:
- Proprietary software requiring license acceptance
- Platform/distro combinations we don't support
- Complex setups that vary by environment

**Option A: Separate manual action**

```toml
[[steps]]
action = "manual"
text = "Download CUDA from https://developer.nvidia.com/cuda-downloads"
when = { os = ["linux"] }
```

**Option B: Fallback field on other actions**

```toml
[[steps]]
action = "apt_install"
packages = ["nvidia-cuda-toolkit"]
fallback = "For newer CUDA versions, visit https://developer.nvidia.com/cuda-downloads"
when = { distro = ["ubuntu"] }
```

## Proposed Action Vocabulary

Based on the discussion, here's a starting vocabulary:

### Package Installation

| Action | Fields | Description |
|--------|--------|-------------|
| `apt_install` | packages | Install Debian/Ubuntu packages |
| `apt_repo` | url, key_url, key_sha256 | Add APT repository |
| `apt_ppa` | ppa | Add Ubuntu PPA |
| `dnf_install` | packages | Install Fedora/RHEL packages |
| `dnf_repo` | url, key_url, key_sha256 | Add DNF repository |
| `brew_install` | packages, tap? | Install Homebrew formulae |
| `brew_cask` | packages, tap? | Install Homebrew casks |
| `pacman_install` | packages | Install Arch packages |

### System Configuration

| Action | Fields | Description |
|--------|--------|-------------|
| `group_add` | group | Add current user to group |
| `service_enable` | service | Enable systemd service |
| `service_start` | service | Start systemd service |

### Verification

| Action | Fields | Description |
|--------|--------|-------------|
| `require_command` | command, version_flag?, version_regex?, min_version? | Verify command exists |

### Fallback

| Action | Fields | Description |
|--------|--------|-------------|
| `manual` | text | Display instructions for manual installation |

## Example: Docker Installation

Complete example showing composable actions:

```toml
# Ubuntu/Debian with official Docker repo
[[steps]]
action = "apt_repo"
url = "https://download.docker.com/linux/ubuntu"
key_url = "https://download.docker.com/linux/ubuntu/gpg"
key_sha256 = "1500c1f56fa9e26b9b8f42452a553675796ade0807cdce11975eb98170b3a570"
when = { distro = ["ubuntu", "debian"] }

[[steps]]
action = "apt_install"
packages = ["docker-ce", "docker-ce-cli", "containerd.io"]
when = { distro = ["ubuntu", "debian"] }

# Fedora
[[steps]]
action = "dnf_install"
packages = ["docker"]
when = { distro = ["fedora"] }

# macOS
[[steps]]
action = "brew_cask"
packages = ["docker"]
when = { os = ["darwin"] }

# Post-install: add to docker group (Linux only)
[[steps]]
action = "group_add"
group = "docker"
when = { os = ["linux"] }

# Post-install: enable service (Linux only, not macOS Docker Desktop)
[[steps]]
action = "service_enable"
service = "docker"
when = { os = ["linux"] }

# Verify
[[steps]]
action = "require_command"
command = "docker"
```

## Relationship to Original Design

This design doc explores the action vocabulary for system dependencies. Once settled, it feeds back into [DESIGN-structured-install-guide.md](DESIGN-structured-install-guide.md) which addresses:

- Sandbox testing for recipes with system dependencies
- Minimal base container strategy
- User consent flow for privileged operations
- Content-addressing for external resources

The two designs are complementary:
- **This doc**: What actions exist and how they compose
- **Original doc**: How to execute them safely in sandbox and on host

## Next Steps

1. Resolve open questions through discussion
2. Prototype `when` clause distro detection
3. Implement a few core actions (apt_install, brew_cask, require_command)
4. Test with existing recipes (docker, cuda)
5. Merge findings back into structured-install-guide design
