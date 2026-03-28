<!-- decision:start id="path-modification-state-deactivation" status="assumed" -->
### Decision: PATH Modification, State Tracking, and Deactivation

**Context**

Decision 1 established that `tsuku hook-env` runs on each prompt (with an early-exit guard when the directory hasn't changed) and that `tsuku shell` serves as an explicit alternative. What remains is the mechanics: how does `hook-env` alter PATH to surface per-project tool versions, what state does it store so it can undo those changes, and how does deactivation restore the original environment?

Tsuku's current PATH layout is `$TSUKU_HOME/bin:$TSUKU_HOME/tools/current:$PATH`, where `tools/current/` contains symlinks to the globally active version of each tool. Per-project activation means overriding some of those tools with project-declared versions from `.tsuku.toml`, while keeping non-overridden tools accessible through the existing `tools/current/` path.

The three sub-questions are coupled: the PATH modification strategy determines what state needs to be tracked, and the state format determines how deactivation works. Modifying the `tools/current/` symlinks is off the table because those symlinks are shared filesystem state -- changing them would affect every open terminal, not just the shell that entered the project.

**Assumptions**

- Modifying `tools/current/` symlinks for per-project activation is unacceptable because it would affect all terminals. Per-project state must be per-shell.
- The number of project-declared tools is small enough (realistically 5-15, max 256) that storing tool paths in a shell env var won't approach shell env size limits.
- PATH modifications by other tools between tsuku activation and deactivation are rare enough that overwriting PATH (rather than surgically removing tsuku's entries) is acceptable.
- The fork+exec cost for computing the new PATH is dominated by config file I/O (~1-5ms), not by the PATH string manipulation itself.

**Chosen: Prepend project paths + save/restore original PATH via env var**

When `hook-env` detects a directory change (via the `_TSUKU_DIR` guard from Decision 1), it:

1. **Saves the clean PATH.** If `_TSUKU_PREV_PATH` is unset (first activation), stores the current `PATH` in `_TSUKU_PREV_PATH`. If `_TSUKU_PREV_PATH` is already set (switching projects), uses the stored value as the base.
2. **Reads `.tsuku.toml`.** Calls `LoadProjectConfig($PWD)` to get the project's tool requirements.
3. **Resolves tool bin directories.** For each tool in the config, computes `$TSUKU_HOME/tools/{name}-{version}/bin`. Skips tools whose declared version isn't installed (they can't be activated).
4. **Builds the new PATH.** Constructs: `{project-tool-bins}:{_TSUKU_PREV_PATH}`. The project-specific bin directories go before everything else, including `$TSUKU_HOME/bin` and `$TSUKU_HOME/tools/current`. This means project-declared tools shadow their global counterparts, while tools not declared in the project fall through to `tools/current/` as before.
5. **Outputs shell commands.** Prints `export PATH="..."`, `export _TSUKU_DIR="..."`, and if first activation, `export _TSUKU_PREV_PATH="..."`.

On **deactivation** (leaving a project directory for a non-project directory):

1. `hook-env` detects that `LoadProjectConfig($PWD)` returns nil (no `.tsuku.toml` found).
2. Outputs: `export PATH="$_TSUKU_PREV_PATH"; unset _TSUKU_PREV_PATH _TSUKU_DIR`.
3. PATH is restored to its exact pre-activation state.

On **project-to-project transition** (entering a different project):

1. `hook-env` detects directory change and finds a new `.tsuku.toml`.
2. Uses `_TSUKU_PREV_PATH` as the base (not current PATH, which has the old project's entries).
3. Prepends the new project's tool bin directories.
4. Updates `_TSUKU_DIR` but keeps `_TSUKU_PREV_PATH` unchanged (it still holds the original pre-any-activation PATH).

**State tracked in shell env vars:**

| Variable | Purpose | Lifetime |
|----------|---------|----------|
| `_TSUKU_DIR` | Last-seen directory (from Decision 1) for early-exit guard | Set on first activation, updated on every directory change, unset on deactivation |
| `_TSUKU_PREV_PATH` | Complete PATH before any tsuku project activation | Set on first activation, never modified, unset on deactivation |

No files are used for state tracking. Env vars are per-shell, can't go stale across terminals, and require no cleanup on abnormal shell exit (the environment dies with the process).

**Rationale**

This approach scores highest on the combination of correctness and simplicity. Storing the complete pre-activation PATH and restoring it on deactivation guarantees clean reversal -- there's no parsing, no filtering, no risk of accumulated stale entries. The "save and restore" pattern is the same one that direnv uses for environment management, proven reliable across millions of users.

Prepending project paths rather than replacing `tools/current/` symlinks is the only viable strategy for per-shell activation. Symlinks are global filesystem state; env-var-based PATH modification is per-shell by design. Prepending also has the nice property that non-project tools remain accessible through `tools/current/` without any extra logic.

The tradeoff of losing PATH changes made by other tools between activation and deactivation is real but bounded. The window is the duration of a single project session, and tools that modify PATH at runtime (as opposed to shell startup) are uncommon. If this becomes an issue, a future enhancement could switch to the "stored entry list" approach (filtering out specific entries instead of restoring the whole PATH), but the added complexity isn't warranted now.

**Alternatives Considered**

- **Replace `tools/current/` symlinks**: Avoids PATH modification entirely by repointing the existing symlinks to project-declared versions. Rejected because symlinks are shared filesystem state -- changing them in one terminal would affect every other open terminal, violating the correctness constraint. Restoring symlinks on deactivation is also fragile: if the shell exits abnormally, symlinks are left pointing at the wrong version with no recovery mechanism.

- **Prepend + stored entry list (surgical removal)**: Instead of saving the whole original PATH, store only the entries tsuku added. On deactivation, filter those specific entries out of the current PATH. This is more resilient when other tools modify PATH between activation and deactivation. Rejected because the added filtering logic (substring matching, edge cases with duplicate entries, colon boundary handling) adds implementation complexity without meaningful practical benefit. The "other tools modifying PATH during a session" scenario is rare.

- **File-based state tracking**: Store activation state in a file (e.g., `$TSUKU_HOME/active/{shell-pid}.json`) instead of env vars. Would allow inspecting activation state from outside the shell and surviving shell restarts. Rejected because files can go stale (shell exits without cleanup), require periodic garbage collection, add filesystem I/O to the hot path, and the cross-shell inspection use case doesn't exist for tsuku's scope.

**Consequences**

PATH in an activated project will look like: `$TSUKU_HOME/tools/go-1.21.0/bin:$TSUKU_HOME/tools/nodejs-20.16.0/bin:$TSUKU_HOME/bin:$TSUKU_HOME/tools/current:<original-PATH>`.

Tools declared in `.tsuku.toml` resolve to their project-declared versions. Tools not declared resolve through `tools/current/` (global active version) as before. The tsuku binary itself (`$TSUKU_HOME/bin/tsuku`) is always reachable since `$TSUKU_HOME/bin` is part of `_TSUKU_PREV_PATH`.

The `_TSUKU_PREV_PATH` variable becomes a dependency for deactivation correctness -- if a user manually unsets it, deactivation will fail gracefully (hook-env should handle the missing variable by logging a warning and doing nothing rather than corrupting PATH).

`tsuku shell` (the explicit alternative) uses the same save/restore mechanism. The only difference is that it's invoked once by the user rather than on every prompt by the hook.
<!-- decision:end -->
