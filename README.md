# tsuku

[![Tests](https://github.com/tsuku-dev/tsuku/actions/workflows/test.yml/badge.svg)](https://github.com/tsuku-dev/tsuku/actions/workflows/test.yml)
[![codecov](https://codecov.io/gh/tsuku-dev/tsuku/graph/badge.svg)](https://codecov.io/gh/tsuku-dev/tsuku)

A modern, universal package manager for development tools.

## Overview

tsuku is a package manager that makes it easy to install and manage development tools across different platforms. It uses action-based recipes to download, extract, and install tools to version-specific directories with automatic PATH management.

## Features

- **Action-based recipes**: Composable actions for downloading, extracting, and installing tools
- **Version management**: Tools installed in version-specific directories
- **Automatic PATH management**: Shell integration for easy access
- **Dependency management**: Automatic installation and cleanup of tool dependencies
- **Package manager integration**: npm_install action for npm tools (pip/cargo pending)
- **No dependencies**: Single binary, no system prerequisites

## Installation

```bash
go build -o tsuku ./cmd/tsuku
sudo mv tsuku /usr/local/bin/
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

## Development Status

- [x] Phase 0.1: Recipe Format Validation
- [x] Phase 0.2: Production CLI Implementation
- [x] Phase 0.3: Recipe Collection & Validation
- [x] Phase 0.4: Dependency Management
  - Automatic dependency installation
  - State tracking (explicit vs. auto-installed)
  - Dependency protection
  - Orphan cleanup
  - Circular dependency detection
- [x] Phase 0.5: Testing Infrastructure
  - Unit tests for critical components
  - CI/CD with GitHub Actions
  - Code coverage tracking (30%+ coverage)
- [ ] Phase 0.6: Package Manager Integration (in progress)
  - npm_install action implemented and working
  - pip_install action (Phase 0.6.3 - planned)
  - cargo_install action (Phase 0.6.4 - planned)

## Architecture

```
tsuku/
├── cmd/tsuku/          # CLI entry point
├── internal/
│   ├── cli/           # Command implementations
│   ├── recipe/        # Recipe loading and management
│   ├── executor/      # Action execution engine
│   └── config/        # Configuration management
└── recipes/           # Bundled recipes (embedded in binary)
```

## License

MIT