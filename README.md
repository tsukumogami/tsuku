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
- **Tool shell integration**: Recipes can register shell functions and completions — loaded automatically via `$TSUKU_HOME/env` at shell startup
- **No dependencies**: Single binary, no system prerequisites

## Installation

```bash
curl -fsSL https://get.tsuku.dev/now | bash
```

The installer downloads the latest release binary, verifies its checksum, and configures your shell. It also registers the command-not-found hook for your shell automatically so you get install hints when you type an unknown command.

### Installer flags

| Flag | Description |
|------|-------------|
| `--no-modify-path` | Skip adding tsuku to PATH in shell config files |
| `--no-hooks` | Skip registering the command-not-found hook |
| `--no-telemetry` | Opt out of anonymous usage statistics |

Pass flags by piping through `bash`:

```bash
# Don't modify PATH or register the hook
curl -fsSL https://get.tsuku.dev/now | bash -s -- --no-modify-path --no-hooks
```

**Hook registration details:** By default, the installer detects your shell from `$SHELL` and runs `tsuku hook install --shell=<shell>` for bash, zsh, or fish. If `$SHELL` is unset or points to an unsupported shell, the installer warns and skips hook registration without failing. Pass `--no-hooks` to skip this step intentionally — you can register the hook later with:

```bash
tsuku hook install
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

### Self-Update

tsuku keeps itself up to date automatically. During regular background update checks, it downloads and applies new versions of itself. On the next invocation after a successful update, you'll see a brief notice: "tsuku has been updated to vX.Y.Z".

To update manually:

```bash
tsuku self-update
```

Self-update is enabled by default. To disable it:

```bash
tsuku config set updates.self_update false
```

In CI environments (`CI=true`), self-update is suppressed automatically. You can also disable it per-invocation with `TSUKU_NO_SELF_UPDATE=1`.

### Update Notifications

tsuku shows brief notifications on stderr when updates are relevant:

- **Applied updates:** When auto-apply is enabled, you'll see `Updated <tool> 1.2.0 -> 1.3.0` for each tool updated in the background.
- **Failed updates:** If a background update fails (and gets rolled back), tsuku tells you what happened and how to recover.
- **Self-update results:** After tsuku updates itself, the next invocation shows the new version.
- **Available updates:** When auto-apply is off, tsuku shows `N updates available. Run 'tsuku update' to apply.` once per check cycle.

Notifications are suppressed automatically in these contexts:

| Condition | Rationale |
|-----------|-----------|
| `CI=true` | CI pipelines shouldn't see update noise |
| Non-TTY stdout | Piped or scripted output stays clean |
| `--quiet` / `-q` flag | User asked for silence |
| `TSUKU_NO_UPDATE_CHECK=1` | Explicit opt-out of all update behavior |

To force notifications in a suppressed environment (for example, a CI job that should auto-update):

```bash
TSUKU_AUTO_UPDATE=1 tsuku install kubectl
```

`TSUKU_AUTO_UPDATE=1` overrides all suppression signals. See [ENVIRONMENT.md](docs/ENVIRONMENT.md) for details.

### Remove a tool

```bash
tsuku remove kubectl
```

### Automatic Tool Discovery

tsuku automatically discovers where to find tools. Just run:

```bash
tsuku install <tool>
```

When a recipe doesn't exist, tsuku queries the curated registry first, then parallel-probes package registries (npm, PyPI, crates.io, RubyGems, Go, CPAN, Homebrew Cask) within 3 seconds. If multiple registries have the tool, tsuku picks the highest-priority match.

Use `--from` to override automatic discovery with an explicit source (see below).

### Install from distributed sources

Tools don't have to be in the central registry. Anyone can host recipes in a GitHub repository, and you can install directly from them:

```bash
# Install a recipe from a GitHub repo
tsuku install acme-corp/internal-tools:deploy-cli

# Install a specific version
tsuku install acme-corp/internal-tools:deploy-cli@2.1.0

# If the repo has only one recipe, the recipe name is optional
tsuku install acme-corp/my-tool
```

The first time you install from a new source, tsuku asks for confirmation. Pass `-y` to skip the prompt in scripts:

```bash
tsuku install -y acme-corp/internal-tools:deploy-cli
```

Once installed, distributed tools work exactly like central registry tools with `update`, `outdated`, `verify`, and `remove`.

See the [Distributed Recipes Guide](docs/guides/GUIDE-distributed-recipes.md) for the full workflow.

### Manage recipe registries

Pin sources you trust so you don't get prompted every time:

```bash
# List configured registries
tsuku registry list

# Trust a GitHub repo as a recipe source
tsuku registry add acme-corp/internal-tools

# Remove a registry
tsuku registry remove acme-corp/internal-tools
```

For CI or team environments where you want to lock down which sources are allowed, enable strict mode in `$TSUKU_HOME/config.toml`:

```toml
strict_registries = true
```

With strict mode on, tsuku only installs from the central registry and explicitly added registries. Anything else is rejected.

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

Some recipe builders use LLM analysis to generate recipes from complex sources:

**Builders requiring LLM**:
- `--from github:owner/repo` - Analyzes GitHub releases
- `--from homebrew:formula` - Analyzes Homebrew formulas

**Builders NOT requiring LLM** (deterministic):
- `--from crates.io` - Uses crates.io API
- `--from npm` - Uses npm registry
- `--from pypi` - Uses PyPI API
- `--from rubygems` - Uses RubyGems API
- `--from cask:<name>` - Uses Homebrew Cask API (macOS applications)

**Local inference is the default.** When no cloud API keys are configured, tsuku runs a language model locally using the `tsuku-llm` addon. This requires a GPU with at least 8 GB VRAM (CUDA, Metal, or Vulkan). On first use, tsuku prompts to download the addon binary (~50 MB) and a model (4.9-9.1 GB depending on your GPU). The addon detects your GPU and picks the right model size automatically.

To pre-download everything for CI or offline use:

```bash
tsuku llm download        # Interactive -- prompts before downloading
tsuku llm download --yes  # Skip prompts (for CI)
```

See the [Local LLM Guide](docs/guides/GUIDE-local-llm.md) for hardware requirements, configuration, and troubleshooting.

**Cloud providers (optional).** If you prefer cloud inference or want higher quality on unusual release layouts, set an API key for Claude or Gemini:

```bash
# Claude (Anthropic)
export ANTHROPIC_API_KEY="sk-ant-..."

# Or Gemini (Google)
export GOOGLE_API_KEY="AIza..."
```

You can also store keys in tsuku's config file instead of environment variables:

```bash
tsuku config set secrets.anthropic_api_key
# Reads the value from stdin to avoid shell history exposure
```

See [Secrets Management](#secrets-management) below for details.

Cost per cloud recipe generation: ~$0.02-0.15 depending on complexity.

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

### Local LLM Management

tsuku includes a local inference runtime for LLM-powered recipe generation. The `tsuku llm` commands manage this runtime:

```bash
# Pre-download the addon binary and model for your hardware
tsuku llm download

# Skip confirmation prompts (useful in CI)
tsuku llm download --yes
```

The `download` command detects your GPU, VRAM, and system RAM, then downloads the appropriate model. Run it ahead of time to avoid download prompts during `tsuku create`. See the [Local LLM Guide](docs/guides/GUIDE-local-llm.md) for details.

### Secrets Management

tsuku can store API keys and tokens in `$TSUKU_HOME/config.toml` as an alternative to environment variables. This is useful when you work across multiple terminals or don't want to manage shell exports.

**Set a secret:**

```bash
# Interactive prompt (value won't appear in shell history)
tsuku config set secrets.anthropic_api_key

# Or pipe the value
echo "sk-ant-..." | tsuku config set secrets.anthropic_api_key
```

Secret values are always read from stdin, never from command-line arguments.

**Check if a secret is configured:**

```bash
tsuku config get secrets.anthropic_api_key
# Output: (set) or (not set)
```

The actual value is never displayed.

**View status of all secrets:**

```bash
tsuku config
```

The output includes a Secrets section showing which keys are configured and which aren't.

**Resolution order:** When tsuku needs an API key, it checks environment variables first, then the config file. Environment variables always take precedence, so you can override stored secrets for a single command without changing your config.

**Known secrets:** `anthropic_api_key`, `google_api_key`, `github_token`, `tavily_api_key`, `brave_api_key`. The config file is written with 0600 permissions to prevent other users from reading your keys.

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

See the [Plan-Based Installation Guide](docs/guides/GUIDE-plan-based-installation.md) for air-gapped deployment and CI distribution workflows.

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
- **Tier 4: Integrity verification** (opt-in via `--integrity`) - Compares current SHA256 checksums against values stored at installation time to detect post-installation tampering

Tier 2 validation also detects ABI mismatches (glibc vs musl) to catch incompatible binary combinations early.

**Example output:**

```bash
$ tsuku verify ruby
Verifying ruby 3.4.0...
  Tier 1: Header validation... PASS
  Tier 2: Dependency validation...
    libyaml-0.so.2: OK (tsuku:libyaml@0.2.5)
    libssl.so.3: OK (tsuku:openssl@3.2.1, external)
    libc.so.6: OK (system)
  Tier 2: 3 dependencies validated
  Tier 3: dlopen load testing... PASS
Verification: PASS
```

**Dependency categories:**
- `tsuku:name@version` - Library installed and managed by tsuku
- `tsuku:name@version, external` - Installed via package manager (apt/brew)
- `system` - Operating system provided library (libc, libm, etc.)

Library verification supports additional flags:

```bash
# Enable Tier 4 integrity verification
tsuku verify gcc-libs --integrity

# Skip Tier 3 dlopen load testing (useful for headless environments)
tsuku verify gcc-libs --skip-dlopen
```

Libraries installed before the integrity feature was added will show "Integrity: SKIPPED" when verified with `--integrity`. Reinstalling the library will store checksums and enable full verification.

### Cache Management

tsuku caches recipes and version information locally. These commands help manage the cache:

```bash
# View cache statistics
tsuku cache info

# Remove cache entries older than 7 days
tsuku cache cleanup --max-age 7d

# Preview what would be removed without deleting
tsuku cache cleanup --dry-run

# Force LRU eviction to enforce size limit
tsuku cache cleanup --force-limit
```

Registry recipe cache configuration is available via environment variables. See [ENVIRONMENT.md](docs/ENVIRONMENT.md) for details on `TSUKU_RECIPE_CACHE_TTL`, `TSUKU_RECIPE_CACHE_SIZE_LIMIT`, and other cache settings.

```bash
# Refresh recipe cache
tsuku update-registry

# Preview what would be refreshed
tsuku update-registry --dry-run

# Refresh a specific recipe only
tsuku update-registry --recipe fzf

# Force refresh all cached recipes
tsuku update-registry --all
```

On a clean machine, the first `tsuku update-registry` fetches all recipe TOMLs to build the binary index — this takes roughly 15-20 seconds. Subsequent runs are fast because the recipes are cached locally.

#### Registry Compatibility

tsuku validates the registry's schema version on every fetch and cache read. If the registry format is newer than your CLI supports, tsuku exits with an error and suggests upgrading. When a registry includes a deprecation notice for an upcoming format change, tsuku prints a one-time warning to stderr with the timeline, the minimum CLI version required, and an upgrade URL. The `--quiet` flag suppresses these warnings.

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
- Runs the recipe's `[verify]` command after installation and reports pass/fail based on both install and verification results
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

**Environment variable passthrough:** Use `--env` to pass environment variables into the sandbox container. This is useful for tokens and configuration that tools need at install time:

```bash
# Pass a token with an explicit value
tsuku install gh --sandbox --env GITHUB_TOKEN=ghp_xxxx

# Read from the host environment (like docker -e)
tsuku install gh --sandbox --env GITHUB_TOKEN
```

The sandbox hardcodes `TSUKU_SANDBOX`, `TSUKU_HOME`, `HOME`, `DEBIAN_FRONTEND`, and `PATH` inside the container. These can't be overridden via `--env`. Note that `TSUKU_REGISTRY_URL` is consumed on the host during plan generation, so set it in your shell environment rather than passing it with `--env`.

**Structured output for CI:** Use `--json` for machine-readable results:

```bash
tsuku install ruff --sandbox --json
```

```json
{
  "tool": "ruff",
  "passed": true,
  "verified": true,
  "install_exit_code": 0,
  "verify_exit_code": 0,
  "duration_ms": 4523,
  "error": null
}
```

| Field | Description |
|-------|-------------|
| `passed` | Overall result: install succeeded AND verification passed |
| `verified` | Whether the verify command passed (true when no verify command exists) |
| `install_exit_code` | Container exit code from the install step |
| `verify_exit_code` | Exit code from the recipe's verify command (-1 if none) |
| `duration_ms` | Total execution time in milliseconds |
| `error` | Error message string, or null on success |

When `--json` is set, human-readable progress output is suppressed. CI workflows can parse results with `jq`:

```bash
tsuku install ruff --sandbox --json | jq '.passed'
```

For technical details, see [DESIGN-install-sandbox.md](docs/DESIGN-install-sandbox.md).

### Command suggestions

`tsuku suggest` looks up which recipe provides a given command and prints an install hint. It reads the local binary index — no network access needed.

```bash
# Single match
$ tsuku suggest jq
Command 'jq' not found. Install with: tsuku install jq

# Multiple matches show a list with installed status
$ tsuku suggest aws
Command 'aws' not found. Provided by:
  aws-cli   tsuku install aws-cli
  awscurl   tsuku install awscurl

# Machine-readable output
$ tsuku suggest jq --json
{"command":"jq","matches":[{"recipe":"jq","binary_path":"bin/jq","installed":false}]}
```

The binary index must be built before `suggest` works. Run `tsuku update-registry` once after installation (or any time you want to pick up new recipes).

Exit codes: `0` on match, `1` on no match, `11` if the index hasn't been built yet.

The shell hook invokes `tsuku suggest` automatically when you type a command that isn't found, so you get install hints without running `suggest` directly.

### Command-not-found hook

The command-not-found hook integrates tsuku suggestions directly into your shell. When you type a command that isn't found, your shell calls `tsuku suggest` automatically and prints an install hint.

**Install the hook:**

```bash
# Auto-detect shell from $SHELL
tsuku hook install

# Or specify the shell explicitly
tsuku hook install --shell=bash
tsuku hook install --shell=zsh
tsuku hook install --shell=fish
```

The three supported shells and the files modified:

| Shell | File modified |
|-------|--------------|
| bash  | `~/.bashrc` |
| zsh   | `~/.zshrc` |
| fish  | `~/.config/fish/conf.d/tsuku.fish` |

For bash and zsh, tsuku appends a marker block to the rc file:

```bash
# tsuku hook
. "$TSUKU_HOME/share/hooks/tsuku.bash"
```

The hook script itself lives in `$TSUKU_HOME/share/hooks/` and is updated automatically when tsuku upgrades, so you don't need to re-run `hook install` after upgrading.

`hook install` is idempotent — running it again when the hook is already installed makes no changes.

**Check hook status:**

```bash
tsuku hook status
```

Reports installed or not installed for each shell detected on the system.

**Remove the hook:**

```bash
# Auto-detect shell
tsuku hook uninstall

# Or specify explicitly
tsuku hook uninstall --shell=bash
```

Removes the marker block from the rc file (or deletes `~/.config/fish/conf.d/tsuku.fish` for fish). Also idempotent.

### Tool Shell Integration

Some tools register shell functions or environment setup during installation. Recipes that include an `install_shell_init` step place init scripts in `$TSUKU_HOME/share/shell.d/`. The managed `$TSUKU_HOME/env` file sources the appropriate per-shell init cache at startup, so tools like direnv or nvm have their shell hooks available in every new terminal — no manual configuration needed.

If you don't want a tool's shell init scripts, pass `--no-shell-init` during installation:

```bash
tsuku install direnv --no-shell-init
```

`tsuku info <tool>` shows whether a tool has shell integration files installed.

If shell functions from a tool aren't available after installation, run `tsuku doctor` to diagnose the issue. If the shell init cache is stale, `tsuku doctor --fix` rebuilds it automatically.

## Operations

tsuku includes a batch operations control plane for managing automated recipe imports:

- **Control file** (`batch-control.json`): Enable/disable batch processing, manage per-ecosystem circuit breakers, and track CI budget usage
- **Circuit breaker** (`scripts/check_breaker.sh`, `scripts/update_breaker.sh`): Automatic pause and recovery when failure rates exceed thresholds
- **Rollback** (`scripts/rollback-batch.sh`): Remove all recipes from a specific batch import by batch ID
- **Runbook** (`docs/runbooks/batch-operations.md`): Incident response procedures for batch success rate drops, emergency stops, rollbacks, budget alerts, and security incidents
- **Seeding workflow** (`seed-queue.yml`): Weekly discovery of CLI tools from multiple ecosystems (cargo, npm, pypi, rubygems) with automated disambiguation to select the best installation source for each package

See the [batch operations runbook](docs/runbooks/batch-operations.md) for detailed operational procedures.

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
