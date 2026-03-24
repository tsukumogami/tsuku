# Decision 2: Chaining with Existing Handlers

## Context

When `tsuku hook install` adds a command-not-found handler, many users already have one. Ubuntu/Debian ships `command_not_found_handle` in `/etc/bash.bashrc` (pointing to `apt`'s database), oh-my-zsh has a command-not-found plugin, and macOS Homebrew installs a similar handler. An approach that can't coexist with these handlers fails most users on first install.

## Key Assumptions

- Hook installation injects a marked snippet into the user's shell rc file (`~/.bashrc`, `~/.zshrc`, `~/.config/fish/config.fish`) rather than managing a separately-sourced file.
- The hook snippet runs at shell startup, not at install time.
- `tsuku suggest` runs quickly (SQLite lookup, 50ms budget). Handler overhead must be negligible.
- Uninstall removes the injected snippet; it does not need to restore state, because the snippet wraps at runtime rather than permanently replacing anything.

## Options Evaluated

### Option A: Detect-and-Wrap

At shell startup, the snippet checks whether a handler is already defined. If so, it copies the existing function under a `_tsuku_original_*` name, then installs tsuku's handler, which calls the original as a fallback.

**Bash mechanics:** `declare -f command_not_found_handle` detects the function (returns non-zero if absent). The function body is captured with `declare -f | tail -n +2` and reconstituted via `eval`. The guard `command -v tsuku > /dev/null 2>&1` (a shell builtin path lookup) prevents infinite recursion if tsuku itself is not in `PATH`.

**Zsh mechanics:** `(( $+functions[command_not_found_handler] ))` detects the handler. `functions -c src dst` copies the function cleanly without eval. Same recursion guard applies.

**Fish mechanics:** `functions --query fish_command_not_found` detects an existing handler. `functions --copy src dst` copies it. Fish uses `command -q tsuku` as the guard.

**Ubuntu/Debian case:** Ubuntu defines `command_not_found_handle` in `/etc/bash.bashrc`, which sources before `~/.bashrc`. So when the tsuku snippet in `~/.bashrc` runs, Ubuntu's handler is already defined. The snippet detects it, saves it as `_tsuku_original_cnf_handle`, installs its wrapper. Users see tsuku's suggestion first, then apt's. `/etc/bash.bashrc` is never modified.

**Uninstall:** Remove the snippet from the rc file. The `_tsuku_original_*` functions exist only in the running shell's memory; they are not persisted anywhere. At next shell startup, Ubuntu's handler from `/etc/bash.bashrc` is active, unmodified.

**Recursion risk:** Bash, zsh, and fish all re-invoke the command-not-found handler for commands inside the handler body that are also not found. If `tsuku` is missing from `PATH`, calling `tsuku suggest` from inside the handler would re-trigger it. The `command -v tsuku` guard (shell builtin, no PATH exec, no recursion) prevents this. Without the guard, the shell enters an infinite recursion loop.

**Performance:** `command -v` is a shell builtin hash lookup — microseconds. If tsuku is found, `tsuku suggest` runs within the 50ms budget. If not found, the guard short-circuits immediately.

**Failure modes:**
- tsuku not in `PATH`: guard catches it, handler silently skips tsuku's suggestion, falls through to original. Graceful degradation.
- Original handler removed between `hook install` and next shell start: `_tsuku_original_*` won't be created at startup; tsuku's handler runs alone and returns 127. Correct.
- Uninstall with wrapper still active in running shell: snippet is removed from rc file; next shell startup is clean. Current session retains wrapper harmlessly.
- `declare -f` captures wrong function body for multi-line functions: `declare -f` is POSIX-standard and handles multi-line functions correctly in bash 3.2+.

### Option B: Detect-and-Abort with Instructions

If an existing handler is detected, print instructions and exit 1.

**Problem:** Ubuntu's `command_not_found_handle` is present on essentially every Ubuntu/Debian desktop and server. This option makes `tsuku hook install` fail for most Linux users. The error message places a manual, error-prone chaining burden on the user. The "instructions" for manually chaining bash functions are not simple.

**Verdict:** Rejected. Fails on the most common setup.

### Option C: Unconditional Replace, Save Original

Always replace the handler, save the original's source text to a file (e.g., `$TSUKU_HOME/saved_cnf_handler.bash`).

**Problems:**
- Saving to a file introduces state that must be maintained and tracked per-shell.
- Restoration from a saved file requires evaling stored shell code — a security-adjacent concern.
- The saved snapshot may be wrong: Ubuntu's handler conditionally checks if `/usr/lib/command-not-found` exists at call time. A file-backed snapshot preserves the function body but not its dynamic conditions.
- More implementation surface than Option A for no benefit.

**Verdict:** Rejected. Option A provides cleaner runtime wrapping without file-backed state.

### Option D: Source-Order Precedence

Use shell-native loading order to ensure tsuku's handler loads "first" or "last." For fish, `conf.d/` files load alphabetically — name the file `00_tsuku.fish` to load before user functions.

**Problems:**
- Doesn't apply to bash or zsh in any meaningful way. Bash/zsh have no conf.d equivalent.
- Even in fish, `conf.d/` loads before `functions/` directory, but if a user's framework defines `fish_command_not_found` in their config, the last definition wins. Alphabetical ordering doesn't reliably win.
- Source ordering alone can't chain — only one definition is active at a time. Chaining still requires detect-and-wrap logic.

**Verdict:** Rejected as standalone. Fish's `conf.d/` ordering can inform *where* tsuku's snippet is placed in fish, but doesn't replace the need for detect-and-wrap.

### Option E: Cooperative Coexistence (Run Original First, Tsuku Fallback)

Run the original handler first; if it returns non-zero, tsuku suggests. Or vice versa.

**Analysis:** This is a special case of Option A's chaining, not a distinct architecture. The question is ordering: original first or tsuku first.

Running tsuku first is preferable because:
1. Tsuku's suggestion is cheap (SQLite) and always informative if the tool is in the registry.
2. Ubuntu's handler calls `/usr/lib/command-not-found`, which queries apt's database — also informative.
3. Users benefit from seeing both: "tsuku can install it" AND "apt can install it."
4. Both handlers return 127 at the end, which is correct shell behavior.

Showing both suggestions is not noise — it's information. The user can choose their installer.

Running original first would suppress tsuku's suggestion if the original handler somehow succeeded (returned 0). Ubuntu's handler never returns 0 for commands that don't exist, so this is moot in practice. Tsuku first is the cleaner design.

**Verdict:** The "tsuku first, fall through" ordering is the right behavior within Option A's wrapper. Not a separate option.

## Chosen Option: Option A — Detect-and-Wrap

The snippet injected into the rc file wraps at shell startup time. It detects any pre-existing handler using shell-native function introspection, copies it under a `_tsuku_original_*` name, then installs a wrapper that calls tsuku first, then the original. The guard `command -v tsuku` prevents recursion if tsuku is absent.

This is the correct approach because:
- It doesn't break Ubuntu/Debian users.
- Uninstall is just snippet removal — no file-backed state to manage.
- The original handler (in `/etc/bash.bashrc` for Ubuntu) is never touched.
- Each shell has a language-native function-copy mechanism (`eval`+`declare -f` for bash, `functions -c` for zsh, `functions --copy` for fish).
- Graceful degradation: if tsuku binary is removed, the guard short-circuits and the original handler works as if tsuku was never installed.

## Rejected Options

- **Option B:** Aborts on Ubuntu by design. Not viable.
- **Option C:** File-backed state is more complex and less correct than runtime wrapping.
- **Option D:** Source-order alone can't chain. Useful as an implementation detail for fish placement, not as a strategy.
- **Option E:** Not an independent strategy; the ordering concern it raises (tsuku first) is already answered within Option A.

## Assumptions

- The snippet is injected into the user's personal rc file, not a system file. This keeps `/etc/bash.bashrc` untouched and makes uninstall simple.
- `tsuku suggest` exits non-zero (127) whether or not it finds a match, so the wrapper always falls through to the original handler.
- Shell function introspection (`declare -f`, `functions -c`, `functions --copy`) is available in all minimum supported shell versions (bash 3.2+, zsh 5.0+, fish 3.0+).
- The `command -v` builtin guard is sufficient to prevent recursion; no additional re-entrancy flag is needed.
- tsuku's handler runs before the original. Both suggestions are shown when both handlers know about the command. This is a feature, not a bug.
