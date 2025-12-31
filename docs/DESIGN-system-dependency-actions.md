# DESIGN: System Dependency Actions

## Status

Proposed

## Upstream Design Reference

This design addresses issue [#722](https://github.com/tsukumogami/tsuku/issues/722), which resolves the "Structured System Dependencies" blocker in [DESIGN-golden-plan-testing.md](DESIGN-golden-plan-testing.md).

**Companion design**: [DESIGN-structured-install-guide.md](DESIGN-structured-install-guide.md) (sandbox container building) depends on this design's action vocabulary.

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

## Scope

This design defines a **machine-readable action vocabulary** for system dependencies. The structured format enables three use cases:

| Use Case | Description | Scope |
|----------|-------------|-------|
| **Documentation Generation** | Generate platform-specific instructions for users | This design |
| **Sandbox Container Building** | Extract dependencies to build minimal test containers | This design |
| **Host Execution** | Guided/automated installation on user's machine | Future design |

**Current scope**: This design focuses on documentation generation and sandbox container building. These features require machine-readable recipes but do NOT execute privileged operations on the user's host.

**Future scope**: Host execution (where tsuku actually runs `apt-get install`, etc. on the user's machine) requires additional design work covering UX, consent flows, and security constraints. The action vocabulary defined here provides the foundation for that future capability.

**Key clarification**: Today, tsuku does not execute system package installations on the host. When a recipe requires system dependencies, tsuku:
1. Detects the user's platform
2. Filters steps to those matching the platform
3. Generates human-readable instructions from the machine-readable specs
4. Displays instructions for the user to follow manually

In sandbox mode, tsuku uses the structured specs to build containers with the required dependencies pre-installed.

## Design Goals

1. **Composable**: Recipe authors control the sequence of operations
2. **Explicit**: No implicit platform assumptions - use `when` clause
3. **Typed**: Each action has its own well-defined schema
4. **Auditable**: Operations can be statically analyzed (no shell commands)
5. **Extensible**: New package managers can be added as new actions

## Decisions

### D1: Action Granularity

**Decision: One action per operation (Option A)**

Each operation is a separate action type: `apt_install`, `apt_repo`, `brew_cask`, `group_add`, etc.

**Rationale:**

- **Consistency**: Each action has exactly one schema, making validation straightforward
- **Learnability**: Naming pattern `<manager>_<operation>` is self-documenting
- **Extensibility**: New package managers are additive; existing actions remain unchanged
- **Error messages**: Precise errors like "apt_install requires 'packages' field"

**Rejected alternatives:**

- Option B (one action per manager with sub-operations): Creates polymorphic schemas where valid fields depend on which operation is intended
- Option C (unified action with manager field): Recreates the original problem of generic containers with platform-specific content

### D2: Linux Family Detection

**Decision: Extend `when` clause with `linux_family` field**

```toml
when = { linux_family = "debian" }
```

**Rationale for `linux_family` over `distro`:**

Research revealed that the meaningful boundary for system dependencies is **package manager**, not individual distro. Ubuntu, Debian, and Linux Mint all use `apt` - distinguishing between them adds noise without benefit. The `linux_family` dimension maps 1:1 to package manager:

| linux_family | Package Manager | Example Distros |
|--------------|-----------------|-----------------|
| debian | apt | Ubuntu, Debian, Linux Mint, Pop!_OS |
| rhel | dnf | Fedora, RHEL, CentOS, Rocky, Alma |
| arch | pacman | Arch, Manjaro, EndeavourOS |
| alpine | apk | Alpine Linux |
| suse | zypper | openSUSE, SLES |

**Detection mechanism:**

Parse `/etc/os-release` on Linux, extracting:
- `ID`: Canonical distro identifier (e.g., "ubuntu", "fedora", "arch")
- `ID_LIKE`: Parent/similar distros (e.g., "debian" for Ubuntu)

**Mapping to family:**

```go
var distroToFamily = map[string]string{
    // Debian family
    "debian": "debian", "ubuntu": "debian", "linuxmint": "debian",
    "pop": "debian", "elementary": "debian", "zorin": "debian",
    // RHEL family
    "fedora": "rhel", "rhel": "rhel", "centos": "rhel",
    "rocky": "rhel", "almalinux": "rhel", "ol": "rhel",
    // Arch family
    "arch": "arch", "manjaro": "arch", "endeavouros": "arch",
    // Alpine
    "alpine": "alpine",
    // SUSE family
    "opensuse": "suse", "opensuse-leap": "suse",
    "opensuse-tumbleweed": "suse", "sles": "suse",
}

func DetectFamily() (string, error) {
    osRelease, err := ParseOSRelease("/etc/os-release")
    if err != nil {
        return "", err
    }

    // Try ID first
    if family, ok := distroToFamily[osRelease.ID]; ok {
        return family, nil
    }

    // Fall back to ID_LIKE chain
    for _, like := range osRelease.IDLike {
        if family, ok := distroToFamily[like]; ok {
            return family, nil
        }
    }

    return "", fmt.Errorf("unknown distro: %s", osRelease.ID)
}
```

**Version constraints: Not in initial implementation.**

Version constraint syntax adds significant complexity due to non-uniform versioning schemes across distros. Defer to feature detection via `require_command` instead.

**Failure mode:**

If `/etc/os-release` is missing or family cannot be determined, steps with `linux_family` conditions are skipped. Fallback `manual` actions can guide users.

**Detection implementation notes:**

- Use `exec.LookPath()` (Go) or `type cmd >/dev/null 2>&1` (shell) for binary detection. The `which` command is not universal (missing on Fedora/Arch base images).
- For RHEL family detection, check for `microdnf` in addition to `dnf`. Minimal RHEL images (AlmaLinux, Rocky Linux) use `microdnf` - a lightweight DNF implementation with identical package names. Treat `microdnf` as `linux_family = "rhel"`.
- Detection order: `dnf` > `microdnf` > `yum` for RHEL family (prefer modern over legacy).

**Validation rules:**

- `linux_family` and `os` are mutually exclusive
- `linux_family` and `platform` are mutually exclusive
- `linux_family` implicitly requires Linux

### D3: Require Semantics

**Decision: Idempotent install + final verify (Option C)**

Package managers handle "already installed" gracefully. Run install actions, then verify with `require_command`:

```toml
# implicit when = { linux_family = "debian" }
[[steps]]
action = "apt_install"
packages = ["docker.io"]

# implicit when = { os = "darwin" }
[[steps]]
action = "brew_cask"
packages = ["docker"]

[[steps]]
action = "require_command"
command = "docker"
```

**Rationale:**

- Simplest mental model: "run installs, then check"
- Package managers are idempotent by design
- `require_command` serves as both assertion and documentation

**Escape hatch:**

For cases where install should be skipped if command exists, add optional `unless_command` field:

```toml
# implicit when = { linux_family = "debian" }
[[steps]]
action = "apt_install"
packages = ["docker.io"]
unless_command = "docker"
```

### D4: Post-Install Configuration

**Decision: Separate actions for each (Option A)**

```toml
# implicit when = { linux_family = "debian" }
[[steps]]
action = "apt_install"
packages = ["docker-ce"]

[[steps]]
action = "group_add"
group = "docker"
when = { os = "linux" }

[[steps]]
action = "service_enable"
service = "docker"
when = { os = "linux" }
```

**Rationale:**

- **Single responsibility**: Each action does one thing
- **Clear errors**: Failures are isolated and easy to diagnose
- **Composability**: Recipe author controls sequence
- **Explicit**: Readers see exactly what will happen

**Rejected alternative:**

Option B (post_install hooks) couples unrelated concerns and bloats the schema.

### D5: Manual/Fallback Instructions

**Decision: Hybrid approach - both `manual` action and `fallback` field**

**`manual` action** for explicit human intervention:

```toml
[[steps]]
action = "manual"
text = "Download CUDA from https://developer.nvidia.com/cuda-downloads"
when = { os = ["darwin"] }
```

**`fallback` field** on install actions for graceful degradation:

```toml
# apt_install has implicit when = { linux_family = "debian" }
[[steps]]
action = "apt_install"
packages = ["nvidia-cuda-toolkit"]
fallback = "For newer CUDA versions, visit https://developer.nvidia.com/cuda-downloads"
```

**Rationale:**

- `manual` expresses "automation not possible"
- `fallback` expresses "automation might fail, here's plan B"
- These are orthogonal concerns that can coexist

### D6: Hardcoded When Clauses for Package Manager Actions

**Decision: Package manager actions have immutable, built-in `when` clauses**

Each `*_install` action carries a hardcoded `linux_family` constraint that cannot be overridden by recipe authors:

| Action | Hardcoded Constraint |
|--------|---------------------|
| `apt_install`, `apt_repo`, `apt_ppa` | `when = { linux_family = "debian" }` |
| `dnf_install`, `dnf_repo` | `when = { linux_family = "rhel" }` |
| `pacman_install` | `when = { linux_family = "arch" }` |
| `apk_install` | `when = { linux_family = "alpine" }` |
| `zypper_install` | `when = { linux_family = "suse" }` |
| `brew_install`, `brew_cask` | `when = { os = "darwin" }` |

**Rationale:**

- **Prevents mistakes**: Cannot accidentally write `apt_install` with `when = { linux_family = "rhel" }`
- **Reduces noise**: Recipe authors don't repeat the same obvious constraint
- **Simplifies validation**: Action type determines valid targets

**Implementation:**

```go
type ActionDefinition struct {
    Name           string
    ImplicitWhen   *WhenClause  // Built-in, cannot be overridden
}

var actionDefinitions = map[string]ActionDefinition{
    "apt_install": {
        Name:         "apt_install",
        ImplicitWhen: &WhenClause{LinuxFamily: "debian"},
    },
    "dnf_install": {
        Name:         "dnf_install",
        ImplicitWhen: &WhenClause{LinuxFamily: "rhel"},
    },
    // ...
}
```

**Recipe authors can still add additional constraints** (e.g., architecture), which are combined with the implicit constraint:

```toml
[[steps]]
action = "apt_install"
packages = ["some-x86-only-package"]
when = { arch = "amd64" }  # Combined with implicit linux_family = "debian"
```

**Note:** This constraint applies only to package manager actions. Other actions (`require_command`, `group_add`, `manual`, etc.) do not have hardcoded when clauses.

## Decision Outcome

**Chosen: D1-A + D2 + D3-C + D4-A + D5-Hybrid + D6**

### Summary

We replace the polymorphic `require_system` with granular typed actions (`apt_install`, `brew_cask`, etc.), using `/etc/os-release` for Linux family detection via `when` clause extension, idempotent installation with final `require_command` verification, separate actions for post-install configuration, a hybrid fallback approach (`manual` action + `fallback` field), and hardcoded when clauses for package manager actions.

### Rationale

These choices work together to create a consistent, auditable system:
- Typed actions (D1) enable static analysis and clear error messages
- Linux family detection (D2) targets package manager ecosystems, not individual distros, reducing plan proliferation
- Idempotent install + verify (D3) leverages package manager behavior with explicit verification
- Separate post-install actions (D4) maintain single-responsibility and clear failure isolation
- Hybrid fallback (D5) covers both "automation not possible" and "automation might fail" scenarios
- Hardcoded when clauses (D6) prevent mistakes and reduce recipe noise for package manager actions

## Action Vocabulary

### Package Installation

| Action | Fields | Description |
|--------|--------|-------------|
| `apt_install` | packages, fallback? | Install Debian/Ubuntu packages |
| `apt_repo` | url, key_url, key_sha256 | Add APT repository with GPG key |
| `apt_ppa` | ppa | Add Ubuntu PPA |
| `dnf_install` | packages, fallback? | Install Fedora/RHEL packages |
| `dnf_repo` | url, key_url, key_sha256 | Add DNF repository |
| `brew_install` | packages, tap?, fallback? | Install Homebrew formulae |
| `brew_cask` | packages, tap?, fallback? | Install Homebrew casks |
| `pacman_install` | packages, fallback? | Install Arch packages |
| `apk_install` | packages, fallback? | Install Alpine packages |
| `zypper_install` | packages, fallback? | Install openSUSE/SLES packages |

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

## Documentation Generation

Each action implements a `Describe()` method that generates human-readable instructions:

```go
type Action interface {
    // Describe returns human-readable instructions for this action
    Describe() string
}

// Example implementations
func (a *AptInstallAction) Describe() string {
    return fmt.Sprintf("Install packages: sudo apt-get install %s",
        strings.Join(a.Packages, " "))
}

func (a *BrewCaskAction) Describe() string {
    return fmt.Sprintf("Install via Homebrew: brew install --cask %s",
        strings.Join(a.Packages, " "))
}

func (a *GroupAddAction) Describe() string {
    return fmt.Sprintf("Add yourself to '%s' group: sudo usermod -aG %s $USER",
        a.Group, a.Group)
}
```

When `tsuku install docker` runs and Docker is not installed, the output looks like:

```
$ tsuku install docker

Docker requires system dependencies that tsuku cannot install directly.

For Ubuntu/Debian:

  1. Add Docker repository:
     curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
     echo "deb [signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list

  2. Install packages:
     sudo apt-get update && sudo apt-get install docker-ce docker-ce-cli containerd.io

  3. Add yourself to docker group:
     sudo usermod -aG docker $USER

  4. Enable Docker service:
     sudo systemctl enable docker

After completing these steps, run: tsuku install docker --verify
```

The structured action vocabulary enables this generation while remaining machine-readable for sandbox container building.

## Sandbox Container Building

In sandbox mode, tsuku extracts package requirements from actions and builds minimal containers. See [DESIGN-structured-install-guide.md - Sandbox Executor Changes](DESIGN-structured-install-guide.md#sandbox-executor-changes) for the `ExtractPackages()` implementation and container building details.

## WhenClause Extension

```go
type WhenClause struct {
    Platform    []string `toml:"platform,omitempty"`
    OS          []string `toml:"os,omitempty"`
    LinuxFamily string   `toml:"linux_family,omitempty"` // NEW
    Arch        string   `toml:"arch,omitempty"`
}
```

**Validation:**

- `LinuxFamily` and `OS` are mutually exclusive
- `LinuxFamily` and `Platform` are mutually exclusive
- If `LinuxFamily` is set, step only runs on Linux

**Detection implementation:**

Create `internal/platform/family.go`:

```go
type OSRelease struct {
    ID              string   // e.g., "ubuntu"
    IDLike          []string // e.g., ["debian"]
    VersionID       string   // e.g., "22.04"
    VersionCodename string   // e.g., "jammy"
}

// DetectFamily returns the linux_family for the current system
func DetectFamily() (string, error) {
    osRelease, err := ParseOSRelease("/etc/os-release")
    if err != nil {
        return "", err
    }
    return MapDistroToFamily(osRelease.ID, osRelease.IDLike)
}

func ParseOSRelease(path string) (*OSRelease, error) {
    // Parse KEY=value format
}
```

## Example: Docker Installation

Complete example showing composable actions:

```toml
# Debian family (Ubuntu, Debian, Mint, etc.) - uses official Docker repo
# Note: apt_* actions have implicit when = { linux_family = "debian" }
[[steps]]
action = "apt_repo"
url = "https://download.docker.com/linux/ubuntu"
key_url = "https://download.docker.com/linux/ubuntu/gpg"
key_sha256 = "1500c1f56fa9e26b9b8f42452a553675796ade0807cdce11975eb98170b3a570"

[[steps]]
action = "apt_install"
packages = ["docker-ce", "docker-ce-cli", "containerd.io"]

# RHEL family (Fedora, RHEL, CentOS, Rocky, etc.)
# Note: dnf_install has implicit when = { linux_family = "rhel" }
[[steps]]
action = "dnf_install"
packages = ["docker"]

# macOS
# Note: brew_cask has implicit when = { os = "darwin" }
[[steps]]
action = "brew_cask"
packages = ["docker"]

# Post-install: add to docker group (Linux only)
[[steps]]
action = "group_add"
group = "docker"
when = { os = "linux" }

# Post-install: enable service (Linux only, not macOS Docker Desktop)
[[steps]]
action = "service_enable"
service = "docker"
when = { os = "linux" }

# Verify
[[steps]]
action = "require_command"
command = "docker"
```

### Additional Examples

#### Simple Package Installation

A minimal example installing a single package:

```toml
# Debian family - implicit when = { linux_family = "debian" }
[[steps]]
action = "apt_install"
packages = ["curl"]

# macOS - implicit when = { os = "darwin" }
[[steps]]
action = "brew_install"
packages = ["curl"]

# Verify
[[steps]]
action = "require_command"
command = "curl"
```

#### Homebrew with Custom Tap

Installing from a third-party tap:

```toml
# implicit when = { os = "darwin" }
[[steps]]
action = "brew_install"
packages = ["some-tool"]
tap = "owner/repo"
```

#### Ubuntu PPA

Adding a PPA and installing packages from it:

```toml
# Both actions have implicit when = { linux_family = "debian" }
# Note: PPAs are Ubuntu-specific but work on Ubuntu derivatives
[[steps]]
action = "apt_ppa"
ppa = "deadsnakes/ppa"

[[steps]]
action = "apt_install"
packages = ["python3.11"]

[[steps]]
action = "require_command"
command = "python3.11"
```

#### Fallback for Graceful Degradation

When automated installation might fail, provide a fallback:

```toml
# implicit when = { linux_family = "debian" }
[[steps]]
action = "apt_install"
packages = ["nvidia-cuda-toolkit"]
fallback = "For newer CUDA versions, visit https://developer.nvidia.com/cuda-downloads"
```

The `fallback` field is shown to users if the installation fails, guiding them to manual alternatives.

## Implementation Approach

Implementation focuses on documentation generation and sandbox container building (current scope).

### Phase 1: Infrastructure

1. Add `linux_family` field to `WhenClause`
2. Implement family detection in `internal/platform/family.go`
3. Implement distro-to-family mapping with ID_LIKE fallback
4. Update `WhenClause.Matches()` for linux_family filtering
5. Add unit tests with `/etc/os-release` fixtures

### Phase 2: Action Vocabulary

1. Define action types with `Describe()` method for documentation generation
2. Implement parameter validation for each action
3. Extract `require_command` from existing `require_system`
4. Implement `manual` action for fallback instructions

Actions at this phase do NOT execute on the host - they provide:
- Parameter validation (preflight checks)
- Human-readable descriptions (documentation generation)
- Structured data (sandbox container building)

### Phase 3: Documentation Generation

1. Implement `Describe()` for all package actions (`apt_install`, `brew_cask`, etc.)
2. Implement `Describe()` for configuration actions (`group_add`, `service_enable`)
3. Update CLI to display platform-filtered instructions when system deps are missing
4. Add `--verify` flag to check if system deps are satisfied after manual installation

### Phase 4: Sandbox Integration

1. Implement `ExtractPackages()` to collect dependencies from filtered plans
2. Integrate with sandbox executor for container building
3. Add sandbox execution capability (actions run inside containers)

See [DESIGN-structured-install-guide.md](DESIGN-structured-install-guide.md) for container building details.

## Security Considerations

**Current scope (documentation generation + sandbox execution):**

- **No privileged host operations**: Tsuku does not execute system package installations on the user's machine
- **Documentation generation**: Only reads recipe files to generate human-readable instructions
- **Sandbox isolation**: Actions execute inside ephemeral containers with no host filesystem access
- **Content-addressing**: External resources (GPG keys, repository URLs) require SHA256 hashes

For security constraints on future host execution, see [Future Work: Host Execution](#host-execution).

## Consequences

### Positive

- **Typed actions enable static analysis**: Every recipe can be audited without execution
- **Consistent platform filtering**: `when` clause with `linux_family` support used everywhere
- **Machine-readable format**: Enables both documentation generation and sandbox container building
- **No shell commands**: Eliminates arbitrary code execution risks
- **Extensible**: New actions follow established patterns

### Negative

- **More verbose than polymorphic `require_system`**: Separate steps per platform
- **Requires Go code changes for new actions**: Higher bar than shell commands (by design)
- **Two-design coordination**: This design + sandbox container building design must stay aligned

### Mitigations

- Verbosity traded for explicit, auditable behavior
- New action barrier is intentional security property
- Clear scope boundaries and cross-references between designs

## Future Work

### Host Execution

The action vocabulary defined here provides the foundation for future host execution, where tsuku could actually run installation commands on the user's machine. This requires a dedicated design covering:

**UX Considerations:**
- Consent flow: How does the user approve operations?
- Progress display: How are multi-step installations shown?
- Error recovery: What happens when one step fails?
- Rollback: Can partial installations be undone?

**Security Constraints:**

When host execution is implemented, these constraints will apply:

| Concern | Constraint |
|---------|------------|
| Group allowlisting | Categorize groups by risk (safe/elevated/dangerous) |
| Repository allowlisting | Maintain list of known-safe repository domains |
| Tiered consent | Different confirmation levels for different risk operations |
| Audit logging | Log all privileged operations with timestamps and outcomes |

**Group Risk Categories (for future reference):**

| Category | Groups | Risk | Consent |
|----------|--------|------|---------|
| Safe | dialout, cdrom, floppy, audio, video | Low | Standard y/n |
| Elevated | docker, libvirt, kvm | Medium | Requires typing "yes" |
| Dangerous | wheel, sudo, root | High | Blocked by default |

This is deferred until the documentation generation and sandbox container building features are complete and validated.

### Composite Shorthand Syntax (`system_dependency`)

The current design is verbose for common cases. A high-level shorthand simplifies the ~95% of packages with consistent names across package managers:

**Common case (same package name everywhere):**

```toml
[system_dependency]
command = "curl"
packages = ["curl", "ca-certificates"]
```

This expands to `apt_install`, `dnf_install`, `pacman_install`, `brew_install`, etc. with the same package list for each.

**Exception handling (different names per family):**

```toml
[system_dependency]
command = "gcc"
packages = ["gcc"]
overrides = { debian = ["build-essential"], rhel = ["@development-tools"], arch = ["base-devel"] }
```

The `overrides` map uses `linux_family` keys (not distro names) to specify family-specific package names.

**macOS-specific handling:**

```toml
[system_dependency]
command = "docker"
packages = ["docker"]
darwin = { cask = ["docker"] }  # Uses brew_cask instead of brew_install
```

This syntax provides the ergonomics of a high-level abstraction while respecting the `linux_family` targeting model. Recipe authors who need fine-grained control (custom repos, post-install hooks) continue using individual steps.

### Additional Package Managers

As needed:
- `apk_install` for Alpine Linux - already in scope (alpine family)
- `zypper_install` for openSUSE - already in scope (suse family)

**Explicitly out of scope:**
- `emerge` for Gentoo - source-based, fundamentally different model
- `nix_install` for NixOS - declarative, fundamentally different model

These systems may use nix-portable as a universal fallback in the future (tsuku already supports Nix).

### Version Constraints

If version-specific package requirements become common:

```toml
when = { linux_family = "debian", version = ">=22.04" }
```

Requires defining version comparison semantics across distro versioning schemes. Deferred until there's demonstrated need.

## Relationship to Original Design

This design doc defines the action vocabulary for system dependencies. It feeds back into [DESIGN-structured-install-guide.md](DESIGN-structured-install-guide.md) which addresses:

- Sandbox testing for recipes with system dependencies
- Minimal base container strategy
- Container building from extracted package requirements
- Content-addressing for external resources

The two designs are complementary:

| Design | Focus | Scope |
|--------|-------|-------|
| **This doc** | Action vocabulary, platform filtering, documentation generation | What actions exist and how they compose |
| **Original doc** | Container building, sandbox execution, caching | How to build and run sandbox containers |

Both designs share the same current scope (documentation generation + sandbox container building) and future scope (host execution).

## References

Agent assessments informing this design:
- `wip/research/system-deps_api-design.md` - API granularity analysis
- `wip/research/system-deps_platform-detection.md` - Distro detection approach
- `wip/research/system-deps_security.md` - Security constraints
- `wip/research/system-deps_authoring-ux.md` - Recipe author experience
- `wip/research/system-deps_implementation.md` - Implementation feasibility
- `wip/research/design-fit_current-behavior.md` - Current require_system behavior analysis
- `wip/research/design-fit_sandbox-executor.md` - Sandbox executor architecture
- `wip/research/design-fit_usecase-alignment.md` - Use case alignment assessment
