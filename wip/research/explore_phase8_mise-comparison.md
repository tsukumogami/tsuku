# Mise Environment Isolation and Tool Versioning Research

Research conducted: 2026-01-29

## Executive Summary

Mise (formerly rtx) is a Rust-based polyglot tool version manager that replaces tools like asdf, nvm, pyenv, and rbenv. It provides both per-directory tool versioning and named environment configurations through `MISE_ENV`. Mise's approach most closely resembles tsuku's proposed Option 1 (named environments), but with a critical difference: mise environments change configuration files rather than installation state, while tsuku's `--env` proposal creates fully isolated installation states.

## 1. Core Model: Multiple Tool Versions

Mise manages multiple versions of tools on the same system using a hierarchical, directory-based activation model:

### Version Resolution Hierarchy

Configuration cascades from broad to specific:
1. `/etc/mise/config.toml` - System-wide defaults
2. `~/.config/mise/config.toml` - Global user defaults
3. `~/work/mise.toml` - Work directory settings
4. `~/work/project/mise.toml` - Project-specific overrides
5. `~/work/project/.tool-versions` - Legacy asdf compatibility

### Activation Model

When you enter a directory, mise automatically activates the versions specified in that directory's configuration. The system:
- Reads all parent directories to determine the complete tool set
- Overrides configuration as it goes lower in the hierarchy
- Exits early if the directory or config files haven't changed (performance optimization)
- Ensures project-specified versions take precedence over global installations

### Multiple File Formats

Mise supports:
- `.mise.toml` (recommended) - Flexible, feature-rich format
- `.tool-versions` (legacy) - asdf compatibility, less flexible
- `.node-version`, `.ruby-version`, etc. - Legacy single-tool formats

**Recommendation from mise docs**: Use `mise.toml` and commit it to version control to share tool configurations with team members.

## 2. Named Environment Concept: `MISE_ENV`

Mise has an explicit "environment" concept, but it works differently than tsuku's proposal.

### How MISE_ENV Works

When `MISE_ENV` is set to a value like "development", "production", or "test", mise looks for environment-specific configuration files:

**Standard config files:**
- `mise.toml`
- `config.toml`

**Environment-specific config files (when `MISE_ENV=production`):**
- `mise.production.toml`
- `config.production.toml`

### Activation Methods

Three ways to set `MISE_ENV`:
1. CLI flag: `mise install -E development` or `--env development`
2. Environment variable: `MISE_ENV=development`
3. Config file: Add `env = ["development"]` to `.miserc.toml`

### Multi-Environment Support

Multiple environments can be specified with comma separation:
```bash
MISE_ENV=ci,test mise install
```

This loads both `mise.ci.toml` and `mise.test.toml`, with later environments overriding earlier ones.

### Configuration Precedence

When both `mise.toml` and `mise.production.toml` exist:
- Both files are read (base + environment-specific)
- Environment-specific settings override base settings
- Writes go to `mise.toml` by default (preserves shared config)
- `.local.toml` files are available for local-only config not committed to version control

### Use Cases (from mise docs)

Environment-specific config files enable:
- Different environment variables per environment
- Different tool versions in development vs staging vs production
- Isolation between CI environments and local development

## 3. Per-Directory Tool Versioning Implementation

### File Locations

Mise searches for config files in:
1. Current directory
2. All parent directories (walking up the tree)
3. `MISE_CONFIG_DIR` (typically `~/.config/mise/`)

### Directory Isolation Mechanism

Per-directory version isolation works because:
- Mise activates tools based on your current working directory
- Each project directory can specify different versions
- Parent directory configs provide defaults, child configs override
- The activation is automatic when entering a directory (via shell integration)

### Shell Integration

Mise integrates with your shell (bash, zsh, fish) to:
- Detect directory changes
- Activate appropriate tool versions
- Update PATH to point to correct binaries
- Minimize overhead by caching and skipping unchanged directories

## 4. Cache and Download Sharing

Mise separates concerns into three directories:

### MISE_DATA_DIR

**Default locations:**
- Linux: `~/.local/share/mise` or `$XDG_DATA_HOME/mise`
- macOS: `~/Library/Application Support/mise`

**Contents:**
- Plugins
- Tool installs (the actual downloaded binaries)

**Sharing:**
- Should NOT be shared across machines
- CAN be cached in CI/CD to speed up runs
- Shared across all projects on the same machine

**CI/CD recommendation:** Cache `MISE_DATA_DIR` to avoid re-downloading tools across pipeline runs.

### MISE_CACHE_DIR

**Default locations:**
- Linux: `~/.cache/mise` or `$XDG_CACHE_HOME/mise`
- macOS: `~/Library/Caches/mise`

**Contents:**
- Temporary cached data
- Performance optimization data

**Sharing:**
- Should NOT be cached in CI/CD
- Provides minimal benefit outside interactive usage
- Can cause issues if shared across runs

### MISE_CONFIG_DIR

**Default location:**
- `~/.config/mise/`

**Contents:**
- System-wide configuration files
- Global tool version specifications

### Key Insight: Global Cache, Per-Directory Activation

Mise uses a **single shared download cache** (`MISE_DATA_DIR`) but achieves isolation through **configuration-based activation**. When you're in project A, mise activates the versions specified in project A's config. When you're in project B, mise activates project B's versions. The underlying tool binaries are stored once in `MISE_DATA_DIR`, but symlinks or PATH manipulation ensure the right version is used per directory.

## 5. Development vs Production Isolation

Mise handles this through `MISE_ENV`, not through separate installation states.

### Example: Production vs Development

**mise.toml** (base config, committed to version control):
```toml
[tools]
node = "20.10.0"
python = "3.11"
```

**mise.development.toml** (development overrides):
```toml
[tools]
node = "20.10.0"
python = "3.11"

[env]
DEBUG = "true"
LOG_LEVEL = "debug"
```

**mise.production.toml** (production overrides):
```toml
[tools]
node = "20.10.0"
python = "3.11"

[env]
DEBUG = "false"
LOG_LEVEL = "error"
```

### Running Commands Per Environment

```bash
# Development
mise exec -E development -- node app.js

# Production
mise exec -E production -- node app.js
```

The same tool versions are used, but different environment variables are loaded. If needed, environment-specific tool versions can also be specified.

### Important Distinction

Mise's `MISE_ENV` changes **which configuration is read**, not **where tools are installed**. All tools are still installed to the same `MISE_DATA_DIR`. The isolation is at the configuration and environment variable level, not at the installation state level.

## 6. Comparison to Tsuku's `--env` Proposal

### Tsuku's Proposed Model (Option 1: Named Environments)

```
$TSUKU_HOME/
├── envs/
│   ├── default/
│   │   ├── bin/
│   │   └── state.json
│   ├── contrib/
│   │   ├── bin/
│   │   └── state.json
│   └── ci/
│       ├── bin/
│       └── state.json
├── tools/          # Shared tool installations
└── cache/          # Shared download cache
```

Each named environment has:
- Its own `state.json` (independent installation records)
- Its own `bin/` directory with symlinks
- Shared access to `tools/` and `cache/`

### Mise's Model (for comparison)

```
~/.local/share/mise/        # MISE_DATA_DIR (all tools)
├── installs/
│   ├── node/
│   │   ├── 20.10.0/
│   │   └── 18.17.0/
│   └── python/
│       ├── 3.11/
│       └── 3.10/
└── plugins/

~/.config/mise/             # MISE_CONFIG_DIR
├── config.toml
├── config.development.toml
└── config.production.toml

~/project/
├── mise.toml
├── mise.development.toml
└── mise.production.toml
```

Mise achieves isolation through:
- Configuration file selection (`MISE_ENV`)
- Directory-based activation (current working directory)
- Shell integration (PATH manipulation)

### Key Differences

| Aspect | Tsuku `--env` (proposed) | Mise `MISE_ENV` |
|--------|-------------------------|-----------------|
| **What is isolated** | Installation state (`state.json`) | Configuration files (`mise.{env}.toml`) |
| **bin/ directories** | One per environment | Shared (or PATH manipulation) |
| **Tool installations** | Shared (`tools/`) | Shared (`installs/`) |
| **Use case focus** | Contributor testing, CI isolation | Dev/staging/prod config, env vars |
| **Isolation guarantee** | Hard (separate state files) | Soft (config-based, directory-aware) |
| **Cleanup on switch** | Separate environments coexist | Same tools, different activation |

### Does Mise Solve "Don't Pollute My Real Install"?

**Short answer: Partially, but differently.**

Mise's approach:
1. **Directory-based isolation**: If you're working on a contribution in `~/projects/tsuku-contrib/`, you can create a `mise.toml` there with specific tool versions. When you're in that directory, those versions are active. When you leave, they're not.

2. **MISE_ENV for config variation**: You can set `MISE_ENV=contrib` to load `mise.contrib.toml`, which might specify different environment variables or tool versions. But this still writes to the same global state in `MISE_DATA_DIR`.

3. **No separate installation state**: There's no concept of "this tool is installed in contrib environment vs default environment". A tool is either installed or not installed in `MISE_DATA_DIR`. Configuration determines which version is active.

**Where mise differs from tsuku's proposal:**
- Mise doesn't prevent a contributor from accidentally installing a tool to their "real" mise state when testing.
- Mise relies on directory context and config files for isolation, not separate state files.
- Mise assumes you're okay with tools being installed globally, but activated per-directory.

**Where mise is similar to tsuku's proposal:**
- Shared download cache (no redundant downloads)
- Multiple environments coexist (via config files)
- Explicit environment naming (`MISE_ENV` vs `--env`)

### Is Mise More Like Option 1 or Option 2?

**Mise is closer to Option 1 (named environments), but with important caveats:**

- **Option 1 (Tsuku's proposal)**: Named environments under `$TSUKU_HOME/envs/<name>/`, each with separate `state.json` and `bin/`, but shared tool installations and cache.

- **Option 2 (rejected)**: Standalone `$TSUKU_HOME` directories via environment variable override. Completely separate everything.

- **Mise's model**: Named environments via `MISE_ENV`, but environments are **configuration-only**. There's no separate installation state per environment. All tools live in one shared `MISE_DATA_DIR`, and configuration files determine which versions are active.

**Mise is like a "lighter" version of Option 1** where environments change behavior (config, env vars, active versions) but not installation state.

## 7. What Tsuku Can Learn from Mise

### Design Insights

1. **Configuration-based isolation may be sufficient**: Mise achieves strong isolation without separate installation states. For many use cases, having `tsuku.toml` with environment-specific overrides (`tsuku.contrib.toml`) might solve the "don't pollute" problem without the complexity of separate state files.

2. **Shell integration is key**: Mise's automatic directory-based activation reduces friction. If tsuku adopts `--env`, consider shell integration that auto-detects environment from directory context.

3. **Clear separation of concerns**:
   - `MISE_DATA_DIR`: Tool binaries (not shared across machines)
   - `MISE_CACHE_DIR`: Temporary data (don't cache in CI)
   - `MISE_CONFIG_DIR`: User-level config

   Tsuku already has a similar model with `tools/`, `cache/`, and configuration, but could be more explicit about which directories should be cached in CI.

4. **Environment naming is powerful**: Mise's `MISE_ENV` allows teams to standardize on environment names (development, staging, production, ci) and have tooling understand those contexts. Tsuku's `--env <name>` could adopt this pattern.

5. **Local-only configs**: Mise supports `.local.toml` files that are meant to be gitignored. This allows contributors to have local overrides without affecting the shared config. Tsuku could adopt a similar pattern (e.g., `tsuku.local.toml`).

6. **Multi-environment support**: Mise allows comma-separated environments (`MISE_ENV=ci,test`). This could be useful for tsuku if, for example, you want both "contrib" and "debug" settings active.

### Differences to Consider

1. **Mise doesn't solve state isolation**: If a tsuku contributor wants a guarantee that their test installations won't affect their real `state.json`, mise's approach doesn't provide that. Tsuku's Option 1 (separate state per environment) is stronger isolation.

2. **Mise assumes directory context**: Mise's model works well when you're working in a project directory with a config file. It's less clear how it helps when you're running ad-hoc commands outside a project context. Tsuku's `--env` flag provides explicit control regardless of directory.

3. **Mise is config-heavy**: Mise relies heavily on TOML config files. Tsuku's model is more imperative (`tsuku install --env contrib gh`). There's a philosophical difference between "declare what you want in a file" vs "run commands with flags".

### Recommendations for Tsuku

1. **Consider a hybrid approach**: Support both directory-based config files (`tsuku.toml`) and explicit `--env` flags. Let users choose the model that fits their workflow.

2. **Make environments lightweight**: If implementing `--env`, consider whether full state isolation is necessary or if config-based isolation (like mise) would suffice. The lighter approach is simpler to implement and reason about.

3. **Document cache sharing clearly**: Mise's docs explicitly state which directories should and shouldn't be cached in CI. Tsuku should do the same.

4. **Consider shell integration**: Mise's automatic activation when entering a directory is powerful. If tsuku adopts config files, shell integration could make the experience seamless.

5. **Support local overrides**: Allow contributors to have local-only config (gitignored) that overrides project config without affecting the team's shared settings.

## 8. Specific Answers to Key Questions

### Does mise solve the "don't pollute my real install" problem differently?

Yes, mise solves it through **directory-based activation** rather than **separate installation states**. When you're in a contrib project directory with a `mise.toml`, mise activates the versions specified there. When you leave that directory, you're back to your global or other project-specific versions. However, all tools are still installed to the same global `MISE_DATA_DIR`, so there's no hard isolation at the installation level.

For a tsuku contributor worried about accidentally installing untrusted tools to their real `state.json`, mise's approach doesn't provide strong guarantees. It assumes you're okay with tools being globally installed but context-aware in activation.

### Is mise's approach more like Option 1 or Option 2?

Mise is closer to **Option 1 (named environments)** but significantly lighter:

- **Like Option 1**: Named environments (`MISE_ENV=contrib`), shared tool installations, shared cache
- **Unlike Option 1**: No separate `state.json` per environment, no separate `bin/` per environment
- **Unlike Option 2**: Not standalone home directories, single shared `MISE_DATA_DIR`

Mise's environments are **configuration-based**, not **state-based**. They change which config file is read and which versions are activated, but not where tools are installed or tracked.

### What can tsuku learn from mise's design choices?

1. **Configuration-based isolation can be powerful** without the complexity of separate installation states
2. **Directory context is a strong UX pattern** for automatic environment detection
3. **Clear documentation on cache sharing** helps users optimize CI/CD pipelines
4. **Multi-environment support** (comma-separated) provides flexibility
5. **Local overrides** (`.local.toml`) allow personal customization without affecting team config
6. **Shell integration** makes environment switching seamless
7. **Explicit separation of data vs cache vs config** directories clarifies what should be persisted and shared

However, mise's lighter approach means it doesn't provide hard guarantees about installation state isolation, which may be important for tsuku's contributor and CI use cases.

## Sources

- [GitHub - jdx/mise: dev tools, env vars, task runner](https://github.com/jdx/mise)
- [Getting Started with Mise | Better Stack Community](https://betterstack.com/community/guides/scaling-nodejs/mise-explained/)
- [Home | mise-en-place](https://mise.jdx.dev/)
- [Environments | mise-en-place](https://mise.jdx.dev/environments/)
- [Configuration | mise-en-place](https://mise.jdx.dev/configuration.html)
- [Config Environments | mise-en-place](https://mise.jdx.dev/configuration/environments.html)
- [Config Environments · jdx/mise · Discussion #4307](https://github.com/jdx/mise/discussions/4307)
- [Dev Tools | mise-en-place](https://mise.jdx.dev/dev-tools/)
- [Walkthrough | mise-en-place](https://mise.jdx.dev/walkthrough.html)
- [Deterministic tool versions across environments with Mise](https://pepicrft.me/blog/2024/01/11/deterministic-tool-versions-across-envs)
- [Managing Development Tool Versions with mise | HARIL](https://haril.dev/en/blog/2024/06/27/Easy-devtools-version-management-mise)
- [Mise vs asdf: Which Version Manager Should You Choose? | Better Stack Community](https://betterstack.com/community/guides/scaling-nodejs/mise-vs-asdf/)
- [using mise in gitlab cicd pipelines · jdx/mise · Discussion #4808](https://github.com/jdx/mise/discussions/4808)
- [Setting `MISE_DATA_DIR` only works for absolute paths · jdx/mise · Discussion #4313](https://github.com/jdx/mise/discussions/4313)
