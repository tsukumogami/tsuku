# Actions and Primitives Guide

This guide explains how tsuku uses actions to define installation steps, the different types of actions available, and how they relate to reproducibility guarantees.

## Overview

Actions are the building blocks of tsuku recipes. Each action describes a single installation step, from downloading files to running build commands.

When you run `tsuku eval`, the tool analyzes the recipe and generates an **installation plan** containing only primitive actions—the simplest, most deterministic operations. This separation between recipe definition and execution plan is fundamental to tsuku's reproducibility model.

## Action Types

tsuku has two categories of actions, distinguished by how they execute and their determinism guarantees:

### File Operation Primitives

File operation primitives are the atomic building blocks of installation. They perform simple, fully deterministic operations on files and don't invoke external tools.

| Action | Purpose | Determinism |
|--------|---------|-------------|
| `download_file` | Fetch a URL to a file with checksum verification | Fully deterministic |
| `extract` | Decompress archives (tar, zip, gzip, etc.) | Fully deterministic |
| `chmod` | Set file permissions | Fully deterministic |
| `install_binaries` | Copy binaries to install directory and create symlinks | Fully deterministic |
| `set_env` | Set environment variables in shell profiles | Fully deterministic |
| `set_rpath` | Modify binary rpath for dependency resolution | Fully deterministic |
| `link_dependencies` | Create symlinks to library dependencies | Fully deterministic |
| `install_libraries` | Install shared libraries to system locations | Fully deterministic |
| `apply_patch_file` | Apply patch file using system patch command | Fully deterministic |
| `text_replace` | Text replacement in files (Homebrew inreplace equivalent) | Fully deterministic |
| `homebrew_relocate` | Relocate Homebrew placeholder paths in binaries | Fully deterministic |
| `require_system` | Validate system dependency exists with version check | Fully deterministic |

**Key property**: All file operation primitives are fully reproducible. Running the same primitive twice produces identical results.

#### require_system Action (Legacy)

The `require_system` action validates that a system dependency exists and optionally checks its version. This is used for dependencies tsuku cannot provision (Docker, CUDA, kernel modules).

**Note**: For new recipes, prefer the M30 system dependency vocabulary (`require_command`, `apt_install`, etc.) which provides better platform targeting and documentation generation. The `require_system` action remains for backward compatibility.

```toml
[[steps]]
action = "require_system"
command = "docker"
version_flag = "--version"
version_regex = "Docker version ([0-9.]+)"
min_version = "20.0"  # Optional minimum version

[steps.install_guide]
darwin = "brew install --cask docker"
linux = "See https://docs.docker.com/engine/install/"
fallback = "Visit https://docs.docker.com/get-docker/"
```

Parameters:
- `command` (required): The command to check for
- `version_flag`: Flag to get version output (e.g., `--version`)
- `version_regex`: Regex to extract version from output
- `min_version`: Minimum required version
- `install_guide`: Platform-specific installation instructions

The action:
1. Checks if the command exists in PATH
2. If `version_flag` is specified, extracts and validates version
3. If missing or version too old, displays platform-specific installation guidance
4. Fails the installation with actionable error message

### System Dependency Primitives

System dependency primitives generate instructions for platform package managers. These actions are filtered by `when` clauses at evaluation time to match the target platform and Linux family.

#### Package Manager Actions

| Action | Package Manager | Implicit Constraint |
|--------|-----------------|---------------------|
| `apt_install` | apt-get | `when.linux_family = "debian"` |
| `apt_ppa` | add-apt-repository | `when.linux_family = "debian"` |
| `apt_repo` | apt sources | `when.linux_family = "debian"` |
| `apk_install` | apk | `when.linux_family = "alpine"` |
| `brew_cask` | Homebrew Cask | `when.platform = "darwin/*"` |
| `brew_install` | Homebrew | (none - works on macOS and Linux) |
| `dnf_install` | dnf | `when.linux_family = "rhel"` |
| `dnf_repo` | dnf config-manager | `when.linux_family = "rhel"` |
| `pacman_install` | pacman | `when.linux_family = "arch"` |
| `zypper_install` | zypper | `when.linux_family = "suse"` |

#### Configuration Actions

| Action | Description | Typical Use |
|--------|-------------|-------------|
| `group_add` | Add user to a group | Docker group membership |
| `service_enable` | Enable a system service | Start service on boot |
| `service_start` | Start a system service | Immediate service activation |

#### Verification Actions

| Action | Description |
|--------|-------------|
| `require_command` | Verify a command is available after installation |
| `manual` | Display manual installation instructions |

**Key properties**:
- Actions with implicit constraints are automatically filtered by platform
- Instructions are displayed to the user (tsuku does not execute privileged commands)
- Multiple actions can be combined for complete installation flows

See [System Dependencies Guide](GUIDE-system-dependencies.md) for user-facing documentation and recipe examples.

### Ecosystem Primitives

Ecosystem primitives delegate to external package managers and build systems (Go, Rust, Node.js, Python, Ruby, Perl, Nix). These capture maximum constraint at evaluation time but may have residual non-determinism due to compiler versions and platform differences.

| Action | Ecosystem | Locked At Eval | Residual Non-Determinism |
|--------|-----------|----------------|--------------------------|
| `cargo_build` | Rust | Cargo.lock | Compiler version, build scripts |
| `cmake_build` | CMake | CMakeLists.txt | Compiler version, platform differences |
| `configure_make` | Autotools | configure script | Compiler version, platform detection |
| `cpan_install` | Perl | cpanfile.snapshot | XS modules, Perl version |
| `meson_build` | Meson | meson.build | Compiler version, build scripts |
| `gem_exec` | Ruby | Gemfile.lock | Native extensions, Ruby version |
| `go_build` | Go | go.sum, module versions | Compiler version, CGO |
| `install_gem_direct` | Ruby | Exact version | Native extensions, Ruby version |
| `nix_realize` | Nix | flake.lock + derivation hash | Binary cache (fully deterministic if built locally) |
| `npm_exec` | Node.js | package-lock.json | Native addons, Node.js version |
| `pip_exec` | Python | requirements.txt with hashes | Platform wheels, Python version |
| `pip_install` | Python | Version only (legacy) | Platform wheels, Python version, transitive deps |

**Key property**: Ecosystem primitives capture dependency versions during evaluation but cannot guarantee bit-for-bit reproducibility due to compiler and platform variations.

#### Build System Primitives

Build system primitives handle source compilation using standard build tools. These are used when tools need to be built from source:

- `configure_make` - Autotools builds (`./configure && make && make install`)
- `cmake_build` - CMake projects
- `meson_build` - Meson build system (common in GNOME/GTK projects)

Build system primitives are ecosystem primitives that depend on system compilers. They capture build configuration but have residual non-determinism from compiler versions and platform detection.

#### setup_build_env

Configures build environment from dependency graph. Sets environment variables for all declared dependencies:
- PKG_CONFIG_PATH - lib/pkgconfig paths from dependencies
- CPPFLAGS - -I flags for include directories
- LDFLAGS - -L flags for library directories
- CMAKE_PREFIX_PATH - for CMake builds
- CC/CXX - compiler paths (zig cc if no system compiler)

Example:
```toml
[[steps]]
action = "setup_build_env"

[[steps]]
action = "configure_make"
source_dir = "curl-{version}"
configure_args = ["--with-openssl", "--with-zlib"]
```

The setup_build_env action validates that dependencies can be discovered by the build system.

**Example use case**: Manual recipe authoring for tools distributed as source tarballs, or when Homebrew formulas lack pre-built bottles.

### Composite Actions (Recipe Authoring)

Composite actions are shortcuts for recipe authors. They decompose into primitives during the eval phase:

#### Standard Download Composites

| Composite | Decomposes To | Example Recipe Use |
|-----------|---------------|--------------------|
| `download` | download_file | Download with automatic checksum computation |
| `download_archive` | download_file + extract + chmod + install_binaries | Download and extract a tarball from any URL |
| `github_archive` | download_file + extract + chmod + install_binaries | Download release asset from GitHub |
| `github_file` | download_file + chmod + install_binaries | Download a single binary from GitHub |

#### Specialized Composites

| Composite | Decomposes To | Example Recipe Use |
|-----------|---------------|--------------------|
| `apply_patch` | download_file + apply_patch_file (or just apply_patch_file) | Apply patch from URL or inline data |
| `homebrew` | download_file + extract + homebrew_relocate | Install Homebrew GHCR bottles |

##### Patch Application

Patches are used to modify source code before building, common in Homebrew formulas and source-based installs:

```toml
[[steps]]
action = "apply_patch"
url = "https://raw.githubusercontent.com/Homebrew/formula-patches/master/example/fix.patch"
checksum = "sha256:abc123..."
strip = 1  # Strip leading path components (-p1)
```

The `apply_patch` composite:
1. Downloads patch file with checksum verification (if URL provided)
2. Applies patch using `apply_patch_file` primitive
3. Validates patch application succeeded

For inline patches, omit the `url` parameter and provide patch data directly.

#### Ecosystem Install Composites

| Composite | Decomposes To | Example Recipe Use |
|-----------|---------------|--------------------|
| `cargo_install` | download + extract + cargo_build | Install Rust crate from source |
| `gem_install` | download + extract + gem_exec | Install Ruby gem from source |
| `go_install` | download + extract + go_build | Install Go module from source |
| `nix_install` | nix_realize | Install from Nix flake |
| `npm_install` | download + extract + npm_exec | Install Node.js package from source |
| `pipx_install` | download + extract + pip_exec | Install Python package from source |

**Important**: Composite actions exist only in recipes, never in plans. When you run `tsuku eval`, composites decompose into their primitive components.

## Action Decomposition

### How It Works

When you run `tsuku eval`, tsuku processes the recipe and:

1. Loads the recipe and resolves version information
2. For each composite action, calls its decomposition logic
3. For `github_archive`, this includes:
   - Resolving asset patterns (e.g., `rg-*-x86_64-linux.tar.gz`)
   - Constructing download URLs
   - Pre-downloading to compute checksums
4. Returns a plan containing only primitives

The decomposition process ensures that the plan is self-contained—everything needed to install is explicitly listed.

### Why Plans Contain Only Primitives

Plans should never contain composite actions because:

1. **Execution transparency**: The executor only needs to understand primitives. No need to re-interpret composite action logic during installation.

2. **Determinism guarantee**: Plans with only file primitives are guaranteed fully reproducible. Plans with ecosystem primitives are marked with `deterministic: false` to signal potential variation.

3. **Auditability**: Reading a plan tells you exactly what will happen. No hidden steps or runtime logic.

4. **Reproducibility**: `tsuku eval foo | tsuku install --plan -` and `tsuku eval foo > plan.json && tsuku install --plan plan.json` produce identical results.

### Example Decomposition

Recipe with `github_archive` composite:

```toml
[[recipe.actions]]
action = "github_archive"
repo = "BurntSushi/ripgrep"
asset_pattern = "rg-*-x86_64-unknown-linux-musl.tar.gz"
strip_dirs = 1
binaries = ["rg"]
```

After `tsuku eval`, the plan contains:

```json
{
  "steps": [
    {
      "action": "download_file",
      "params": {
        "url": "https://github.com/BurntSushi/ripgrep/releases/download/14.1.0/rg-14.1.0-x86_64-unknown-linux-musl.tar.gz",
        "dest": "rg-14.1.0-x86_64-unknown-linux-musl.tar.gz"
      },
      "checksum": "sha256:1234567890abcdef...",
      "size": 2048576
    },
    {
      "action": "extract",
      "params": {
        "archive": "rg-14.1.0-x86_64-unknown-linux-musl.tar.gz",
        "format": "tar.gz",
        "strip_dirs": 1
      }
    },
    {
      "action": "chmod",
      "params": {"files": ["rg"]}
    },
    {
      "action": "install_binaries",
      "params": {"binaries": ["rg"]}
    }
  ]
}
```

## Determinism Model

### File Operation Primitives: Fully Deterministic

Plans containing only file operation primitives are guaranteed to produce identical installations. The same plan applied to the same system always produces the same result.

```json
{
  "format_version": 2,
  "deterministic": true,
  "steps": [
    {"action": "download_file", "checksum": "sha256:..."},
    {"action": "extract"},
    {"action": "chmod"},
    {"action": "install_binaries"}
  ]
}
```

### Ecosystem Primitives: Captured Constraints

For ecosystem-based installations, tsuku captures dependency lockfiles during evaluation and embeds them in the plan. These lockfiles minimize but cannot eliminate non-determinism.

```json
{
  "action": "go_build",
  "params": {
    "module": "github.com/jesseduffield/lazygit",
    "version": "v0.40.2",
    "executables": ["lazygit"]
  },
  "locks": {
    "go_version": "1.21.0",
    "go_sum": "h1:abc...=\ngithub.com/foo/bar v1.0.0 h1:xyz...=\n..."
  },
  "deterministic": false
}
```

Key details:
- **go.sum**: Lists all module dependencies and their hashes
- **go_version**: Locked Go version
- **deterministic: false**: Signals potential variation from compiler or platform

### Lockfile Capture

During evaluation, tsuku captures dependency lockfiles to constrain ecosystem builds:

- **Go**: go.sum (module checksums and versions)
- **Rust**: Cargo.lock (crate dependencies)
- **Node.js**: package-lock.json (dependency tree with integrity hashes)
- **Python**: requirements.txt with hashes (pinned package versions)
- **Ruby**: Gemfile.lock (gem versions and checksums)
- **Perl**: cpanfile.snapshot (distribution versions)
- **Nix**: flake.lock (pinned flake inputs)

These lockfiles are embedded in the plan and used during execution to ensure the exact same dependency versions and revisions are installed.

### Residual Non-Determinism

Even with captured lockfiles, some variation remains. Common sources:

| Source | Affected Ecosystems | Why It Happens |
|--------|-------------------|-----------------|
| Compiler version | Go, Rust, C extensions | Different optimization flags, object code layout |
| Platform differences | pip, gem, Node.js (native addons) | CPU-specific instructions, ABI changes |
| Build script behavior | Rust, npm, gem, CPAN | Scripts may make runtime decisions |
| Timestamps | All | Embedded in archives or compiled objects |

These variations are usually small and don't affect functionality, but they prevent bit-for-bit reproducibility.

### The `deterministic` Flag

Plans include a `deterministic` flag indicating reproducibility guarantees:

**`deterministic: true`** - Plan contains only file primitives
- Same plan = identical installation on same system
- Safe for distribution and caching
- No execution variation

**`deterministic: false`** - Plan contains ecosystem primitives
- Same plan = functionally equivalent installation
- Binaries may differ due to compiler/platform
- Installation is reproducible within expected variance

Check a plan's determinism:

```bash
tsuku eval lazygit | jq '.deterministic'
```

## Using Actions in Recipes

Recipe authors write composites for convenience:

```toml
[[recipe.actions]]
action = "github_archive"
repo = "BurntSushi/ripgrep"
asset_pattern = "rg-*-{{os}}-{{arch}}.tar.gz"
binaries = ["rg"]
```

Users interact with primitives in plans:

```bash
# Generate plan (composites decompose automatically)
tsuku eval rg > rg-plan.json

# Inspect to see only primitives
cat rg-plan.json | jq '.steps[].action'
# Output: download_file, extract, chmod, install_binaries

# Install from plan (executor runs primitives)
tsuku install --plan rg-plan.json
```

## Ecosystem Primitives and Build Reproducibility

Ecosystem primitives handle complex build scenarios that require external tooling. Each captures maximum constraint:

### Go Example

Recipe defines a Go module to build:

```toml
[[recipe.actions]]
action = "go_build"
module = "github.com/jesseduffield/lazygit"
executables = ["lazygit"]
```

During eval, tsuku:
1. Downloads source at specified version
2. Runs `go mod download` to fetch dependencies
3. Captures go.sum into the plan
4. Records Go version

During exec, tsuku:
1. Downloads source (same version)
2. Writes captured go.sum
3. Runs `go build` with `GOPROXY=off` (use only local cache)
4. Installs resulting binary

Result: Reproducible Go builds with locked dependencies, but compiler version may vary.

### Rust Example

Recipe defines a Rust crate:

```toml
[[recipe.actions]]
action = "cargo_build"
crate = "ripgrep"
executables = ["rg"]
```

During eval, tsuku:
1. Downloads crate at specified version
2. Runs `cargo fetch` to download dependencies
3. Captures Cargo.lock into plan
4. Records Rust version

During exec, tsuku:
1. Downloads crate
2. Writes captured Cargo.lock
3. Runs `cargo build --locked --offline`
4. Installs resulting binary

Result: Reproducible Rust builds with locked crates.

## Troubleshooting

### Why is my plan marked `deterministic: false`?

Your recipe uses ecosystem primitives (go_build, cargo_build, etc.). This is expected. The flag indicates that while the plan is reproducible, binaries may vary slightly due to compiler versions.

To make a plan fully deterministic, use only file operation primitives (download_file, extract, chmod, install_binaries).

### What if I need bit-for-bit reproducibility?

Use pre-built binaries (prefer file primitives) or Nix:

```toml
# Option 1: Pre-built binaries (deterministic)
[[recipe.actions]]
action = "github_archive"
repo = "BurntSushi/ripgrep"
binaries = ["rg"]

# Option 2: Nix (fully deterministic if built locally)
[[recipe.actions]]
action = "nix_realize"
flake_ref = "nixpkgs#ripgrep"
```

## Build Environment Configuration

When executing source builds (`configure_make`, `cmake_build`), tsuku automatically configures the build environment to use tsuku-provided dependencies:

- **CC/CXX**: Set to `zig cc`/`zig c++` when system compiler unavailable (or use system compiler if present)
- **PKG_CONFIG_PATH**: Includes all dependency `lib/pkgconfig` directories
- **CPPFLAGS**: Includes `-I` flags for all dependency include directories
- **LDFLAGS**: Includes `-L` flags for all dependency lib directories
- **CMAKE_PREFIX_PATH**: Semicolon-separated list of dependency installation paths

This ensures autotools and cmake can find tsuku-provided dependencies without manual configuration. Build essentials (compilers, make, libraries) are installed automatically as implicit dependencies when needed.

Example:
```toml
# This recipe depends on zlib
[[recipe.actions]]
action = "configure_make"
url = "https://ftp.gnu.org/gnu/gdbm/gdbm-1.23.tar.gz"
configure_flags = ["--disable-shared"]

# During build, tsuku automatically:
# 1. Installs make and zig as implicit dependencies
# 2. Sets CC=zig cc (if no system compiler)
# 3. Sets CPPFLAGS=-I$TSUKU_HOME/tools/zlib-x.y.z/include
# 4. Sets LDFLAGS=-L$TSUKU_HOME/tools/zlib-x.y.z/lib
```

### How do I inspect captured lockfiles in a plan?

Extract lockfiles from the plan:

```bash
# Show all steps with locks
tsuku eval lazygit | jq '.steps[] | select(.locks)'

# Extract go.sum from go_build step
tsuku eval lazygit | jq '.steps[] | select(.action=="go_build") | .locks.go_sum'
```

## See Also

- [Plan-Based Installation Guide](GUIDE-plan-based-installation.md) - How to use plans for reproducible deployments
- [Recipe Verification Guide](GUIDE-recipe-verification.md) - How tsuku verifies installations
- [Design: Decomposable Actions](../DESIGN-decomposable-actions.md) - Technical architecture details
