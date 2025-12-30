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

2. **Accidental dependencies**: The current sandbox base containers (`debian:bookworm-slim`, `ubuntu:22.04`) include various pre-installed packages. A recipe might work due to packages "accidentally" present in the base image, not because the recipe correctly declares its dependencies.

3. **Platform gaps**: Free-form text cannot reliably tell the sandbox executor which packages to install on which platform.

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

#### Option 1C: Structured Field with Generated Text

Replace `install_guide` with structured `packages` field. Generate human-readable instructions on the fly:

```toml
[[steps]]
action = "require_system"
command = "docker"

[steps.packages]
linux = { apt = ["docker.io"] }
darwin = { brew = ["docker"], cask = true }
fallback = { text = "Visit docker.com for installation instructions" }
```

When displaying to users, tsuku generates:
- `{ apt = ["docker.io"] }` → "Run: `sudo apt install docker.io`"
- `{ brew = ["docker"], cask = true }` → "Run: `brew install --cask docker`"
- `{ text = "..." }` → Shows the text as-is

**Pros:**
- No duplication between human and machine formats
- Single source of truth for package specs
- Automation and display use the same data
- `text` field available for cases that can't be generated (URLs, complex instructions)

**Cons:**
- Breaking change to current `install_guide` format
- Requires migration of existing recipes (currently ~3 with `require_system`)
- Generated text may be less polished than hand-written

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

We replace the free-form `install_guide` field with a structured `packages` field. Human-readable instructions are generated from the structured data. The sandbox base container is stripped to minimal (tsuku + glibc only), forcing complete dependency declarations. We support core package managers (apt, brew, dnf) initially with a schema designed for extension.

### Rationale

The combination of structured packages with generated text (1C) and minimal container (2A) provides the best balance:

- **Single source of truth**: Package specs serve both automation (sandbox testing) and display (user instructions). No duplication to maintain.

- **Generated text**: Tsuku generates "Run: `sudo apt install docker.io`" from `{ apt = ["docker.io"] }`. For edge cases (URLs, complex instructions), the `text` field provides an escape hatch.

- **Forced completeness**: The minimal base container catches recipes that accidentally depend on packages in the current base images. When sandbox testing fails, it's clear which packages need to be declared.

- **Small migration scope**: Only ~3 recipes currently use `require_system`. Migration is manageable.

Why not Option 1A or 1B:
- Option 1A is verbose for simple cases
- Option 1B mixes string and table values, confusing authors
- Both require migration anyway; 1C is cleaner

Why Option 2A (minimal container) over 2B (current) or 2C (curated):
- 2A forces explicit dependency declaration, which is the goal
- 2B allows silent dependencies, defeating the purpose
- 2C requires infrastructure investment with diminishing returns

Why Option 3C (extensible core) over 3A or 3B:
- Covers ~95% of actual usage (most recipes target Linux/macOS)
- Simpler initial implementation
- Clear path to add pacman, apk etc. when needed

## Solution Architecture

### Package Specification Schema

The `packages` field replaces the current `install_guide` field. It is a map from platform key to package specification:

```toml
[[steps]]
action = "require_system"
command = "docker"

[steps.packages]
linux = { apt = ["docker.io"] }
darwin = { brew = ["docker"], cask = true }
fallback = { text = "Visit docker.com for installation instructions" }
```

Package specification fields:

| Field | Type | Description |
|-------|------|-------------|
| `apt` | []string | Debian/Ubuntu package names |
| `brew` | []string | Homebrew formula names |
| `cask` | bool | If true, use `brew install --cask` for brew packages |
| `dnf` | []string | Fedora/RHEL package names |
| `text` | string | Custom human-readable text (overrides generated text) |

Platform key resolution follows the same hierarchy as the old `install_guide`:
1. Exact tuple match (e.g., "linux/amd64")
2. OS match (e.g., "linux")
3. "fallback" key

### Human-Readable Text Generation

When displaying installation guidance to users, tsuku generates text from the structured spec:

```go
func GenerateInstallGuide(spec PackageSpec) string {
    if spec.Text != "" {
        return spec.Text  // Use custom text if provided
    }

    if len(spec.Apt) > 0 {
        return fmt.Sprintf("Run: sudo apt install %s", strings.Join(spec.Apt, " "))
    }
    if len(spec.Brew) > 0 {
        cmd := "brew install"
        if spec.Cask {
            cmd = "brew install --cask"
        }
        return fmt.Sprintf("Run: %s %s", cmd, strings.Join(spec.Brew, " "))
    }
    if len(spec.Dnf) > 0 {
        return fmt.Sprintf("Run: sudo dnf install %s", strings.Join(spec.Dnf, " "))
    }

    return "Please install the required system dependency manually."
}
```

### When Generated Text Is Insufficient

Some system dependencies require complex installation procedures (GPG keys, apt repositories, user groups, service configuration). For these cases, use the `text` field to provide proper instructions:

```toml
[steps.packages]
# For sandbox testing, docker.io from Debian repos is sufficient
# For user instructions, link to official Docker docs
linux = { apt = ["docker.io"], text = "See https://docs.docker.com/engine/install/ for your distribution" }
darwin = { brew = ["docker"], cask = true }
```

When both `apt` and `text` are present:
- **Sandbox automation** uses `apt` to install the package
- **User display** shows the `text` field (takes precedence over generated text)

This allows recipes to use simple packages for testing while providing comprehensive instructions for users.

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

### Phase 1: Schema and Parsing

1. Update `require_system` action to accept `packages` field (replaces `install_guide`)
2. Add `GetMapStringSlice` helper for parsing package arrays
3. Add `GenerateInstallGuide()` function to produce human-readable text from specs
4. Add preflight validation for `packages` structure
5. Deprecate `install_guide` with warning (continue to support for backwards compatibility)
6. Update action documentation

### Phase 2: Container Infrastructure

1. Create minimal base container Dockerfile
2. Publish base image to GHCR (tsukumogami/sandbox-base)
3. Add `DeriveContainerSpec` function to sandbox package
4. Implement container image name hashing and caching

### Phase 3: Sandbox Integration

1. Modify sandbox executor to detect `require_system` with `packages`
2. Generate derived Dockerfile from package specs
3. Build or retrieve cached derived container
4. Run sandbox test with derived container
5. Add tests for the new flow

### Phase 4: Recipe Migration

1. Add `packages` specs to docker.toml, cuda.toml, test-tuples.toml
2. Verify sandbox testing works for these recipes
3. Create tracking issue for remaining recipes
4. Update CONTRIBUTING.md with `packages` documentation

## Security Considerations

### Trust Model

This design inherits the trust model from the package managers it relies upon:

| Component | Trust Assumption |
|-----------|-----------------|
| apt repositories | Trusted (signed by Debian/Ubuntu) |
| Homebrew formulae | Trusted (community-reviewed) |
| dnf repositories | Trusted (signed by Fedora/RHEL) |
| Recipe TOML files | Reviewed via PR process |
| Container runtime | Trusted (podman/docker on developer machine or CI runner) |

Users who run `tsuku install` are already trusting the recipes they install. Adding structured `packages` extends this trust to system package declarations. The same review process that validates recipe downloads also validates package declarations.

### Download Verification

Not applicable. This design does not change how artifacts are downloaded or verified. Package installation happens via trusted package managers (apt, brew, dnf) from their official repositories. Package managers handle their own verification (GPG signatures for apt/dnf, SHA256 for Homebrew).

### Execution Isolation

**Container privilege**: The sandbox executor builds containers using `docker build` or `podman build`. These operations require container runtime access but do not require root on the host.

**Package installation**: The derived container installs packages during build, not at sandbox runtime. This means:
- The sandbox user cannot modify the package set at runtime
- Package installation errors fail the build, not the sandbox test
- The sandbox runs with the same isolation as before

**Build caching**: Cached container images are stored in the local container registry. An attacker with access to the registry could poison cached images. Mitigation: use content-addressable image names (hash of package list). The hash is computed from sorted package names, making it predictable but requiring the attacker to know the exact package set.

**Runtime constraints**: Sandbox containers run with:
- No additional capabilities (default container security)
- Resource limits (memory, CPU, process count)
- Ephemeral filesystem (destroyed after test)
- No host network unless explicitly required

### Supply Chain Risks

**Package manager trust**: This design trusts apt, brew, and dnf repositories. An attacker who compromises these repositories could inject malicious packages. This is an existing risk for anyone using these package managers. Tsuku does not add new trust requirements here.

**Image registry trust**: Base images are pulled from trusted registries (GHCR for tsuku images, Docker Hub for debian/ubuntu). Standard container supply chain security practices apply.

**Recipe-declared packages**: Package names come from recipe TOML files. A malicious recipe could declare dangerous packages. Mitigations:
- Recipe PR review catches suspicious package declarations
- Packages are installed in isolated containers, not on the host
- Sandbox containers have no access to host filesystems beyond mounts

**Version pinning** (not implemented): Packages are installed without version pins (e.g., `apt-get install docker.io` not `docker.io=5:20.10.12`). This means container builds may get different versions over time. For sandbox testing this is acceptable - the goal is to verify the recipe works, not to provide reproducible builds. Version pinning could be added in the future if needed.

### User Data Exposure

The sandbox container is isolated and ephemeral. No user data is mounted into the container beyond:
- The pre-generated installation plan (read-only)
- The download cache (read-only)
- The tsuku binary (read-only)

Package installation does not expose user data.

## Consequences

### Positive

- **Complete golden coverage**: All recipes can be sandbox-tested, including those with system dependencies.
- **Explicit dependencies**: The minimal base container forces recipes to declare all required packages.
- **Incremental migration**: Existing recipes work unchanged; structured specs can be added gradually.
- **Caching**: Container image caching prevents rebuilding for common package sets.

### Negative

- **Infrastructure**: Requires building and publishing base container images.
- **Build time**: First sandbox run with a new package set requires container build.
- **Schema maintenance**: Adding new package managers requires schema updates.
- **Migration effort**: Existing recipes with `require_system` need `packages` specs added.

### Mitigations

- **Infrastructure**: GitHub Actions can build and publish base images on release.
- **Build time**: Aggressive caching minimizes rebuild frequency. CI pre-warms common package sets.
- **Schema maintenance**: Core managers (apt, brew, dnf) cover vast majority of cases.
- **Migration**: Track progress via coverage reports. Prioritize recipes used in golden file tests.
