# Contributing to tsuku

Thank you for your interest in contributing to tsuku! This document provides guidelines and workflows for development.

## Development Setup

### Prerequisites

- Go 1.24 or later (check `go.mod` for exact version)
- Git
- golangci-lint (optional, for local linting)

### Build and Run

```bash
# Clone the repository
git clone https://github.com/tsukumogami/tsuku.git
cd tsuku

# Build
go build -o tsuku ./cmd/tsuku

# Install locally (optional)
go install ./cmd/tsuku
```

### Verify Setup

```bash
# Check the build works
./tsuku --help

# Run tests
go test ./...
```

## Testing

### Unit Tests

Run the full test suite:

```bash
go test ./...
```

Run tests with race detection:

```bash
go test -race ./...
```

Run a specific package:

```bash
go test ./internal/install
```

### Coverage

Generate and view coverage:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

CI tracks coverage through Codecov. Contributions should maintain or improve coverage.

### Integration Tests

Integration tests run in CI on both Linux and macOS. They install actual tools using tsuku to verify end-to-end functionality.

To run integration tests locally:

```bash
# Build tsuku first
go build -o tsuku ./cmd/tsuku

# Add tsuku bin directory to PATH
export PATH="$HOME/.tsuku/bin:$PATH"

# Test a specific tool installation
./tsuku install gh
gh --version
```

## Code Style

### Formatting

All Go code must be formatted with `gofmt`:

```bash
gofmt -w .
```

### Linting

The project uses golangci-lint:

```bash
# Quick check (catches most issues)
go vet ./...

# Full lint (if golangci-lint is installed)
golangci-lint run --timeout=5m ./...
```

See `.golangci.yaml` for the full linter configuration.

### Commit Messages

Follow conventional commit format:

```
<type>(<scope>): <description>

<body>
```

Types:
- `feat`: New functionality
- `fix`: Bug fixes
- `refactor`: Code changes that neither fix bugs nor add features
- `test`: Adding or updating tests
- `docs`: Documentation changes
- `chore`: Maintenance tasks

Examples:
```
feat(install): add support for npm packages
fix(version): handle pre-release version parsing
docs(readme): update installation instructions
```

Reference issue numbers in the body when applicable: `Fixes #123`

## Pull Request Process

### Branch Naming

Use descriptive branch names with a prefix:

- `feature/<N>-<description>` - New functionality
- `fix/<N>-<description>` - Bug fixes
- `docs/<N>-<description>` - Documentation
- `chore/<N>-<description>` - Maintenance

Where `<N>` is the issue number.

Examples:
```
feature/42-npm-install-action
fix/55-version-parsing-edge-case
docs/10-contributing-guide
```

### Creating a Pull Request

1. Fork the repository (or create a branch if you have write access)
2. Make your changes following the code style guidelines
3. Write or update tests as needed
4. Ensure all checks pass locally:
   ```bash
   go vet ./...
   go test ./...
   go build ./cmd/tsuku
   ```
5. Push your branch and create a PR

### PR Description

Include:
- Summary of changes
- Related issue number (e.g., "Fixes #42")
- Test plan or verification steps

### Review Process

- All PRs require review before merge
- CI checks must pass (lint, tests)
- Address reviewer feedback

Monitor PR status:
```bash
gh pr checks --watch
```

## Adding Recipes

Recipes are embedded in the monorepo at `internal/recipe/recipes/`.

### Recipe Format

Recipes are TOML files with the following structure:

```toml
[metadata]
name = "tool-name"
description = "Brief description"
homepage = "https://example.com"
version_format = "semver"

[version]
github_repo = "owner/repo"

[[steps]]
action = "github_archive"
repo = "owner/repo"
asset_pattern = "tool_{version}_{os}_{arch}.tar.gz"
format = "tar.gz"
strip_dirs = 1
binaries = ["tool-name"]

[verify]
command = "{binary} --version"
pattern = "{version}"
```

### Submitting Recipes

1. Create your recipe in `internal/recipe/recipes/<first-letter>/<tool-name>.toml`
2. Test locally:
   ```bash
   go build -o tsuku ./cmd/tsuku
   ./tsuku install <tool-name>
   ```
3. Submit a PR to this repository

See the existing recipes in `internal/recipe/recipes/` for examples.

## Troubleshooting

### Linter Failures

If golangci-lint fails in CI:
- Check the CI logs for specific issues
- Update golangci-lint: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`

### Test Failures

If tests fail:
- Check test logs for specific failures
- Run with race detection: `go test -race ./...`
- Check if the failure is in CI-specific environment

## Releases

Releases are automated via GitHub Actions using GoReleaser.

### Creating a Release

To create a new release, push a version tag:

```bash
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

The release workflow triggers only on tags matching `v*` (e.g., `v0.1.0`, `v1.0.0-beta.1`). Regular tags that don't start with `v` will not trigger a release.

### What Gets Built

Each release automatically builds binaries for:
- linux/amd64
- linux/arm64
- darwin/amd64
- darwin/arm64

The release includes:
- Binary files named `tsuku-{os}-{arch}`
- SHA256 checksums in `checksums.txt`
- Changelog generated from commit messages

### Pre-releases

Tags containing a hyphen after the version (e.g., `v1.0.0-rc.1`, `v0.2.0-beta`) are automatically marked as pre-releases on GitHub.

## Getting Help

- Open an issue for bugs or feature requests
- Check existing issues before creating new ones
- See [README.md](README.md) for project overview
