# Dependency Provisioning

## Status

Proposed

## Context and Problem Statement

Tsuku recipes need to declare dependencies on external tools and libraries. These dependencies have different provisioning strategies:

1. **Downloadable**: Tools tsuku downloads and installs (pre-built binaries, Homebrew bottles)
2. **Buildable**: Tools tsuku builds from source (using compilers it provides)
3. **System-required**: Tools that must already exist on the system (Docker, CUDA, kernel modules)

Currently, tsuku has no way to:
- Declare dependencies on things it cannot provide
- Give users clear guidance when system dependencies are missing
- Proactively provide build essentials that users assume come from the "system"

This creates friction:
- Builds fail with cryptic errors when prerequisites are missing
- Users don't know what to install or how
- Recipe authors can't express "this needs Docker installed"
- No consistency across environments (CI vs local, macOS vs Linux)

### Key Insight

**All dependencies should be recipes.** The recipe's *actions* determine how to provision:

- `homebrew_bottle` → download and install pre-built binary
- `configure_make` → build from source
- `require_system` → validate system has it, guide user if missing

This unified model means recipe authors just declare `dependencies = ["docker", "gcc", "zlib"]` - tsuku looks up each recipe and provisions according to its actions.

### What Tsuku CAN Provide

Most traditional "system" dependencies can be provided via Homebrew bottles:
- **Compilers**: gcc, clang, binutils
- **Build tools**: make, cmake, autoconf, meson
- **Libraries**: zlib, openssl, libffi, ncurses

These work when relocated to `$TSUKU_HOME` (validated per-platform).

### What Tsuku CANNOT Provide

Some dependencies fundamentally cannot be relocated or installed without system privileges:

| Category | Examples | Why Tsuku Cannot Provide |
|----------|----------|--------------------------|
| **C Runtime** | libc, libSystem | Everything links against it; cannot be relocated |
| **Kernel Interfaces** | /dev, /proc, system calls | OS-level, not user-space |
| **Privileged Daemons** | Docker, systemd services | Require root, kernel features (cgroups, namespaces) |
| **Kernel Modules** | GPU drivers, filesystem drivers | Must be loaded into kernel |
| **System Services** | D-Bus, launchd agents | Require system-wide integration |
| **Hardware Access** | Direct GPU, USB, network drivers | Require kernel-level permissions |

**Docker example**: Docker requires:
- Kernel features (cgroups, namespaces) - not user-space
- A privileged daemon (dockerd runs as root) - tsuku is unprivileged
- System service integration (systemd/launchd) - outside tsuku's scope
- Cannot be "relocated" - it's fundamentally a system-level component

These dependencies have recipes too - but their recipes use the `require_system` action instead of download/build actions.

### Relationship to Existing Dependency Model

The existing model (see [DESIGN-dependency-pattern.md](DESIGN-dependency-pattern.md)) already handles:
- **Install-time dependencies** (`dependencies`) - needed during `tsuku install`
- **Runtime dependencies** (`runtime_dependencies`) - needed when the tool runs
- **Implicit action dependencies** - actions declare what they need (e.g., `npm_install` needs `nodejs`)

This design extends that model:
- Build actions (`configure_make`, `cmake_build`) declare implicit dependencies on build tools
- Tsuku provides all build tools, not just ecosystem runtimes
- System-required dependencies are recipes with `require_system` action
- No special syntax needed - just declare dependencies normally

### Homebrew Mapping

| Homebrew Field | Tsuku Mapping |
|----------------|---------------|
| `dependencies` | `dependencies` + `runtime_dependencies` |
| `build_dependencies` | `dependencies` only (not in `runtime_dependencies`) |
| `uses_from_macos` | Tsuku provides these too (validated per-platform) |

### Scope

**In scope:**
- **Unified Recipe Model**: All dependencies are recipes with appropriate actions
  - Provisionable tools use `homebrew_bottle`, `configure_make`, etc.
  - System-required tools use new `require_system` action
  - Recipe authors just declare `dependencies = ["docker", "gcc"]`
- **Build Essentials**: Proactively provide compilers, build tools, and libraries
  - Create recipes for baseline dependencies (gcc, make, zlib, etc.)
  - Validate cross-platform functionality via test matrix
- **System-Required Dependencies**: Handle tools tsuku cannot provide
  - New `require_system` action for detection and guidance
  - Clear error messages with installation guidance
  - Optional assisted installation with user consent

**Out of scope:**
- Automatic silent installation of system dependencies
- System dependency version management (beyond minimum version checks)

## Decision Drivers

- **Zero prerequisites**: Users shouldn't need to install anything before tsuku works
- **Cross-platform consistency**: Same recipe works on macOS and Linux
- **Validation over assumption**: Test that relocated tools actually work
- **Fail fast**: If something truly can't be provided, error clearly
- **Reuse existing patterns**: Extend implicit dependency system, don't reinvent

## Considered Options

### Decision 1: How to handle provisionable dependencies (gcc, zlib, etc.)

#### Option 1A: Require System Installation

Require users to pre-install build tools via apt/brew before using tsuku.

**Pros:**
- No additional work for tsuku
- Smaller disk footprint

**Cons:**
- Friction for users (must install prerequisites)
- Cryptic errors when deps missing
- Inconsistent across platforms
- Violates "self-contained" philosophy

#### Option 1B: Tsuku Provides All Build Essentials

Tsuku proactively provides compilers, build tools, and common libraries via Homebrew bottles.

**Pros:**
- Zero prerequisites for users
- Consistent behavior across platforms
- Solves the actual problem (missing deps)

**Cons:**
- Larger disk footprint
- More recipes to maintain
- Bootstrap complexity (need pre-built bottles)

### Decision 2: How to handle unprovisionable dependencies (Docker, etc.)

#### Option 2A: No Declaration (Status Quo)

Don't declare system dependencies; let tools fail with their own error messages.

**Pros:**
- No additional work

**Cons:**
- Cryptic error messages
- Users don't know what to install
- No way for recipes to express requirements

#### Option 2B: Annotation Prefix (`system:docker`)

Add a `system:` prefix to declare dependencies tsuku cannot provide:

```toml
dependencies = ["system:docker", "zlib"]
```

**Pros:**
- Works with existing dependency model

**Cons:**
- Recipe authors must know tsuku internals to classify correctly
- Cognitive burden: is it `docker` or `system:docker`?
- LLMs will frequently misclassify (30-50% error rate per prompt engineer review)
- No validation feedback if author uses wrong classification

#### Option 2C: Unified Recipe Model (All Dependencies Are Recipes)

Every dependency has a recipe. The recipe's actions determine provisioning strategy:

```toml
# gcc.toml - provisionable via Homebrew
[[steps]]
action = "homebrew_bottle"
formula = "gcc"

# docker.toml - system-required
[[steps]]
action = "require_system"
command = "docker"
install_guide = { darwin = "brew install --cask docker", linux = "..." }
```

Recipe authors just declare:
```toml
dependencies = ["docker", "gcc", "zlib"]  # No prefix needed
```

**Pros:**
- No special syntax for recipe authors to learn
- Tsuku auto-classifies based on recipe content
- Adding new system deps = adding a recipe file (not code changes)
- LLM generation simplified (just list dependencies)
- Validation: unknown dependency = recipe doesn't exist
- Enables future assisted installation per-tool

**Cons:**
- Must create recipes for system dependencies (docker.toml, cuda.toml)
- Recipe lookup adds minor overhead

## Decision Outcome

**Chosen: 1B + 2C (Unified Recipe Model)**

### Summary

All dependencies are recipes. Provisionable tools (gcc, zlib) have recipes with `homebrew_bottle` or `configure_make` actions. System-required tools (Docker, CUDA) have recipes with the new `require_system` action. Recipe authors just declare `dependencies = ["docker", "gcc"]` without any special syntax - tsuku looks up each recipe and provisions according to its actions.

### Rationale

**For Decision 1 (provisionable deps):** Option 1B aligns with tsuku's "self-contained" philosophy. If tsuku can provide something, it should.

**For Decision 2 (unprovisionable deps):** Option 2C (unified model) was chosen over 2B (annotation prefix) based on expert panel feedback:

- **UX Expert**: The `system:` prefix creates cognitive burden and leaky abstractions
- **Prompt Engineer**: LLMs will misclassify 30-50% of dependencies without canonical lists
- **Systems Engineer**: Detection logic belongs in recipes, not hardcoded registry

The unified model solves all these problems:
- Recipe authors don't need to classify - just list dependencies
- LLMs generate correct recipes without understanding tsuku internals
- Adding new system deps is adding a recipe file, not modifying code
- Each recipe is self-documenting about its provisioning strategy

## Build Essentials Inventory

### Compilers and Toolchains

| Tool | Purpose | Homebrew Available | Priority |
|------|---------|-------------------|----------|
| gcc | C/C++ compiler | Yes | P0 |
| clang/llvm | C/C++ compiler | Yes | P1 |
| binutils | Linker, assembler | Yes | P0 |

### Build Systems

| Tool | Purpose | Homebrew Available | Priority |
|------|---------|-------------------|----------|
| make | GNU Make | Yes | P0 |
| cmake | CMake build system | Yes (likely exists) | P0 |
| autoconf | Autotools configure | Yes | P1 |
| automake | Autotools Makefile generation | Yes | P1 |
| libtool | Library build helper | Yes | P1 |
| meson | Meson build system | Yes | P2 |
| ninja | Ninja build tool | Yes | P1 |

### Build Utilities

| Tool | Purpose | Homebrew Available | Priority |
|------|---------|-------------------|----------|
| pkg-config | Library discovery | Yes (likely exists) | P0 |
| m4 | Macro processor | Yes | P1 |
| bison | Parser generator | Yes | P2 |
| flex | Lexer generator | Yes | P2 |

### Common Libraries

| Library | Purpose | Homebrew Available | Priority |
|---------|---------|-------------------|----------|
| zlib | Compression | Yes | P0 |
| openssl | TLS/crypto | Yes (likely exists) | P0 |
| libffi | Foreign function interface | Yes | P1 |
| ncurses | Terminal UI | Yes | P1 |
| readline | Line editing | Yes | P1 |
| sqlite | Database | Yes | P2 |
| libxml2 | XML parsing | Yes | P1 |
| libyaml | YAML parsing | Yes | P2 |

## Solution Architecture

### Implicit Dependencies for Build Actions

Build actions declare their baseline requirements in the action dependency registry:

```go
var ActionDependencies = map[string]ActionDeps{
    "configure_make": {
        InstallTime: []string{"make", "gcc", "pkg-config", "autoconf"},
        Runtime:     nil,
    },
    "cmake_build": {
        InstallTime: []string{"cmake", "make", "gcc", "pkg-config"},
        Runtime:     nil,
    },
    "meson_build": {
        InstallTime: []string{"meson", "ninja", "gcc", "pkg-config"},
        Runtime:     nil,
    },
}
```

When a recipe uses `configure_make`, tsuku automatically ensures gcc, make, pkg-config, and autoconf are installed.

### Recipe-Level Library Dependencies

Libraries needed for a specific build are declared in the recipe:

```toml
[metadata]
name = "curl"

[[steps]]
action = "setup_build_env"

[[steps]]
action = "configure_make"
dependencies = ["openssl", "zlib", "nghttp2"]
runtime_dependencies = ["openssl", "zlib", "nghttp2"]
configure_flags = ["--with-openssl", "--with-zlib"]

[[steps]]
action = "install_binaries"
binaries = ["src/curl"]
```

The resolver combines:
1. Action implicit deps (make, gcc, pkg-config, autoconf)
2. Recipe explicit deps (openssl, zlib, nghttp2)

### Build Environment Setup

The `setup_build_env` action configures paths for all dependencies:

```go
func (a *SetupBuildEnvAction) Execute(ctx *ExecutionContext) error {
    var pkgConfigPaths, includePaths, libPaths []string

    for _, dep := range ctx.ResolvedDeps.InstallTime {
        toolPath := ctx.State.GetToolPath(dep)
        pkgConfigPaths = append(pkgConfigPaths, filepath.Join(toolPath, "lib", "pkgconfig"))
        includePaths = append(includePaths, filepath.Join(toolPath, "include"))
        libPaths = append(libPaths, filepath.Join(toolPath, "lib"))
    }

    ctx.Env["PKG_CONFIG_PATH"] = strings.Join(pkgConfigPaths, ":")
    ctx.Env["CPPFLAGS"] = formatFlags("-I", includePaths)
    ctx.Env["LDFLAGS"] = formatFlags("-L", libPaths)
    ctx.Env["CMAKE_PREFIX_PATH"] = strings.Join(toolPaths, ";")
    ctx.Env["CC"] = filepath.Join(ctx.State.GetToolPath("gcc"), "bin", "gcc")
    ctx.Env["CXX"] = filepath.Join(ctx.State.GetToolPath("gcc"), "bin", "g++")

    return nil
}
```

## System-Required Dependencies (Unified Recipe Model)

### The `require_system` Action

A new primitive action that validates system dependencies and provides installation guidance:

```go
type RequireSystemParams struct {
    Command       string            // Command to check (e.g., "docker")
    VersionFlag   string            // Flag to get version (e.g., "--version")
    VersionRegex  string            // Regex to extract version
    MinVersion    string            // Minimum required version (optional)
    InstallGuide  map[string]string // Platform-specific install instructions
    AssistedInstall map[string]string // Commands for assisted install (optional)
}

func (a *RequireSystemAction) Execute(ctx *ExecutionContext) error {
    // 1. Run detection
    found, version := detectCommand(ctx.Params.Command, ctx.Params.VersionFlag)

    if found {
        if ctx.Params.MinVersion != "" && !versionSatisfied(version, ctx.Params.MinVersion) {
            return &VersionMismatchError{...}
        }
        ctx.Log("Found %s version %s", ctx.Params.Command, version)
        return nil  // Success
    }

    // 2. Not found - check assisted install option
    if ctx.Config.AssistedInstall && ctx.Params.AssistedInstall != nil {
        if userConsents("Install " + ctx.Params.Command + "?") {
            return runAssistedInstall(ctx.Params.AssistedInstall)
        }
    }

    // 3. Show guidance and fail
    return &SystemDepMissingError{
        Dependency: ctx.Params.Command,
        Guide:      getGuideForPlatform(ctx.Params.InstallGuide),
    }
}
```

### Example Recipes

**Docker (system-required):**
```toml
# recipes/docker.toml
[metadata]
name = "docker"
description = "Container runtime"

[[steps]]
action = "require_system"
command = "docker"
version_flag = "--version"
version_regex = "Docker version ([0-9.]+)"

[steps.install_guide]
darwin = "brew install --cask docker"
linux.ubuntu = "sudo apt install docker.io && sudo usermod -aG docker $USER"
linux.fedora = "sudo dnf install docker && sudo systemctl enable docker"
fallback = "See https://docs.docker.com/engine/install/"

# Future: assisted installation
# [steps.assisted_install]
# darwin = "brew install --cask docker"
# requires_sudo = false
```

**CUDA (system-required):**
```toml
# recipes/cuda.toml
[metadata]
name = "cuda"
description = "NVIDIA CUDA Toolkit"

[[steps]]
action = "require_system"
command = "nvcc"
version_flag = "--version"
version_regex = "release ([0-9.]+)"

[steps.install_guide]
darwin = "CUDA is not supported on macOS"
linux = "See https://developer.nvidia.com/cuda-downloads"

[steps.min_version]
version = "11.0"
message = "CUDA 11.0+ required"
```

**GCC (provisionable - for comparison):**
```toml
# recipes/gcc.toml
[metadata]
name = "gcc"
description = "GNU Compiler Collection"

[version]
source = "homebrew"
formula = "gcc"

[[steps]]
action = "homebrew_bottle"
formula = "gcc"

[[steps]]
action = "install_binaries"
binaries = ["bin/gcc", "bin/g++", "bin/cpp"]
```

### How Recipe Authors Use This

Recipe authors simply declare dependencies - no special syntax:

```toml
# recipes/my-docker-tool.toml
[metadata]
name = "my-docker-tool"

[[steps]]
action = "docker_build"
dependencies = ["docker"]  # Tsuku looks up docker.toml, sees require_system

[[steps]]
action = "install_binaries"
binaries = ["my-tool"]
```

```toml
# recipes/gpu-app.toml
[metadata]
name = "gpu-app"

[[steps]]
action = "cmake_build"
dependencies = ["cuda", "zlib"]  # cuda = require_system, zlib = homebrew_bottle

[[steps]]
action = "install_binaries"
binaries = ["gpu-app"]
```

### Error Messages

When a system dependency is missing:

```
Error: Missing required dependency: docker

Docker is required but not installed on your system.
Tsuku cannot install Docker because it requires system privileges.

To install Docker:
  macOS: brew install --cask docker
  Ubuntu: sudo apt install docker.io && sudo usermod -aG docker $USER

After installing, run: tsuku install my-docker-tool
```

### Installation Flow (Unified Model)

```
tsuku install my-docker-tool
        │
        ▼
┌─────────────────────────────────────────┐
│ 1. Load recipe (my-docker-tool.toml)    │
│    - Deps: [docker, curl]               │
└─────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────┐
│ 2. For each dependency, load its recipe │
│    - docker.toml → has require_system   │
│    - curl.toml → has homebrew_bottle    │
└─────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────┐
│ 3. Install dependencies in order        │
│    - docker: Execute require_system     │
│      → Check: docker --version          │
│      → Found? Continue                  │
│      → Missing? Show guide, FAIL        │
│    - curl: Execute homebrew_bottle      │
│      → Download and install             │
└─────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────┐
│ 4. Execute my-docker-tool build steps   │
└─────────────────────────────────────────┘
```

### Component Changes

#### Build Essentials (Provisionable)

| Component | Change |
|-----------|--------|
| `internal/actions/dependencies.go` | Add build tools to action implicit deps |
| `internal/actions/setup_build_env.go` | NEW: Action to configure build environment |
| `internal/executor/executor.go` | Ensure implicit deps installed before build |
| `recipes/gcc.toml` | NEW: GCC compiler recipe |
| `recipes/make.toml` | NEW: GNU Make recipe |
| `recipes/zlib.toml` | NEW: zlib library recipe |

#### System-Required Dependencies

| Component | Change |
|-----------|--------|
| `internal/actions/require_system.go` | NEW: `require_system` action implementation |
| `internal/actions/registry.go` | Register `require_system` as primitive action |
| `recipes/docker.toml` | NEW: Docker system-required recipe |
| `recipes/cuda.toml` | NEW: CUDA system-required recipe |
| `recipes/systemd.toml` | NEW: systemd system-required recipe |

## Validation Plan

### Phase 0: Bootstrap Validation (Pre-requisite)

Before creating any recipes, validate that Homebrew bottles exist and can be relocated for all P0 tools on all target platforms.

**Validation script**: `scripts/validate-bottle-availability.sh`
```bash
#!/bin/bash
# For each P0 tool and platform combination:
# 1. Query Homebrew API for bottle availability
# 2. Download bottle to temp directory
# 3. Extract and verify binary can execute from non-standard path
# 4. Check RPATH/install_name for relocation compatibility
```

**Bottle availability matrix** (must pass before Phase 1):

| Tool | Linux x86_64 | Linux arm64 | macOS x86_64 | macOS arm64 |
|------|--------------|-------------|--------------|-------------|
| gcc | [ ] | [ ] | [ ] | [ ] |
| make | [ ] | [ ] | [ ] | [ ] |
| cmake | [ ] | [ ] | [ ] | [ ] |
| pkg-config | [ ] | [ ] | [ ] | [ ] |
| zlib | [ ] | [ ] | [ ] | [ ] |
| openssl | [ ] | [ ] | [ ] | [ ] |

**Fallback strategy**: If a bottle is unavailable for a platform:
1. Check if alternative source exists (e.g., linuxbrew vs homebrew)
2. Consider nix-portable as fallback for that platform
3. Document the gap and defer that platform/tool combination

### Relocation Validation Criteria

For each tool/library, verify these relocation requirements:

**Linux binaries:**
- RPATH must use `$ORIGIN` relative paths or absolute `$TSUKU_HOME` paths
- No hardcoded `/usr/local` or `/home/linuxbrew` paths
- Verify with: `readelf -d <binary> | grep RPATH`

**macOS binaries:**
- install_name must use `@rpath` or `@loader_path`
- No hardcoded `/usr/local` or `/opt/homebrew` paths
- Verify with: `otool -L <binary>`

**Validation tooling**: `scripts/verify-relocation.sh`
```bash
#!/bin/bash
# Usage: verify-relocation.sh <tool-name>
# Checks:
# 1. No hardcoded system paths in binary
# 2. RPATH/install_name uses relocatable references
# 3. Binary executes successfully from $TSUKU_HOME/tools/<name>/
# 4. ldd/otool shows only tsuku-provided or system (libc) deps
```

### Phase 1: Recipe Creation (P0 Tools)

Create recipes for all P0 build essentials:

| Recipe | Source | Exists? | Notes |
|--------|--------|---------|-------|
| gcc | Homebrew bottle | No | Primary C compiler |
| make | Homebrew bottle | No | GNU Make |
| cmake | Homebrew bottle | Verify | May already exist |
| pkg-config | Homebrew bottle | Verify | May already exist |
| zlib | Homebrew bottle | No | Common compression lib |
| openssl | Homebrew bottle | Verify | May already exist |

### Phase 2: Cross-Platform Recipe Validation

Each recipe must be tested on all platforms. Create test matrix:

| Recipe | Linux x86_64 | Linux arm64 | macOS x86_64 | macOS arm64 |
|--------|--------------|-------------|--------------|-------------|
| gcc | [ ] | [ ] | [ ] | [ ] |
| make | [ ] | [ ] | [ ] | [ ] |
| cmake | [ ] | [ ] | [ ] | [ ] |
| pkg-config | [ ] | [ ] | [ ] | [ ] |
| zlib | [ ] | [ ] | [ ] | [ ] |
| openssl | [ ] | [ ] | [ ] | [ ] |

**Test criteria for each cell:**
1. Recipe installs successfully
2. Tool/library is functional (compile test, link test)
3. Works from tsuku's relocated path
4. No system dependencies used (except libc)

### Phase 3: Integration Test Matrix

Build real-world tools using ONLY tsuku-provided dependencies. This validates the entire toolchain works together.

| Test Tool | Build Deps | Linux x86_64 | Linux arm64 | macOS x86_64 | macOS arm64 |
|-----------|------------|--------------|-------------|--------------|-------------|
| sqlite | gcc, make | [ ] | [ ] | [ ] | [ ] |
| zlib | gcc, make | [ ] | [ ] | [ ] | [ ] |
| ncurses | gcc, make | [ ] | [ ] | [ ] | [ ] |
| readline | gcc, make, ncurses | [ ] | [ ] | [ ] | [ ] |
| openssl | gcc, make, zlib | [ ] | [ ] | [ ] | [ ] |
| libxml2 | gcc, make, zlib | [ ] | [ ] | [ ] | [ ] |
| curl | gcc, make, openssl, zlib, nghttp2 | [ ] | [ ] | [ ] | [ ] |
| git | gcc, make, openssl, zlib, curl | [ ] | [ ] | [ ] | [ ] |
| python | gcc, make, openssl, zlib, libffi, readline | [ ] | [ ] | [ ] | [ ] |

**Test criteria for each cell:**
1. Build completes successfully using only tsuku deps
2. Resulting binary executes correctly
3. Works in clean container/VM with NO dev tools pre-installed
4. All linked libraries come from tsuku (verify with ldd/otool)

### Phase 4: CI Integration

Add test matrix to CI pipeline:

```yaml
# .github/workflows/build-essentials-test.yml
name: Build Essentials Validation

on:
  push:
    paths:
      - 'recipes/gcc.toml'
      - 'recipes/make.toml'
      - 'recipes/zlib.toml'
      # ... etc

jobs:
  recipe-validation:
    strategy:
      matrix:
        os: [ubuntu-latest, ubuntu-24.04-arm, macos-latest, macos-14]
        recipe: [gcc, make, cmake, pkg-config, zlib, openssl]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - name: Bootstrap tsuku
        run: go build -o tsuku ./cmd/tsuku
      - name: Install recipe
        run: ./tsuku install ${{ matrix.recipe }}
      - name: Validate functionality
        run: ./scripts/validate-recipe.sh ${{ matrix.recipe }}

  integration-test:
    needs: recipe-validation
    strategy:
      matrix:
        os: [ubuntu-latest, ubuntu-24.04-arm, macos-latest, macos-14]
        test-tool: [sqlite, curl, git, python]
    runs-on: ${{ matrix.os }}
    container:
      image: ${{ matrix.os == 'ubuntu-latest' && 'ubuntu:22.04' || '' }}
      # Use minimal container with NO dev tools
    steps:
      - uses: actions/checkout@v4
      - name: Verify clean environment
        run: |
          ! which gcc  # Should not have system gcc
          ! which make # Should not have system make
      - name: Bootstrap tsuku
        run: # ... bootstrap without system deps
      - name: Build test tool from source
        run: ./tsuku install ${{ matrix.test-tool }}
      - name: Verify tool works
        run: |
          ${{ matrix.test-tool }} --version
      - name: Verify no system deps used
        run: ./scripts/verify-no-system-deps.sh ${{ matrix.test-tool }}
```

## Implementation Approach

### Phase 0: Bootstrap Validation

**Goal**: Prove Homebrew bottles work before building infrastructure.

1. Create `scripts/validate-bottle-availability.sh`
2. Run validation for all P0 tools on all platforms
3. Document any gaps or fallback requirements
4. **Gate**: Do not proceed until all P0 tools pass on at least 2 platforms

### Phase 1: Infrastructure (Dependencies and Environment)

**Goal**: Build the infrastructure that recipes depend on.

1. Update `ActionDependencies` registry with build tool requirements
2. Update resolver to combine action + recipe dependencies
3. Implement `setup_build_env` action
4. Set CC, CXX, PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS
5. Add `--build-deps` flag to `tsuku list` for visibility
6. **Gate**: Unit tests pass for dependency resolution

### Phase 2: P0 Recipes

**Goal**: Create and validate P0 build essential recipes.

1. Create recipes for P0 tools: gcc, make, cmake, pkg-config
2. Create recipes for P0 libraries: zlib, openssl
3. Run relocation validation on each
4. Test each on all 4 platform variants
5. **Gate**: All P0 recipes pass relocation validation

### Phase 3: Integration Testing

**Goal**: Prove the full toolchain works together.

1. Create integration test matrix in CI
2. Build sqlite, zlib, ncurses from source (simpler tools first)
3. Build curl, git with complex dependencies
4. Verify on all platform variants in clean containers
5. **Gate**: All integration tests pass

### Phase 4: P1/P2 Tools

**Goal**: Expand coverage to additional build tools.

1. Add autoconf, automake, libtool recipes
2. Add ncurses, readline, libffi recipes
3. Add meson, ninja for alternative build systems
4. Expand test matrix
5. Build python from source as final validation

### Phase 5: System-Required Action

**Goal**: Implement the `require_system` action for system dependencies.

1. Implement `require_system` action in `internal/actions/require_system.go`
2. Add command detection with version parsing
3. Add platform-specific install guide rendering
4. Add min_version checking support
5. Register action in action registry
6. **Gate**: Unit tests for require_system action

### Phase 6: System-Required Recipes

**Goal**: Create recipes for common system dependencies.

1. Create `recipes/docker.toml` with require_system action
2. Create `recipes/cuda.toml` with require_system action
3. Create `recipes/systemd.toml` (Linux-only via `when` clause)
4. Add `tsuku check-deps <recipe>` command to verify prerequisites
5. Document system-required recipe authoring in guide
6. **Gate**: Integration test installing a docker-dependent recipe

### Phase 7: Assisted Installation (Future)

**Goal**: Enable optional assisted installation with user consent.

1. Add `assisted_install` parameter support to require_system
2. Implement privilege escalation flow with explicit user consent
3. Add `--assist` flag to `tsuku install` command
4. Start with macOS brew commands (no sudo required)
5. **Gate**: User can opt-in to assisted Docker installation on macOS

## Security Considerations

Build tools represent an elevated security concern because compilers and linkers are **trust anchors** - a compromised compiler affects ALL binaries it produces.

### Download Verification

All build essentials come from Homebrew bottles with checksums.

**Current mitigations:**
- SHA256 checksum verification on every download
- Only official Homebrew bottles from known URLs

**Future enhancements (recommended):**
- GPG signature verification where available
- Reproducible build verification for critical tools (gcc, binutils)
- Provenance tracking: log exact bottle URLs and checksums used

### Supply Chain

Build tools have elevated trust requirements:

| Component | Risk Level | Justification |
|-----------|------------|---------------|
| gcc/clang | Critical | Complete control over compiled code |
| binutils (ld, as) | Critical | Controls linking and final binary |
| make/cmake/meson | High | Execute arbitrary build scripts |
| openssl/libffi | High | Runtime security-critical libraries |
| pkg-config | Medium | Can inject compiler/linker flags |

**Mitigations:**
- Only official Homebrew bottles (no third-party sources)
- Version pinning for build essentials (no automatic updates)
- Explicit user consent before updating build tools
- Future: SBOM generation for audit trail

### Execution Isolation

Build tools execute arbitrary code during compilation. While some risk is inherent to source builds, isolation mechanisms can limit damage from compromised tools.

**Current approach:** Build tools run with user permissions in user's environment.

**Recommended enhancements:**
1. **Environment filtering**: Strip sensitive variables before builds
   - Filter: `AWS_*`, `GITHUB_TOKEN`, `SSH_AUTH_SOCK`, `GPG_*`
   - Pass only build-relevant variables (CC, CFLAGS, PATH, etc.)

2. **Filesystem restrictions** (future):
   - No access to `~/.ssh`, `~/.aws`, `~/.config` during builds
   - Restrict writes to build directory and `$TSUKU_HOME`

3. **Network isolation** (future):
   - Block network access during build phase
   - Only allow downloads during explicit download steps

**Not implemented:** Full container/chroot isolation. This would significantly complicate the user experience and is deferred until demand materializes.

### User Data Exposure

Build tools may access environment variables and files during execution.

**Mitigations:**
- Environment filtering (see above)
- Build in isolated directory, not user's project
- Clear documentation of what data build tools can access

### Visibility

Hidden dependencies (build tools not shown in `tsuku list`) reduce user awareness.

**Mitigations:**
- `tsuku list --build-deps` shows all build dependencies
- `tsuku verify gcc` works for build tools
- Installation logs show all dependencies installed
- `tsuku audit-log` (future) shows full installation history

## Consequences

### Positive

#### Unified Recipe Model
- No special syntax for recipe authors - just declare `dependencies = ["docker", "gcc"]`
- LLMs can generate correct recipes without understanding tsuku internals
- Adding new system deps = adding a recipe file (no code changes)
- Each recipe is self-documenting about its provisioning strategy
- Validation is simple: unknown dependency = recipe doesn't exist

#### Build Essentials
- Source builds work without manual prerequisite installation
- Consistent behavior across platforms
- Clear validation that relocated tools actually work

#### System-Required Dependencies
- Clear declaration of unprovisionable requirements (Docker, CUDA, etc.)
- User-friendly error messages with installation guidance
- Platform-specific requirements supported via `when` clause
- Future: assisted installation with user consent

### Negative

- More recipes to create and maintain (gcc, make, docker, cuda, etc.)
- Larger disk footprint (build tools installed even if system has them)
- Initial setup takes longer (must install build tools)
- Bootstrap complexity (requires pre-built Homebrew bottles)
- Elevated security responsibility (compilers are trust anchors)

### Mitigations

- Build tools installed as hidden dependencies (not shown in `tsuku list`)
- Lazy installation (only when a source build is attempted)
- Share build tools across multiple source builds
- Use Homebrew bottles (pre-built) to avoid bootstrap problem
- System-required recipes are simple (just require_system action)

### Neutral

- All dependencies are recipes - unified model
- Shifts complexity from recipe authors to tsuku maintainers
- Aligns with tsuku's "self-contained" philosophy while acknowledging limits
