---
status: Proposed
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
spawned_from:
  issue: 1678
  repo: tsukumogami/tsuku
  parent_design: docs/designs/DESIGN-shell-integration-building-blocks.md
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

### Decision 1: Hook Delivery and Lifecycle

Should the tsuku install script auto-register shell hooks? If so, how — source a shipped
file vs inject a snippet into the rc file? How do hooks stay current when tsuku is upgraded?

**Key assumptions:**
- The installer is a curl-pipe-sh script similar to rustup and nvm
- Shell detection is limited to `$SHELL` at install time; secondary shells require manual setup
- Fish's `~/.config/fish/conf.d/` is the supported extension point and does not require touching `config.fish`
- "Clean uninstall" means the rc file is identical to its pre-install state

A precedent survey of eight tools (nvm, rustup, mise, asdf, starship, oh-my-zsh, Homebrew, pyenv)
confirms that tools which auto-modify rc files universally use the source-a-managed-file pattern.
This matches the pattern tsuku already uses for PATH setup via `$TSUKU_HOME/env`.

#### Chosen: Auto-register during install, source a shipped file

The install script runs `tsuku hook install` for the detected shell. For bash and zsh, it
injects one stable line into the rc file:

```bash
# tsuku hook
. "$TSUKU_HOME/share/hooks/tsuku.bash"
```

The actual hook function lives in `$TSUKU_HOME/share/hooks/`, shipped with the tsuku binary
and updated on every upgrade. The injected line never changes; only the file it sources changes.
Fish uses `~/.config/fish/conf.d/tsuku.fish`, its native extension point.

Users who prefer manual setup pass `--no-hooks` to the installer. The `tsuku hook install`
command is also available for secondary shells or post-install use.

#### Alternatives Considered

**Auto-register, inject generated snippet:** Write the full hook function body between
`# tsuku hook begin`/`# tsuku hook end` markers in the rc file. Upgrade requires users to
re-run `tsuku hook update` or they are silently left with stale hook behavior. Uninstall
must remove a multi-line block precisely, which is fragile if users manually edit the region.
Diverges from the pattern tsuku already uses for PATH setup.

**Manual only:** Print instructions; users run `tsuku hook install` themselves. Most users
skip this step. Breaks consistency with the existing installer UX, which already auto-configures
PATH. Tools that follow this pattern (mise, asdf, starship) are consistently cited for setup
friction in their communities.

**Per-shell native mechanisms only:** Use `conf.d/` for fish and equivalent drop-in directories
for bash/zsh. No widely-supported conf.d equivalent exists for bash/zsh (`/etc/profile.d/`
requires root; `~/.bash.d/` is not built-in). This option collapses to the chosen option for
bash/zsh while adding implementation complexity with no material benefit.

---

### Decision 2: Chaining with Existing command_not_found Handlers

What does `tsuku hook install` do when a command_not_found handler already exists in the shell
environment — detect-and-wrap, abort with instructions, or unconditionally replace?

**Key assumptions:**
- Ubuntu/Debian defines `command_not_found_handle` in `/etc/bash.bashrc`, which sources before `~/.bashrc`
- Shell function introspection is available in all minimum supported versions: bash 3.2+, zsh 5.0+, fish 3.0+
- `command -v tsuku` builtin (not tsuku itself) checks for tsuku presence — prevents infinite recursion
- Both tsuku's suggestion and the original handler's suggestion are shown when both apply

#### Chosen: Detect-and-Wrap

At shell startup, the sourced hook file checks whether a handler is already defined using
shell-native function introspection. If one is found, it is copied under `_tsuku_original_cnf_*`
and tsuku's wrapper is installed in its place. The wrapper calls `tsuku suggest` first, then
always falls through to the original handler.

Shell-native copy mechanisms:
- **Bash**: `declare -f command_not_found_handle` detects, `eval` + `declare -f` reconstitutes
- **Zsh**: `(( $+functions[command_not_found_handler] ))` detects, `functions -c src dst` copies
- **Fish**: `functions --query fish_command_not_found` detects, `functions --copy src dst` copies

Ubuntu/Debian defines `command_not_found_handle` in `/etc/bash.bashrc`, which sources before
`~/.bashrc`. When tsuku's hook file is sourced from `~/.bashrc`, Ubuntu's handler is already
defined and is detected and wrapped. `/etc/bash.bashrc` is never touched.

Uninstall is simple: remove the source line from the rc file. The `_tsuku_original_cnf_*`
copies exist only in running shell memory and are gone on next shell start.

#### Alternatives Considered

**Detect-and-abort:** Print instructions and exit 1 if an existing handler is found. Ubuntu's
handler is present on essentially every Ubuntu/Debian system, making this option fail for most
Linux users on first install.

**Unconditional replace, save original to file:** Always replace the handler and save the original's
source text to `$TSUKU_HOME/saved_cnf_handler.<shell>`. Introduces file-backed state per shell.
Restoring from a saved file requires evaling stored code. Snapshots may be wrong for handlers
that check runtime conditions (Ubuntu's handler conditionally checks if `/usr/lib/command-not-found`
exists). More implementation surface than detect-and-wrap for no benefit.

**Source-order precedence:** Use loading order to ensure tsuku's definition wins. Fish's `conf.d/`
is alphabetically ordered; no equivalent exists for bash/zsh. Source ordering alone can't chain —
only the last definition wins — so detect-and-wrap logic is still required regardless.

**Run original first, tsuku fallback:** A variant of the chosen option with reversed ordering.
Running the original first risks suppressing tsuku's suggestion if the original returns 0 (none do
for missing commands in practice, but the dependency is fragile). Tsuku first is the cleaner
design: both suggestions are shown when both handlers know about the command.

---

### Decision 3: `tsuku suggest` Output and Interface

What does the command print when it finds a match, multiple matches, or no match? What exit code
does it return? Should it support machine-readable output?

**Key assumptions:**
- The 50ms budget is dominated by process startup; output formatting adds negligible time
- `tsuku suggest` is never called in a non-interactive pipeline except by scripters explicitly invoking it
- Block 3 (`tsuku run`) calls `BinaryIndex.Lookup()` directly and does not parse `tsuku suggest` output
- Exit code 2 (`ExitUsage`) is already reserved for argument errors; "index not built" must use a different code

#### Chosen: Simple text, one line per match, with `--json` flag

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

No match: silent. The shell's own "command not found" message fires via the 127 return;
adding a second line from tsuku on every typo or missing system binary would be noise.

Exit codes:
| Condition | Exit code |
|-----------|-----------|
| One or more matches found | 0 |
| No match | 1 |
| Index not built | 4 (`ExitIndexNotBuilt`, new constant) |
| Other error | 1 |

Exit 4 is a new constant in `cmd/tsuku/exitcodes.go`. Exit 0/1 mirrors `grep` and `which` conventions.
The `--json` flag adds machine-readable output consistent with `tsuku search --json` and `tsuku info --json`.

#### Alternatives Considered

**Action only, no prose:** Print only the install command (`tsuku install jq`). Too terse for
multi-match — without prose context, a user whose terminal has scrolled has no idea why
`tsuku install jq` appeared. Multiple install commands with no labels give no basis for choosing
between them.

**Rich output with recipe metadata:** Include description lines and version info alongside each
match. For the common single-match case, the extra information is noise — `tsuku info <recipe>`
already provides rich metadata on demand. The hook fires on every unknown command including typos;
output must be scannable at a glance.

**Interactive prompting:** Rejected categorically. Prompting inside a command_not_found hook
interrupts the user's thought process on every typo and breaks non-TTY contexts (scripts, CI).
Prompting belongs to Block 3 (`tsuku run`).

---

### Cross-Validation: D2 and D3 Assumption Alignment

Decision 2's original assumption stated "tsuku suggest exits 127 whether or not it finds a match,
so the wrapper always falls through to the original handler." Decision 3 chose distinct exit codes
(0=match, 1=not found, 4=index not built).

These are consistent when correctly interpreted: the wrapper always falls through to the original
handler regardless of tsuku suggest's exit code. The exit code signals whether output was printed,
not whether chaining should occur. Decision 2's intent — that both suggestions are shown when both
handlers know about a command — is preserved.

Resolved hook wrapper (bash example):

```bash
command_not_found_handle() {
    if command -v tsuku >/dev/null 2>&1; then
        tsuku suggest "$1"   # prints if match found (exit 0), silent if not (exit 1/4)
    fi
    _tsuku_original_cnf_handle "$@"   # always fall through to original
}
```

When no original handler exists, the wrapper exits 127 directly.

## Decision Outcome

### Summary

The binary index (Block 1) is queried by `tsuku suggest`, which the shell's command-not-found
hook calls whenever a user types an unknown command. Hooks are registered automatically during
`tsuku install` by sourcing a managed file from the user's rc file. Any pre-existing handler is
preserved via detect-and-wrap. The install command and uninstall command together provide the
full lifecycle interface.

### Rationale

Each decision reinforces the others. Auto-registration via source-a-managed-file (Decision 1)
gives every user hooks without extra steps and keeps the upgrade path self-maintaining. Detect-and-wrap
(Decision 2) is the only approach that survives the Ubuntu/Debian case without touching system
files or requiring manual configuration. Simple text output with distinct exit codes (Decision 3)
keeps the hook fast and composable without adding complexity that belongs in Block 3.

The three decisions are consistent after cross-validation: chaining is unconditional (the hook
always falls through to the original handler), and exit codes are informational rather than
gating. This means users on Ubuntu see both tsuku's suggestion and apt's suggestion — not one
or the other.

### Trade-offs Accepted

- **Single-shell auto-configuration**: the install script configures only the detected shell.
  Secondary shells require `tsuku hook install --shell=<shell>`. This matches the existing
  PATH setup behavior and is the same limitation users accept today.
- **Shell startup overhead**: detect-and-wrap runs at every shell start, not only when an unknown
  command is typed. The cost is a `command -v` builtin check (microseconds) plus conditional
  function copy — imperceptible in practice.
- **Fish conf.d persistence**: the fish hook file is not automatically removed if the tsuku binary
  is manually deleted. `tsuku uninstall` handles this; manual binary deletion is an unsupported
  path.
- **eval in bash**: copying an existing bash handler requires eval. This is the accepted pattern
  for bash function copying and is bounded to a function body, not user input.

## Solution Architecture

### Overview

When a user types an unknown command, the shell fires its command-not-found hook. Tsuku registers
that hook by shipping a hook file to `$TSUKU_HOME/share/hooks/` and sourcing it from the user's
rc file via a single stable line. The hook calls `tsuku suggest`, which looks up the binary index
(SQLite, Block 1) and prints an install suggestion. Any pre-existing handler is detected at shell
startup time and preserved via detect-and-wrap.

### Components

```
website/install.sh
  └── calls tsuku hook install (detected shell, --no-hooks to skip)

tsuku hook install [--shell=<shell>]
  ├── bash/zsh: appends "# tsuku hook" + ". $TSUKU_HOME/share/hooks/tsuku.<shell>" to rc file
  └── fish: writes $TSUKU_HOME/share/hooks/tsuku.fish, copies to ~/.config/fish/conf.d/tsuku.fish

$TSUKU_HOME/share/hooks/tsuku.bash  (shipped with binary, updated on upgrade)
$TSUKU_HOME/share/hooks/tsuku.zsh
$TSUKU_HOME/share/hooks/tsuku.fish
  └── each defines the detect-and-wrap handler that calls "tsuku suggest $1"

tsuku suggest <command>
  └── calls lookupBinaryCommand() → BinaryIndex.Lookup() → formats output → exit 0/1/4

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
    Command     string
    BinaryPath  string
    Installed   bool
    Source      string
}
```

`tsuku suggest` shares a `lookupBinaryCommand()` helper with `tsuku which` — both commands
call the same function for index open, lookup, and error handling. `tsuku suggest` does not
shell out to another subcommand.

**Hook file marker** (bash/zsh rc file):

```bash
# tsuku hook
. "$TSUKU_HOME/share/hooks/tsuku.bash"
```

The comment line is the stable identifier used by `tsuku hook uninstall`. The source path
never changes; only the file it points to changes on upgrade.

**Fish conf.d path:** `~/.config/fish/conf.d/tsuku.fish`

**Bash hook file** (`$TSUKU_HOME/share/hooks/tsuku.bash`):

```bash
# tsuku command-not-found handler
# The eval below is safe: it applies declare -f output (the shell's own function
# serialization), not user input. See DESIGN-command-not-found.md for rationale.
if command -v tsuku >/dev/null 2>&1; then
    if declare -f command_not_found_handle >/dev/null 2>&1; then
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

### Phase 1: `tsuku suggest` Command

`tsuku which` already implements `BinaryIndex.Lookup()` with full error handling. Extract a shared
`lookupBinaryCommand()` helper so both commands use the same path — no parallel implementations.

Deliverables:
- `cmd/tsuku/lookup.go` — shared `lookupBinaryCommand(cfg, command string) ([]index.BinaryMatch, error)` helper
- `cmd/tsuku/cmd_suggest.go` — suggest subcommand: calls helper, formats output, handles `--json` flag
- `cmd/tsuku/cmd_suggest_test.go` — unit tests: single match, multi-match, no match, index-not-built (exit 4)
- `cmd/tsuku/exitcodes.go` — add `ExitIndexNotBuilt = 4`

### Phase 2: Hook Files

Write the three hook files as static assets embedded in the binary via `embed.FS`. They are
written to `$TSUKU_HOME/share/hooks/` by `tsuku hook install`.

Deliverables:
- `internal/hooks/tsuku.bash` — bash hook with detect-and-wrap
- `internal/hooks/tsuku.zsh` — zsh hook with detect-and-wrap
- `internal/hooks/tsuku.fish` — fish hook with detect-and-wrap
- `internal/hooks/embed.go` — `embed.FS` declaration; files written with `0644` permissions
- Shell integration tests (bash, zsh, fish in CI containers) verifying detect-and-wrap behavior

### Phase 3: `tsuku hook` Subcommands

Implement `hook install`, `hook uninstall`, and `hook status`. The `$TSUKU_HOME/share/hooks/`
path is derived at the `cmd/tsuku/` layer via `config.DefaultConfig()` and passed into
`internal/hook/` as a parameter — `internal/hook/` must not import `internal/config` directly.

Deliverables:
- `cmd/tsuku/cmd_hook.go` — hook subcommand router; derives config and passes paths to internal/hook
- `internal/hook/install.go` — rc file detection, atomic marker insertion, fish conf.d management
- `internal/hook/uninstall.go` — marker removal, idempotent
- `internal/hook/status.go` — presence check, reports per-shell state
- `internal/hook/install_test.go` — tests with temp home directories

### Phase 4: Install Script Integration

Update `website/install.sh` to call `tsuku hook install` as the final setup step.

Deliverables:
- `website/install.sh` — detect shell, call `tsuku hook install`, handle `--no-hooks`
- Updated install output messages

## Security Considerations

### Install Script Delivery

The `website/install.sh` bootstrap script is delivered via curl-pipe-sh, the same pattern used
by rustup and nvm. Users who require stronger guarantees can download the script and inspect it
before execution. Checksums for any downloaded artifacts must be published on GitHub Releases
(or another independent source), not co-located on the same CDN — a CDN compromise would defeat
a co-located checksum.

### rc File Writes

`tsuku hook install` modifies `~/.bashrc` or `~/.zshrc`. Implementations must use atomic writes
(write to a temp file, then rename) to prevent rc file corruption on interrupted writes, check
for the tsuku marker before appending so repeated `hook install` calls don't accumulate duplicate
entries, and on uninstall remove only the known marker block.

### eval in Bash Hook

The bash hook uses `eval` to copy an existing `command_not_found_handle`. This applies to bash
only — zsh uses `functions -c` and fish uses `functions --copy`, which are safe copy operations
without eval. In bash, the eval input is `declare -f` output: the shell's own serialization of
a currently-defined function, not user input. The hook file includes a comment explaining this
reasoning for future maintainers. Users in hardened environments should audit their existing
handler before installing.

### Hook File Permissions

`$TSUKU_HOME/share/hooks/tsuku.*` are sourced on every shell start. These files must be written
with `0644` permissions. The implementation must not create `$TSUKU_HOME/share/hooks/` with a
permissive umask that allows group or world writes.

### Command Name Privacy

Every unrecognized command the user types is passed to `tsuku suggest` as a process argument.
`tsuku suggest` must remain network-free: it reads only the local binary index and must not
transmit command names or query results externally. This constraint should be enforced in code
review and noted in the implementation with a godoc comment.

### Suggest Timeout

A corrupted binary index could cause `tsuku suggest` to hang, blocking every interactive shell
session. The hook implementation should enforce the 50ms budget at the process level using a
context timeout, not just rely on SQLite's typical performance.

### No Privilege Escalation

This feature requires no sudo, no elevated capabilities, and no setuid. All operations are scoped
to the current user's home directory.

## Consequences

### Positive

- **Zero-friction discovery**: hooks are active after a standard tsuku install with no extra steps.
  Users who type `jq` for the first time see an install suggestion immediately.
- **Upgrade-safe by design**: hook logic lives in `$TSUKU_HOME/share/hooks/`, updated when the
  tsuku binary upgrades. The source line in the rc file never needs to change.
- **Non-destructive coexistence**: Ubuntu's `command_not_found_handle`, oh-my-zsh plugins, and
  other existing handlers are detected at shell startup and preserved. Both suggestions appear
  when both handlers match.
- **Clean uninstall**: a single identifiable marker line is all that's written to the user's rc
  file. Removal leaves the file byte-for-byte identical to its pre-install state.
- **Transparent**: the hook file at `$TSUKU_HOME/share/hooks/tsuku.bash` is human-readable and
  inspectable at any time. Security-conscious users can audit exactly what runs on shell startup.

### Negative

- **Single-shell auto-configuration**: the install script detects one shell (via `$SHELL`). Users
  who regularly use multiple shells (bash + fish, zsh + bash) must run `tsuku hook install --shell=<shell>`
  manually for secondary shells.
- **Shell startup overhead**: sourcing the hook file and running the detect-and-wrap logic adds a
  small amount of work to every shell startup. In practice this is microseconds, but it's not zero.
- **Fish conf.d is a persistent file**: unlike bash/zsh where the rc line simply stops sourcing if
  tsuku is uninstalled, the fish conf.d file remains until explicitly removed by `tsuku hook uninstall`.
  Users who delete the tsuku binary without uninstalling get a silent no-op on fish startup (the guard
  handles it), but the file lingers.
- **eval in bash wrapper**: copying an existing bash handler via `declare -f | eval` is the accepted
  pattern but involves eval. This is the only eval in the hook path.

### Mitigations

- **Multi-shell users**: `tsuku hook install --shell=<shell>` is the documented path for secondary
  shells. `tsuku hook status` shows per-shell state. The install output notes which shell was configured.
- **Startup overhead**: the `command -v tsuku` guard uses a shell builtin (not a process exec).
  If tsuku is in PATH, the guard passes in microseconds. The full cost is well within the 50ms
  budget and only manifests when an unknown command is typed.
- **Fish conf.d persistence**: `tsuku uninstall` (full removal) runs `tsuku hook uninstall` as
  part of cleanup, catching the fish file. The fish hook's `command -q tsuku` guard silently
  no-ops if the binary is missing.
- **eval in bash**: the eval is applied to a function body captured by `declare -f`, which is
  the shell's own serialization. The input is never user-supplied text.
