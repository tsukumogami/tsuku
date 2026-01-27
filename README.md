# tsuku

[![Tests](https://github.com/tsukumogami/tsuku/actions/workflows/test.yml/badge.svg)](https://github.com/tsukumogami/tsuku/actions/workflows/test.yml)
[![codecov](https://codecov.io/gh/tsukumogami/tsuku/graph/badge.svg)](https://codecov.io/gh/tsukumogami/tsuku)

A modern, universal package manager for development tools.

## Overview

tsuku is a package manager that makes it easy to install and manage development tools across different platforms. It uses action-based recipes to download, extract, and install tools to version-specific directories with automatic PATH management.

## Features

- **Action-based recipes**: Composable actions for downloading, extracting, and installing tools
- **Version management**: Tools installed in version-specific directories
- **Automatic PATH management**: Shell integration for easy access
- **Dependency management**: Automatic installation and cleanup of tool dependencies
- **Ecosystem integration**: Full support for npm, cargo, go, pip, gem, nix, and cpan with lockfile-based reproducibility
- **No dependencies**: Single binary, no system prerequisites

## Installation

```bash
curl -fsSL https://get.tsuku.dev/now | bash
```

## Usage

### Install a tool

```bash
tsuku install kubectl
tsuku install terraform
tsuku install gh
```

### List installed tools

```bash
tsuku list
```

### List installed macOS applications

```bash
tsuku list --apps
```

Applications installed via cask recipes are stored in `$TSUKU_HOME/apps/` and symlinked to `~/Applications` for Launchpad and Spotlight integration.

### Update a tool

```bash
tsuku update kubectl
```

### Remove a tool

```bash
tsuku remove kubectl
```

### Create recipes from package ecosystems

Generate recipes automatically from package registry metadata:

```bash
# From crates.io (Rust)
tsuku create ripgrep --from crates.io

# From RubyGems
tsuku create jekyll --from rubygems

# From PyPI (Python)
tsuku create ruff --from pypi

# From npm (Node.js)
tsuku create prettier --from npm

# From GitHub releases (uses LLM)
tsuku create gh --from github:cli/cli

# From Homebrew bottles (pre-built binaries for Linux/macOS)
tsuku create zlib --from homebrew:zlib
tsuku create jq --from homebrew:jq

# From Homebrew Cask (macOS GUI applications)
tsuku create iterm2 --from cask:iterm2
tsuku create firefox --from cask:firefox
```

#### LLM-Powered Recipe Generation

Some recipe builders use LLM analysis to generate recipes from complex sources. These require an API key:

**Builders requiring LLM**:
- `--from github:owner/repo` - Analyzes GitHub releases
- `--from homebrew:formula` - Analyzes Homebrew formulas

**Builders NOT requiring LLM** (deterministic):
- `--from crates.io` - Uses crates.io API
- `--from npm` - Uses npm registry
- `--from pypi` - Uses PyPI API
- `--from rubygems` - Uses RubyGems API
- `--from cask:<name>` - Uses Homebrew Cask API (macOS applications)

To use LLM-powered builders, export an API key for Claude or Gemini:

```bash
# Claude (Anthropic)
export ANTHROPIC_API_KEY="sk-ant-..."

# Or Gemini (Google)
export GOOGLE_API_KEY="AIza..."
```

Cost per recipe generation: ~$0.02-0.15 depending on complexity.

#### Dependency Discovery

When you request a Homebrew formula, tsuku automatically discovers all dependencies and estimates generation cost:

```bash
$ tsuku create neovim --from homebrew:neovim

Discovering dependencies...

Dependency tree for neovim:
  neovim (needs recipe)
  ├── gettext (needs recipe)
  ├── libuv (has recipe ✓)
  ├── lpeg (needs recipe)
  │   └── lua (needs recipe)
  ...

Recipes to generate: 8
Estimated cost: ~$0.40

Proceed? [y/N]
```

Recipes are generated in dependency order (leaves first) to ensure validation succeeds.

#### Platform Support

Homebrew recipes are platform-agnostic - a single recipe works on:
- macOS ARM64 (Apple Silicon)
- macOS x86_64 (Intel)
- Linux ARM64
- Linux x86_64

The `homebrew` action automatically selects the correct bottle for your platform at install time.

#### Recipe Validation

Generated recipes are automatically validated in isolated containers with a repair loop:

```bash
# Validation happens automatically during tsuku create
tsuku create jq --from homebrew:jq
# Downloads bottle, validates in container, repairs if needed (max 2 attempts)

# Skip validation if you trust the source
tsuku create jq --from homebrew:jq --skip-sandbox
```

Validation catches issues like wrong executable names, missing dependencies, incorrect verification commands, and platform-specific problems. If validation fails, the LLM attempts repairs before returning an error.

Generated recipes are stored in `$TSUKU_HOME/recipes/` and take precedence over registry recipes. You can inspect and edit them before installation:

```bash
# View generated recipe
cat ~/.tsuku/recipes/ripgrep.toml

# Install the tool
tsuku install ripgrep

# List local recipes
tsuku recipes --local
```

Use `--force` to overwrite an existing local recipe.

### Verbosity and Debugging

tsuku supports multiple verbosity levels for troubleshooting:

```bash
# Quiet mode - errors only
tsuku install kubectl --quiet
tsuku install kubectl -q

# Verbose mode - show operational details
tsuku install kubectl --verbose
tsuku install kubectl -v

# Debug mode - full diagnostic output (includes timestamps and source locations)
tsuku install kubectl --debug
```

Verbosity can also be controlled via environment variables:

```bash
TSUKU_QUIET=1 tsuku install kubectl    # Errors only
TSUKU_VERBOSE=1 tsuku install kubectl  # Verbose output
TSUKU_DEBUG=1 tsuku install kubectl    # Debug output
```

Flags take precedence over environment variables. Debug mode displays a warning banner since output may contain file paths and URLs.

### Dependency Management

tsuku automatically handles tool dependencies:

- **Automatic installation**: When installing a tool, all dependencies are installed automatically
- **State tracking**: Tracks which tools were explicitly installed vs. auto-installed as dependencies
- **Dependency protection**: Prevents removal of tools that are required by other tools
- **Orphan cleanup**: Automatically removes dependencies when they're no longer needed

Example:
```bash
# If tool-a depends on tool-b:
tsuku install tool-a  # Installs both tool-a and tool-b

# Attempting to remove a required dependency fails:
tsuku remove tool-b   # Error: tool-b is required by: tool-a

# Removing the parent tool auto-removes orphaned dependencies:
tsuku remove tool-a   # Removes both tool-a and tool-b
```

If you explicitly install a dependency, it won't be auto-removed:
```bash
tsuku install tool-b  # Explicitly installed
tsuku install tool-a  # tool-b already present
tsuku remove tool-a   # tool-b remains (it was explicit)
```

### Build Dependency Provisioning

tsuku automatically provides build tools needed for source builds, eliminating the need for system dependencies.

#### Build System Actions

- **cmake_build** - CMake-based builds (auto-provisions cmake, make, zig, pkg-config)
- **configure_make** - Autotools builds (auto-provisions make, zig, pkg-config)
- **setup_build_env** - Configures build environment (PKG_CONFIG_PATH, CPPFLAGS, LDFLAGS)

#### Core Build Tools

- **zig** - C/C++ compiler fallback via `zig cc` when no system compiler exists
- **cmake** - Modern build system with dedicated cmake_build action
- **pkg-config** - Library discovery tool with automatic path configuration
- **make** - Build automation

#### Automatic Dependency Provisioning

When you install a tool that requires libraries, tsuku automatically provisions all dependencies:

```bash
tsuku install sqlite
# Automatically installs: sqlite → readline → ncurses
# No apt-get or brew needed
```

All dependencies are isolated to `$TSUKU_HOME` - no system modifications required.

### System Dependencies

Some tools require dependencies that tsuku cannot provision - things like Docker, CUDA, or kernel modules that require system-level installation. For these, tsuku provides clear guidance.

#### Check Dependencies

Before installing a tool, check what dependencies it requires:

```bash
# Check dependencies for a tool
tsuku check-deps docker-compose

# Output shows:
# - Provisionable: Dependencies tsuku will install automatically
# - System-required: Dependencies you must install manually
```

The `check-deps` command:
- Shows which dependencies tsuku can provision vs. which require manual installation
- Provides platform-specific installation instructions for system dependencies
- Exits with code 1 if any system dependency is missing (useful for CI)

#### Verify System Dependencies

After installing system dependencies manually, verify they are correctly configured:

```bash
# Verify all require_command checks for a recipe pass
tsuku verify-deps docker

# JSON output for scripting
tsuku verify-deps --json docker
```

The `verify-deps` command:
- Checks all `require_command` steps for the current platform
- Verifies commands exist in PATH
- Checks version requirements when specified
- Exits with code 0 if all pass, non-zero if any fail

#### System-Required Dependencies

When a tool depends on something tsuku cannot provide, the recipe uses the `require_system` action. This:
- Validates the command exists on your system
- Checks version requirements if specified
- Provides installation guidance if missing

Example output when Docker is missing:
```
Error: System dependency 'docker' not found

To install on macOS:
  brew install --cask docker

To install on Linux:
  See https://docs.docker.com/engine/install/ for platform-specific installation
```

#### Preview Instructions for Other Platforms

Use `--target-family` to see system dependency instructions for a different Linux distribution family:

```bash
# See Fedora/RHEL instructions while on Ubuntu
tsuku install docker --target-family rhel

# See Arch Linux instructions
tsuku install docker --target-family arch
```

Supported families: `debian`, `rhel`, `arch`, `alpine`, `suse`

#### Query Platform Support

Use `tsuku info` with JSON output to programmatically query which platforms a recipe supports:

```bash
# Get supported platforms for a recipe
tsuku info docker --metadata-only --json | jq '.supported_platforms'
```

Output includes platform objects with `os`, `arch`, and optionally `linux_family` fields.

### Multi-Version Support

tsuku supports installing and managing multiple versions of the same tool:

```bash
# Install specific versions
tsuku install nodejs@18.20.0
tsuku install nodejs@20.10.0

# List shows all installed versions with active indicator
tsuku list
#   nodejs  18.20.0
#   nodejs  20.10.0 (active)

# Switch between versions instantly
tsuku activate nodejs 18.20.0

# Remove a specific version (keeps others)
tsuku remove nodejs@18.20.0

# Remove all versions of a tool
tsuku remove nodejs
```

Key behaviors:
- **Parallel installation**: Installing a new version preserves existing versions
- **Active version**: The most recently installed or activated version is symlinked to PATH
- **Version-specific removal**: Use `tool@version` syntax to remove only that version
- **Automatic fallback**: If you remove the active version, tsuku switches to the most recently installed remaining version

### Reproducible Installations

tsuku ensures reproducible installations through installation plan caching:

- **First install**: tsuku generates a plan with exact URLs and checksums
- **Re-install**: Same version reuses the cached plan for identical results
- **Verification**: Downloaded files are verified against cached checksums

This guarantees that installing `kubectl@1.29.0` produces the same binaries across time and machines.

To force regeneration of a plan (e.g., after upstream changes):

```bash
tsuku install kubectl --fresh
```

Use `--fresh` when:
- Upstream releases are re-tagged with new assets
- You want to verify the latest artifacts
- Checksum verification fails (tsuku will suggest using `--fresh`)

#### Plan-Based Installation

For air-gapped environments or CI distribution, use explicit plan-based installation:

```bash
# Generate a plan
tsuku eval kubectl > kubectl-plan.json

# Install from the plan (on any machine)
tsuku install --plan kubectl-plan.json

# Or pipe directly
tsuku eval kubectl | tsuku install --plan -
```

See the [Plan-Based Installation Guide](docs/GUIDE-plan-based-installation.md) for air-gapped deployment and CI distribution workflows.

### Ecosystem-Native Installation

tsuku integrates with multiple package ecosystems to capture dependencies and ensure reproducible builds:

- **npm** (Node.js): Captures package.json and package-lock.json for deterministic dependency resolution
- **cargo** (Rust): Captures Cargo.lock for bit-for-bit reproducible builds
- **go** (Go): Captures go.mod and go.sum for exact version pinning
- **pip** (Python): Captures requirements.txt or pip-lock files for consistent environments
- **gem** (Ruby): Captures Gemfile.lock for reproducible Ruby tool installations
- **nix** (Nix): Leverages Nix's hermetic environment system for complex tooling
- **cpan** (Perl): Captures dependency specifications for Perl module installation

Each ecosystem integration ensures that lockfiles are captured during the plan phase, guaranteeing that subsequent installations produce identical binaries and dependencies across different machines and time.

### Security and Verification

tsuku verifies downloaded files against checksums computed during plan generation:

- **On install**: Downloaded files are verified against the cached plan's checksums
- **Mismatch detection**: If upstream assets change, tsuku fails with a checksum mismatch error
- **Recovery**: Use `--fresh` to acknowledge the change and generate a new plan

This protects against supply chain attacks and detects unauthorized re-tagging of releases.

#### Installation Verification

After installation, use `tsuku verify` to validate tool integrity:

```bash
tsuku verify kubectl
```

For libraries, verification includes multiple tiers:
- **Tier 1: Header validation** - Validates that library files are valid shared libraries (ELF or Mach-O) for the current platform
- **Tier 2: Dependency checking** - Validates dynamic library dependencies are satisfied, classifying them as:
  - System libraries (libc, libm, libSystem.B.dylib, etc.)
  - Tsuku-managed libraries (dependencies installed by tsuku)
  - Externally-managed libraries (from package managers like apt, brew)
- **Tier 3: dlopen load testing** - Loads the library with dlopen() to verify it can be dynamically loaded at runtime

Tier 2 validation also detects ABI mismatches (glibc vs musl) to catch incompatible binary combinations early.

Library verification supports additional options:

```bash
# Enable checksum verification (Tier 4)
tsuku verify gcc-libs --integrity

# Skip dlopen load testing (useful for headless environments)
tsuku verify gcc-libs --skip-dlopen
```

When a library is installed, tsuku computes and stores SHA256 checksums of all library files. The `--integrity` flag compares current checksums against these stored values to detect post-installation tampering.

### Sandbox Testing

Test installations in isolated containers to verify recipes work correctly:

```bash
# Test an installation in a sandbox container
tsuku install kubectl --sandbox

# Test a local recipe file
tsuku install --recipe ./my-recipe.toml --sandbox

# Combine with plan-based workflow
tsuku eval rg | tsuku install --plan - --sandbox
```

Sandbox testing:
- Runs installation in an isolated Docker/Podman container
- Verifies the tool installs and runs correctly
- Automatically configures network access based on recipe requirements
- Useful for testing recipes before submission or production deployment

**System dependency handling:** When a recipe declares system dependencies (e.g., `apt_install`, `dnf_install`), sandbox mode builds a custom container image with those packages pre-installed. This allows testing recipes that require system packages without modifying your host system. Container images are cached based on their dependency fingerprint for efficient re-use.

**Multi-family support:** Use `--linux-family` to test recipes on different distribution families:

```bash
# Test on Debian-based container
tsuku install cmake --sandbox --linux-family debian

# Test on Fedora-based container
tsuku install cmake --sandbox --linux-family rhel
```

For technical details, see [DESIGN-install-sandbox.md](docs/DESIGN-install-sandbox.md).

## Testing

tsuku has comprehensive test coverage for critical components:

- **State Management**: 30.1% coverage - Load/Save, UpdateTool, RequiredBy tracking
- **Recipe Parsing**: 28.1% coverage - TOML unmarshaling, dependencies, steps
- **Dependency Logic**: Tests for circular detection, orphan cleanup, resolution

### Running Tests

```bash
# Run all tests
go test ./...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific package
go test ./internal/install
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed testing guide and best practices.

## Development Environment

### Using Docker (Recommended)

The fastest way to get an isolated development environment:

```bash
# Install Docker Engine (one-time setup - see DOCKER.md for full instructions)
# Quick install:
sudo apt update && sudo apt install ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/ubuntu \
$(. /etc/os-release && echo "$VERSION_CODENAME") stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt update && sudo apt install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
sudo usermod -aG docker $USER
# Logout and login for group change to take effect

# Start interactive shell in container
./docker-dev.sh shell

# Inside container: build and test
go build -o tsuku ./cmd/tsuku
./test-npm-install.sh
./tsuku install serve

# Your code changes on the host are instantly visible!
```

**Benefits:**
- **No Secure Boot issues** (no kernel modules required)
- **Fast** - starts in ~2 seconds
- **Lightweight** - uses minimal RAM and disk
- **Clean environment** (no npm, Python, or Rust pre-installed)
- **Perfect for testing** auto-bootstrap features

### Local Development

If you prefer to develop directly on your host:

```bash
# Build
go build -o tsuku ./cmd/tsuku

# Run tests
go test ./...

# Test npm integration
./test-npm-install.sh
```

**Note:** Testing auto-bootstrap features locally may install Node.js, Python, or Rust on your system.