# Build Essentials Provisioning

## Status

Proposed

## Context and Problem Statement

Source builds require baseline tools and libraries: compilers, build systems, and common libraries. Traditionally, these are assumed to come from the "system" - users must pre-install them via apt, brew, or similar.

This creates friction:
- Users must manually install prerequisites before tsuku can build from source
- Different platforms have different packages available
- Builds fail with cryptic errors when prerequisites are missing
- No consistency across environments (CI vs local, macOS vs Linux)

### Key Insight

**Tsuku can provide most "system" dependencies.** Homebrew bottles exist for gcc, make, zlib, and other build essentials. If tsuku proactively provides these, source builds "just work" without requiring users to pre-install anything.

### What Tsuku Genuinely Cannot Provide

Only truly fundamental OS components:
- **libc / libSystem** - The C runtime library. Everything links against it. Cannot be relocated.
- **Kernel interfaces** - System calls, /dev, /proc
- **Hardware drivers** - GPU, network, etc.

Everything else - compilers, build tools, libraries - can potentially be provided by tsuku.

**This assumption must be validated.** We need recipes and cross-platform tests to prove these tools work when relocated.

### Relationship to Existing Dependency Model

The existing model (see [DESIGN-dependency-pattern.md](DESIGN-dependency-pattern.md)) already handles:
- **Install-time dependencies** (`dependencies`) - needed during `tsuku install`
- **Runtime dependencies** (`runtime_dependencies`) - needed when the tool runs
- **Implicit action dependencies** - actions declare what they need (e.g., `npm_install` needs `nodejs`)

This design extends that model:
- Build actions (`configure_make`, `cmake_build`) declare implicit dependencies on build tools
- Tsuku provides all build tools, not just ecosystem runtimes
- No `system:` annotation needed - if tsuku can provide it, tsuku provides it

### Homebrew Mapping

| Homebrew Field | Tsuku Mapping |
|----------------|---------------|
| `dependencies` | `dependencies` + `runtime_dependencies` |
| `build_dependencies` | `dependencies` only (not in `runtime_dependencies`) |
| `uses_from_macos` | Tsuku provides these too (validated per-platform) |

### Scope

**In scope:**
- Identify all baseline dependencies needed for source builds
- Create recipes for each baseline dependency
- Validate cross-platform functionality via test matrix
- Design auto-provisioning mechanism for build actions
- Integration tests proving source builds work with only tsuku-provided deps

**Out of scope:**
- Truly system-only deps (libc) - these are assumed present
- The `system:` annotation concept - eliminated by this design

## Decision Drivers

- **Zero prerequisites**: Users shouldn't need to install anything before tsuku works
- **Cross-platform consistency**: Same recipe works on macOS and Linux
- **Validation over assumption**: Test that relocated tools actually work
- **Fail fast**: If something truly can't be provided, error clearly
- **Reuse existing patterns**: Extend implicit dependency system, don't reinvent

## Considered Options

### Option A: System Dependency Annotation

Add a `system:` prefix annotation to mark dependencies expected from the system:

```toml
[[steps]]
action = "configure_make"
dependencies = ["system:zlib", "system:openssl"]
```

**Pros:**
- Explicit about what comes from where
- Recipe author decides system vs tsuku
- Works with existing dependency model

**Cons:**
- Ambiguous behavior when system dep is missing
- Platform differences require additional complexity
- Shifts responsibility to recipe authors
- Doesn't solve the underlying problem (missing deps cause build failures)

### Option B: Tsuku Provides All Build Essentials

Tsuku proactively provides compilers, build tools, and common libraries via Homebrew bottles:

```toml
[[steps]]
action = "configure_make"
# Implicit: tsuku ensures gcc, make, pkg-config are installed
dependencies = ["openssl", "zlib"]
# Also provided by tsuku, not system
```

**Pros:**
- Zero prerequisites for users
- Consistent behavior across platforms
- No annotation needed - everything comes from tsuku
- Solves the actual problem (missing deps)

**Cons:**
- Larger disk footprint
- More recipes to maintain
- Bootstrap complexity (need pre-built bottles)
- Elevated security responsibility

### Option C: Hybrid with System Fallback

Prefer system dependencies when available, install via tsuku if missing:

```toml
[[steps]]
action = "configure_make"
dependencies = ["zlib", "openssl"]
# Tsuku checks: system has it? Use system. Missing? Install via tsuku.
```

**Pros:**
- Smaller footprint when system has deps
- Still works when system lacks deps
- Flexible

**Cons:**
- Non-deterministic behavior
- Different binaries depending on environment
- Complex detection logic
- Harder to reproduce builds

## Decision Outcome

**Chosen: Option B (Tsuku Provides All Build Essentials)**

### Summary

Tsuku will provide all build tools (gcc, make, cmake) and common libraries (zlib, openssl) via Homebrew bottles. Build actions declare implicit dependencies on these tools. No `system:` annotation is needed because tsuku provides everything.

### Rationale

The core insight is that tsuku CAN provide most "system" dependencies. Only truly fundamental OS components (libc, kernel interfaces) cannot be relocated. Everything else - compilers, build tools, libraries - can be provided via Homebrew bottles.

Option A (annotation) was rejected because it doesn't solve the problem - if someone lacks zlib, marking it `system:zlib` just makes the build fail with a slightly better error message. Option C (fallback) was rejected because non-deterministic builds are worse than larger disk footprint.

Option B aligns with tsuku's "self-contained" philosophy: users only need tsuku, nothing else. The trade-off is disk space and security responsibility, which are acceptable given the user experience benefits.

**Key assumption requiring validation:** Homebrew bottles are relocatable and work from `$TSUKU_HOME`. The validation plan tests this assumption before implementation.

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

### No `system:` Annotation Needed

Since tsuku provides all build essentials:
- No need to distinguish "system" vs "tsuku" dependencies
- No need for `system:` prefix annotation
- Platform differences only needed when behavior genuinely differs (rare)

### Installation Flow

```
tsuku install curl (source build)
        │
        ▼
┌─────────────────────────────────────────┐
│ 1. Load recipe                          │
│    - Action: configure_make             │
│    - Deps: [openssl, zlib, nghttp2]     │
└─────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────┐
│ 2. Resolve dependencies                 │
│    - Action implicit: [make, gcc,       │
│      pkg-config, autoconf]              │
│    - Recipe explicit: [openssl, zlib,   │
│      nghttp2]                           │
│    - Combined: [make, gcc, pkg-config,  │
│      autoconf, openssl, zlib, nghttp2]  │
└─────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────┐
│ 3. Install all dependencies             │
│    - tsuku install make (if needed)     │
│    - tsuku install gcc (if needed)      │
│    - tsuku install openssl (if needed)  │
│    - ... etc                            │
└─────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────┐
│ 4. Execute build steps                  │
│    - setup_build_env sets CC, CFLAGS... │
│    - configure_make runs with env       │
└─────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────┐
│ 5. Record state                         │
│    - curl installed                     │
│    - Dependencies tracked               │
└─────────────────────────────────────────┘
```

### Component Changes

| Component | Change |
|-----------|--------|
| `internal/actions/dependencies.go` | Add build tools to action implicit deps |
| `internal/actions/setup_build_env.go` | NEW: Action to configure build environment |
| `internal/executor/executor.go` | Ensure implicit deps installed before build |
| `recipes/gcc.toml` | NEW: GCC compiler recipe |
| `recipes/make.toml` | NEW: GNU Make recipe |
| `recipes/zlib.toml` | NEW: zlib library recipe |
| ... | Additional build essential recipes |

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

- Source builds work without manual prerequisite installation
- Consistent behavior across platforms
- Clear validation that relocated tools actually work
- Eliminates "system dependency" complexity entirely
- No `system:` annotation needed

### Negative

- More recipes to create and maintain (gcc, make, etc.)
- Larger disk footprint (build tools installed even if system has them)
- Initial setup takes longer (must install build tools)
- Bootstrap complexity (requires pre-built Homebrew bottles)
- Elevated security responsibility (compilers are trust anchors)

### Mitigations

- Build tools installed as hidden dependencies (not shown in `tsuku list`)
- Lazy installation (only when a source build is attempted)
- Share build tools across multiple source builds
- Use Homebrew bottles (pre-built) to avoid bootstrap problem

### Neutral

- Removes `system:` annotation concept entirely
- Shifts complexity from recipe authors to tsuku maintainers
- Aligns with tsuku's "self-contained" philosophy
