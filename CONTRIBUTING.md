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

# Build for development (uses .tsuku-dev as home directory)
make build

# Run tests
make test

# Clean build artifacts and dev data
make clean
```

The `make build` target produces a `tsuku` binary that defaults to `.tsuku-dev/`
in the current directory instead of `~/.tsuku`. This keeps development state
isolated from any production installation. The `TSUKU_HOME` environment variable
still takes precedence if set.

To configure your shell for the dev binary (so installed tools are on PATH):

```bash
eval $(./tsuku shellenv)
```

To verify your environment is set up correctly:

```bash
./tsuku doctor
```

### Production Build

To build without the dev directory override (same as release binaries):

```bash
go build -o tsuku ./cmd/tsuku
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

Tsuku has three recipe directories, each serving a different purpose:

| Directory | Purpose | Embedded in Binary | When to Use |
|-----------|---------|-------------------|-------------|
| `internal/recipe/recipes/` | Action dependencies | Yes | Only recipes in [EMBEDDED_RECIPES.md](docs/EMBEDDED_RECIPES.md) |
| `recipes/` | User-installable tools | No (fetched from registry) | Most new recipes |
| `testdata/recipes/` | CI feature coverage | Yes | Testing package manager actions |

### Choosing the Right Directory

Use this flowchart to determine where your recipe belongs:

```
Is this recipe required by a tsuku action?
└─ Yes → Check docs/EMBEDDED_RECIPES.md
   └─ Listed there? → Embedded (internal/recipe/recipes/)
   └─ Not listed? → Open issue - action dependency missing
└─ No → Is this recipe for integration testing only?
   └─ Yes → testdata/recipes/
   └─ No → Registry (recipes/)
```

**Most contributors should add recipes to `recipes/`** - the registry directory. Embedded recipes are reserved for action dependencies (Go, Rust, Node.js, etc.) that tsuku needs to bootstrap itself.

For the complete list of embedded recipes and their rationale, see [docs/EMBEDDED_RECIPES.md](docs/EMBEDDED_RECIPES.md).

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

1. Determine the correct directory using the flowchart above
2. Create your recipe in the appropriate location:
   - Registry recipes: `recipes/<first-letter>/<tool-name>.toml`
   - Embedded recipes: `internal/recipe/recipes/<tool-name>.toml` (only if listed in EMBEDDED_RECIPES.md)
3. Test locally:
   ```bash
   go build -o tsuku ./cmd/tsuku
   ./tsuku install <tool-name>
   ./tsuku install <tool-name> --sandbox  # Test in isolated container
   ```
4. Submit a PR to this repository

See the existing recipes in `recipes/` for examples of registry recipes.

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

For technical details on family detection logic and platform metadata, see [docs/designs/current/DESIGN-golden-plan-testing.md](docs/designs/current/DESIGN-golden-plan-testing.md).

### CI Validation Workflows

These workflows run automatically on pull requests:

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `validate-golden-recipes.yml` | Recipe file changes | Validates golden files for changed recipes |
| `validate-golden-code.yml` | Plan generation code changes | Validates ALL golden files when core code changes |
| `validate-golden-execution.yml` | Golden file changes | Executes plans on platform matrix to verify downloads |

### Design Reference

For the complete design rationale, validation workflows, and security considerations, see [docs/DESIGN-golden-plan-testing.md](docs/DESIGN-golden-plan-testing.md).

## System Dependency Actions

Some recipes require packages from the system's package manager (apt, brew, dnf, etc.). Tsuku provides typed actions for these cases.

### When to Use System Dependency Actions

Use system dependency actions when:
- A tool requires external binaries that cannot be provisioned by tsuku (e.g., CUDA drivers, Docker)
- A tool needs libraries or packages only available via system package managers
- You want to provide platform-specific installation instructions

**Do NOT use** system dependency actions when tsuku can install the tool directly (download, homebrew bottle, cargo_install, etc.).

### Available Actions

| Action | Purpose | Implicit Platform Constraint |
|--------|---------|------------------------------|
| `apt_install` | Install packages via apt-get | `linux_family = "debian"` |
| `apt_repo` | Add apt repository with GPG key | `linux_family = "debian"` |
| `apt_ppa` | Add Ubuntu PPA repository | `linux_family = "debian"` |
| `dnf_install` | Install packages via dnf | `linux_family = "rhel"` |
| `dnf_repo` | Add dnf repository | `linux_family = "rhel"` |
| `pacman_install` | Install packages via pacman | `linux_family = "arch"` |
| `apk_install` | Install packages via apk | `linux_family = "alpine"` |
| `zypper_install` | Install packages via zypper | `linux_family = "suse"` |
| `brew_install` | Install Homebrew formula | `os = "darwin"` |
| `brew_cask` | Install Homebrew cask | `os = "darwin"` |
| `require_command` | Verify a command exists | None (runs on all platforms) |
| `manual` | Display manual instructions | None (runs on all platforms) |

### Implicit Constraints

Package manager actions have **implicit platform constraints** that are automatically applied. You don't need to add a `when` clause for these:

```toml
# apt_install automatically applies linux_family = "debian"
# No need for when = { linux_family = "debian" }
[[steps]]
action = "apt_install"
packages = ["docker.io"]
```

The implicit constraints are:
- **Debian family** (Ubuntu, Debian, Linux Mint): `apt_*` actions
- **RHEL family** (Fedora, RHEL, CentOS, Rocky): `dnf_*` actions
- **Arch family** (Arch, Manjaro): `pacman_install`
- **Alpine family**: `apk_install`
- **SUSE family** (openSUSE, SLES): `zypper_install`
- **macOS**: `brew_*` actions

### Action Parameters

**Package installation actions** (`apt_install`, `dnf_install`, etc.):
```toml
[[steps]]
action = "apt_install"
packages = ["package1", "package2"]  # Required: list of package names
```

**Repository actions** (`apt_repo`, `dnf_repo`):
```toml
[[steps]]
action = "apt_repo"
url = "https://example.com/ubuntu"       # Required: repository URL
key_url = "https://example.com/key.gpg"  # Required: GPG key URL
key_sha256 = "abc123..."                 # Required: SHA256 of GPG key
```

**Verification action** (`require_command`):
```toml
[[steps]]
action = "require_command"
command = "docker"                       # Required: command to check
version_flag = "--version"               # Optional: flag to get version
version_regex = "version ([0-9.]+)"      # Optional: regex to extract version
min_version = "20.10.0"                  # Optional: minimum required version
```

**Manual instructions** (`manual`):
```toml
[[steps]]
action = "manual"
text = "Visit https://example.com for installation instructions"
```

### Recipe Examples

**Simple: Single platform requirement**
```toml
[metadata]
name = "cuda"
description = "NVIDIA CUDA Toolkit"
supported_os = ["linux"]

[[steps]]
action = "manual"
text = "Visit https://developer.nvidia.com/cuda-downloads for installation"

[[steps]]
action = "require_command"
command = "nvcc"
version_flag = "--version"
version_regex = "release ([0-9.]+)"
min_version = "11.0"

[verify]
command = "nvcc --version"
pattern = "release {version}"
```

**Multi-platform: Different package managers**
```toml
[metadata]
name = "docker"
description = "Docker container runtime"

# macOS via Homebrew Cask
[[steps]]
action = "brew_cask"
packages = ["docker"]

# Debian/Ubuntu
[[steps]]
action = "apt_install"
packages = ["docker.io"]

# Fedora/RHEL
[[steps]]
action = "dnf_install"
packages = ["docker-ce"]

# Verify on all platforms
[[steps]]
action = "require_command"
command = "docker"

[verify]
command = "docker --version"
pattern = "{version}"
```

### Testing System Dependency Recipes

Test recipes with system dependencies using the `--sandbox` flag with family specification:

```bash
# Build tsuku
go build -o tsuku ./cmd/tsuku

# Test on different Linux families
./tsuku install <recipe> --sandbox --linux-family debian
./tsuku install <recipe> --sandbox --linux-family rhel
./tsuku install <recipe> --sandbox --linux-family arch
./tsuku install <recipe> --sandbox --linux-family alpine
./tsuku install <recipe> --sandbox --linux-family suse

# Test macOS actions (on macOS only)
./tsuku install <recipe> --sandbox
```

**Verification commands:**

```bash
# Check system dependencies for a recipe
./tsuku check-deps <recipe>

# Verify installed dependencies
./tsuku verify-deps <recipe>
```

## Troubleshooting

### Recipe Works Locally But Fails in CI

**Symptom**: `tsuku install <tool>` works on your machine but CI fails with "recipe not found"

**Causes**:
1. Recipe in wrong directory (registry recipe but CI expects embedded)
2. Missing from EMBEDDED_RECIPES.md (if action dependency)
3. Network timeout during registry fetch

**Solutions**:
1. Check if your recipe is an action dependency (see [docs/EMBEDDED_RECIPES.md](docs/EMBEDDED_RECIPES.md)) - if so, use `internal/recipe/recipes/`
2. For registry recipes, ensure CI has network access to GitHub
3. Run `./tsuku install --verbose <tool>` to see fetch attempts

### Recipe Not Found (Network Issues)

**Error messages**:
- "Could not reach recipe registry. Check your internet connection."
- "Recipe may be stale (cached X hours ago)"

**Solutions**:
1. Check internet connectivity
2. Run `tsuku update-registry` to refresh cache
3. Use `tsuku install --fresh <tool>` to bypass cache
4. For offline use, pre-cache recipes or use local overrides in `$TSUKU_HOME/recipes/`

### Linter Failures

If golangci-lint fails in CI:
- Check the CI logs for specific issues
- Update golangci-lint: `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`

### Test Failures

If tests fail:
- Check test logs for specific failures
- Run with race detection: `go test -race ./...`
- Check if the failure is in CI-specific environment

## Nightly Registry Validation

Registry recipes are validated nightly to catch external drift (URL changes, version rot, upstream breakage).

### How It Works

- **Schedule**: Daily at 2 AM UTC
- **Scope**: All recipes in `recipes/` directory
- **Actions**: Generates fresh plans and compares against stored golden files
- **Reporting**: Creates a GitHub issue on failure with the list of broken recipes

### What Contributors Should Do

1. **Subscribe to notifications**: Watch the repository to receive nightly failure issue notifications
2. **Check nightly status before major changes**: If nightly validation is failing, your recipe PR may be blocked
3. **Reference nightly issues when fixing recipes**: Link to the nightly failure issue when submitting a fix

### Why This Matters

Unlike embedded recipes (validated on every code change), registry recipes are only validated when they change or during nightly runs. This means:
- External changes (upstream URL moves, new release formats) are caught within 24 hours
- If your recipe suddenly fails, check if upstream made changes

## Security Incident Response

This section outlines the response procedures for a repository compromise affecting recipes.

### Detection

Signs of a potential compromise:
- Unexpected recipe changes in git history (malicious URLs, checksums)
- User reports of suspicious behavior after `tsuku install`
- Automated alerts from commit signature verification (if enabled)
- Unusual patterns in CI logs or workflow runs

### Immediate Actions

1. **Revert malicious commits**: Use `git revert` to remove the compromised content from main branch
2. **Post security advisory**: Create a GitHub security advisory with affected recipes and timeframe
3. **Notify users**: Instruct users to clear their cache:
   ```bash
   rm -rf $TSUKU_HOME/registry/
   ```

### Recovery Steps

1. **Audit changes**: Review all recipe changes since last known good state
2. **Verify embedded recipes**: Compare embedded recipes against known-good checksums
3. **Issue emergency CLI release**: If embedded recipes were affected, release a new CLI version immediately
4. **Document timeline**: Create a post-mortem with:
   - Timeline of compromise
   - Affected recipes
   - Impact assessment
   - Lessons learned

### Prevention

- **Branch protection**: Require signed commits for recipe changes
- **Review all recipe changes**: Check URLs and checksums for suspicious patterns
- **Monitor for unexpected changes**: Set up alerts for recipe file modifications outside normal PR flow
- **Regular credential rotation**: Rotate any secrets used in CI workflows quarterly

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

## Dependency Installation Consent Model

When a tsuku command needs to install a dependency (toolchain, runtime, etc.) as a side effect of the user's request, the consent model depends on which command is running:

### `tsuku install`: auto-install without prompting

The `install` command auto-installs dependencies silently. Users running `tsuku install` expect software to be installed, so installing prerequisites (declared in `Metadata.Dependencies` and `Metadata.RuntimeDependencies`) doesn't require confirmation. This is handled by `installWithDependencies()` in `install_deps.go`.

### All other commands: prompt or require `--yes`

Commands like `create`, `eval`, and any future command that might need to install dependencies should **always prompt the user** in interactive mode and **auto-install only with `--yes`**. Users running these commands don't necessarily expect new software to be installed, so explicit consent is required.

Example from `eval.go`: `installEvalDeps()` prompts with `[y/N]` during standalone `tsuku eval`, but auto-installs when called from `tsuku install` (which passes `autoAccept=true`).

### When to prompt

| Context | Behavior |
|---------|----------|
| `tsuku install` installing a dependency | Auto-install, no prompt |
| Any other command installing a dependency | Prompt in interactive mode |
| Any command with `--yes` | Auto-install, no prompt |

### Security-sensitive prompts

Prompts for security-sensitive actions (checksum bypass, sandbox skip) always require explicit confirmation regardless of `--yes`. These use `--force` instead.

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
