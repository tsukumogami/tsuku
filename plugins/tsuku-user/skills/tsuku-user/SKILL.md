---
name: tsuku-user
description: |
  End-user reference for managing tools with tsuku: project configuration
  via .tsuku.toml, CLI commands, shell integration, troubleshooting,
  auto-updates, and user settings. Use this skill when helping someone
  install tools, configure a project, debug PATH issues, or understand
  tsuku's update behavior.
---

## .tsuku.toml Project Configuration

A `.tsuku.toml` file at your project root declares which tools the project needs. When a collaborator clones the repo and runs `tsuku install -y`, they get the same toolchain.

### Creating a Project Config

```bash
tsuku init
```

This writes a starter `.tsuku.toml` in the current directory. Use `--force` to overwrite an existing one.

### [tools] Section

```toml
[tools]
node = "20"
python = "3.12.0"
jq = "latest"
go = { version = "1.23" }
```

Each key is a tool name. The value controls how tightly the version is pinned:

| Pin Level | Syntax | Example | Auto-Update Behavior |
|-----------|--------|---------|----------------------|
| Latest | `""` or `"latest"` | `jq = "latest"` | Updates to any new version |
| Major | `"N"` | `node = "20"` | Updates within 20.x.y |
| Minor | `"N.M"` | `python = "3.12"` | Updates within 3.12.z |
| Exact | `"N.M.P"` | `go = "1.23.4"` | No auto-updates |
| Channel | `"@name"` | `rust = "@nightly"` | Provider-specific |

### Installing Project Tools

```bash
# Install all tools from .tsuku.toml (prompts for confirmation)
tsuku install

# Skip confirmation
tsuku install -y

# Preview without installing
tsuku install --dry-run
```

tsuku finds `.tsuku.toml` by walking up from the current directory, stopping at `$HOME` (or directories listed in `TSUKU_CEILING_PATHS`).

## Core CLI Commands

### Install and Manage

| Command | Description | Common Flags |
|---------|-------------|--------------|
| `tsuku install <tool>` | Install a tool (supports `@version` suffix) | `--force`, `--sandbox`, `--dry-run` |
| `tsuku install` | Install all tools from `.tsuku.toml` | `--yes`, `--dry-run`, `--fresh` |
| `tsuku remove <tool>` | Remove a tool (or specific version with `@version`) | `--force` |
| `tsuku update <tool>` | Update within pin boundaries | `--dry-run` |
| `tsuku update --all` | Update all tools (skips exact-pinned) | `--dry-run` |
| `tsuku list` | List installed tools | `--json`, `--all` |
| `tsuku outdated` | Show tools with available updates | `--json` |

### Discover

| Command | Description | Common Flags |
|---------|-------------|--------------|
| `tsuku search <query>` | Search recipes by name or description | `--json` |
| `tsuku recipes` | List all available recipes | `--local`, `--json` |
| `tsuku info <tool>` | Tool details (homepage, deps, status) | `--json` |
| `tsuku versions <tool>` | Available versions for a tool | `--refresh`, `--json` |
| `tsuku which <command>` | Which recipe provides a command | |

### Utilities

| Command | Description | Common Flags |
|---------|-------------|--------------|
| `tsuku run <tool> [args]` | Install if missing, then execute | `--mode suggest/confirm/auto` |
| `tsuku verify <tool>` | Check binary integrity and deps | `--system-deps`, `--integrity` |
| `tsuku doctor` | Environment health check | `--rebuild-cache` |
| `tsuku cache clear` | Clear download and version caches | `--downloads`, `--versions` |
| `tsuku update-registry` | Refresh recipe cache and binary index | `--force` |

All commands accept `--verbose` (`-v`), `--quiet` (`-q`), and `--debug` for log control.

## Shell Integration

tsuku needs two directories on your PATH: `$TSUKU_HOME/bin` (wrapper scripts) and `$TSUKU_HOME/tools/current` (active tool symlinks). The `shellenv` command sets this up.

### Setup

Add one line to your shell profile:

**bash** (`~/.bashrc`):
```bash
eval "$(tsuku shellenv)"
```

**zsh** (`~/.zshrc`):
```zsh
eval "$(tsuku shellenv)"
```

**fish** (`~/.config/fish/config.fish`):
```fish
tsuku shellenv | source
```

`tsuku shellenv` prints the PATH exports and sources any tool-specific shell init scripts from `$TSUKU_HOME/share/shell.d/`.

### Verifying Your Setup

```bash
tsuku doctor
```

Doctor checks that `$TSUKU_HOME` exists, both directories are on PATH, the state file is accessible, shell init caches are current, and no orphaned staging directories remain. If something's wrong, it tells you what to fix.

Use `--rebuild-cache` to force a rebuild of shell init caches.

## Troubleshooting

### Exit Codes

When a command fails, the exit code tells you what went wrong:

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid arguments or usage |
| 3 | Recipe not found |
| 4 | Version not found |
| 5 | Network error |
| 6 | Installation failed (or all tools failed in batch install) |
| 7 | Verification failed |
| 8 | Dependency resolution failed |
| 15 | Partial failure (some tools failed in batch install) |
| 130 | Cancelled (Ctrl+C) |

### Diagnosing Issues

**Tool won't run after install?** Check that shell integration is set up:
```bash
tsuku doctor
```

**Suspect a corrupted install?** Verify the binary:
```bash
tsuku verify <tool>
```
This checks that the binary exists, its version matches the recorded state, and runs the tool's verification command if one is defined.

**Registry out of date?** Refresh it:
```bash
tsuku update-registry
```

## Auto-Update Workflow

By default, tsuku checks for updates in the background and applies them within your pin boundaries.

### How It Works

1. On most commands, tsuku spawns a background check for newer versions.
2. If `updates.auto_apply` is enabled, updates within pin boundaries are installed automatically.
3. After the command finishes, a summary of any updates is displayed.

Exact-pinned tools (`go = "1.23.4"`) are never auto-updated.

### Controlling Updates

| Setting | Effect |
|---------|--------|
| `TSUKU_NO_UPDATE_CHECK=1` | Disable all background checks |
| `TSUKU_AUTO_UPDATE=1` | Force auto-apply even in CI |
| `CI=true` | Suppresses auto-apply (unless overridden) |
| `TSUKU_NO_SELF_UPDATE=1` | Disable tsuku self-updates |

Or configure via `config.toml` (see below).

### Checking Manually

```bash
# See what's outdated
tsuku outdated

# Update one tool
tsuku update node

# Update everything
tsuku update --all

# Preview changes
tsuku update --all --dry-run
```

## User Configuration

User settings live in `$TSUKU_HOME/config.toml`. View and modify them with:

```bash
# Show all settings
tsuku config

# Get a specific value
tsuku config get telemetry

# Set a value
tsuku config set telemetry false
```

### Key Settings

**Telemetry**: Opt out of anonymous usage stats with `tsuku config set telemetry false`.

**Updates** (`[updates]` section):
- `enabled` -- toggle background update checks (default: true)
- `auto_apply` -- auto-install updates within pin boundaries (default: true)
- `check_interval` -- minimum time between checks, e.g. `"12h"` (default: `"24h"`)
- `self_update` -- check for tsuku self-updates (default: true)
- `version_retention` -- how long to keep old versions, e.g. `"168h"` (default: 7 days)

**Registries**: Add third-party recipe sources:
```bash
tsuku config set registries.myorg/recipes.url https://github.com/myorg/recipes
```

**Custom home directory**: Set `TSUKU_HOME` in your shell profile to move tsuku's data out of `~/.tsuku`:
```bash
export TSUKU_HOME="$HOME/.local/share/tsuku"
```

**Secrets**: Store API keys (for LLM-powered recipe generation):
```bash
echo "sk-..." | tsuku config set secrets.anthropic_api_key
```
Secrets can also be provided via `TSUKU_SECRET_<NAME>` environment variables, which take precedence over config.toml.
