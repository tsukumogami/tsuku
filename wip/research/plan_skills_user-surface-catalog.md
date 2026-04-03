# tsuku End-User Surface Catalog

Comprehensive catalog of tsuku's CLI commands, .tsuku.toml project config format, shell integration, user configuration options, version pinning semantics, and update workflows.

---

## CLI Commands

### Installation & Management

| Command | Syntax | Description | Key Flags |
|---------|--------|-------------|-----------|
| install | `tsuku install [tool]...` | Install development tools from recipe registry. No args installs all tools from .tsuku.toml project config. Supports `@version` syntax (e.g., `kubectl@v1.29.0`, `node@latest`). | `--force`, `--fresh`, `--dry-run`, `--json`, `--plan <file>`, `--recipe <path>`, `--from <source>`, `--sandbox`, `--skip-security`, `--yes`, `--no-shell-init` |
| update | `tsuku update [tool]` | Update installed tool to latest version within pin boundaries (respects Requested field). | `--all`, `--dry-run` |
| remove | `tsuku remove <tool>[@version]` | Remove tool or specific version. Without @version, removes all versions. | `--force` |
| list | `tsuku list` | List all installed tools (does not include system dependencies by default). | `--show-system-dependencies`, `--all` (includes libraries and apps), `--apps`, `--json` |
| outdated | `tsuku outdated` | Check for outdated tools. Skips exact-pinned tools. Shows "Latest" (within pin) and "Latest Overall" (if different). | `--json` |

### Discovery & Information

| Command | Syntax | Description | Key Flags |
|---------|--------|-------------|-----------|
| search | `tsuku search [query]` | Search for tools by name or description. | `--json` |
| recipes | `tsuku recipes` | List all available recipes from all sources. | `--local` (local recipes only), `--json` |
| versions | `tsuku versions <tool>` | List all available versions for a tool. Requires tool to support version listing. | `--refresh` (bypass cache), `--json` |
| info | `tsuku info <tool>` | Show detailed tool information (description, homepage, status, dependencies). | `--recipe <path>`, `--metadata-only`, `--deps-only`, `--system`, `--family <name>`, `--json` |
| which | `tsuku which <command>` | Show which recipe provides a command (requires binary index built via `update-registry`). | (none) |

### Configuration & Setup

| Command | Syntax | Description | Key Flags |
|---------|--------|-------------|-----------|
| init | `tsuku init` | Create .tsuku.toml project config file in current directory. | `--force` (overwrite existing) |
| shellenv | `tsuku shellenv` | Print shell commands to configure PATH for tsuku (eval $(tsuku shellenv)). Outputs exports for bin and tools/current. | (none) |
| config | `tsuku config` | Manage user configuration (config.toml). Displays all settings when no subcommand given. | `--json` |
| config get | `tsuku config get <key>` | Get specific config value. For `secrets.*` keys, shows status only (never value). | (none) |
| config set | `tsuku config set <key> [value]` | Set config value. Reads from stdin if value omitted (useful for secrets). | (none) |
| doctor | `tsuku doctor` | Check tsuku environment health (home dir, PATH, state file, shell.d, notices). Exits non-zero if any check fails. | `--rebuild-cache` (rebuild shell cache) |

### Project Install

| Command | Syntax | Description | Key Flags |
|---------|--------|-------------|-----------|
| install | `tsuku install` (no args) | Batch-install all tools from nearest .tsuku.toml. | `--dry-run`, `--yes` (skip confirmation), `--fresh` |

Exit codes for project install:
- 0: All tools installed successfully
- 6 (ExitInstallFailed): All tools failed
- 15 (ExitPartialFailure): Some tools failed

### Recipe & Plan Generation

| Command | Syntax | Description | Key Flags |
|---------|--------|-------------|-----------|
| create | `tsuku create <tool> --from <source>` | Generate recipe from package ecosystem, GitHub, or Homebrew. Sources: `crates.io`, `rubygems`, `pypi`, `npm`, `go:module`, `cpan`, `github:owner/repo`, `homebrew:formula`. Writes to `$TSUKU_HOME/recipes/<tool>.toml`. | `--from <source>` (required), `--force`, `--deterministic-only` |
| eval | `tsuku eval <tool>[@version]` | Generate deterministic installation plan as JSON. Supports cross-platform plan generation. | `--os <os>`, `--arch <arch>`, `--linux-family <family>`, `--recipe <path>`, `--version <version>`, `--install-deps`, `--pin-from <file>`, `--require-embedded` |
| plan show | `tsuku plan show <tool>` | Display stored installation plan for installed tool. | `--json` |
| plan export | `tsuku plan export <tool>` | Export installation plan to JSON file. Default filename: `<tool>-<version>-<os>-<arch>.plan.json`. | `--output <path>` (`-o`, use `-` for stdout) |
| validate | `tsuku validate <recipe-file>` | Validate recipe file (TOML syntax, required fields, action validation, security checks). | `--json`, `--strict`, `--check-libc-coverage` |

### Advanced

| Command | Syntax | Description | Key Flags |
|---------|--------|-------------|-----------|
| run | `tsuku run <tool>[@version] [args...]` | Install tool if missing, then execute it with arguments. | `--mode <mode>` (suggest/confirm/auto) |
| verify | `tsuku verify [tool...]` | Verify installed tools (binary exists, version matches state, runs --verify command). | `--system-deps`, `--integrity`, `--skip-dlopen` |
| cache clear | `tsuku cache clear` | Clear all caches (downloads and versions). | `--downloads` (clear downloads only), `--versions` (clear versions only) |
| update-registry | `tsuku update-registry` | Update central recipe registry cache and build binary index. | `--force` |

### Global Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--verbose` | `-v` | false | Show INFO level logs |
| `--quiet` | `-q` | false | Show errors only |
| `--debug` | | false | Show DEBUG level logs with timestamps and file locations |

Environment variable equivalents: `TSUKU_VERBOSE`, `TSUKU_QUIET`, `TSUKU_DEBUG` (flags take precedence).

---

## .tsuku.toml Format

Project configuration file declaring tool requirements. Located at project root (discovery via parent directory traversal, stopped by `$HOME` or `TSUKU_CEILING_PATHS`).

### Schema

```toml
[tools]
# Simple version string shorthand
node = "20.16.0"
python = "3.12"
terraform = "1.8.0"
kubectl = "latest"  # or empty string "" for latest

# Inline table form (equivalent to above)
go = { version = "1.23" }
```

### Syntax Details

- **Tool name**: lowercase identifier, becomes requirement key
- **Version field**: string value
  - `"20.16.0"` → exact pin (PinExact)
  - `"20.16"` → minor pin (PinMinor, allows 20.16.z)
  - `"20"` → major pin (PinMajor, allows 20.x.y)
  - `"latest"` or `""` (empty) → tracks latest stable (PinLatest)
  - `"@lts"` → named channel pin (PinChannel, provider-specific)
  - `"@stable"`, `"@next"` → other channels (if supported by recipe)

### Constraints

- Maximum 256 tools per config file
- Parsed via BurntSushi TOML library
- Discovery stops at home directory boundary or ceiling paths set via `TSUKU_CEILING_PATHS` env var

### Example

```toml
[tools]
# Development tools
node = "20.16.0"
python = "3.12.0"
go = "1.23.1"

# CLIs with different pinning strategies
terraform = "1"        # Major pin: allows 1.x.y
kubectl = "1.29"      # Minor pin: allows 1.29.z
jq = "latest"         # Latest stable
rust = "@nightly"     # Named channel
```

---

## Version Pinning

Determines how strictly tools are pinned and enables auto-update within boundaries.

### Pin Levels

| Level | Name | Example | Behavior | Can Auto-Update |
|-------|------|---------|----------|-----------------|
| 0 | PinLatest | `""` or `"latest"` | Tracks latest stable release | Yes, to any new version |
| 1 | PinMajor | `"20"` | Matches `20.x.y` (dot-boundary semantics) | Yes, within major (e.g., 20.0→20.99) |
| 2 | PinMinor | `"1.29"` | Matches `1.29.z` (dot-boundary semantics) | Yes, within minor (e.g., 1.29.0→1.29.99) |
| 3 | PinExact | `"1.29.3"` | Exact match, never auto-updates | No (always ExitSuccess on `update`) |
| 4 | PinChannel | `"@lts"`, `"@nightly"` | Named channel, resolution provider-specific | Yes, within channel |

### Dot-Boundary Matching

Version matching uses dot-boundary semantics to prevent fuzzy matches:
- Request `"1"` matches `1.0.0`, `1.99.99` but **NOT** `10.0.0`
- Implemented via `strings.HasPrefix(version, requested+".")`

### Validation

- ValidateRequested() checks for invalid characters (allows: alphanumerics, `.`, `@`, `-`)
- Rejects: path traversal (`..`), path separators (`/`, `\`)

---

## Shell Integration

Tsuku manages tool binaries via PATH and shell initialization hooks.

### Directory Structure

```
$TSUKU_HOME/
├── bin/                          # Executable wrapper scripts
├── tools/
│   ├── current/                  # Symlink to active versions directory
│   │   ├── node/                 # Auto-symlinked to tools/node/v20.16.0/bin
│   │   └── python/               # etc.
│   ├── node/
│   │   ├── v20.10.0/bin
│   │   ├── v20.16.0/bin
│   │   └── v20.17.0/bin
│   ├── .staging-{uuid}/          # Temporary installation staging (cleaned up)
│   └── .node-backup-{timestamp}/ # Backup directories (retained per version_retention)
├── state.json                    # Installation state (tools, versions, checksums)
├── config.toml                   # User configuration
├── share/
│   └── shell.d/
│       ├── node.bash             # Tool-specific shell init (content hash in state)
│       ├── python.zsh
│       ├── node.fish
│       ├── .init-cache.bash      # Cached concatenation of *.bash files
│       ├── .init-cache.zsh
│       ├── .init-cache.fish
│       └── .lock                 # File lock for atomic cache rebuilds
└── cache/
    ├── downloads/                # Binary download cache
    ├── versions/                 # Version metadata cache
    └── distributed/              # Distributed registry cache
```

### PATH Configuration

**shellenv output:**
```bash
export PATH="$TSUKU_HOME/bin:$TSUKU_HOME/tools/current:$PATH"
. "$TSUKU_HOME/share/shell.d/.init-cache.{shell}"  # If shell init exists
```

**Setup pattern (in shell profile):**
```bash
# .bashrc / .zshrc / config.fish
eval $(tsuku shellenv)
```

**Or one-off session:**
```bash
eval $(./tsuku shellenv)  # For dev builds
```

### Shell Initialization Hooks

Tools can register shell-specific initialization (e.g., `nvm` behavior). These are stored in `$TSUKU_HOME/share/shell.d/` and cached per-shell.

- **Supported shells**: bash, zsh, fish (detected via `$SHELL` env var)
- **File naming**: `<toolname>.<shell>` (e.g., `node.bash`, `node.zsh`, `node.fish`)
- **Caching**: `RebuildShellCache()` concatenates all `*.<shell>` files sorted alphabetically into `.init-cache.<shell>`
- **Security**: Symlinks are rejected (Lstat check), content hashes verified, cache written with 0600 permissions
- **Atomicity**: File lock prevents concurrent cache rebuilds

### Doctor Checks

`tsuku doctor` validates:

1. **Home directory exists** (not a symlink or file)
2. **tools/current in PATH** (absolute path comparison)
3. **bin in PATH**
4. **State file accessible** (not required to exist for fresh installs)
5. **Shell integration health**
   - Detects active scripts per shell (bash, zsh, fish)
   - Checks cache staleness and content hash mismatches
   - Warns on symlinks (security risk)
   - Validates syntax
6. **Orphaned staging directories** (`.staging-*`, warns to clean up manually)
7. **Stale notices** (>30 days old, suggests cleanup)

Exit code: 0 if all checks pass, 1 if any fail.

---

## User Configuration (config.toml)

User-level settings stored in `$TSUKU_HOME/config.toml`. File permissions: 0600. Created/updated atomically via temp file + rename.

### Available Keys

#### Telemetry

- **Key**: `telemetry` | **Type**: boolean | **Default**: true
- **Description**: Enable anonymous usage statistics collection

#### LLM Settings

- **Key**: `llm.enabled` | **Type**: boolean | **Default**: true
  - Enable/disable LLM features for recipe generation
  
- **Key**: `llm.local_enabled` | **Type**: boolean | **Default**: true
  - Enable local LLM inference via tsuku-llm addon
  
- **Key**: `llm.local_preemptive` | **Type**: boolean | **Default**: true
  - Start addon server early at `tsuku create` to hide loading latency
  
- **Key**: `llm.idle_timeout` | **Type**: duration | **Default**: 5m
  - How long addon stays alive after last request (format: `30s`, `5m`, `2h`)
  - Env var override: `TSUKU_LLM_IDLE_TIMEOUT`
  
- **Key**: `llm.providers` | **Type**: []string | **Default**: auto-detect
  - Preferred LLM provider order (comma-separated, e.g., `claude,gemini`)
  
- **Key**: `llm.daily_budget` | **Type**: float64 | **Default**: 5.0
  - Daily LLM cost limit in USD (0 = unlimited)
  
- **Key**: `llm.hourly_rate_limit` | **Type**: int | **Default**: 10
  - Maximum LLM generations per hour (0 = unlimited)
  
- **Key**: `llm.backend` | **Type**: string | **Default**: "" (auto-detect)
  - Override GPU backend for tsuku-llm. Valid: `"cpu"` (force CPU variant)

#### Auto-Install Mode

- **Key**: `auto_install_mode` | **Type**: string | **Default**: "confirm"
- **Valid values**: `"suggest"`, `"confirm"`, `"auto"`
- **Description**: Default consent mode for `tsuku run` when tool is missing
- **Env var override**: `TSUKU_AUTO_INSTALL_MODE`

#### Strict Registries

- **Key**: `strict_registries` | **Type**: boolean | **Default**: false
- **Description**: When true, restricts recipe resolution to explicitly registered sources only

#### Update Settings

- **Key**: `updates.enabled` | **Type**: boolean | **Default**: true
  - Enable automatic background update checks
  - Env var override: `TSUKU_NO_UPDATE_CHECK=1` (disables)
  
- **Key**: `updates.auto_apply` | **Type**: boolean | **Default**: true
  - Automatically install updates within pin boundaries
  - Suppressed in CI (when `CI=true`) unless `TSUKU_AUTO_UPDATE=1` overrides
  
- **Key**: `updates.check_interval` | **Type**: duration | **Default**: 24h
  - Minimum time between update checks (range: 1h–30d, format: `24h`, `12h`, `1h`)
  - Env var override: `TSUKU_UPDATE_CHECK_INTERVAL`
  
- **Key**: `updates.notify_out_of_channel` | **Type**: boolean | **Default**: true
  - Notify about versions outside pin boundary
  
- **Key**: `updates.self_update` | **Type**: boolean | **Default**: true
  - Check for and apply tsuku self-updates
  - Suppressed when `TSUKU_NO_SELF_UPDATE=1` or in CI
  
- **Key**: `updates.version_retention` | **Type**: duration | **Default**: 168h (7 days)
  - Minimum time to keep old version directories (format: `168h`, `720h`)

#### Secrets

- **Key**: `secrets.<name>` | **Type**: string
- **Description**: API keys and tokens stored securely. Resolved via:
  1. Environment variable `TSUKU_SECRET_<NAME>` (uppercase)
  2. `config.toml` secrets map
- **Example**: `secrets.anthropic_api_key`
- **Set via stdin**: `echo "sk-..." | tsuku config set secrets.anthropic_api_key`

#### Registries

- **Key**: `registries` | **Type**: map[string]{url, auto_registered}
- **Description**: Distributed recipe sources (owner/repo format)
- **auto_registered**: Set true if auto-registered during install (can be cleaned up)

### Example config.toml

```toml
telemetry = true

[llm]
enabled = true
local_enabled = true
local_preemptive = true
idle_timeout = "5m"
providers = ["claude", "gemini"]
daily_budget = 10.0
hourly_rate_limit = 20
# backend = "cpu"  # Force CPU, or omit for auto-detect

auto_install_mode = "confirm"

[updates]
enabled = true
auto_apply = true
check_interval = "24h"
notify_out_of_channel = true
self_update = true
version_retention = "168h"

[secrets]
anthropic_api_key = "sk-..."
openai_api_key = "..."

[registries]
"myorg/recipes" = { url = "https://github.com/myorg/recipes", auto_registered = false }
```

---

## Update Workflow

### Automatic Checks

**Trigger**: Every non-excluded command (e.g., NOT `check-updates`, `hook-env`, `run`, `help`, `version`, `completion`, `self-update`).

**Background**: `updates.CheckAndSpawnUpdateCheck()` spawns a background goroutine to check for updates without blocking command execution.

**Display**: `updates.DisplayAvailableSummary()` shows summary after command output.

### Outdated Command

**Purpose**: Report tools with newer versions available.

**Logic**:
1. Skip exact-pinned tools (PinExact = no updates)
2. For each tool, load recipe from recorded source (distributed or default chain)
3. Resolve "Latest" version within pin boundary via `version.ResolveWithinBoundary()`
4. Resolve "Latest Overall" if different from within-pin
5. Include self-update info if available

**Output**: Table or JSON format:
```
TOOL         CURRENT    LATEST     OVERALL
kubectl      1.28.0     1.29.3     1.30.0
node         18.19.0    18.21.0    20.16.0
```

### Update Command

**Syntax**: `tsuku update [tool]` or `tsuku update --all`

**Logic**:
1. Check tool is installed
2. Load recipe from recorded source (fallback to chain if distributed source unreachable)
3. Cache recipe in loader to avoid shadowing
4. Read `Requested` field from state to respect install-time constraint
5. Perform installation (bypasses old version cleanup until verified)
6. Snapshot old version's cleanup actions
7. Compute stale cleanup (files old version created that new version no longer needs)
8. Execute stale cleanup (lifecycle-aware: e.g., remove shell.d scripts for shells new version dropped)
9. Send telemetry events (success/failure with constraint and update mode)

**Flags**:
- `--dry-run`: Show what would be updated without making changes
- `--all`: Update all tools within pin boundaries (skips exact-pinned)

**Update All**:
1. List all installed tools
2. Skip exact-pinned tools
3. For each tool, perform update (respects tool's Requested field)
4. Print summary: "Updated X/Y tools (Z failed)"

### Version Pinning in Updates

- **PinLatest** (`""` or `"latest"`): Updates to absolute latest
- **PinMajor** (`"20"`): Updates within 20.x.y
- **PinMinor** (`"1.29"`): Updates within 1.29.z
- **PinExact** (`"1.29.3"`): No updates (skipped in `update --all`)
- **PinChannel** (`"@lts"`): Updates within channel

### Notification System

**Out-of-Channel Notifications**: When `updates.notify_out_of_channel=true`, user is warned about newer versions outside their pin boundary.

**Suppression**: Environment variables and CI detection:
- `TSUKU_NO_UPDATE_CHECK=1`: Disables all checks
- `TSUKU_AUTO_UPDATE=1`: Forces auto-apply even in CI
- `CI=true`: Suppresses auto-apply by default
- `TSUKU_NO_SELF_UPDATE=1`: Disables self-update

### Auto-Apply (Background Updates)

**Trigger**: Via `updates.MaybeAutoApply()` in PersistentPreRun hook.

**Logic**:
1. Check `updates.enabled` (default true, can be disabled)
2. Check `updates.auto_apply` (suppressed in CI unless TSUKU_AUTO_UPDATE=1)
3. For each installed tool, check if newer version within pin boundary exists
4. Auto-install if found
5. Display results via `updates.DisplayNotifications()`

**Telemetry**: Records success/failure with constraint and "auto" mode.

---

## Exit Codes

| Code | Name | Meaning |
|------|------|---------|
| 0 | ExitSuccess | Successful execution |
| 1 | ExitGeneral | General error |
| 2 | ExitUsage | Invalid arguments or usage error |
| 3 | ExitRecipeNotFound | Recipe not found in registry |
| 4 | ExitVersionNotFound | Version not found for tool |
| 5 | ExitNetwork | Network error (download, registry lookup) |
| 6 | ExitInstallFailed | Installation failed; with `install --all`, all tools failed |
| 7 | ExitVerifyFailed | Verification (binary/version/verify command) failed |
| 8 | ExitDependencyFailed | Dependency resolution failed |
| 9 | ExitDeterministicFailed | Deterministic recipe generation failed (--deterministic-only blocked LLM fallback) |
| 10 | ExitAmbiguous | Multiple ecosystem sources found; use --from to disambiguate |
| 11 | ExitIndexNotBuilt | Binary index not built; run `tsuku update-registry` |
| 12 | ExitNotInteractive | Confirm mode used without TTY; set TSUKU_AUTO_INSTALL_MODE or use --mode |
| 13 | ExitUserDeclined | User declined interactive prompt |
| 14 | ExitForbidden | Operation blocked for security (e.g., running as root) |
| 15 | ExitPartialFailure | With `install --all`, some tools failed but others succeeded |
| 130 | ExitCancelled | Operation cancelled by user (Ctrl+C, SIGINT/SIGTERM) |

---

## Environment Variables

### Global Control

| Variable | Values | Default | Purpose |
|----------|--------|---------|---------|
| `TSUKU_HOME` | path | `~/.tsuku` | Override tsuku home directory |
| `TSUKU_CEILING_PATHS` | colon-separated paths | (none) | Additional directory boundaries to stop config traversal (HOME always a ceiling) |
| `TSUKU_VERBOSE` | 1/true/yes/on | false | Enable INFO level logging |
| `TSUKU_QUIET` | 1/true/yes/on | false | Show errors only |
| `TSUKU_DEBUG` | 1/true/yes/on | false | Enable DEBUG level logging |

### Updates

| Variable | Values | Default | Purpose |
|----------|--------|---------|---------|
| `TSUKU_NO_UPDATE_CHECK` | 1 | (disabled) | Disable automatic update checks |
| `TSUKU_AUTO_UPDATE` | 1 | (disabled) | Force auto-apply updates even in CI |
| `TSUKU_UPDATE_CHECK_INTERVAL` | duration | 24h | Minimum time between checks (1h–30d) |
| `TSUKU_NO_SELF_UPDATE` | 1 | (disabled) | Disable tsuku self-updates |
| `CI` | true | (auto-detect) | Suppress auto-apply unless TSUKU_AUTO_UPDATE=1 |

### LLM

| Variable | Values | Default | Purpose |
|----------|--------|---------|---------|
| `TSUKU_LLM_IDLE_TIMEOUT` | duration | 5m | Override addon idle timeout (e.g., 10m, 30s) |
| `TSUKU_SECRET_<NAME>` | string | (none) | API keys (takes precedence over config.toml) |

### Installation

| Variable | Values | Default | Purpose |
|----------|--------|---------|---------|
| `TSUKU_AUTO_INSTALL_MODE` | suggest/confirm/auto | (none) | Default mode for `tsuku run` when tool missing |

### Development

| Variable | Values | Default | Purpose |
|----------|--------|---------|---------|
| (ldflags) `defaultHomeOverride` | path | (none) | Dev builds: override ~/.tsuku (not overridden by TSUKU_HOME env var in some contexts) |

---

## Summary Table: Command Families

| Family | Purpose | Key Commands |
|--------|---------|--------------|
| **Install** | Add tools | install, run, sandbox |
| **Manage** | Update/remove | update, remove, outdated |
| **Discover** | Find tools | search, recipes, versions, which, info |
| **Plan** | Generate/inspect | eval, plan show/export, validate |
| **Project** | Reproducible env | init, install (from .tsuku.toml) |
| **Config** | User settings | config get/set, doctor, shellenv |
| **Cache** | Performance | cache clear, update-registry |
| **Verify** | Quality assurance | verify, validate |

