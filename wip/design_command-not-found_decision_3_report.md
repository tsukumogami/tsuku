# Decision 3: tsuku suggest Output and Interface

## Context

`tsuku suggest <command>` is called by shell command_not_found hooks when a user types an unknown command. It looks up the binary index (SQLite at `$TSUKU_HOME/cache/binary-index.db`) to find which recipe(s) provide that command, then prints a human-readable suggestion. The shell hook returns 127 regardless of what `tsuku suggest` prints; `tsuku suggest`'s own exit code is a signal back to the hook (and to scripters) about whether a match was found.

## Key Assumptions

- The 50ms total budget is dominated by process startup, not the SQLite lookup; output formatting adds negligible time
- `tsuku suggest` is never called in a non-interactive pipeline except by scripters explicitly invoking it — the hook only fires in interactive shells
- Block 3 (`tsuku run`) will read the same binary index; the output format of `suggest` does not constrain how `run` triggers install
- The `BinaryMatch.Installed` field is already populated by the index, so displaying installed status costs nothing extra
- Fish shell uses `fish_command_not_found` with identical semantics for the purpose of this design

## Options Evaluated

### Option A: Simple text, one line per match (modified)

**Single match:**
```
Command 'jq' not found. Install with: tsuku install jq
```

**Multiple matches:**
```
Command 'vi' not found. Provided by:
  vim (installed)   tsuku install vim
  neovim            tsuku install neovim
```

**No match:**
(silent — print nothing)

**Exit codes:**
- 0 if one or more matches found
- 1 if no match found

**`--json` flag:** supported

This is the option chosen (see below for reasoning).

---

### Option B: NixOS-style — action only, no prose

**Single match:**
```
tsuku install jq
```

**Multiple matches:**
```
tsuku install vim
tsuku install neovim
tsuku install micro
```

**No match:**
(silent)

NixOS's `command-not-found.pl` prints exactly one suggested nix-env command. The output is extremely short and is designed to be copied. Ubuntu's `command-not-found` handler adds a package name and a brief description line — still terse, but with slightly more context.

Tsuku's equivalent would be even more action-focused than NixOS because tsuku's recipe names are meant to match command names where possible (e.g., recipe `jq` provides `jq`).

**Exit codes:** same as Option A

**Verdict:** Too terse when multiple matches exist. With only install commands and no context, a multi-match list gives the user no basis for choosing. Also, the complete absence of the command name in the output would confuse users who don't realize the output relates to the command they typed.

---

### Option C: Rich output with recipe metadata

**Single match:**
```
Command 'jq' not found.

  jq — Command-line JSON processor (v1.7.1 available)
  Install with: tsuku install jq
```

**Multiple matches:**
```
Command 'vi' not found. Multiple recipes provide 'vi':

  vim (installed)   — Vi IMproved, a programmer's text editor
  neovim            — Vim-fork focused on extensibility and usability
  micro             — Modern and intuitive terminal-based text editor

  Install one: tsuku install vim
```

This approach shows recipe descriptions alongside each match. It is more informative but adds visual complexity that slows scanning in the fast-path (single-match) case. Version metadata adds a network/cache lookup for uninstalled tools — violating the no-network-calls constraint and potentially adding latency. Recipe descriptions are available locally from the registry cache, so they don't require network access, but the output becomes noticeably longer for a suggestion message that most users will read once.

**Verdict:** Rejected. The description adds noise in the common single-match case. For the multi-match case, a short description is genuinely helpful, but it can be included in Option A's multi-match output without adopting the full rich format. The `tsuku info <recipe>` command already provides rich metadata for users who want details.

---

### Option D: Structured JSON with `--json` flag (additive, not standalone)

This is not a standalone option — it is a flag that adds machine-readable output to whichever text format is chosen. Evaluated here as a modifier.

**Use case:** A user who chains suggest output into a script, or a custom shell hook that wants to parse matches programmatically (e.g., to auto-install the single installed match without prompting).

**JSON output (single match):**
```json
{"command":"jq","matches":[{"recipe":"jq","binary_path":"bin/jq","installed":false}]}
```

**JSON output (no match):**
```json
{"command":"vi","matches":[]}
```

**Verdict:** Include `--json` as a flag. The exit code semantics are the same. The flag is not advertised in the hook output — it is for scripters who call `tsuku suggest` directly. This is consistent with `tsuku search --json` and `tsuku info --json`, which already exist in the codebase. Adding `--json` costs nothing in the implementation and future-proofs the interface for Block 3 tooling, CI utilities, or shell hooks that want structured data.

---

## Chosen Option: Option A (simple text) with `--json` flag

### Single match

```
Command 'jq' not found. Install with: tsuku install jq
```

Output goes to stdout. This mirrors the exact phrasing used in `cmd_which.go`'s existing "jq is provided by recipe 'jq'" message and Ubuntu's handler, adapted for tsuku's install verb.

### Multiple matches

```
Command 'vi' not found. Provided by:
  vim (installed)   tsuku install vim
  neovim            tsuku install neovim
  micro             tsuku install micro
```

The installed recipe (ranked first by the index) gets an `(installed)` tag. Column alignment is fixed-width so the `tsuku install` commands line up for easy scanning. No description column — the recipe name is the identifier and descriptions are available via `tsuku info`.

### No match

Print nothing. Exit with code 1.

**Rationale:** When there is no match, the shell's own "command not found" error message is already displayed (the hook returns 127). Adding a second line from `tsuku suggest` — e.g., "No recipe found for 'xyz'" — would clutter every single typo or legitimately-missing system binary. NixOS, Ubuntu, and Homebrew's `brew install <cmd>` suggestion handlers all stay silent on no-match and let the shell's default error stand. Silence is the right call here.

The hook wrapper in bash/zsh can check the exit code to decide whether to print anything additional:

```bash
command_not_found_handle() {
    if tsuku suggest "$1"; then
        return 127
    fi
    # no match: let shell print its default error, or print custom message here
    return 127
}
```

### Exit codes

| Condition | Exit code |
|-----------|-----------|
| One or more matches found | 0 |
| No match | 1 |
| Index not built | 2 (ExitUsage equivalent) |
| Other error (DB corrupt, etc.) | 1 |

Exit 0 on match, 1 on no-match is the cleanest convention for a lookup command. It mirrors how `grep` (0 = found, 1 = not found) and `which` (0 = found, 1 = not found) work. The shell hook does not change its return value based on `tsuku suggest`'s exit code — it always returns 127 — but the exit code lets shell authors write conditional logic (e.g., fall through to another handler on no-match).

Using a dedicated exit code (2) for "index not built" lets hook authors distinguish transient errors from expected no-match. This is consistent with how tsuku already uses distinct exit codes (`ExitGeneral = 1`, `ExitUsage = 2`, `ExitRecipeNotFound = 3`, etc.) in `exitcodes.go`.

### Machine-readable output

Add `--json` flag, consistent with `tsuku search --json` and `tsuku info --json`.

```json
{
  "command": "vi",
  "matches": [
    {"recipe": "vim",    "binary_path": "bin/vi",  "installed": true},
    {"recipe": "neovim", "binary_path": "bin/vi",  "installed": false},
    {"recipe": "micro",  "binary_path": "bin/micro","installed": false}
  ]
}
```

On no-match, `matches` is an empty array (never null) and exit code is 1.

The `--json` flag is not mentioned in the human-readable hook output. It is documented in `tsuku suggest --help` for scripters.

### Interactivity

Never interactive. `tsuku suggest` never prompts the user, never reads from stdin, and never waits for input. Interactivity belongs to Block 3 (`tsuku run`). If `tsuku suggest` were to prompt (e.g., "Install now? [y/N]"), it would block every typo in the shell and violate the 50ms budget conceptually (even if the lookup itself is fast, the user now has to respond). The shell hook architecture assumes `tsuku suggest` completes and returns control to the shell promptly.

### Block 3 handoff

`tsuku suggest` prints the install command but does not run it. Block 3 (`tsuku run`) will be a separate command with its own confirm/auto modes. The output format of `tsuku suggest` does not constrain Block 3's design; the two commands are independent. However, the `--json` flag provides a clean interface if Block 3's hook variant ever wants to parse suggest output rather than calling `BinaryIndex.Lookup()` directly.

## Rejected Options

**Option B (action-only):** Too terse for multi-match. Without labeling which recipe a command comes from, the user has no way to distinguish `tsuku install vim` from `tsuku install neovim` in a list. The prose prefix (`Command 'x' not found.`) also grounds the output — without it, a user whose terminal scrolled could see just `tsuku install jq` and not know why it appeared.

**Option C (rich output):** Adds visual noise in the common single-match case. Recipe descriptions, while informative, are better accessed via `tsuku info`. The hook fires on every unknown command; that includes typos and system binaries that have no recipe. The output must be scannable at a glance.

**Interactive prompting:** Rejected categorically. Prompting inside a command_not_found hook is disruptive — it interrupts the user's thought process on every typo. It also creates problems in non-TTY contexts (scripts, CI) where the hook might fire but there is no terminal to read from. Block 3 is the right place for install confirmation.

## Assumptions

- The `BinaryMatch.Command` field in the index correctly reflects what the user typed (i.e., the index maps the actual binary name, not just the recipe name). When these differ (e.g., recipe `neovim` provides binary `nvim`), the output must show the recipe name for the install command and the binary name context is already in the hook invocation.
- Exit code 0 on match is safe: shell hooks that call `tsuku suggest` and check exit codes will not misinterpret "match found" as a general success/failure signal, since the hook always returns 127 to the shell regardless.
- `--json` output consumers tolerate an empty `matches` array on no-match (not a missing field or null).
- The shell hook snippets (bash/zsh/fish) are designed to call `tsuku suggest "$1"` without redirection — output goes directly to the user's terminal. This means `tsuku suggest` should write to stdout (not stderr) for its suggestion text, since hook output appears inline with the shell prompt.
- Stale-index warnings (already handled in `cmd_which.go`) should be printed to stderr by `tsuku suggest` as well, so they do not pollute `--json` output or confuse scripts parsing stdout.
