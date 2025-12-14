# System Dependencies

## Status

Proposed

## Context and Problem Statement

Tsuku's dependency system (see [DESIGN-dependency-pattern.md](DESIGN-dependency-pattern.md)) handles **tsuku-provided dependencies** well: actions declare implicit dependencies, recipes can override or extend them, and the resolver computes install-time and runtime dependency sets.

However, source builds often require **system-provided dependencies** - libraries and tools that exist on the host system and cannot (or should not) be installed by tsuku. Examples include:
- `libc`, `libm` - fundamental system libraries
- `zlib`, `libxml2` - common libraries provided by macOS but not Linux
- `make`, `cc` - build toolchain typically provided by the system

When building software from source, builds fail with cryptic errors if these system dependencies are missing. Users have no way to know what system packages to install before attempting a build.

Additionally, build systems (configure, cmake, meson) need environment setup (PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS) to locate both tsuku-managed and system-provided dependencies.

### Relationship to Existing Dependency Model

The existing model already distinguishes:
- **Install-time dependencies** (`dependencies`) - needed during `tsuku install`
- **Runtime dependencies** (`runtime_dependencies`) - needed when the tool runs

A "build-only" dependency is simply one that appears in `dependencies` but NOT in `runtime_dependencies`. **No new field is needed for this distinction.**

What's missing is the ability to mark a dependency as **system-provided** rather than **tsuku-provided**.

### Homebrew Mapping

| Homebrew Field | Tsuku Mapping |
|----------------|---------------|
| `dependencies` | `dependencies` + `runtime_dependencies` |
| `build_dependencies` | `dependencies` only (not in `runtime_dependencies`) |
| `uses_from_macos` | `system:` annotated dependencies |

### Scope

**In scope:**
- Annotation syntax for system-provided dependencies
- Step-level system dependency declarations (following existing pattern)
- Platform-specific system dependencies via existing `when` clause
- Optional verification of system dependencies
- Build environment setup for source builds

**Out of scope:**
- Automatic detection of system dependencies
- Cross-compilation support
- New recipe-level fields (reuse existing patterns)

## Decision Drivers

- **Reuse existing patterns**: Follow the dependency pattern from DESIGN-dependency-pattern.md
- **Minimal new concepts**: Extend existing fields rather than add new ones
- **Step-level declarations**: Dependencies belong on steps, not just metadata
- **Platform handling via `when`**: Use existing conditional mechanism
- **Fail-fast for system deps**: Builds should fail early with clear messages

## Considered Options

### Decision 1: System Dependency Representation

How should system-provided dependencies be distinguished from tsuku-provided ones?

#### Option 1A: Separate Fields

Add parallel fields for system dependencies:

```toml
[[steps]]
action = "configure_make"
dependencies = ["openssl", "pkg-config"]
runtime_dependencies = ["openssl"]
system_dependencies = ["zlib"]
system_runtime_dependencies = ["zlib"]
```

**Pros:**
- Explicit separation
- Simple parsing

**Cons:**
- Four new fields (system_dependencies, system_runtime_dependencies, extra_system_dependencies, extra_system_runtime_dependencies)
- Doesn't follow existing pattern where dependencies are a single concept with attributes

#### Option 1B: Annotation Syntax (SELECTED)

Use a `system:` prefix to mark system-provided dependencies:

```toml
[[steps]]
action = "configure_make"
dependencies = ["openssl", "pkg-config", "system:zlib"]
runtime_dependencies = ["openssl", "system:zlib"]
```

**Pros:**
- Minimal new syntax - just a prefix
- Works with all existing dependency fields
- A dependency is a dependency; "system" is an attribute
- Follows existing override/extend patterns

**Cons:**
- Requires string parsing
- Can't easily add metadata (pkg-config name) inline

#### Option 1C: Structured Dependencies

Use structured objects instead of strings:

```toml
[[steps.dependencies]]
name = "zlib"
system = true
pkg_config = "zlib"
```

**Pros:**
- Rich metadata support
- Type-safe

**Cons:**
- Breaking change to recipe format
- Verbose for common case
- Harder migration

### Decision 2: Platform-Specific System Dependencies

How should platform-specific system dependencies be handled?

#### Option 2A: Platform in Annotation

Embed platform in the annotation:

```toml
dependencies = ["system:zlib:darwin", "zlib:linux"]
```

**Cons:**
- New syntax to learn
- Doesn't reuse existing patterns

#### Option 2B: Use Existing `when` Clause (SELECTED)

Use conditional steps for platform differences:

```toml
# macOS - zlib from system
[[steps]]
action = "configure_make"
dependencies = ["openssl", "system:zlib"]
[steps.when]
os = "darwin"

# Linux - zlib from tsuku
[[steps]]
action = "configure_make"
dependencies = ["openssl", "zlib"]
[steps.when]
os = "linux"
```

**Pros:**
- Reuses existing `when` mechanism
- No new syntax
- Explicit and readable

**Cons:**
- Some duplication when only one dep differs

### Decision 3: System Dependency Verification

How should tsuku verify system dependencies exist?

#### Option 3A: No Verification

Document only - let builds fail naturally.

**Cons:**
- Poor UX with cryptic build errors

#### Option 3B: Optional pkg-config Verification (SELECTED)

Verify via pkg-config when available, with optional detailed specification:

```toml
# Simple case - verify via pkg-config using dep name
dependencies = ["system:zlib"]

# Detailed case - specify pkg-config name and install hints
[steps.system_details]
zlib = { pkg_config = "zlib", apt = "zlib1g-dev", brew = "zlib" }
```

**Pros:**
- Simple case is simple
- Detailed specification available when needed
- Graceful degradation if pkg-config unavailable

### Decision 4: Build Environment Setup

How should build systems find dependencies?

#### Option 4A: Automatic Injection

Automatically set PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS.

**Cons:**
- Magic behavior
- Hard to debug

#### Option 4B: Explicit Action (SELECTED)

New `setup_build_env` action that configures the environment:

```toml
[[steps]]
action = "setup_build_env"
# Sets PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS based on step's dependencies
```

**Pros:**
- Explicit in recipe
- Can be customized or omitted
- Clear what's happening

## Decision Outcome

**Chosen: 1B + 2B + 3B + 4B**

### Summary

System dependencies are marked with a `system:` prefix in existing dependency fields. Platform differences use the existing `when` clause. Verification uses pkg-config with optional detailed specification. Build environment setup is an explicit action.

### Rationale

This approach maximizes reuse of existing patterns:
- Dependencies are declared at step level (per DESIGN-dependency-pattern.md)
- Platform handling uses existing `when` clause
- No new recipe-level fields needed
- Annotation is minimal and intuitive

## Solution Architecture

### Annotation Syntax

```
dependency     = name | "system:" name
name           = [a-z0-9][a-z0-9-]*
```

Examples:
- `openssl` - tsuku-provided
- `system:zlib` - system-provided

### Step-Level Declaration

Following the existing dependency pattern, system dependencies are declared at the step level:

```toml
[[steps]]
action = "configure_make"
dependencies = ["openssl", "pkg-config", "system:zlib"]
runtime_dependencies = ["openssl", "system:zlib"]
extra_dependencies = ["system:libxml2"]  # extend, don't replace
configure_flags = ["--with-openssl", "--with-zlib"]
```

### Platform-Specific Example

Building curl with platform-specific system dependencies:

```toml
[metadata]
name = "curl"
description = "Command line tool for transferring data"

[[steps]]
action = "github_archive"
repo = "curl/curl"
asset_pattern = "curl-{version}.tar.gz"

[[steps]]
action = "extract"
strip_components = 1

# macOS build - zlib from system
[[steps]]
action = "setup_build_env"
[steps.when]
os = "darwin"

[[steps]]
action = "configure_make"
dependencies = ["openssl", "nghttp2", "pkg-config", "system:zlib"]
runtime_dependencies = ["openssl", "nghttp2", "system:zlib"]
configure_flags = ["--with-openssl", "--with-nghttp2", "--with-zlib"]
[steps.when]
os = "darwin"

# Linux build - zlib from tsuku
[[steps]]
action = "setup_build_env"
[steps.when]
os = "linux"

[[steps]]
action = "configure_make"
dependencies = ["openssl", "nghttp2", "pkg-config", "zlib"]
runtime_dependencies = ["openssl", "nghttp2", "zlib"]
configure_flags = ["--with-openssl", "--with-nghttp2", "--with-zlib"]
[steps.when]
os = "linux"

[[steps]]
action = "install_binaries"
binaries = ["src/curl"]
```

### Action Implicit System Dependencies

Actions can declare implicit system dependencies in the registry:

```go
type ActionDeps struct {
    InstallTime       []string  // Tsuku-provided install-time
    Runtime           []string  // Tsuku-provided runtime
    SystemInstallTime []string  // System-provided install-time
    SystemRuntime     []string  // System-provided runtime
}

var ActionDependencies = map[string]ActionDeps{
    "configure_make": {
        InstallTime:       nil,
        Runtime:           nil,
        SystemInstallTime: []string{"make", "cc"},
        SystemRuntime:     nil,
    },
    "cmake_build": {
        InstallTime:       []string{"cmake"},
        Runtime:           nil,
        SystemInstallTime: []string{"make", "cc"},
        SystemRuntime:     nil,
    },
    // ...
}
```

### Dependency Resolution Changes

The resolver parses the `system:` prefix and separates dependencies:

```go
type ResolvedDeps struct {
    InstallTime       map[string]string  // tsuku-provided
    Runtime           map[string]string  // tsuku-provided
    SystemInstallTime []string           // system-provided
    SystemRuntime     []string           // system-provided
}

func parseDependency(dep string) (name string, isSystem bool) {
    if strings.HasPrefix(dep, "system:") {
        return strings.TrimPrefix(dep, "system:"), true
    }
    return dep, false
}
```

### System Dependency Verification

Before executing build steps, verify system dependencies:

```go
func verifySystemDeps(deps []string, details map[string]SystemDepDetail) error {
    var missing []string
    for _, dep := range deps {
        if !checkSystemDep(dep, details[dep]) {
            missing = append(missing, dep)
        }
    }
    if len(missing) > 0 {
        return &MissingSystemDepsError{Missing: missing, Details: details}
    }
    return nil
}

func checkSystemDep(name string, detail SystemDepDetail) bool {
    pkgName := name
    if detail.PkgConfig != "" {
        pkgName = detail.PkgConfig
    }
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    cmd := exec.CommandContext(ctx, "pkg-config", "--exists", pkgName)
    return cmd.Run() == nil
}
```

### setup_build_env Action

Sets up the build environment based on the step's resolved dependencies:

```go
func (a *SetupBuildEnvAction) Execute(ctx *ExecutionContext) error {
    var pkgConfigPaths, includePaths, libPaths []string

    // Add paths from tsuku-provided dependencies
    for _, dep := range ctx.ResolvedDeps.InstallTime {
        toolPath := ctx.State.GetToolPath(dep)
        pkgConfigPaths = append(pkgConfigPaths, filepath.Join(toolPath, "lib", "pkgconfig"))
        includePaths = append(includePaths, filepath.Join(toolPath, "include"))
        libPaths = append(libPaths, filepath.Join(toolPath, "lib"))
    }

    // Set environment variables
    ctx.Env["PKG_CONFIG_PATH"] = strings.Join(pkgConfigPaths, ":")
    ctx.Env["CPPFLAGS"] = formatFlags("-I", includePaths)
    ctx.Env["LDFLAGS"] = formatFlags("-L", libPaths)
    ctx.Env["CMAKE_PREFIX_PATH"] = strings.Join(toolPaths, ";")

    return nil
}
```

### Installation Flow

```
tsuku install curl
        │
        ▼
┌─────────────────────────────────────────┐
│ 1. Load recipe, evaluate `when` clauses │
│    Select platform-appropriate steps    │
└─────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────┐
│ 2. Resolve dependencies from steps      │
│    - Parse system: prefix               │
│    - Separate tsuku vs system deps      │
└─────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────┐
│ 3. Verify system dependencies           │
│    - pkg-config --exists <dep>          │
│    - Fail with install instructions     │
└─────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────┐
│ 4. Install tsuku dependencies           │
│    - openssl, nghttp2, pkg-config       │
└─────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────┐
│ 5. Execute steps                        │
│    - setup_build_env sets PKG_CONFIG... │
│    - configure_make runs with env       │
└─────────────────────────────────────────┘
        │
        ▼
┌─────────────────────────────────────────┐
│ 6. Record state                         │
│    - InstallDeps: [openssl, nghttp2]    │
│    - RuntimeDeps: [openssl, nghttp2]    │
│    - SystemDeps not recorded (external) │
└─────────────────────────────────────────┘
```

### Component Changes

| Component | Change |
|-----------|--------|
| `internal/actions/dependencies.go` | Add `SystemInstallTime`, `SystemRuntime` to `ActionDeps` |
| `internal/actions/resolver.go` | Parse `system:` prefix, separate dep types |
| `internal/actions/setup_build_env.go` | NEW: Action to configure build environment |
| `internal/actions/verify_system_deps.go` | NEW: Verify system deps via pkg-config |
| `internal/executor/executor.go` | Call verification before build steps |
| `internal/builders/homebrew.go` | Map `uses_from_macos` to `system:` deps |

## Implementation Approach

### Phase 1: Dependency Parsing (Low risk)
1. Add `system:` prefix parsing to resolver
2. Extend `ActionDeps` with system dependency fields
3. Update resolution algorithm to separate dep types
4. Add validation for dependency names

### Phase 2: System Dependency Verification (Low risk)
1. Implement pkg-config verification
2. Add `system_details` section parsing
3. Generate helpful error messages with install hints
4. Add 5-second timeout for pkg-config queries

### Phase 3: setup_build_env Action (Medium risk)
1. Implement action to set PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS
2. Add `Env` field to ExecutionContext if not present
3. Update build actions to inherit environment
4. Register as primitive action

### Phase 4: Homebrew Integration (Low risk)
1. Map `uses_from_macos` to `system:` annotated deps
2. Generate platform-conditional steps for mixed deps
3. Auto-insert `setup_build_env` for source builds

## Security Considerations

### Download Verification
**Not applicable.** System dependencies are not downloaded by tsuku.

### Execution Isolation
**Low risk.** The `setup_build_env` action only sets environment variables. System dependency verification runs read-only pkg-config queries.

### Supply Chain Risks
**Shifted to system.** System dependencies come from the OS package manager, not tsuku. Users must trust their system's package sources.

### User Data Exposure
**Not applicable.** This feature doesn't access or transmit user data.

### Input Validation

Dependency names (including after `system:` prefix) must be validated:

```go
var validDepName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
```

### pkg-config Execution

- 5-second timeout prevents DoS
- Only `--exists` flag used (read-only query)
- Input validated before passing to pkg-config

## Consequences

### Positive
- System dependencies explicitly declared and verified
- Builds fail fast with helpful error messages
- Reuses existing dependency patterns
- Platform differences handled by existing `when` mechanism
- No new recipe-level fields needed

### Negative
- Some step duplication for platform-specific deps
- String parsing for `system:` prefix
- pkg-config verification not 100% reliable

### Neutral
- Recipes with system deps slightly more verbose
- Learning curve for `system:` annotation (but minimal)
