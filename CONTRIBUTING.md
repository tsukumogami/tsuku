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

### Testing Build Essentials

Build essential recipes (compilers, libraries) require additional validation beyond standard tests. Three validation scripts ensure build essentials work correctly across platforms:

**verify-tool.sh** - Functional verification
```bash
# Ensures the tool runs correctly
./test/scripts/verify-tool.sh zlib
./test/scripts/verify-tool.sh make
```

**verify-relocation.sh** - Relocation verification
```bash
# Ensures no hardcoded paths in binaries
# Linux: RPATH uses $ORIGIN, no /usr/local or /home/linuxbrew
# macOS: install_name uses @rpath, no /opt/homebrew hardcoding
./test/scripts/verify-relocation.sh zlib
```

**verify-no-system-deps.sh** - Self-containment verification
```bash
# Ensures tool uses only tsuku-provided deps
# Linux: ldd shows only $TSUKU_HOME paths and libc
# macOS: otool -L shows only @rpath and system frameworks
./test/scripts/verify-no-system-deps.sh zlib
```

Example workflow for testing a build essential recipe:
```bash
# Build and install
go build -o tsuku ./cmd/tsuku
./tsuku install zlib

# Run all three validation scripts
./test/scripts/verify-tool.sh zlib
./test/scripts/verify-relocation.sh zlib
./test/scripts/verify-no-system-deps.sh zlib
```

See `.github/workflows/build-essentials.yml` for the complete validation matrix (3 platforms: Linux x86_64, macOS Intel, macOS ARM).

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

### Using Recipe Builders

For tools from supported package ecosystems, you can generate a recipe automatically:

```bash
# Generate a recipe from crates.io, rubygems, pypi, npm, GitHub releases, or Homebrew
tsuku create <tool> --from <ecosystem>

# Example: generate a recipe for a Rust tool
tsuku create bat --from crates.io

# Example: generate a recipe from Homebrew
tsuku create jq --from homebrew:jq
```

Generated recipes are stored in `$TSUKU_HOME/recipes/` and can be used as a starting point. If you want to contribute the recipe to the registry, copy and adapt it to `internal/recipe/recipes/`.

For tools not in supported ecosystems, or when you need more control, create a recipe manually.

### Recipe Format

Recipes are TOML files with the following structure:

```toml
[metadata]
name = "tool-name"
description = "Brief description"
homepage = "https://example.com"
version_format = "semver"

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

### Version Inference

Many actions automatically infer the version source from their parameters, so an explicit `[version]` section is often unnecessary:

| Action | Inferred Source | Parameter Used |
|--------|----------------|----------------|
| `cargo_install` | `crates_io` | `crate` |
| `pipx_install` | `pypi` | `package` |
| `npm_install` | `npm` | `package` |
| `gem_install` | `rubygems` | `gem` |
| `cpan_install` | `metacpan` | `distribution` |
| `github_archive` | `github_releases` | `repo` |
| `github_file` | `github_releases` | `repo` |
| `go_install` | `goproxy` | `module` |

**When to add `[version]`:**

- **Different source**: When version comes from a different source than the action implies (e.g., using GitHub releases for version but installing from crates.io)
- **go_install edge cases**: When the install path doesn't follow common patterns (see below)
- **download_archive**: Always requires explicit version configuration

**go_install automatic module inference:**

For `go_install`, tsuku automatically infers the version module from the install path using these patterns:

| Pattern | Install Path | Inferred Version Module |
|---------|-------------|------------------------|
| GitHub repos | `github.com/<owner>/<repo>/...` | `github.com/<owner>/<repo>` |
| `/cmd/` convention | `some.url/path/cmd/tool` | `some.url/path` |
| No pattern | `mvdan.cc/gofumpt` | `mvdan.cc/gofumpt` (unchanged) |

**Example - go_install (simple case, no `[version]` needed):**

```toml
[[steps]]
action = "go_install"
module = "mvdan.cc/gofumpt"
executables = ["gofumpt"]
```

**Example - go_install (GitHub pattern, no `[version]` needed):**

```toml
# Version automatically inferred from github.com/go-delve/delve
[[steps]]
action = "go_install"
module = "github.com/go-delve/delve/cmd/dlv"
executables = ["dlv"]
```

**Example - go_install (/cmd/ pattern, no `[version]` needed):**

```toml
# Version automatically inferred from honnef.co/go/tools
[[steps]]
action = "go_install"
module = "honnef.co/go/tools/cmd/staticcheck"
executables = ["staticcheck"]
```

**Example - go_install (edge case, `module` required):**

When the install path doesn't match any pattern, specify the version module explicitly:

```toml
[version]
module = "go.uber.org/mock"  # Version module differs from install path

[[steps]]
action = "go_install"
module = "go.uber.org/mock/mockgen"  # No /cmd/, not on github.com
executables = ["mockgen"]
```

**Example - override version source:**

```toml
# Version from GitHub, install from crates.io
[version]
source = "github_releases"
github_repo = "cargo-bins/cargo-binstall"

[[steps]]
action = "cargo_install"
crate = "cargo-binstall"
```

Running `tsuku validate --strict` will warn if a `[version]` section duplicates what would be inferred automatically.

### Testing Recipes

Before submitting a recipe, test it both locally and in a sandbox container:

```bash
# Build tsuku
go build -o tsuku ./cmd/tsuku

# Test local installation
./tsuku install <tool-name>

# Test in isolated container (requires Docker or Podman)
./tsuku install <tool-name> --sandbox
```

The `--sandbox` flag runs the installation in an isolated container to verify:
- The recipe works in a clean environment
- Dependencies are correctly declared
- Verification commands work as expected

If Docker/Podman is unavailable (e.g., in some CI environments), you can skip sandbox testing:

```bash
tsuku create <tool> --from <source> --skip-sandbox
```

**Note:** Always test with sandbox when possible, as it catches environment-specific issues.

### Submitting Recipes

1. Create your recipe in `internal/recipe/recipes/<first-letter>/<tool-name>.toml`
2. Test locally:
   ```bash
   go build -o tsuku ./cmd/tsuku
   ./tsuku install <tool-name>
   ./tsuku install <tool-name> --sandbox  # Test in isolated container
   ```
3. Submit a PR to this repository

See the existing recipes in `internal/recipe/recipes/` for examples.

## Golden File Testing

Golden files are pre-generated installation plans stored in `testdata/golden/plans/`. They serve as regression tests: when you change a recipe or tsuku's plan generation code, CI verifies the changes produce expected results.

### Why Golden Files?

1. **Regression detection**: Any code change that affects plan generation is immediately visible in PR diffs
2. **Recipe validation**: Recipe authors see exactly what their recipe produces before merging
3. **Cross-platform coverage**: Plans are generated for all supported platforms (linux-amd64, darwin-amd64, darwin-arm64)

### Local Workflow

When you modify a recipe:

```bash
# Build tsuku
go build -o tsuku ./cmd/tsuku

# Validate golden files (shows diff if out of date)
./scripts/validate-golden.sh <recipe>

# If validation fails, regenerate
./scripts/regenerate-golden.sh <recipe>

# Review the changes
git diff testdata/golden/plans/

# Test execution locally (current platform only)
./tsuku install --plan testdata/golden/plans/<letter>/<recipe>/<version>-<os>-<arch>.json --sandbox

# Commit both recipe and golden file changes
git add internal/recipe/recipes/<letter>/<recipe>.toml testdata/golden/plans/<letter>/<recipe>/
git commit -m "feat(recipe): update <recipe>"
```

When you modify tsuku code (executor, actions, etc.):

```bash
# Validate ALL golden files
./scripts/validate-all-golden.sh

# If validation fails, regenerate affected recipes
./scripts/regenerate-golden.sh <recipe>

# Or regenerate with constraints
./scripts/regenerate-golden.sh <recipe> --os linux --arch amd64
./scripts/regenerate-golden.sh <recipe> --version v1.2.3
```

### Cross-Platform Generation

Developers often work on a single platform but need golden files for all platforms. The **Generate Golden Files** workflow solves this:

**Manual trigger (GitHub Actions UI):**
1. Go to Actions → "Generate Golden Files" → Run workflow
2. Enter the recipe name
3. Optionally enable "Commit results back to current branch"
4. Workflow runs on Linux, macOS Intel, and macOS Apple Silicon in parallel
5. Golden files are committed to your branch (or download artifacts manually)

**Programmatic trigger (from another workflow):**

```yaml
jobs:
  generate:
    uses: ./.github/workflows/generate-golden-files.yml
    with:
      recipe: fzf
      commit_back: true
      branch: ${{ github.head_ref }}
```

**CLI trigger:**

```bash
gh workflow run generate-golden-files.yml \
  -f recipe=fzf \
  -f commit_back=true \
  -f branch=my-feature-branch
```

### Platform Coverage

| Platform | Plan Generation | Execution Validation | CI Runner |
|----------|-----------------|---------------------|-----------|
| linux-amd64 | Yes | Yes | ubuntu-latest |
| darwin-arm64 | Yes | Yes | macos-14 |
| darwin-amd64 | Yes | Yes | macos-15-intel |
| linux-arm64 | No | No | None available |

linux-arm64 is excluded because GitHub Actions doesn't provide arm64 Linux runners.

### Golden File Families

Golden files support Linux family-specific plans for recipes that vary by distribution.

**Family-agnostic recipes** use actions like `download`, `github_archive`, or `go_install` that work identically on any Linux distribution. Their plans do not include a `linux_family` field and produce a single Linux golden file:

```
testdata/golden/plans/f/fzf/
├── v0.60.0-linux-amd64.json
├── v0.60.0-darwin-amd64.json
└── v0.60.0-darwin-arm64.json
```

**Family-aware recipes** use package manager actions (`apt_install`, `dnf_install`, `pacman_install`, `apk_install`, `zypper_install`) or `{{linux_family}}` interpolation in parameters. Their plans include `linux_family` in the platform object and produce five Linux golden files (one per supported family):

```
testdata/golden/plans/b/build-tools-system/
├── v1.0.0-linux-debian-amd64.json
├── v1.0.0-linux-rhel-amd64.json
├── v1.0.0-linux-arch-amd64.json
├── v1.0.0-linux-alpine-amd64.json
├── v1.0.0-linux-suse-amd64.json
├── v1.0.0-darwin-amd64.json
└── v1.0.0-darwin-arm64.json
```

**File naming conventions:**

| Recipe Type | Pattern | Example |
|-------------|---------|---------|
| Family-agnostic | `{version}-{os}-{arch}.json` | `v0.60.0-linux-amd64.json` |
| Family-aware | `{version}-{os}-{family}-{arch}.json` | `v1.0.0-linux-debian-amd64.json` |

Supported families: `debian`, `rhel`, `arch`, `alpine`, `suse`

**Regenerating golden files:**

The regeneration scripts automatically detect whether a recipe is family-aware by querying `tsuku info --metadata-only`:

```bash
# Family-agnostic recipe - produces 1 Linux file
./scripts/regenerate-golden.sh fzf --os linux --arch amd64

# Family-aware recipe - produces 5 family-specific Linux files
./scripts/regenerate-golden.sh build-tools-system --os linux --arch amd64
```

**Recipe transition handling:**

When a recipe's family awareness changes (e.g., adding `apt_install` to a previously family-agnostic recipe):

- Old `linux-amd64.json` file is deleted
- Five new family-specific files are created
- PR diff shows 1 deletion and 5 additions

The reverse transition (removing family-aware actions) shows 5 deletions and 1 addition.

For technical details on family detection logic and platform metadata, see [docs/DESIGN-golden-family-support.md](docs/DESIGN-golden-family-support.md).

### Design Reference

For the complete design rationale, validation workflows, and security considerations, see [docs/DESIGN-golden-plan-testing.md](docs/DESIGN-golden-plan-testing.md).

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

## Code Organization

### File Size Guidelines

Keep individual files focused and reasonably sized to reduce merge conflicts and improve maintainability:

- **Target**: 200-400 lines per file
- **Maximum**: 600 lines before considering a split
- **Indicator**: If a file regularly causes merge conflicts, consider splitting it

### When to Split Files

Split a file when:
1. It has multiple distinct responsibilities (violates single-responsibility principle)
2. Different parts change at different rates (high churn in one area)
3. It causes frequent merge conflicts
4. It's difficult to navigate or understand

### How to Split Files

1. **Identify functional boundaries**: Group related functions, types, and constants
2. **Preserve cohesion**: Keep tightly coupled code together
3. **Follow Go conventions**:
   - One package per directory
   - Related types and their methods in the same file
   - Test files next to implementation (`foo.go` and `foo_test.go`)
4. **Use clear naming**: File names should describe their contents (`state_tool.go`, `state_lib.go`)

### Refactoring Patterns

Common patterns used in this codebase:

- **Facade pattern**: Main file delegates to specialized modules (see `internal/version/resolver.go`)
- **Functional options**: Replace multiple constructors with `With*` functions
- **Type-per-file**: Large types with many methods get their own file

### Package Structure

```
internal/
  package/
    types.go        # Core types shared across the package
    foo.go          # Implementation for foo functionality
    foo_test.go     # Tests for foo.go
    bar.go          # Implementation for bar functionality
    bar_test.go     # Tests for bar.go
```

## Getting Help

- Open an issue for bugs or feature requests
- Check existing issues before creating new ones
- See [README.md](README.md) for project overview
