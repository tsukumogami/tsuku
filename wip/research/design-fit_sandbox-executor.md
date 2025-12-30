# Design Fit Assessment: Sandbox Executor

## Overview

This assessment evaluates how the current sandbox executor architecture in tsuku relates to the proposed design in `docs/DESIGN-structured-install-guide.md`, which envisions slim containers and per-recipe container building.

## Current Sandbox Architecture

### File Locations

| File | Purpose |
|------|---------|
| `internal/sandbox/executor.go` | Main sandbox orchestration logic |
| `internal/sandbox/requirements.go` | Computes container requirements from installation plans |
| `internal/sandbox/executor_test.go` | Unit tests for executor |
| `internal/sandbox/requirements_test.go` | Unit tests for requirements computation |
| `cmd/tsuku/install_sandbox.go` | CLI integration for `tsuku install --sandbox` |

### Core Components

1. **Executor** (`internal/sandbox/executor.go`)
   - Orchestrates container-based sandbox testing
   - Detects available container runtime (Podman/Docker)
   - Writes installation plan as JSON to workspace
   - Generates a shell script to run inside the container
   - Mounts tsuku binary, plan, and download cache into container
   - Runs container with resource limits

2. **SandboxRequirements** (`internal/sandbox/requirements.go`)
   - Computes container configuration from the installation plan
   - Determines network requirements, container image, and resource limits
   - Queries actions via the `NetworkValidator` interface

### Current Base Containers

The sandbox uses two fixed base images (defined in `requirements.go`):

```go
const DefaultSandboxImage = "debian:bookworm-slim"
const SourceBuildSandboxImage = "ubuntu:22.04"
```

**Selection Logic:**
- `debian:bookworm-slim` is used for simple binary installations (offline, no build actions)
- `ubuntu:22.04` is used when:
  - Any action requires network (ecosystem builds like `cargo_build`, `go_build`, `npm_install`)
  - Plan contains build actions (`configure_make`, `cmake_build`, `cargo_build`, `go_build`)

**Why These Images:**
The comment in `requirements.go` explains: "Uses Debian because the tsuku binary is dynamically linked against glibc." These are standard Debian/Ubuntu images that include a large set of pre-installed packages.

### Current Resource Limits

| Mode | Memory | CPUs | PidsMax | Timeout |
|------|--------|------|---------|---------|
| Default (binary) | 2g | 2 | 100 | 2 minutes |
| Source Build | 4g | 4 | 500 | 15 minutes |

### Sandbox Script Generation

The `buildSandboxScript()` function generates a shell script that runs inside the container:

```bash
#!/bin/bash
set -e

# For network builds: install system dependencies
apt-get update -qq
apt-get install -qq -y ca-certificates curl >/dev/null 2>&1

# For build plans: install build-essential
apt-get install -qq -y build-essential >/dev/null 2>&1

# Setup TSUKU_HOME
mkdir -p /workspace/tsuku/recipes
mkdir -p /workspace/tsuku/bin
mkdir -p /workspace/tsuku/tools

# Add bin to PATH
export PATH=/workspace/tsuku/bin:$PATH

# Run tsuku install with pre-generated plan
tsuku install --plan /workspace/plan.json --force
```

**Key observations:**
1. System packages are installed at sandbox runtime via `apt-get`
2. The sandbox uses a "fat container" approach where packages are installed dynamically
3. Package installation happens every sandbox run (not cached in the image)

### How `require_system` Steps Are Handled

**Current implementation in `internal/actions/require_system.go`:**

The `require_system` action:
1. Checks if command exists via `exec.LookPath()`
2. Optionally validates version using `version_flag` and `version_regex`
3. Returns `SystemDepMissingError` with installation guidance if not found

**Critical gap for sandbox:**

Looking at the sandbox executor code, there is **no special handling for `require_system` steps**. The current flow:

1. `ComputeSandboxRequirements()` iterates through plan steps
2. It checks if actions implement `NetworkValidator` to determine network needs
3. `require_system` is not in the `buildActions` map that triggers resource upgrades
4. No code extracts `packages` or `primitives` from `require_system` steps

This means when `tsuku install docker-compose --sandbox` runs (assuming docker-compose depends on docker):
1. The sandbox container starts with `debian:bookworm-slim` or `ubuntu:22.04`
2. Docker is NOT pre-installed in these images
3. The `require_system` action runs `exec.LookPath("docker")` and fails
4. Sandbox test fails with "required system dependency not found: docker"

**Current recipes using `require_system`:**

From `docker.toml`:
```toml
[[steps]]
action = "require_system"
command = "docker"
version_flag = "--version"
version_regex = "Docker version ([0-9.]+)"

[steps.install_guide]
darwin = "brew install --cask docker"
linux = "See https://docs.docker.com/engine/install/"
```

The `install_guide` is **free-form text** that cannot be machine-executed. There is no `packages` or `primitives` field in current recipes.

## Design Document Analysis

### DESIGN-structured-install-guide.md

This design proposes significant changes to enable sandbox testing for recipes with system dependencies:

**Key changes proposed:**

1. **Remove `install_guide` field** - Replace with structured `packages` or `primitives` parameters
2. **Minimal base container** - Strip base container to only tsuku + glibc
3. **Per-recipe container building** - Derive container image from recipe's system requirements
4. **Platform filtering via `when`** - Use step-level `when` clause instead of platform keys in parameters

**Proposed step structure:**
```toml
[[steps]]
action = "require_system"
command = "docker"
packages = { apt = ["docker.io"] }
when = { os = ["linux"] }
```

**Container derivation logic (from design):**
```go
func DeriveContainerSpec(plan *executor.InstallationPlan) (*ContainerSpec, error) {
    spec := &ContainerSpec{
        Base:       MinimalBaseImage,
        Packages:   make(map[string][]string),
    }

    for _, step := range plan.Steps {
        if step.Action != "require_system" {
            continue
        }
        // Extract packages/primitives and add to spec
    }

    return spec, nil
}
```

**Container caching:**
```go
func ContainerImageName(spec *ContainerSpec) string {
    // Generate deterministic image name from package set hash
    hash := sha256.Sum256([]byte(packageList))
    return fmt.Sprintf("tsuku/sandbox-cache:%s", hex.EncodeToString(hash[:8]))
}
```

### Gap Analysis: Current vs. Design

| Aspect | Current State | Design Target |
|--------|---------------|---------------|
| Base container | `debian:bookworm-slim` or `ubuntu:22.04` (fat) | Minimal: tsuku + glibc only |
| System deps in sandbox | Not installed, tests fail | Derived from recipe, pre-installed in container |
| Package specification | `install_guide` (free-form text) | `packages` or `primitives` (structured) |
| Platform filtering | Keys inside `install_guide` | Step-level `when` clause |
| Container building | None (uses stock images) | Build per-recipe containers from base + packages |
| Container caching | N/A | Content-addressed image cache by package hash |
| Sandbox for `require_system` | Cannot test (always fails) | Full sandbox support |

### Does the Design Accurately Describe the Use Case?

**Yes, the design accurately captures the problem and proposes a coherent solution:**

1. **Problem statement is correct**: The design correctly identifies that current `install_guide` is free-form text that cannot be machine-executed, and that sandbox testing fails for recipes with `require_system` steps.

2. **Solution architecture is sound**: The proposed `DeriveContainerSpec()` + container caching pattern would enable sandbox testing for any recipe, including those with system dependencies.

3. **Migration path is realistic**: Since tsuku is pre-GA with few recipes using `require_system` (docker.toml, cuda.toml, test-tuples.toml), the breaking change to remove `install_guide` is manageable.

**Minor observations:**

1. **Container runtime support**: The design mentions building containers via Dockerfile. Current executor uses `podman run` or `docker run`. Building images would require `podman build` or `docker build` capability.

2. **Primitive execution in sandbox**: The design shows primitives like `apt_repo` and `group_add`. In sandbox context these run as root (the container runs as root). This is called out in the security section.

3. **Dependency on DESIGN-system-dependency-actions.md**: The structured install guide design references this other design doc for the primitive vocabulary. Both designs should be implemented together.

## Changes Needed to Enable Slim + Per-Recipe Container Building

### Phase 1: Update `require_system` Action Schema

1. Add `packages` parameter to `require_system` action
2. Add `primitives` parameter (mutually exclusive with `packages`)
3. Update preflight validation
4. Migrate existing recipes (docker.toml, cuda.toml)

### Phase 2: Container Specification Derivation

1. Add `DeriveContainerSpec()` function to `internal/sandbox/`
2. Parse `require_system` steps from plan
3. Extract package requirements by package manager type
4. Handle `primitives` array for complex cases

### Phase 3: Per-Recipe Container Building

1. Add container build capability to sandbox executor
2. Generate Dockerfile from minimal base + package spec
3. Implement container image caching by content hash
4. Integrate with `podman build` / `docker build`

### Phase 4: Minimal Base Container

1. Create minimal base image Dockerfile (tsuku + glibc only)
2. Publish base image to GHCR
3. Update `DefaultSandboxImage` constant

### Phase 5: Update Sandbox Script Generation

1. Remove dynamic `apt-get install` from sandbox script
2. Assume container already has all required packages
3. Script becomes simpler: just setup TSUKU_HOME and run tsuku

## Estimated Effort

| Phase | Effort | Dependencies |
|-------|--------|--------------|
| Phase 1 | Medium | None |
| Phase 2 | Medium | Phase 1 |
| Phase 3 | Large | Phase 2 |
| Phase 4 | Medium | Phase 3 |
| Phase 5 | Small | Phase 4 |

The design is well-thought-out and addresses the core problem. Implementation would be a multi-milestone effort, with Phase 3 (container building) being the most complex addition to the current architecture.

## Summary

**Current state:**
- Sandbox uses "fat containers" (debian:bookworm-slim, ubuntu:22.04)
- System packages installed dynamically at sandbox runtime via apt-get
- `require_system` steps cannot be sandbox-tested (command not found)
- `install_guide` is free-form text, not machine-executable

**Design target:**
- Minimal base containers (tsuku + glibc only)
- Per-recipe containers built from base + declared packages
- Content-addressed container caching by package hash
- Full sandbox support for recipes with system dependencies

**Design accuracy:**
- Correctly identifies the problem
- Proposes coherent solution
- Realistic migration path for pre-GA state
- Depends on companion design (DESIGN-system-dependency-actions.md)

**Key implementation work:**
- Update `require_system` action schema to accept `packages`/`primitives`
- Add container building capability to sandbox executor
- Create and publish minimal base container image
- Implement container image caching
