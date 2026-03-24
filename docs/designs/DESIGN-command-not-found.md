---
status: Proposed
spawned_from:
  issue: 1678
  repo: tsukumogami/tsuku
  parent_design: docs/designs/DESIGN-shell-integration-building-blocks.md
problem: |
  When users type an unknown command, the shell prints "command not found" and
  stops. Tsuku has a binary index that can reverse-map command names to installable
  recipes, but there is no integration between the shell's hook mechanism and that
  index. Users must separately discover tsuku can provide the tool, find the recipe
  name, and run tsuku install — friction that undermines the zero-friction install
  promise.
decision: |
  The install script automatically calls `tsuku hook install` for the detected shell.
  For bash and zsh, a single stable source line is injected into the rc file pointing
  to `$TSUKU_HOME/share/hooks/tsuku.<shell>` — a file shipped with the tsuku binary
  and updated on every upgrade. For fish, a file is written to
  `~/.config/fish/conf.d/`. The hook uses shell-native function introspection to
  detect and preserve any existing command-not-found handler via detect-and-wrap. The
  `tsuku suggest <command>` subcommand performs the lookup and prints one suggestion
  line per match to stdout, silently on no-match.
rationale: |
  Auto-registration follows the precedent set by rustup and nvm and extends the
  approach tsuku already uses for PATH setup. Source-a-managed-file decouples the
  upgrade path from re-registration — hook logic evolves with the binary without
  users doing anything. Detect-and-wrap is the only option that works on
  Ubuntu/Debian where a handler is always pre-defined in /etc/bash.bashrc. Silent
  no-match output is correct because the shell's own "command not found" message
  already fires via the 127 return.
---

# DESIGN: Command-Not-Found Handler

## Status

Proposed

## Upstream Design Reference

Parent: [DESIGN: Shell Integration Building Blocks](DESIGN-shell-integration-building-blocks.md)
Relevant sections: Block 2 (Command-Not-Found Handler), Key Interfaces, Security Considerations.

## Context and Problem Statement

Tsuku currently requires users to know what they need and install it explicitly. When
a user types a command that isn't installed — `jq`, `rg`, `kubectl` — the shell prints
"command not found" and stops. The user must separately discover that tsuku can provide
it, find the right recipe name, and run `tsuku install`. This friction erodes the core
value proposition: tools should be discoverable at the moment of need.

All major shells provide a hook mechanism for exactly this case: `command_not_found_handle`
in bash, `command_not_found_handler` in zsh, and `fish_command_not_found` in fish. These
hooks fire when the shell can't find a command, giving tsuku an opportunity to suggest or
install what the user actually wanted. The binary index (Block 1, #1677) provides the
reverse lookup from command name to recipe — this design defines how to wire the shell
hook to that lookup and how to deliver the hook to users.

The parent design identified the key open questions: installation mechanism, uninstallation
safety, shell-specific implementations, and chaining with existing handlers. This design
resolves each of them.

### Scope

**In scope:**
- Shell hook functions for bash, zsh, and fish
- Hook delivery: how hooks reach users' shells (install script, explicit command, or both)
- `tsuku hook install [--shell=<shell>]` and `tsuku hook uninstall [--shell=<shell>]`
- `tsuku hook status` for verifying hook installation
- `tsuku suggest <command>` CLI command and output format
- Hook lifecycle across tsuku upgrades
- Chaining with existing command-not-found handlers

**Out of scope:**
- Auto-install flow (`tsuku run`, Block 3, #1679)
- Project configuration or version pinning (Block 4, #1680)
- LLM-based recipe discovery (future Block)
- Windows shell support (PowerShell, cmd.exe)

## Decision Drivers

- **Zero-friction discovery**: users should get suggestions without needing to run any additional setup command after installing tsuku, if that's feasible without security compromise
- **Don't break existing shell configs**: the hook mechanism must detect and preserve any existing `command_not_found` handler rather than silently overwriting it
- **50ms lookup budget**: the hook's contribution to latency must stay within the overall 50ms shell-integration budget defined by the parent design; `tsuku suggest` itself is network-free and uses the SQLite binary index
- **Clean uninstall**: `tsuku hook uninstall` must remove exactly what `tsuku hook install` added, leaving the rc file in a state indistinguishable from before install
- **Upgrade path**: tsuku upgrades must not leave stale hook code requiring manual re-registration
- **Security**: command names are passed as arguments to tsuku, never interpolated into shell; the hook must not introduce injection vectors
- **Precedent alignment**: UX choices should align with widely-accepted conventions from comparable developer tools

## Considered Options

### Decision 1: Hook delivery and lifecycle

**Question:** Should the tsuku install script auto-register shell hooks? If so, how — source a shipped file vs inject a snippet into the rc file? How do hooks stay current when tsuku is upgraded?

Four options were evaluated:

**Option A — Auto-register during install, source a shipped file (chosen):** The install script runs `tsuku hook install` for the detected shell. For bash and zsh, it injects one stable line into the rc file: `. "$TSUKU_HOME/share/hooks/tsuku.<shell>"`. The actual hook function lives in `$TSUKU_HOME/share/hooks/`, shipped with the tsuku binary and updated on every upgrade. The injected line never changes; only the file it sources changes. Fish uses `~/.config/fish/conf.d/tsuku.fish`, its native extension point.

**Option B — Auto-register, inject generated snippet:** The install script writes the full hook function body directly between `# tsuku hook begin`/`# tsuku hook end` markers in the rc file. Upgrade requires re-running `tsuku hook update` or users are silently left with stale behavior. Uninstall must remove a multi-line block precisely. Diverges from the pattern tsuku already uses for PATH setup.

**Option C — Manual only:** The install script prints instructions; users run `tsuku hook install` themselves. Most users skip this. Breaks with the established installer UX and undermines the zero-friction goal. Tools that use this pattern (mise, asdf, starship) are consistently cited for setup friction.

**Option D — Per-shell native mechanisms only:** Use conf.d for fish and equivalent drop-in directories for bash/zsh. No widely-supported conf.d equivalent exists for bash/zsh, so this collapses to Option A for those shells — adding no benefit over Option A while increasing implementation complexity.

A precedent survey of eight tools (nvm, rustup, mise, asdf, starship, oh-my-zsh, Homebrew, pyenv) confirms that tools which auto-modify rc files universally use the source-a-managed-file pattern. This matches the pattern tsuku already uses for PATH setup via `$TSUKU_HOME/env`.

### Decision 2: Chaining with existing command_not_found handlers

**Question:** What does `tsuku hook install` do when a command_not_found handler already exists — detect-and-wrap, abort with instructions, or unconditionally replace?

Five options were evaluated:

**Option A — Detect-and-wrap (chosen):** At shell startup, the sourced hook file checks whether a handler is already defined. If so, it copies the existing function under a `_tsuku_original_cnf_*` name using shell-native mechanisms (`declare -f`+`eval` for bash, `functions -c` for zsh, `functions --copy` for fish), then installs tsuku's wrapper. The wrapper calls `tsuku suggest` first, then always falls through to the original. A `command -v tsuku` guard (shell builtin, not tsuku itself) prevents infinite recursion when the tsuku binary is absent.

Ubuntu/Debian defines `command_not_found_handle` in `/etc/bash.bashrc`, which sources before `~/.bashrc`. When tsuku's hook file is sourced from `~/.bashrc`, Ubuntu's handler is already defined in the shell environment and is detected and wrapped. `/etc/bash.bashrc` is never touched.

**Option B — Detect-and-abort:** Print instructions and exit 1 if an existing handler is found. Ubuntu's handler is present on essentially every Ubuntu/Debian system, making this option fail for most Linux users on first install.

**Option C — Unconditional replace, save original to file:** Always replace the handler; save the original's source text to `$TSUKU_HOME/saved_cnf_handler.<shell>`. Introduces file-backed state that must be maintained per-shell. Restoring from a saved file requires evaling stored shell code. The snapshot may be wrong for handlers that check runtime conditions (Ubuntu's handler conditionally checks if `/usr/lib/command-not-found` exists).

**Option D — Source-order precedence:** Use loading order to ensure tsuku's definition wins. Fish's `conf.d/` is alphabetically ordered; naming the file `00_tsuku.fish` loads it early. No equivalent exists for bash/zsh. Source ordering alone can't chain — only the last definition wins. Still requires detect-and-wrap logic to chain effectively.

**Option E — Run original first, tsuku fallback:** A variant of Option A with reversed ordering. Running the original first risks suppressing tsuku's suggestion if the original returns 0 (none do for missing commands in practice, but it's fragile). Tsuku first is the cleaner design: both suggestions are shown when both handlers know about the command, giving users the choice of installer.

### Decision 3: `tsuku suggest` output and interface

**Question:** What does the command print when it finds a match, multiple matches, or no match? What exit code does it return? Should it support machine-readable output?

Three output formats were evaluated, with `--json` as an additive flag:

**Option A — Simple text, one line per match, with `--json` (chosen):**

Single match:
```
Command 'jq' not found. Install with: tsuku install jq
```
Multiple matches:
```
Command 'vi' not found. Provided by:
  vim (installed)   tsuku install vim
  neovim            tsuku install neovim
```
No match: silent. Exit 0 on match, 1 on no-match, 2 if index not built.

**Option B — Action only, no prose:** Print only the install command (`tsuku install jq`). Too terse for multi-match — without prose context, a user whose terminal has scrolled has no idea why `tsuku install jq` appeared. Multiple install commands with no labels give no basis for choosing between them.

**Option C — Rich output with recipe metadata:** Include description lines and version info alongside each match. Recipe descriptions require a registry cache lookup; version info may add latency. For the common single-match case, the extra information is noise — `tsuku info <recipe>` already provides rich metadata on demand. The hook fires on every unknown command including typos; output must be scannable at a glance.

**`--json` flag:** Not a standalone option. Adds machine-readable output (`{"command":"jq","matches":[...]}`) consistent with `tsuku search --json` and `tsuku info --json`. Costs nothing in implementation. Useful for scripters and future Block 3 tooling. Not advertised in hook output — documented in `tsuku suggest --help`.

### Cross-validation: D2 and D3 assumption alignment

Decision 2's original assumption stated "tsuku suggest exits 127 whether or not it finds a match, so the wrapper always falls through to the original handler." Decision 3 chose distinct exit codes (0=match, 1=not found, 2=index not built).

These are consistent when correctly interpreted: the wrapper always falls through to the original handler regardless of tsuku suggest's exit code. The exit code signals whether output was printed, not whether chaining should occur. Decision 2's intent — that both suggestions are shown when both handlers know about a command — is preserved. The stale assumption in Decision 2 is updated: tsuku suggest exits 0/1/2, but chaining is unconditional.

The resolved hook wrapper (bash example):

```bash
command_not_found_handle() {
    if command -v tsuku >/dev/null 2>&1; then
        tsuku suggest "$1"   # prints if match found (exit 0), silent if not (exit 1/2)
    fi
    _tsuku_original_cnf_handle "$@"   # always fall through to original
}
```

When no original handler exists, the wrapper exits 127 directly (the shell's own "command not found" message fires via the 127 return, no explicit message from tsuku's side).

## Decision Outcome

The three decisions compose into a coherent design:

**Hook delivery:** The install script calls `tsuku hook install` automatically for the detected shell (with `--no-hooks` to opt out). For bash and zsh, one stable source line is injected into the user's rc file; the actual hook logic lives in `$TSUKU_HOME/share/hooks/tsuku.<shell>` and is updated on every tsuku upgrade without requiring re-registration. For fish, `tsuku hook install` writes to `~/.config/fish/conf.d/tsuku.fish` — fish's native extension point. This mirrors rustup's `~/.cargo/env` pattern and extends the approach tsuku already uses for PATH setup.

**Chaining:** The sourced hook file detects any pre-existing `command_not_found` handler at shell startup time using shell-native function introspection. If one is found, it is copied under `_tsuku_original_cnf_*` and wrapped. Tsuku's suggestion runs first; the original handler always runs after. Ubuntu's handler in `/etc/bash.bashrc` is detected naturally because it is defined before `~/.bashrc` is sourced. Uninstall is simple: remove the source line from the rc file. No file-backed state, no permanent modifications to system files.

**`tsuku suggest` interface:** Single-line output per match with a prose prefix. Silent on no-match (the shell's own error fires via the 127 return). Exit 0 on match, 1 on no-match, 2 if the index is not built. `--json` flag for machine-readable output. Never interactive — prompting belongs to Block 3.

**Upgrade path:** When the tsuku binary upgrades, new hook files are written to `$TSUKU_HOME/share/hooks/`. The source line in the user's rc file picks up the updated logic on next shell start. No stale hook problem.

**Uninstall contract:** `tsuku hook uninstall` removes the `# tsuku hook` marker and source line from the rc file (bash/zsh) or deletes `~/.config/fish/conf.d/tsuku.fish` (fish). The rc file reverts to its pre-install state. The `_tsuku_original_cnf_*` copies exist only in running shell memory and are gone on next shell start.

## Solution Architecture

### Overview

When a user types an unknown command, the shell fires its command-not-found hook. Tsuku registers that hook by shipping a hook file to `$TSUKU_HOME/share/hooks/` and sourcing it from the user's rc file via a single stable line. The hook calls `tsuku suggest`, which looks up the binary index (SQLite, Block 1) and prints an install suggestion. Any pre-existing handler is detected at shell startup time and preserved via detect-and-wrap.

### Components

```
website/install.sh
  └── calls tsuku hook install (detected shell, --no-hooks to skip)

tsuku hook install [--shell=<shell>]
  ├── bash/zsh: appends "# tsuku hook" + ". $TSUKU_HOME/share/hooks/tsuku.<shell>" to rc file
  └── fish: writes $TSUKU_HOME/share/hooks/tsuku.fish, symlinks to ~/.config/fish/conf.d/tsuku.fish

$TSUKU_HOME/share/hooks/tsuku.bash  (shipped with binary, updated on upgrade)
$TSUKU_HOME/share/hooks/tsuku.zsh
$TSUKU_HOME/share/hooks/tsuku.fish
  └── each defines the detect-and-wrap handler that calls "tsuku suggest $1"

tsuku suggest <command>
  └── calls BinaryIndex.Lookup(command) → formats output → exit 0/1/2

tsuku hook uninstall [--shell=<shell>]
  ├── bash/zsh: removes "# tsuku hook" line + source line from rc file
  └── fish: removes ~/.config/fish/conf.d/tsuku.fish

tsuku hook status [--shell=<shell>]
  └── checks rc file for marker, reports installed/not-installed
```

### Key Interfaces

**Binary index lookup** (from DESIGN-binary-index):

```go
type BinaryIndex interface {
    Lookup(ctx context.Context, command string) ([]BinaryMatch, error)
}

type BinaryMatch struct {
    Recipe      string
    BinaryPath  string
    Installed   bool
}
```

`tsuku suggest` calls `BinaryIndex.Lookup()` directly — it does not shell out to another tsuku subcommand.

**Hook file marker** (bash/zsh):

```bash
# tsuku hook
. "$TSUKU_HOME/share/hooks/tsuku.bash"
```

The comment line is the stable identifier used by `tsuku hook uninstall`. The source path never changes; only the file it points to changes on upgrade.

**Fish conf.d path:** `~/.config/fish/conf.d/tsuku.fish`

**Bash hook file** (`$TSUKU_HOME/share/hooks/tsuku.bash`):

```bash
# tsuku command-not-found handler
if command -v tsuku >/dev/null 2>&1; then
    if declare -f command_not_found_handle >/dev/null 2>&1; then
        # Save existing handler before wrapping
        eval "_tsuku_original_cnf_handle() $(declare -f command_not_found_handle | tail -n +2)"
        command_not_found_handle() {
            tsuku suggest "$1"
            _tsuku_original_cnf_handle "$@"
        }
    else
        command_not_found_handle() {
            tsuku suggest "$1"
            return 127
        }
    fi
fi
```

**Zsh hook file** (`$TSUKU_HOME/share/hooks/tsuku.zsh`):

```zsh
# tsuku command-not-found handler
if (( $+commands[tsuku] )); then
    if (( $+functions[command_not_found_handler] )); then
        functions -c command_not_found_handler _tsuku_original_cnf_handler
        command_not_found_handler() {
            tsuku suggest "$1"
            _tsuku_original_cnf_handler "$@"
        }
    else
        command_not_found_handler() {
            tsuku suggest "$1"
            return 127
        }
    fi
fi
```

**Fish hook file** (`$TSUKU_HOME/share/hooks/tsuku.fish`):

```fish
# tsuku command-not-found handler
if command -q tsuku
    if functions --query fish_command_not_found
        functions --copy fish_command_not_found _tsuku_original_fish_cnf
        function fish_command_not_found
            tsuku suggest $argv[1]
            _tsuku_original_fish_cnf $argv
        end
    else
        function fish_command_not_found
            tsuku suggest $argv[1]
        end
    end
end
```

**`tsuku suggest` exit codes:**

| Condition | Exit code |
|-----------|-----------|
| One or more matches found | 0 |
| No match | 1 |
| Index not built | 4 (`ExitIndexNotBuilt`, new constant) |
| Other error | 1 |

Exit 2 (`ExitUsage`) is reserved for argument errors. Exit 4 is a new constant added to `cmd/tsuku/exitcodes.go` for "index not built" so shell scripts can distinguish the three states.

**`tsuku suggest --json` output:**

```json
{
  "command": "vi",
  "matches": [
    {"recipe": "vim",    "binary_path": "bin/vi",  "installed": true},
    {"recipe": "neovim", "binary_path": "bin/vi",  "installed": false}
  ]
}
```

No-match: `{"command":"xyz","matches":[]}` with exit 1. `matches` is never null.

### Data Flow

```
user types: jq
     │
     ▼
shell: command not found
     │
     ▼
command_not_found_handle "jq"
     │
     ├── command -v tsuku (shell builtin, microseconds)
     │       │
     │       ▼
     │   tsuku suggest jq
     │       │
     │       ├── BinaryIndex.Lookup("jq")  ← SQLite, ~5ms
     │       │       │
     │       │       └── match found
     │       │
     │       └── stdout: "Command 'jq' not found. Install with: tsuku install jq"
     │           exit 0
     │
     └── _tsuku_original_cnf_handle "jq"  (if one was wrapped, else return 127)
             │
             └── may print: "apt suggests installing package 'jq'"
                 return 127
```

## Implementation Approach

### Phase 1: `tsuku suggest` command

Implement the `suggest` subcommand in the CLI. `tsuku which` already implements `BinaryIndex.Lookup()` with full error handling for `ErrIndexNotBuilt` and `StaleIndexWarning`. `tsuku suggest` must share that lookup path — not duplicate it. Extract a `lookupBinaryCommand(cfg, command string) ([]index.BinaryMatch, error)` helper in `cmd/tsuku/` that both commands call.

Deliverables:
- `cmd/tsuku/lookup.go` — shared `lookupBinaryCommand()` helper used by both `tsuku which` and `tsuku suggest`
- `cmd/tsuku/cmd_suggest.go` — suggest subcommand: calls `lookupBinaryCommand()`, formats output, handles `--json` flag
- `cmd/tsuku/cmd_suggest_test.go` — unit tests: single match, multi-match, no match, index-not-built (exit 4)
- `cmd/tsuku/exitcodes.go` — add `ExitIndexNotBuilt = 4`

### Phase 2: Hook files

Write the three hook files as static assets shipped with the binary. These files live at `$TSUKU_HOME/share/hooks/` after install.

Deliverables:
- `internal/hooks/tsuku.bash` — bash hook with detect-and-wrap
- `internal/hooks/tsuku.zsh` — zsh hook with detect-and-wrap
- `internal/hooks/tsuku.fish` — fish hook with detect-and-wrap
- `internal/hooks/embed.go` — `embed.FS` declaration so hooks are bundled into the binary; written with `0644` permissions
- Shell integration tests (bash, zsh, fish in CI containers) verifying detect-and-wrap behavior

### Phase 3: `tsuku hook` subcommands

Implement `hook install`, `hook uninstall`, and `hook status`. These read/write the user's rc file and manage the fish conf.d file. The `$TSUKU_HOME/share/hooks/` path is derived at the `cmd/tsuku/` layer via `config.DefaultConfig()` and passed into `internal/hook/` functions as a parameter — `internal/hook/` must not import `internal/config` directly.

Deliverables:
- `cmd/tsuku/cmd_hook.go` — hook subcommand router; derives config and passes paths to internal/hook
- `internal/hook/install.go` — rc file detection, atomic marker insertion, fish conf.d management
- `internal/hook/uninstall.go` — marker removal, idempotent
- `internal/hook/status.go` — presence check, reports per-shell state
- `internal/hook/install_test.go` — tests with temp home directories

### Phase 4: Install script integration

Update `website/install.sh` to call `tsuku hook install` as the final setup step. Add `--no-hooks` flag.

Deliverables:
- `website/install.sh` — detect shell, call `tsuku hook install`, handle `--no-hooks`
- Updated install output messages

## Security Considerations

**Install script delivery.** The `website/install.sh` bootstrap script is delivered via curl-pipe-sh, the same pattern used by rustup and nvm. Users who require stronger guarantees can download the script and inspect it before execution. Checksums for any downloaded artifacts must be published on GitHub Releases (or another independent source), not co-located on the same CDN — a CDN compromise would defeat a co-located checksum.

**rc file writes.** `tsuku hook install` modifies `~/.bashrc` or `~/.zshrc`. Implementations must use atomic writes (write to a temp file, then rename) to prevent rc file corruption on interrupted writes, check for the tsuku marker before appending so repeated `hook install` calls don't accumulate duplicate entries, and on uninstall remove only the known marker block.

**eval in bash hook.** The bash hook uses `eval` to copy an existing `command_not_found_handle`. This applies to bash only — zsh uses `functions -c` and fish uses `functions --copy`, which are safe copy operations without eval. In bash, the eval input is `declare -f` output: the shell's own serialization of a currently-defined function, not user input. The hook file should include a comment explaining this reasoning for future maintainers. Users in hardened environments should audit their existing handler before installing.

**Hook file permissions.** `$TSUKU_HOME/share/hooks/tsuku.*` are sourced on every shell start. These files must be written with `0644` permissions. The implementation must not create `$TSUKU_HOME/share/hooks/` with a permissive umask that allows group or world writes.

**Command name privacy.** Every unrecognized command the user types is passed to `tsuku suggest` as a process argument. `tsuku suggest` must remain network-free: it reads only the local binary index and must not transmit command names or query results externally. This constraint should be enforced in code review and noted in the implementation with a godoc comment.

**Suggest timeout.** A corrupted binary index could cause `tsuku suggest` to hang, blocking every interactive shell session. The hook implementation should enforce the 50ms budget at the process level using a context timeout, not just rely on SQLite's typical performance.

**No privilege escalation.** This feature requires no sudo, no elevated capabilities, and no setuid. All operations are scoped to the current user's home directory.

## Consequences

### Positive

- **Zero-friction discovery**: hooks are active after a standard tsuku install with no extra steps. Users who type `jq` for the first time see an install suggestion immediately.
- **Upgrade-safe by design**: hook logic lives in `$TSUKU_HOME/share/hooks/`, updated when the tsuku binary upgrades. The source line in the rc file never needs to change.
- **Non-destructive coexistence**: Ubuntu's `command_not_found_handle`, oh-my-zsh plugins, and other existing handlers are detected at shell startup and preserved. Both suggestions appear when both handlers match.
- **Clean uninstall**: a single identifiable marker line is all that's written to the user's rc file. Removal leaves the file byte-for-byte identical to its pre-install state.
- **Transparent**: the hook file at `$TSUKU_HOME/share/hooks/tsuku.bash` is human-readable and inspectable at any time. Security-conscious users can audit exactly what runs on shell startup.

### Negative

- **Single-shell auto-configuration**: the install script detects one shell (via `$SHELL`). Users who regularly use multiple shells (bash + fish, zsh + bash) must run `tsuku hook install --shell=<shell>` manually for secondary shells.
- **Shell startup overhead**: sourcing the hook file and running the detect-and-wrap logic adds a small amount of work to every shell startup, even when no unknown command is typed. In practice this is microseconds, but it's not zero.
- **Fish conf.d is a persistent file**: unlike bash/zsh where the rc line simply stops sourcing if tsuku is uninstalled, the fish conf.d file remains until explicitly removed by `tsuku hook uninstall`. Users who uninstall tsuku without running `tsuku hook uninstall` get a silent error on fish startup.
- **`declare -f` bash limitation**: copying an existing bash handler via `declare -f | eval` is POSIX-standard but involves eval. This is the only eval in the hook path and is bounded to a function body, but it's worth noting.

### Mitigations

- **Multi-shell users**: `tsuku hook install --shell=<shell>` is the documented path for secondary shells. `tsuku hook status` shows per-shell state. The install output notes which shell was configured.
- **Startup overhead**: the `command -v tsuku` guard in the hook file uses a shell builtin (not a process exec). If tsuku is in PATH, the guard passes in microseconds. The full cost is well within the 50ms budget and only manifests when an unknown command is typed.
- **Fish conf.d persistence**: `tsuku uninstall` (full removal) runs `tsuku hook uninstall` as part of cleanup, catching the fish file. Users who manually delete the tsuku binary are already in a supported recovery path: the fish hook file checks for tsuku presence before defining the handler.
- **eval in bash wrapper**: the eval is applied to a function body captured by `declare -f`, which is the shell's own serialization of a defined function. The input is never user-supplied text. This is the accepted pattern for bash function copying.

### Additional Decision: rc file target for bash

The install script writes the source line to `~/.bashrc` (interactive shells) rather than `~/.bash_profile` (login shells) or both. This matches tsuku's existing PATH setup behavior and is consistent with the primary audience (interactive terminal users). Users who rely on login shells exclusively (e.g., remote SSH sessions that don't source `~/.bashrc`) would need to manually add the source line to `~/.bash_profile`. The alternative — writing to both files — risks double-sourcing for users whose `~/.bash_profile` sources `~/.bashrc`, which is the common case on Linux.
