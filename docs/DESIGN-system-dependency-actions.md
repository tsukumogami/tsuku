# DESIGN: System Dependency Actions

## Status

Proposed

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

### D2: Distro Detection

**Decision: Extend `when` clause with `distro` field, using `/etc/os-release`**

```toml
when = { distro = ["ubuntu", "debian"] }
```

**Detection mechanism:**

Parse `/etc/os-release` on Linux, extracting:
- `ID`: Canonical distro identifier (e.g., "ubuntu", "fedora", "arch")
- `ID_LIKE`: Parent/similar distros (e.g., "debian" for Ubuntu)

**Matching semantics:**

1. Match `ID` first (exact match)
2. Fall back to `ID_LIKE` chain (handles derivatives like Linux Mint, Pop!_OS)

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

**Version constraints: Not in initial implementation.**

Version constraint syntax (e.g., `ubuntu>=22.04`) adds significant complexity due to non-uniform versioning schemes across distros. Defer to feature detection via `require_command` instead.

**Failure mode:**

If `/etc/os-release` is missing or distro is unknown, steps with `distro` conditions are skipped. Fallback `manual` actions can guide users.

**Validation rules:**

- `distro` and `os` are mutually exclusive (like `platform` and `os`)
- `distro` implicitly requires Linux (distro detection only makes sense on Linux)

### D3: Require Semantics

**Decision: Idempotent install + final verify (Option C)**

Package managers handle "already installed" gracefully. Run install actions, then verify with `require_command`:

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

**Rationale:**

- Simplest mental model: "run installs, then check"
- Package managers are idempotent by design
- `require_command` serves as both assertion and documentation

**Escape hatch:**

For cases where install should be skipped if command exists, add optional `unless_command` field:

```toml
[[steps]]
action = "apt_install"
packages = ["docker.io"]
unless_command = "docker"
when = { distro = ["ubuntu", "debian"] }
```

### D4: Post-Install Configuration

**Decision: Separate actions for each (Option A)**

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
[[steps]]
action = "apt_install"
packages = ["nvidia-cuda-toolkit"]
fallback = "For newer CUDA versions, visit https://developer.nvidia.com/cuda-downloads"
when = { distro = ["ubuntu"] }
```

**Rationale:**

- `manual` expresses "automation not possible"
- `fallback` expresses "automation might fail, here's plan B"
- These are orthogonal concerns that can coexist

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

In sandbox mode, tsuku extracts package requirements from actions and builds minimal containers:

```go
// ExtractPackages collects all package requirements from a filtered plan
func ExtractPackages(plan *InstallationPlan) map[string][]string {
    packages := make(map[string][]string)
    for _, step := range plan.Steps {
        switch step.Action {
        case "apt_install":
            packages["apt"] = append(packages["apt"], step.Packages...)
        case "brew_install", "brew_cask":
            packages["brew"] = append(packages["brew"], step.Packages...)
        case "dnf_install":
            packages["dnf"] = append(packages["dnf"], step.Packages...)
        }
    }
    return packages
}
```

This enables building per-recipe containers from a minimal base image. See [DESIGN-structured-install-guide.md](DESIGN-structured-install-guide.md) for container building details.

## WhenClause Extension

```go
type WhenClause struct {
    Platform       []string `toml:"platform,omitempty"`
    OS             []string `toml:"os,omitempty"`
    Distro         []string `toml:"distro,omitempty"`         // NEW
    PackageManager string   `toml:"package_manager,omitempty"`
}
```

**Validation:**

- `Distro` and `OS` are mutually exclusive
- `Distro` and `Platform` are mutually exclusive
- If `Distro` is set, step only runs on Linux

**Detection implementation:**

Create `internal/platform/distro.go`:

```go
type OSRelease struct {
    ID              string   // e.g., "ubuntu"
    IDLike          []string // e.g., ["debian"]
    VersionID       string   // e.g., "22.04"
    VersionCodename string   // e.g., "jammy"
}

func Detect() (*OSRelease, error) {
    return ParseFile("/etc/os-release")
}

func ParseFile(path string) (*OSRelease, error) {
    // Parse KEY=value format
}
```

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

## Implementation Approach

Implementation focuses on documentation generation and sandbox container building (current scope).

### Phase 1: Infrastructure

1. Add `distro` field to `WhenClause`
2. Implement distro detection in `internal/platform/distro.go`
3. Update `WhenClause.Matches()` for distro filtering
4. Add unit tests with `/etc/os-release` fixtures

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

### Composite Shorthand Syntax

The current design is verbose for common cases. A future enhancement could add a high-level syntax:

```toml
[system_dependency]
command = "docker"
apt = ["docker.io"]
brew_cask = ["docker"]
dnf = ["docker"]
```

This would expand internally to the full step sequence. Recipe authors who need fine-grained control would continue using individual steps.

This is deferred to gather real usage patterns before designing the expansion rules.

### Additional Package Managers

As needed:
- `apk_install` for Alpine Linux
- `zypper_install` for openSUSE
- `emerge` for Gentoo
- `nix_install` for NixOS

### Version Constraints

If version-specific package requirements become common:

```toml
when = { distro = ["ubuntu>=22.04"] }
```

Requires defining version comparison semantics across distro versioning schemes.

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
