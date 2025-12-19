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
```

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

tsuku automatically provides build tools and libraries needed for source builds, eliminating the need for system dependencies:

- **Compilers**: zig (C/C++ via zig cc fallback when system compiler unavailable)
- **Build tools**: make, pkg-config, cmake, autoconf, automake
- **Common libraries**: zlib, openssl, ncurses, readline

When you install a tool that requires compilation, tsuku automatically installs the necessary build dependencies. No manual setup required.

Example:
```bash
# Build gdbm from source - tsuku provides make and zig automatically
tsuku install gdbm-source
```

Build essentials are installed to `$TSUKU_HOME/tools/` just like any other tool and are subject to the same dependency management rules.

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