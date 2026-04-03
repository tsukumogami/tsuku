---
status: Proposed
problem: |
  When a tool with install_shell_init is installed, its shell functions are written
  to ~/.tsuku/share/shell.d/ and the init cache is rebuilt, but ~/.tsuku/env (the
  static file sourced from .bashrc) only sets PATH. The init cache is never sourced
  in new terminals, so shell integration silently fails for every user.
---

# DESIGN: Shell Env Integration

## Status

Proposed

## Context and Problem Statement

`tsuku install` can install tools that provide shell functions (completions, aliases,
auto-cd hooks) via the `install_shell_init` action. These functions are written to
`~/.tsuku/share/shell.d/<tool>.<shell>` and aggregated into
`~/.tsuku/share/shell.d/.init-cache.<shell>` by `RebuildShellCache`.

The problem: `~/.tsuku/env` — the static file the installer adds to users' `.bashrc`
— only exports `PATH`. It doesn't source `.init-cache.<shell>`. So tools' shell
functions are built correctly but never loaded in new terminals.

This is a design sequencing gap: `~/.tsuku/env` was introduced on March 1, 2026;
the `shell.d` system on March 28, 2026. They were never wired together.

The impact is universal. Every user who installs via the official script is broken
for tools with shell integration. `tsuku shellenv` does source the init cache, but
it was designed as a fallback for non-standard installs, not as the primary path.

As of this writing, niwa is the only recipe using `install_shell_init` (~1,400 total
recipes), making this the right moment to fix the infrastructure.

## Decision Drivers

- **No subprocess overhead**: Shell startup performance matters. The fix should not
  add a subprocess call (as `eval "$(tsuku shellenv)"` would) to every terminal open.
- **Automatic migration for active users**: `EnsureEnvFile()` is called on every
  `tsuku install` and rewrites the file when content differs from `envFileContent` in
  `config.go`. Updating the constant is the migration path for active users.
- **Static file with shell detection**: `~/.tsuku/env` is sourced by both bash and zsh.
  It needs to source the right cache file. Shell detection via `$BASH_VERSION` /
  `$ZSH_VERSION` is the standard approach for static env files.
- **Don't break CI/Docker**: The init cache may not exist in non-interactive environments.
  Any sourcing line must use `[ -f ... ] &&` to fail safely.
- **TSUKU_NO_TELEMETRY preservation**: `EnsureEnvFile()` currently clobbers any
  customizations added by the installer (e.g., `TSUKU_NO_TELEMETRY=1`). The rewrite
  behavior needs to preserve these or be refactored to avoid losing user preferences.
- **Doctor repair path**: Users who never run `tsuku install` after this fix need a
  way to update their env file. `tsuku doctor --rebuild-cache` is referenced in doctor's
  error messages but not implemented. This is the repair surface.

## Decisions Already Made

From exploration (Round 1):

- **Update static env file, not switch to eval**: Avoids subprocess overhead on every
  shell start. `EnsureEnvFile()` provides an automatic migration path.
- **Fix in tsuku repo, not user dotfiles**: The problem is in `EnsureEnvFile()` and
  `website/install.sh`, not in individual user configurations.
- **Three-part fix scope**:
  1. Update `envFileContent` in `internal/config/config.go` to source the init cache
     with shell detection. Update `website/install.sh` to match.
  2. Fix `EnsureEnvFile()` to preserve user customizations (TSUKU_NO_TELEMETRY and
     any others) when rewriting.
  3. Implement `tsuku doctor --rebuild-cache` and add a check for env file staleness.
- **Niwa recipe fix is out of scope**: The `source_command` bare binary name bug is a
  separate PR in the niwa repo.
- **Fish shell deferred**: Handle bash and zsh first. Fish shell integration for the
  env file is a follow-on.

## Open Design Questions

1. **Shell detection in the static env file**: What's the correct guard? Options:
   - `[ -n "$BASH_VERSION" ]` / `[ -n "$ZSH_VERSION" ]` (standard, widely compatible)
   - `case "$0" in *bash*) ... ;; *zsh*) ... ;; esac` (more explicit)
   What's the right idiom for a file sourced by both bash and zsh?

2. **EnsureEnvFile rewrite behavior**: Currently idempotent against `envFileContent`.
   How should it preserve user-added content like `TSUKU_NO_TELEMETRY=1`? Options:
   - Replace only the managed section (marker-delimited block)
   - Read existing file, parse out user additions, and write them back
   - Stop clobbering: move the telemetry opt-out to a separate file

3. **Doctor --rebuild-cache scope**: Should `tsuku doctor --rebuild-cache` also update
   the env file if it's stale? Or are these two separate actions? Should there be a
   `--fix` flag that runs all repairs?

4. **Staleness detection**: How does doctor know whether the env file is outdated?
   Check if it contains the init cache sourcing line? Compare against `envFileContent`?
   What's the right heuristic for "env file needs updating"?
