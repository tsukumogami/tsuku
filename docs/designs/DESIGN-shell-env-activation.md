# DESIGN: Shell Environment Activation

## Status

Proposed

## Upstream Design Reference

Parent: [DESIGN: Shell Integration Building Blocks](DESIGN-shell-integration-building-blocks.md)
Block 5 in the six-block architecture. This design specifies how tsuku dynamically modifies PATH based on the current directory's project configuration, enabling per-project tool versions without manual switching. Consumes the `ProjectConfig` interface from Block 4 (#1680, implemented).

## Context and Problem Statement

Tsuku currently manages tool versions globally. `tsuku activate <tool> <version>` switches a symlink in `$TSUKU_HOME/tools/current/`, and `tsuku shellenv` adds that directory to PATH. This works for single-tool switching but doesn't handle the per-project use case: a developer working on project A needs Go 1.22, but project B needs Go 1.21. Switching between them requires manual `tsuku activate` calls every time they change directories.

With Block 4 implemented, projects can now declare their tool requirements in `.tsuku.toml`. But the config is only consumed by `tsuku install` (batch install) -- there's no mechanism to automatically activate the right tool versions when entering a project directory.

Shell environment activation bridges this gap. When a developer enters a project directory (or runs `tsuku shell`), tsuku reads `.tsuku.toml` and modifies PATH to point to the project's declared tool versions instead of the global `tools/current/` symlinks. When they leave, PATH reverts.

### Scope

**In scope:**
- Activation mechanism (prompt hooks and/or explicit `tsuku shell` command)
- PATH modification strategy (how project tool paths replace global paths)
- State tracking (how tsuku knows what's active and can restore)
- Deactivation behavior (restoring original PATH on directory change)
- Shell-specific implementations (bash, zsh, fish)
- Integration with existing `shellenv` and `activate` commands
- `EnvActivator` interface specification

**Out of scope:**
- Auto-install during activation (Block 6, #2168 handles install-on-demand)
- Environment variable management beyond PATH (future enhancement)
- Windows support
- LLM integration

### Existing Infrastructure

**What we have today:**
- `tsuku shellenv` -- prints static `export PATH="$TSUKU_HOME/bin:$TSUKU_HOME/tools/current:$PATH"`
- `tsuku activate <tool> <version>` -- creates symlinks in `tools/current/` for a specific version
- Shell hooks in `internal/hooks/` -- bash/zsh/fish scripts for command-not-found only
- `tsuku hook install` -- appends source lines to shell config files
- `internal/project.LoadProjectConfig(startDir)` -- discovers and parses `.tsuku.toml`
- `internal/project.ConfigResult` -- carries parsed config with file path and directory
- `internal/config.Config.ToolDir(name, version)` -- returns `$TSUKU_HOME/tools/{name}-{version}`

**What needs to change:**
- New prompt hook functions alongside the existing command-not-found hooks
- A `tsuku shell` (or extended `shellenv`) command that outputs activation scripts
- State tracking to enable clean deactivation
- Per-project PATH entries that override global `tools/current/` entries

## Decision Drivers

- **Performance**: Shell hooks must complete in under 50ms to avoid perceptible delay on every prompt
- **Correctness**: PATH must reflect the project's declared tools accurately; stale state is worse than no activation
- **Reversibility**: Deactivation must cleanly restore the original PATH without residue
- **Shell hooks are optional**: `tsuku shell` must work as an explicit alternative to prompt hooks
- **Compatibility**: Must coexist with the existing `shellenv` output and `activate` command
- **Simplicity**: Prefer the simplest mechanism that delivers correct behavior
- **Cross-shell**: Must work on bash, zsh, and fish (the same shells as existing hooks)
