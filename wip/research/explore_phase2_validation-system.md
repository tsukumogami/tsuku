# Validation System Research

## Core Architecture

The validation system uses **container-based recipe testing** with two main pathways:

### Binary Validation (`Validate()`)
- **Image**: debian:bookworm-slim
- **Network**: none (fully offline)
- **Filesystem**: Read-only
- **Resources**: Memory=2GB, CPUs=2, PidsMax=100
- **Use case**: Pre-built binaries (GitHub releases, Homebrew bottles)

### Source Build Validation (`ValidateSourceBuild()`)
- **Image**: ubuntu:22.04
- **Network**: host (needs apt-get and ecosystem dependencies)
- **Filesystem**: Writable
- **Resources**: Memory=4GB, CPUs=4, PidsMax=500, Timeout=15min
- **Use case**: Source compilation (configure/make, cargo, go)

## How Builders Choose Validation Method

Currently hardcoded in each builder:

```go
// github_release.go
result, err := b.executor.Validate(ctx, r, assetURL)

// homebrew.go (bottle path)
result, err := b.executor.Validate(ctx, r, "")

// homebrew.go (source path)
validationResult, err := b.executor.ValidateSourceBuild(ctx, r)
```

The builder knows what type of recipe it generated and calls the appropriate method.

## detectRequiredBuildTools() Analysis

Location: `internal/validate/source_build.go:288-333`

Maps recipe actions to apt packages via switch statement:

```go
switch step.Action {
case "configure_make":
    toolsNeeded["autoconf"] = true
    toolsNeeded["automake"] = true
    toolsNeeded["libtool"] = true
    toolsNeeded["pkg-config"] = true
case "cmake_build":
    toolsNeeded["cmake"] = true
    toolsNeeded["ninja-build"] = true
case "cargo_build", "cargo_install":
    toolsNeeded["curl"] = true  // For rustup
case "go_build", "go_install":
    toolsNeeded["curl"] = true  // For Go download
case "apply_patch":
    toolsNeeded["patch"] = true
case "cpan_install":
    toolsNeeded["perl"] = true
    toolsNeeded["cpanminus"] = true
}
```

Always includes `build-essential` as baseline.

**Problem**: This duplicates knowledge that should live with the actions themselves.

## RunOptions Construction

Both validation methods construct RunOptions similarly:

```go
opts := RunOptions{
    Image:   e.image,  // or SourceBuildValidationImage
    Command: []string{"/bin/sh", "/workspace/validate.sh"},
    Network: "none",   // or "host" for source builds
    WorkDir: "/workspace",
    Env: []string{
        "TSUKU_VALIDATION=1",
        "TSUKU_HOME=/workspace/tsuku",
        "HOME=/workspace",
    },
    Limits: limits,
    Mounts: []Mount{
        {Source: workspaceDir, Target: "/workspace", ReadOnly: false},
        {Source: cacheDir, Target: "/workspace/tsuku/cache/downloads", ReadOnly: true},
        {Source: e.tsukuBinary, Target: "/usr/local/bin/tsuku", ReadOnly: true},
    },
}
```

Key differences:
- **Image**: debian:bookworm-slim vs ubuntu:22.04
- **Network**: "none" vs "host"
- **Limits**: Different memory/CPU/timeout values

## Validation Script Generation

### Binary Validation Script (`buildPlanInstallScript`)

```bash
#!/bin/sh
set -e

# Setup TSUKU_HOME
mkdir -p /workspace/tsuku/recipes
mkdir -p /workspace/tsuku/bin
mkdir -p /workspace/tsuku/tools

# Copy recipe to tsuku recipes
cp /workspace/recipe.toml /workspace/tsuku/recipes/<name>.toml

# Run tsuku install with pre-generated plan (offline mode)
tsuku install --plan /workspace/plan.json --force

# Run verify command
export PATH="/workspace/tsuku/tools/current:$PATH"
<verify_command>
```

### Source Build Script (`buildSourceBuildPlanScript`)

```bash
#!/bin/bash
set -e

# Update package lists and install base requirements
apt-get update -qq
apt-get install -qq -y ca-certificates curl wget >/dev/null 2>&1

# Install build tools required by recipe
apt-get install -qq -y <detected_tools> >/dev/null 2>&1

# Setup TSUKU_HOME
mkdir -p /workspace/tsuku/recipes
mkdir -p /workspace/tsuku/bin
mkdir -p /workspace/tsuku/tools

# Copy recipe to tsuku recipes
cp /workspace/recipe.toml /workspace/tsuku/recipes/<name>.toml

# Run tsuku install with pre-generated plan
tsuku install --plan /workspace/plan.json --force
```

## Information NOT in Recipe/Plan But Needed for Validation

### Currently derived externally:
1. **Whether network is needed** - builder knows this, not in plan
2. **Build tools required** - derived by scanning recipe steps in validator
3. **Container image** - hardcoded per validation method
4. **Resource limits** - hardcoded per validation method

### Information available in plan:
- Step actions and parameters
- Checksums and URLs for downloads
- Deterministic flag per step
- Evaluable flag per step

### Missing from plan:
- **Network requirements per step**
- **Build tool requirements per step**
- **Aggregate validation requirements** (image, network, resources)

## Flow Diagram

```
Builder
   |
   v
[Builder knows recipe type]
   |
   +-- Bottle recipe --> Validate() --> Network: none, Debian
   |
   +-- Source recipe --> ValidateSourceBuild() --> Network: host, Ubuntu
                              |
                              v
                    detectRequiredBuildTools()
                              |
                              v
                    [apt-get install <tools>]
```

## Key Insight

The validation method choice is made by the builder based on transient context ("I just generated a source recipe"). This information is NOT persisted in the recipe or plan.

To centralize validation, the plan must surface enough information to derive:
1. Network requirements (any step needs network?)
2. Build tools (what apt packages are needed?)
3. Appropriate container configuration
