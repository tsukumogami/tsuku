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

### Phase 1: Build Essentials Recipes (High priority)

1. Create recipes for P0 tools: gcc, make, cmake, pkg-config
2. Create recipes for P0 libraries: zlib, openssl
3. Test each on all 4 platform variants
4. Document any platform-specific issues or limitations

### Phase 2: Action Implicit Dependencies

1. Update `ActionDependencies` registry with build tool requirements
2. Update resolver to combine action + recipe dependencies
3. Ensure hidden installation (build tools installed but not shown in `tsuku list`)
4. Handle circular bootstrap (gcc recipe might need gcc to build?)

### Phase 3: Environment Setup

1. Implement `setup_build_env` action
2. Set CC, CXX, PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS
3. Update configure_make, cmake_build to use environment
4. Test with real builds

### Phase 4: Integration Testing

1. Create integration test matrix in CI
2. Build curl, git, sqlite, python from source
3. Verify on all platform variants
4. Ensure clean environment tests (no pre-installed dev tools)

### Phase 5: P1/P2 Tools (Lower priority)

1. Add autoconf, automake, libtool recipes
2. Add ncurses, readline, libffi recipes
3. Expand test matrix
4. Add meson, ninja for alternative build systems

## Security Considerations

### Download Verification

All build essentials come from Homebrew bottles with checksums. Same verification as other tsuku packages.

### Supply Chain

Build tools (gcc, make) have elevated trust - they process source code. Mitigations:
- Only official Homebrew bottles
- Checksum verification on every download
- Consider reproducible builds verification in future

### Execution Isolation

Build tools execute arbitrary code (compiling source). This is inherent to source builds. Users must trust the source being compiled.

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
- Bootstrap complexity (how to build gcc without gcc?)

### Mitigations

- Build tools installed as hidden dependencies (not shown in `tsuku list`)
- Lazy installation (only when a source build is attempted)
- Share build tools across multiple source builds
- Use Homebrew bottles (pre-built) to avoid bootstrap problem

### Neutral

- Removes `system:` annotation concept entirely
- Shifts complexity from recipe authors to tsuku maintainers
- Aligns with tsuku's "self-contained" philosophy
