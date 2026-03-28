# Decision 3: Shell Infrastructure Integration

## Question

How should shell environment activation integrate with tsuku's existing shell infrastructure (shellenv command, tools/current/ symlinks, command-not-found hooks, hook install command)? Should shellenv be extended or should a new tsuku shell command be added?

## Decision Drivers

- **Compatibility**: Must coexist with existing shellenv output and activate command
- **Shell hooks are optional**: All features work via explicit CLI invocation
- **Simplicity**: Prefer minimal changes to existing infrastructure

## Alternatives Evaluated

### Option A: New `tsuku shell` command alongside shellenv

Add a new `tsuku shell` command that outputs directory-aware activation scripts. `shellenv` stays unchanged (static PATH). Users choose between `eval $(tsuku shellenv)` for static behavior and `eval $(tsuku shell)` for dynamic per-project activation.

**Pros:**
- Zero risk to existing users -- shellenv output is identical
- Clear naming: "shell" implies an active, session-aware concept; "shellenv" implies static configuration
- Clean separation of concerns: static PATH setup vs dynamic project activation
- Easy to document: two commands with distinct purposes

**Cons:**
- Two commands that both modify PATH could confuse new users
- Users who want both static PATH and activation need to understand which to use
- Adds a new top-level command

### Option B: Extend shellenv with --activate flag or auto-detection

`tsuku shellenv` gains a `--activate` flag (or detects .tsuku.toml automatically) to output project-aware PATH alongside the static PATH entries.

**Pros:**
- Single entry point for all shell configuration
- Users who already have `eval $(tsuku shellenv)` can add `--activate` easily

**Cons:**
- Risk of breaking existing users if auto-detection changes output without opt-in
- Overloads shellenv's contract: it currently promises static, deterministic output
- With `--activate`, output becomes directory-dependent, making shell profile use unreliable (shellenv runs once at login, not per-prompt)
- Conflates two different execution models: run-once-at-login vs run-every-prompt

### Option C: New `tsuku activate --project` mode

Extend the existing `tsuku activate` command to read .tsuku.toml and activate all declared tools.

**Pros:**
- Reuses existing command name
- Natural extension of "activate" concept

**Cons:**
- `activate` currently takes exactly two positional args (tool + version). Adding --project changes its interface contract
- Per-tool activation (global symlinks) and per-project activation (PATH manipulation) are fundamentally different mechanisms -- one modifies files, the other outputs shell code
- Doesn't address the prompt hook question at all
- Would need to output shell code (like shellenv) rather than modifying symlinks, which is a different return type than the current activate

### Option D: Separate prompt hook via hook install

`tsuku hook install --activate` adds prompt hooks (precmd/PROMPT_COMMAND) alongside the existing command-not-found hooks. The prompt hook calls a new `tsuku hook-env` subcommand that outputs activation/deactivation shell code (similar to mise's `hook-env` approach).

**Pros:**
- Follows mise's proven pattern, which users of mise/asdf/direnv already understand
- `hook install` already handles shell detection, rc file modification, and multi-shell support
- Hook files already have the embed/write infrastructure in internal/hooks/
- Clean separation: `hook-env` is an internal subcommand called by the hook, not user-facing
- The prompt hook approach is the only way to get truly automatic directory-change activation

**Cons:**
- `hook-env` as a hidden subcommand adds a command that users shouldn't call directly
- Prompt hooks add overhead to every prompt render (must stay under 50ms)
- More moving parts: hook file + hook-env subcommand + state tracking

## Analysis

The core tension is between two usage patterns:

1. **Explicit activation**: User runs a command when they want project tools active. This needs a user-facing command (A or C).
2. **Automatic activation**: Directory changes trigger activation without user action. This needs prompt hooks (D).

Both patterns are needed. The design doc's scope includes "activation via prompt hooks or tsuku shell." The question is how these map to commands and infrastructure.

Option B is eliminated because shellenv's contract is "static PATH setup at login time." Making it directory-aware breaks the mental model and the execution context (login vs per-prompt).

Option C is eliminated because `activate` operates on the filesystem (symlinks), while project activation operates on shell state (PATH exports). Different mechanisms shouldn't share a command.

The real choice is between A and D, and they're not mutually exclusive -- in fact, they complement each other:

- **Option A** (`tsuku shell`) provides explicit, on-demand activation. A user can run `eval $(tsuku shell)` to activate the current directory's tools without installing any hooks.
- **Option D** (`tsuku hook install --activate` + `hook-env`) provides automatic activation via prompt hooks for users who want hands-free switching.

Both need the same underlying logic: read .tsuku.toml, compute PATH modifications, output shell code. The difference is the trigger: manual vs prompt hook.

### Infrastructure mapping

| Existing piece | Role in new feature |
|---|---|
| `tsuku shellenv` | Unchanged. Static PATH for $TSUKU_HOME/bin and tools/current. |
| `tsuku activate <tool> <version>` | Unchanged. Global per-tool version switching via symlinks. |
| `tsuku hook install` | Extended with `--activate` flag to install prompt hooks alongside command-not-found hooks. |
| Shell hook files (internal/hooks/) | New hook files added for prompt integration (tsuku-activate.{bash,zsh,fish}). |
| `$TSUKU_HOME/env` file | Unchanged. Static PATH sourced by shell profile. |
| New: `tsuku shell` | User-facing command for explicit one-shot activation. |
| New: `tsuku hook-env` | Internal subcommand called by prompt hooks. Outputs activation/deactivation diff. |

### Why not just D alone?

A standalone `tsuku shell` command (Option A) matters because:
- Not all users want prompt hooks. Some prefer explicit control.
- Scripts and CI environments need activation without interactive shell hooks.
- It provides a debugging tool: run `tsuku shell` to see what would be activated.

### Why not just A alone?

Prompt hooks (Option D) matter because:
- Manual activation defeats the purpose of per-project environments. The whole point is "cd into project, tools are ready."
- mise, asdf, direnv, and nvm all use prompt hooks for this reason. It's the established pattern.

## Decision

**Chosen: Hybrid of A + D** -- Add a new `tsuku shell` command for explicit activation AND extend `tsuku hook install` with `--activate` for automatic prompt-based activation.

Specifically:

1. **`tsuku shell`** -- New user-facing command. Reads .tsuku.toml from current directory (via existing `project.LoadProjectConfig`), computes per-project PATH entries, outputs shell code. Analogous to shellenv but directory-aware. Usage: `eval $(tsuku shell)`.

2. **`tsuku hook-env`** -- New internal subcommand (hidden from help). Called by prompt hooks. Same activation logic as `tsuku shell` but optimized for repeated invocation: checks whether activation state changed before emitting output (skip if same directory, same config).

3. **`tsuku hook install --activate`** -- Extends existing hook install to also install prompt hooks. Writes new hook files (tsuku-activate.{bash,zsh,fish}) to `$TSUKU_HOME/share/hooks/`. Appends source lines to rc files using the same marker-block pattern. The prompt hooks call `tsuku hook-env` on each prompt.

4. **shellenv stays unchanged** -- `tsuku shellenv` continues to output static PATH. No flags, no auto-detection.

5. **activate stays unchanged** -- `tsuku activate <tool> <version>` continues to manage global symlinks. No --project mode.

## Confidence

High. The hybrid approach is well-established (mise uses exactly this pattern: `mise activate` for hooks + `mise shell` for explicit use). It requires no changes to existing commands and adds new functionality through new commands and a flag on an existing command.

## Assumptions

- The `tsuku hook-env` subcommand can complete in under 50ms (config file read + PATH diff computation). This is plausible since it only reads a small TOML file and compares paths.
- Prompt hooks for bash (PROMPT_COMMAND), zsh (precmd), and fish (fish_prompt) can be installed alongside existing command-not-found hooks without conflicts.
- The marker-block pattern in `internal/hook/install.go` can be extended to support a second marker for activation hooks without interfering with the command-not-found marker.

## Rejected Alternatives

| Option | Reason for rejection |
|---|---|
| B: Extend shellenv | Breaks shellenv's static-output contract. Conflates login-time and per-prompt execution models. |
| C: activate --project | Overloads a command that operates on filesystems (symlinks) with one that must output shell code (PATH exports). Different mechanisms, different return types. |
| A alone (no hooks) | Doesn't deliver automatic activation, which is the primary value proposition. |
| D alone (no explicit command) | Leaves no option for users who don't want hooks, and no debugging/scripting tool. |
