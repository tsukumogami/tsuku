# Learnings: Shell Integration Track A — Design Auto-Install Flow

Session context for design of issue #1679 (`docs: design auto-install flow`).

## What's Already Done

### Track A — Issues Completed

**#1677: Binary Index** — done, merged into PR #2169 (same branch: `docs/shell-integration-auto-install`)

- Package: `internal/index`
- `Lookup(ctx, command) ([]BinaryMatch, error)` — SQLite-backed, returns all recipes providing a command
- `BinaryMatch{Recipe, Command, BinaryPath, Installed, Source}`
- Errors: `ErrIndexNotBuilt` (exit 11), `ErrIndexCorrupt`, `StaleIndexWarning` (advisory, results still valid)
- `Open(dbPath, registryDir) (*BinaryIndex, error)` — opens the SQLite DB
- Shared helper: `lookupBinaryCommand(ctx, cfg, command)` in `cmd/tsuku/lookup.go`
- Exit code `ExitIndexNotBuilt = 11` in `cmd/tsuku/exitcodes.go`

**#1678: Command-Not-Found Handler** — done, merged into PR #2169

Key design decisions implemented:
- Shell hooks: `internal/hooks/tsuku.bash`, `internal/hooks/tsuku.zsh`, `internal/hooks/tsuku.fish`
- Hook install/uninstall via `internal/hook` package: `Install(shell, homeDir, shareHooksDir)`, `Uninstall(shell, homeDir)`, `Status(shell, homeDir)`
- Bash hook: wraps existing `command_not_found_handle` if present (detect-and-wrap pattern)
- Zsh hook: installs `command_not_found_handler`
- Fish hook: installs `fish_command_not_found`
- `_TSUKU_BASH_HOOK_LOADED` sentinel prevents double-install when bash sources rc file twice
- Recursion guard: hook checks `command -v tsuku` before calling `tsuku suggest` (prevents infinite loop if tsuku itself not found)
- Shell hooks call `tsuku suggest <command>` and print output to user
- `tsuku hook install --shell=<shell>` CLI subcommand in `cmd/tsuku/cmd_hook.go`
- Hook files go to `$TSUKU_HOME/share/hooks/` at install time
- Install script (`website/install.sh`) calls hook install best-effort (wrapped in if/else, not under set -e)

**`tsuku suggest` command:**
- `cmd/tsuku/cmd_suggest.go`
- Exit 0: matches found, Exit 1: no match, Exit 11: index not built
- `--json` flag for machine-readable output
- `runSuggest(ctx, stdout, stderr, cfg, command, jsonOutput) int` — injectable writers for unit tests

**Design doc:**
- `docs/designs/current/DESIGN-command-not-found.md` — status: Current (moved from `docs/designs/`)
- Doc: `docs/GUIDE-command-not-found.md` — user-facing guide

**PR #2169:** `feat: add command-not-found hook integration with tsuku suggest and shell hooks`
- Branch: `docs/shell-integration-auto-install`
- Status: all CI checks pass (green)
- wip/ is clean (CI check-artifacts enforces empty wip/ on non-draft PRs)

---

## What's Next

**#1679: Design auto-install flow** — THIS SESSION

Goal: create `docs/designs/DESIGN-auto-install.md` for `tsuku run <command> [args...]`.

Key scope from issue acceptance criteria:
- Three modes: `suggest` (print instructions), `confirm` (interactive prompt), `auto` (silent install)
- Config key: `auto_install_mode` with default `confirm`
- TTY detection for `confirm` mode; fallback when stdin not a TTY
- Version resolution: project config > latest (stub for #1680)
- Security: threat model, `auto` mode is opt-in, audit logging

After design is approved, implement all issue 1679 items in the same PR (#2169).

---

## Testing Patterns in This Codebase

### Unit tests
- Table-driven: `[]struct{ name, input, want }` pattern
- `t.TempDir()` for filesystem isolation — do not use hardcoded paths
- Injectable writers: pass `io.Writer` instead of writing to `os.Stdout` directly
- Example: `runSuggest(ctx, stdout, stderr, cfg, command, jsonOutput)` returns exit code, callers inject `bytes.Buffer`

### Container (Docker) tests
- Package: `internal/hooks/integration_test.go`
- Uses `containerimages.DefaultImage()` — do NOT inline a custom image lookup
- `runInContainer(t, script) string` — calls `t.Fatalf` on non-zero docker exit (no silent failures)
- Tests skip automatically when docker not available: `skipIfNoDocker(t)`
- Pattern: copy hook files into container, source them, run a command not in PATH, check output

### Hook unit tests
- Package: `internal/hook/install_test.go`
- Tests install/uninstall/idempotency without Docker, using `t.TempDir()` as fake home dir
- `makeShareHooksDir(t)` creates temp dir for hook files

### CI
- `go test ./...` runs all unit tests
- Container tests only run in `hook-integration-tests` job (`.github/workflows/container-tests.yml`)
- `check-artifacts` job enforces empty `wip/` on non-draft PRs — clean before pushing a non-draft PR
- `golangci-lint run --timeout=5m ./...` for lint
- `gofmt` required on all Go files

---

## Key Config Patterns

Config lives in `$TSUKU_HOME/config.toml`. User-facing config managed via `internal/userconfig` package.

Existing config keys (from `cmd/tsuku/config.go`):
- `telemetry` — bool
- `llm.enabled`, `llm.local_enabled`, `llm.idle_timeout`, `llm.providers`

New key to add: `auto_install_mode` (string: `suggest` | `confirm` | `auto`)

---

## Key Design Constraints

1. **Network-free lookup**: `lookupBinaryCommand` reads only local SQLite index — must stay offline
2. **No external calls on suggest**: hook fires on every unknown command; must be fast and safe
3. **`auto` mode is dangerous**: silent installs without user consent — must be explicit opt-in with documented warning
4. **`confirm` mode TTY requirement**: interactive prompt requires stdin to be a TTY; degrade gracefully in CI/scripts
5. **Project config integration (#1680)**: design must expose a clean interface so #1680 can plug in version pinning without modifying auto-install core
6. **Exit code preservation**: `tsuku run <command> [args]` must exit with the command's exit code, not tsuku's
7. **Partial install cleanup**: if install fails partway, must not leave broken state

---

## Relevant Code Paths for Implementation

After design is approved, implementation will touch:
- `cmd/tsuku/main.go` — register `run` subcommand
- New file: `cmd/tsuku/cmd_run.go` — `tsuku run <command> [args...]`
- `internal/userconfig/` — add `AutoInstallMode` field
- `cmd/tsuku/config.go` — register new config key
- `cmd/tsuku/exitcodes.go` — new exit codes if needed

The design should define:
- The CLI surface (`tsuku run` or alternative)
- The mode resolution order (flag > config > default)
- The install-then-exec flow
- Interface stub for project config version override (`ProjectVersionFor(command) (string, bool, error)`)
- Audit log location and format
- Error handling and exit codes
