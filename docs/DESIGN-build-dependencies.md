# Build Dependencies

## Status

Proposed

## Context and Problem Statement

Tsuku currently distinguishes between **install-time dependencies** (tools needed during `tsuku install`) and **runtime dependencies** (tools needed when the installed tool runs). This works well for prebuilt binaries and simple source builds, but falls short for complex source builds that require additional build tooling.

When building software from source using Homebrew formulas, builds frequently fail because required build tools (cmake, pkg-config, autoconf) are not available, even though tsuku has the capability to install them. The current system treats all dependencies uniformly, missing the distinction between:

1. **Build dependencies** - Tools needed only during compilation (cmake, pkg-config, meson)
2. **Runtime dependencies** - Libraries/tools needed when running the built software
3. **System dependencies** - OS-provided libraries that tsuku cannot install (libc, system SSL)

Additionally, build tools need specific environment setup (PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS) to find headers and libraries from tsuku-managed dependencies. Without this, configure scripts and build systems cannot locate dependencies even when installed.

### Scope

**In scope:**
- Recipe-level representation of build vs runtime vs system dependencies
- Environment setup for build dependencies (PKG_CONFIG_PATH, etc.)
- Automatic installation of build dependencies before source builds
- Clear separation so build deps don't pollute runtime dependency tracking
- Representation of system dependencies that users must provide

**Out of scope:**
- Automatic detection of system dependencies (users must specify them)
- Cross-compilation support
- Conditional/optional dependency features (future work)
- Platform-specific dependency variations beyond existing `[steps.when]` support

## Decision Drivers

- **Recipe portability**: Dependencies should be expressed at the recipe level, not tied to Homebrew
- **Explicit over implicit**: Users should understand what dependencies a build requires
- **Fail-fast for system deps**: Builds should fail early with clear messages if system dependencies are missing
- **Minimal runtime footprint**: Build-only tools shouldn't be tracked as runtime dependencies
- **Existing patterns**: Leverage current install-time/runtime dependency infrastructure where possible

## Considered Options

### Decision 1: Recipe Representation

How should build and system dependencies be represented in recipes?

#### Option 1A: Extend Existing Fields

Add new fields alongside existing `dependencies` and `runtime_dependencies`:

```toml
[metadata]
dependencies = ["openssl"]           # Existing: install-time deps
runtime_dependencies = ["python"]    # Existing: runtime deps
build_dependencies = ["cmake", "pkg-config"]  # NEW
system_dependencies = ["zlib", "libxml2"]     # NEW
```

**Pros:**
- Clear, explicit fields for each dependency type
- Easy to understand and document
- No changes to existing dependency semantics

**Cons:**
- Four separate dependency fields may be confusing
- Requires updates to recipe schema and validation

#### Option 1B: Structured Dependency Object

Replace flat arrays with a structured dependency specification:

```toml
[dependencies]
build = ["cmake", "pkg-config"]
runtime = ["python"]
install = ["openssl"]
system = ["zlib", "libxml2"]
```

**Pros:**
- Cleaner grouping of related concepts
- Easier to extend with additional metadata (versions, optional flags)

**Cons:**
- Breaking change to recipe format
- More complex parsing logic
- Harder migration path for existing recipes

#### Option 1C: Annotated Dependencies

Use annotations within a single list:

```toml
[metadata]
dependencies = [
  "cmake:build",
  "pkg-config:build",
  "openssl",
  "python:runtime",
  "zlib:system",
]
```

**Pros:**
- Single field to manage
- Flexible annotation system

**Cons:**
- More complex parsing
- Less readable for long dependency lists
- Error-prone string parsing

### Decision 2: System Dependency Handling

How should tsuku handle dependencies it cannot provide?

#### Option 2A: Fail with Instructions

When a system dependency is declared, check if it's available and fail with clear instructions if not:

```
Error: System dependency 'zlib' not found.

This dependency must be installed using your system package manager:
  Ubuntu/Debian: sudo apt install zlib1g-dev
  Fedora/RHEL:   sudo dnf install zlib-devel
  macOS:         brew install zlib  (or use system zlib)

After installing, run: tsuku install <tool>
```

**Pros:**
- Clear guidance for users
- Fails fast before wasting time on partial builds
- Can provide platform-specific instructions

**Cons:**
- Detection may not be reliable across all systems
- Requires maintaining platform-specific package name mappings

#### Option 2B: Document Only (No Verification)

System dependencies are documented in recipes but not verified:

```toml
[metadata]
system_dependencies = ["zlib", "libxml2"]  # Informational only
```

**Pros:**
- Simple implementation
- No false positives from detection failures
- Users can review requirements before installing

**Cons:**
- Builds fail later with cryptic errors if deps are missing
- Poor user experience

#### Option 2C: Optional Verification with pkg-config

Use pkg-config to verify system dependencies when available:

```toml
[metadata.system_dependencies]
zlib = { pkg_config = "zlib" }           # Verify via pkg-config
libxml2 = { pkg_config = "libxml-2.0" }  # Different pkg-config name
custom = { command = "custom-config --version" }  # Custom check
```

**Pros:**
- Reliable verification when pkg-config is available
- Flexible for packages with different detection methods
- Graceful degradation if pkg-config unavailable

**Cons:**
- More complex recipe syntax
- pkg-config not always available or accurate

### Decision 3: Build Environment Setup

How should build dependencies be made available to configure/make/cmake?

#### Option 3A: Automatic Environment Injection

Automatically set PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS based on installed build dependencies:

```go
// Before running configure/cmake/make
env := map[string]string{
    "PKG_CONFIG_PATH": joinPaths(deps, "lib/pkgconfig"),
    "CPPFLAGS":        joinFlags(deps, "-I", "include"),
    "LDFLAGS":         joinFlags(deps, "-L", "lib"),
    "CMAKE_PREFIX_PATH": joinPaths(deps, ""),
}
```

**Pros:**
- Zero recipe changes needed for most builds
- Consistent behavior across all source builds
- Matches how Homebrew handles this

**Cons:**
- Magic behavior may cause confusion
- May conflict with user-set environment variables
- Hard to debug when things go wrong

#### Option 3B: Explicit set_env Steps

Require recipes to explicitly set environment variables:

```toml
[[steps]]
action = "set_env"
name = "PKG_CONFIG_PATH"
value = "$TSUKU_HOME/tools/pkg-config-*/lib/pkgconfig"
```

**Pros:**
- Explicit and visible in recipe
- Full control over exact values
- Easy to debug

**Cons:**
- Verbose for common cases
- Easy to forget required variables
- Duplicates logic across recipes

#### Option 3C: Hybrid with link_build_deps Action

New action that sets up the build environment from declared build dependencies:

```toml
[[steps]]
action = "link_build_deps"
# Automatically sets PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS
# based on metadata.build_dependencies
```

**Pros:**
- Explicit action in recipe (visible)
- Automatic environment setup (convenient)
- Can be customized with parameters if needed

**Cons:**
- New action to implement
- Coupling between metadata and steps

## Decision Outcome

**Chosen: 1A + 2C + 3C**

### Summary

We extend the recipe format with explicit `build_dependencies` and `system_dependencies` fields (Option 1A), verify system dependencies using pkg-config where available (Option 2C), and introduce a `link_build_deps` action that automatically configures the build environment (Option 3C).

### Rationale

**Option 1A (Extend Existing Fields)** preserves backwards compatibility and keeps the recipe format simple. While four dependency fields seem like many, each serves a distinct purpose that users building from source need to understand anyway. The alternative structured format (1B) would require migrating all existing recipes.

**Option 2C (Optional pkg-config Verification)** balances reliability with simplicity. Pure documentation (2B) provides poor UX when builds fail, while mandatory verification (2A) risks false negatives. Using pkg-config when available catches most missing dependencies while gracefully degrading when detection isn't possible.

**Option 3C (link_build_deps Action)** makes the environment setup explicit in recipes while avoiding boilerplate. Users can see that build environment setup happens, but don't need to manually specify PKG_CONFIG_PATH for every dependency. This also allows the action to be omitted for builds that don't need it.

## Solution Architecture

### Recipe Schema Changes

```toml
[metadata]
name = "curl"
description = "Command line tool for transferring data"
dependencies = ["openssl", "nghttp2"]        # Install-time (existing)
runtime_dependencies = []                     # Runtime (existing)
build_dependencies = ["pkg-config", "cmake"]  # NEW: Build-only tools
system_dependencies = ["zlib"]                # NEW: OS-provided libs

# Optional: detailed system dependency specification
[metadata.system_dependencies_detail]
zlib = { pkg_config = "zlib", apt = "zlib1g-dev", brew = "zlib" }
```

### New Action: link_build_deps

```toml
[[steps]]
action = "link_build_deps"
# Optional parameters:
# extra_pkg_config_paths = ["custom/path"]
# extra_include_paths = ["custom/include"]
# extra_lib_paths = ["custom/lib"]
```

**Behavior:**
1. Read `build_dependencies` from recipe metadata
2. Resolve each to installed tool path
3. Set environment variables:
   - `PKG_CONFIG_PATH` = all `{dep}/lib/pkgconfig` paths
   - `CPPFLAGS` = all `-I{dep}/include` flags
   - `LDFLAGS` = all `-L{dep}/lib` flags
   - `CMAKE_PREFIX_PATH` = all `{dep}` paths

### Installation Flow Changes

```
┌─────────────────────────────────────────────────────────┐
│                    tsuku install curl                    │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│ 1. Load recipe, resolve dependencies                     │
│    - dependencies: [openssl, nghttp2]                   │
│    - build_dependencies: [pkg-config, cmake]            │
│    - system_dependencies: [zlib]                        │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│ 2. Verify system dependencies                           │
│    - Run: pkg-config --exists zlib                      │
│    - If missing: show install instructions, fail        │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│ 3. Install build dependencies (if not present)          │
│    - tsuku install pkg-config (hidden)                  │
│    - tsuku install cmake (hidden)                       │
│    - NOT recorded in curl's runtime deps                │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│ 4. Install install-time dependencies                    │
│    - tsuku install openssl                              │
│    - tsuku install nghttp2                              │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│ 5. Execute build steps                                  │
│    - link_build_deps sets PKG_CONFIG_PATH, etc.        │
│    - configure_make / cmake runs with proper env       │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│ 6. Record installation state                            │
│    - InstallDependencies: [openssl, nghttp2]           │
│    - RuntimeDependencies: []                            │
│    - (build deps NOT recorded - ephemeral)              │
└─────────────────────────────────────────────────────────┘
```

### Component Changes

| Component | Change |
|-----------|--------|
| `internal/recipe/types.go` | Add `BuildDependencies`, `SystemDependencies` fields |
| `internal/actions/link_build_deps.go` | NEW: Action to set up build environment |
| `internal/actions/resolver.go` | Handle build_dependencies in resolution |
| `cmd/tsuku/install_deps.go` | Install build deps before build, verify system deps |
| `internal/builders/homebrew.go` | Map Homebrew build_dependencies to recipe field |

## Implementation Approach

### Phase 1: Recipe Schema (Low risk)
1. Add `BuildDependencies []string` to MetadataSection
2. Add `SystemDependencies []string` to MetadataSection
3. Update recipe validation and tests

### Phase 2: Build Dependency Installation (Medium risk)
1. Modify install flow to install build deps as hidden
2. Ensure build deps are installed before build steps execute
3. Build deps should not appear in installed tool's dependency list

### Phase 3: link_build_deps Action (Medium risk)
1. Implement action that reads build_dependencies
2. Resolve each dependency to its install path
3. Set PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS, CMAKE_PREFIX_PATH
4. Register as a primitive action

### Phase 4: System Dependency Verification (Low risk)
1. Implement pkg-config based verification
2. Add platform-specific package name hints
3. Generate helpful error messages on failure

### Phase 5: Homebrew Builder Integration (Low risk)
1. Map `build_dependencies` from formula JSON to recipe field
2. Auto-insert `link_build_deps` step for source builds
3. Extract `uses_from_macos` as system dependencies on Linux

## Security Considerations

### Download Verification
**Not applicable.** This feature doesn't introduce new download sources. Build dependencies are regular tsuku packages with their own verification.

### Execution Isolation
**Low risk.** Build dependencies run in the same environment as other build steps. The new `link_build_deps` action only sets environment variables - it doesn't execute arbitrary code.

### Supply Chain Risks
**Inherited from existing system.** Build dependencies come from the same sources as other tsuku packages (Homebrew bottles, GitHub releases). No new supply chain vectors introduced.

### User Data Exposure
**Not applicable.** This feature doesn't access or transmit user data.

### System Dependency Detection
**Low risk.** The pkg-config verification runs a read-only query (`pkg-config --exists`). Malformed system_dependencies could cause confusing error messages but not security issues.

## Consequences

### Positive
- Source builds succeed more reliably with proper build tooling
- Clear separation between build-time and runtime dependencies
- Users understand system requirements before starting builds
- Build environment setup is automatic but explicit in recipes

### Negative
- Recipe authors must learn new dependency fields
- System dependency verification may have false negatives
- Additional complexity in dependency resolution logic

### Neutral
- Recipes become slightly more verbose for source builds
- Migration needed for existing source build recipes that rely on implicit behavior
