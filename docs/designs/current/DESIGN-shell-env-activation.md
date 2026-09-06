---
schema: design/v1
status: Current
spawned_from:
  issue: 1681
  repo: tsukumogami/tsuku
  parent_design: docs/designs/DESIGN-shell-integration-building-blocks.md
problem: |
  Tsuku manages tool versions globally via symlinks in tools/current/. When a
  developer switches between projects that need different tool versions, they
  must manually run tsuku activate for each tool. There's no mechanism to
  automatically activate per-project tool versions when entering a directory,
  despite .tsuku.toml now declaring project requirements.
decision: |
  Add prompt hooks (PROMPT_COMMAND/precmd/fish_prompt) that call a new tsuku
  hook-env subcommand on each prompt. hook-env compares PWD against a cached
  directory and installation state's stat against a cached stamp, exits
  immediately if both are unchanged (<5ms), and otherwise reads .tsuku.toml and
  resolves each declaration against the installed versions to prepend
  project-specific tool bin paths to PATH. A declaration that cannot be honored
  is reported on stderr with the reason. State is tracked in shell env vars
  (_TSUKU_DIR, _TSUKU_PREV_PATH, _TSUKU_STATE_STAMP) for clean deactivation and
  so an install performed without leaving the directory takes effect at the next
  prompt. A new tsuku shell command provides explicit activation without hooks.
  Existing shellenv and activate commands are unchanged.
rationale: |
  Prompt hooks with early-exit are the proven pattern (mise, direnv). The
  fork+exec cost is under 5ms when neither the directory nor installation
  state has changed, well within
  the 50ms budget. Env-var-based state tracking is per-shell (won't leak
  between terminals) and requires no cleanup on abnormal exit. Prepending
  project paths before tools/current/ means project tools shadow global ones
  while non-project tools remain accessible. The hybrid approach (hooks +
  explicit command) satisfies both automatic and manual activation users.
---

# DESIGN: Shell Environment Activation

## Status

Current

## Upstream Design Reference

Parent: [DESIGN: Shell Integration Building Blocks](DESIGN-shell-integration-building-blocks.md)
Block 5 in the six-block architecture. This design specifies dynamic PATH modification based on project configuration for per-directory tool version activation. Consumes the `ProjectConfig` interface from Block 4 (#1680, implemented in `internal/project`).

## Context and Problem Statement

Tsuku currently manages tool versions globally. `tsuku activate <tool> <version>` switches a symlink in `$TSUKU_HOME/tools/current/`, and `tsuku shellenv` adds that directory to PATH. This works for single-tool switching but doesn't handle the per-project use case: a developer working on project A needs Go 1.22, but project B needs Go 1.21. Switching between them requires manual `tsuku activate` calls every time they change directories.

With Block 4 implemented, projects can declare their tool requirements in `.tsuku.toml`. But the config is only consumed by `tsuku install` (batch install) -- there's no mechanism to automatically activate the right tool versions when entering a project directory.

Shell environment activation bridges this gap. When a developer enters a project directory (or runs `tsuku shell`), tsuku reads `.tsuku.toml` and modifies PATH to point to the project's declared tool versions instead of the global `tools/current/` symlinks. When they leave, PATH reverts.

### Scope

**In scope:**
- Activation mechanism (prompt hooks and explicit `tsuku shell` command)
- PATH modification strategy (prepending project tool paths)
- State tracking (env vars for directory and original PATH)
- Deactivation behavior (restoring original PATH on leaving project)
- Shell-specific implementations (bash, zsh, fish)
- Integration with existing `shellenv`, `activate`, and `hook install` commands
- `EnvActivator` interface specification

**Out of scope:**
- Auto-install during activation (Block 6, #2168)
- Environment variable management beyond PATH
- Windows support
- LLM integration

### Existing Infrastructure

- `tsuku shellenv` -- static `export PATH="$TSUKU_HOME/bin:$TSUKU_HOME/tools/current:$PATH"`
- `tsuku activate <tool> <version>` -- creates symlinks in `tools/current/`
- Shell hooks in `internal/hooks/` -- bash/zsh/fish for command-not-found only
- `tsuku hook install` -- appends source lines to shell config files
- `internal/project.LoadProjectConfig(startDir)` -- discovers and parses `.tsuku.toml`
- `internal/config.Config.ToolDir(name, version)` -- returns `$TSUKU_HOME/tools/{name}-{version}`

## Decision Drivers

- **Performance**: Shell hooks must complete in under 50ms per prompt; the fast path (no directory change) should be under 5ms
- **Correctness**: PATH must reflect the project's declared tools accurately; stale state is worse than no activation
- **Reversibility**: Deactivation must cleanly restore the original PATH without residue
- **Shell hooks are optional**: `tsuku shell` must work as an explicit alternative to prompt hooks
- **Compatibility**: Must coexist with existing `shellenv` output, `activate` command, and command-not-found hooks
- **Simplicity**: Prefer the simplest mechanism that delivers correct behavior
- **Cross-shell**: Must work on bash, zsh, and fish

## Considered Options

### Decision 1: Activation Mechanism

Tsuku needs to detect when a user enters a project directory and activate the right tool versions. The mechanism must fire reliably on directory changes, perform well on every prompt, and remain optional.

Three approaches exist in the ecosystem: prompt hooks (mise, direnv), cd wrappers, and shims (asdf). The key tension is between reliability (catching all directory changes) and simplicity (minimal shell integration).

Key assumptions:
- Fork+exec cost for `tsuku hook-env` stays under 5ms on modern Linux/macOS
- `.tsuku.toml` config lookup completes under 10ms
- Users accept a one-time `tsuku hook install` setup step for automatic activation

#### Chosen: Prompt Hook with Early-Exit Guard

Use a single prompt-based hook per shell (`PROMPT_COMMAND` for bash, `precmd` for zsh, `fish_prompt` event for fish) that calls `tsuku hook-env`. The hook-env command compares `$PWD` against a cached directory (`$_TSUKU_DIR`) **and** installation state's stat against a cached stamp (`$_TSUKU_STATE_STAMP`), exits immediately if both are unchanged, and performs config lookup + PATH rewrite when either has moved. A directory that has not changed is not on its own a reason to exit: an install performed without leaving the project has to take effect at the next prompt.

The early-exit optimization means the per-prompt cost when neither has changed is: one fork+exec (~2-4ms), two string comparisons, one `os.Stat` of `state.json` (measured at 2.6 µs), exit with empty stdout. The shell's `eval` of empty output is a no-op.

Hooks are opt-in, installed via `tsuku hook install --activate`. An explicit `tsuku shell` command provides identical activation logic without hooks.

#### Alternatives Considered

**cd/pushd/popd wrapper**: Wrap directory-changing builtins to trigger activation at the moment of change. Rejected because it misses directory changes from external sources (git worktree switches, subshells, CDPATH), cd wrapping conflicts with other tools (rvm, nvm), and fish doesn't have an equivalent mechanism. The prompt hook with early-exit has identical practical performance without the correctness gaps.

**Dual hooks (chpwd + precmd)**: Use zsh's `chpwd_functions` for immediate activation, plus `precmd` as a safety net. This is what mise does. Rejected because the added complexity (two hook points per shell, divergent implementations per shell) doesn't deliver meaningful benefit over a single prompt hook with early-exit. The sub-millisecond latency difference is imperceptible.

**Shim-based resolution (no hooks)**: Like asdf, use shim scripts that resolve versions per-invocation. Rejected because the per-invocation overhead (20-50ms per tool execution) violates the performance constraint, breaks `argv[0]` inspection for multi-call binaries, and confuses `which` output. Shims are better suited for Block 6 (project-aware exec wrapper).

### Decision 2: PATH Modification and State Tracking

When activation fires (directory changed, `.tsuku.toml` found), tsuku must modify PATH so project-declared tool versions shadow their global counterparts. When deactivation fires (leaving a project directory), PATH must revert cleanly.

The three sub-questions are coupled: the PATH strategy determines what state to track, and the state format determines how deactivation works. Modifying `tools/current/` symlinks is off the table -- those are shared filesystem state that would affect every terminal.

Key assumptions:
- Per-project state must be per-shell (env vars, not files)
- The number of project-declared tools is small enough (5-15, max 256) that PATH won't hit shell limits
- PATH modifications by other tools between activation and deactivation are rare

#### Chosen: Prepend Project Paths + Save/Restore Original PATH

When `hook-env` detects a directory change:

1. **Save the clean PATH.** If `_TSUKU_PREV_PATH` is unset (first activation), store current `PATH`. If already set (switching projects), use the stored value as base.
2. **Read `.tsuku.toml`.** Call `LoadProjectConfig($PWD)`.
3. **Resolve tool bin directories.** For each declaration, derive the bare recipe
   name (an org-scoped key such as `owner/repo:tool` installs to
   `tools/tool-{version}`, not `tools/owner/repo/tool-{version}`), then choose
   among the versions installation state records: filter to those satisfying the
   declaration, then to those whose directory is present, and take the newest by
   version comparison. This resolves all four documented forms -- `latest`, an
   omitted version, a major-only prefix and a major-minor prefix -- not only an
   exact match. A declaration that yields nothing is reported on stderr with the
   reason; see "Reporting" below.
4. **Build new PATH.** `{project-tool-bins}:{_TSUKU_PREV_PATH}`. Project bins go before everything, including `$TSUKU_HOME/bin` and `tools/current/`.
5. **Output shell commands.** `export PATH="..."`, `export _TSUKU_DIR="..."`, `export _TSUKU_PREV_PATH="..."` and `export _TSUKU_STATE_STAMP="..."`. The full block is emitted on every re-resolve, including when the computed PATH is byte-identical to the current one, because the stamp has to be recorded even when nothing else changed.

On **deactivation** (no `.tsuku.toml` found): restore `PATH` from `_TSUKU_PREV_PATH`, unset all three tracking variables.

On **project-to-project transition**: use `_TSUKU_PREV_PATH` as base (not current PATH), prepend new project's bins.

State variables:

| Variable | Purpose | Lifetime |
|----------|---------|----------|
| `_TSUKU_DIR` | Last-seen directory for early-exit guard | Set on activation, unset on deactivation |
| `_TSUKU_PREV_PATH` | Complete PATH before any project activation | Set on first activation, unset on deactivation |
| `_TSUKU_STATE_STAMP` | mtime and size of installation state, so an install performed without leaving the directory takes effect at the next prompt | Set on activation, unset on deactivation |

#### Alternatives Considered

**Replace tools/current/ symlinks**: Avoid PATH modification by repointing symlinks to project versions. Rejected because symlinks are shared filesystem state -- changing them in one terminal affects every other terminal. Abnormal shell exit leaves symlinks pointing at wrong versions with no recovery.

**Prepend + surgical removal (stored entry list)**: Track which PATH entries tsuku added and filter them out on deactivation. More resilient to other tools modifying PATH mid-session. Rejected because the filtering logic (substring matching, duplicate handling, colon boundaries) adds complexity for a rare edge case.

**File-based state tracking**: Store activation state in `$TSUKU_HOME/active/{pid}.json`. Rejected because files can go stale on abnormal exit, require garbage collection, and add filesystem I/O to the hot path.

### Decision 3: Integration with Existing Shell Infrastructure

Tsuku has several shell integration touch-points: `shellenv` (static PATH), `activate` (per-tool symlinks), `hook install` (command-not-found hooks), and hook files (`internal/hooks/`). The new activation feature must work with all of them without breaking existing behavior.

Key assumptions:
- `tsuku hook-env` can complete in under 50ms
- Prompt hooks can be installed alongside command-not-found hooks without conflicts
- The marker-block pattern in `hook install` can support a second marker

#### Chosen: Hybrid -- New `tsuku shell` + Extended `hook install --activate`

Add new functionality through new commands and a flag, leaving existing commands unchanged:

| Component | Change |
|-----------|--------|
| `tsuku shellenv` | Unchanged. Static PATH. |
| `tsuku activate <tool> <version>` | Unchanged. Global symlinks. |
| `tsuku hook install` | Extended: `--activate` flag installs prompt hooks alongside command-not-found hooks |
| Shell hook files | New: activation hook scripts (tsuku-activate.{bash,zsh,fish}) |
| **New: `tsuku shell`** | User-facing command for explicit one-shot activation |
| **New: `tsuku hook-env`** | Internal subcommand called by prompt hooks, outputs activation/deactivation diff |

`tsuku shell` reads `.tsuku.toml` from the current directory and outputs shell code to set PATH. Usage: `eval $(tsuku shell)`. It's a one-shot command, not a hook.

`tsuku hook-env` is the optimized version called by prompt hooks. It checks `_TSUKU_DIR` and `_TSUKU_STATE_STAMP` for early exit, and emits the full export block on every re-resolve — including when the computed PATH is byte-identical, because the stamp still has to be recorded.

#### Alternatives Considered

**Extend shellenv with --activate**: Make `shellenv` directory-aware. Rejected because it breaks shellenv's contract of static, deterministic output. `shellenv` runs once at login, while activation runs per-prompt -- conflating these execution models confuses both the implementation and the user.

**activate --project mode**: Extend `activate` to read `.tsuku.toml`. Rejected because `activate` operates on the filesystem (symlinks), while project activation operates on shell state (PATH exports). Different mechanisms shouldn't share a command.

## Decision Outcome

**Chosen: Prompt hooks with early-exit, env-var state tracking, hybrid explicit/automatic activation**

### Summary

Shell environment activation adds three new commands: `tsuku shell` (explicit activation), `tsuku hook-env` (prompt hook entry point), and `tsuku hook install --activate` (opt-in prompt hook setup). Existing commands (`shellenv`, `activate`) are untouched.

When a user enters a project directory with `.tsuku.toml`, `hook-env` reads the config and prepends per-project tool bin paths to PATH. For a project declaring `go = "1.22"` and `node = "20.16.0"`, PATH becomes `$TSUKU_HOME/tools/go-1.22.5/bin:$TSUKU_HOME/tools/nodejs-20.16.0/bin:{original PATH}`. Tools declared in the project shadow their global counterparts; undeclared tools fall through to `tools/current/` as before.

State lives in three shell env vars: `_TSUKU_DIR` (last-seen directory, for early-exit), `_TSUKU_PREV_PATH` (original PATH before activation, for clean deactivation) and `_TSUKU_STATE_STAMP` (mtime and size of installation state, so an install performed without leaving the directory takes effect). On deactivation (leaving a project directory), PATH restores exactly. On project-to-project transition, `_TSUKU_PREV_PATH` stays as the base while the new project's paths replace the old.

The prompt hook fires on every prompt. When neither the directory nor installation state has changed it exits in under 5ms: fork+exec, two string comparisons, and a single `os.Stat` of `state.json` -- measured at 2.6 µs, or 0.05% of that budget. The claim that this path does no filesystem I/O was true before the stamp existed and is not now. On a change, the full path costs ~10-15ms (config lookup + PATH construction). Both are well within the 50ms budget.

### Rationale

The three decisions form a coherent stack. Prompt hooks (D1) provide the trigger. Prepend + save/restore (D2) provides the mechanism. The hybrid integration (D3) provides the user interface. Each layer is the simplest correct option at its level.

Prompt hooks over cd-wrappers because they catch all directory changes. Prepend + full PATH save over surgical filtering because it's simpler and guarantees clean restoration. New commands over extending existing ones because the execution models are fundamentally different (per-prompt vs one-shot vs filesystem).

### Trade-offs Accepted

- **Fork+exec on every prompt**: Even with early-exit, each prompt pays ~2-4ms for spawning `tsuku hook-env`. This is acceptable (mise and direnv do the same) but not free.
- **PATH changes by other tools lost on deactivation**: If another tool modifies PATH while a project is active, deactivation restores the pre-activation PATH, losing those changes. Uncommon in practice.
- **Version must be installed**: Activation only works for already-installed versions. If `.tsuku.toml` declares `go = "1.22"` and nothing matching is installed, that tool is not activated and the reason is written to stderr -- it is no longer silently skipped. Block 6 (#2168) handles install-on-demand.

## Solution Architecture

### Overview

Shell environment activation adds a new `internal/activation` package that computes per-project PATH modifications, a `tsuku hook-env` subcommand for prompt hooks, a `tsuku shell` command for explicit activation, and updated hook scripts in `internal/hooks/`.

### Components

```
┌─────────────────────────────────────────────────────────────────┐
│                         User Shell                              │
│  ┌────────────────────┐    ┌────────────────────┐              │
│  │ Prompt hook        │    │ eval $(tsuku shell) │              │
│  │ (_tsuku_hook)      │    │ (explicit)          │              │
│  └────────┬───────────┘    └────────┬────────────┘              │
└───────────┼──────────────────────────┼──────────────────────────┘
            │                          │
            ▼                          ▼
┌───────────────────────┐    ┌────────────────────┐
│ tsuku hook-env <shell>│    │ tsuku shell         │
│ (fast-path check)     │    │ (one-shot)          │
└───────────┬───────────┘    └────────┬────────────┘
            │                          │
            ▼                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                internal/activation/activate.go                   │
│                                                                  │
│  ComputeActivation(cwd, prevPath, curDir, stamp, cfg, installed) │
│    -> reads .tsuku.toml via project.LoadProjectConfig            │
│    -> reads recorded versions once via InstalledSet              │
│    -> resolves tool bin dirs via config.ToolBinDir               │
│    -> returns ActivationResult{PATH, Dir, PrevPath, Stamp,       │
│                                Unhonorable, Unreadable, Entered} │
│                                                                  │
│  FormatExports(result, shell)                                   │
│    -> formats export/set statements for bash/zsh/fish            │
└─────────────────────────────────────────────────────────────────┘
            │              │                    │
            ▼              ▼                    ▼
┌──────────────────┐ ┌──────────────────┐ ┌──────────────────┐
│ internal/project │ │ internal/config  │ │ internal/install │
│ LoadProjectConfig│ │ Config.ToolBinDir│ │ pin matching,    │
│ SplitOrgKey      │ │                  │ │ installed set    │
└──────────────────┘ └──────────────────┘ └──────────────────┘
```

Activation lives in `internal/activation` rather than `internal/shellenv`.
`internal/install` imports `internal/shellenv` for its shell.d cache and
PATH-precedence helpers, so activation's dependency on the pin-matching
routines and installation state would have been a cycle. Activation shared no
symbol with the rest of `shellenv`, so the split cost nothing and tells the
truth about the dependency graph. The invariant is recorded in the package
doc: `internal/install` must never import `internal/activation`, nor must
anything in `internal/install`'s dependency graph.

### Key Interfaces

```go
package activation

// ActivationResult holds the computed PATH and state for shell export.
type ActivationResult struct {
    PATH        string           // new PATH value
    Dir         string           // project directory (for _TSUKU_DIR)
    PrevPath    string           // original PATH to save (for _TSUKU_PREV_PATH)
    Active      bool             // true if a project is active, false if deactivating
    Stamp       string           // installation-state stamp (for _TSUKU_STATE_STAMP)
    Unhonorable []Unhonorable    // declarations that put nothing on PATH, with reasons
    Unreadable  *StateUnreadable // non-nil when installation state could not be read at all
    Entered     bool             // true when this activation entered a project not already recorded
}

// InstalledSet reads what installation state records. Consumer-declared and one
// method wide, and read at most once per activation.
type InstalledSet interface {
    InstalledVersionsFor(names []string) (map[string][]string, error)
}

// ComputeActivation determines what PATH should be based on the current
// directory, previous activation state, and the versions installation state
// records.
//
// cwd: current working directory
// prevPath: value of _TSUKU_PREV_PATH (empty if no prior activation)
// curDir: value of _TSUKU_DIR (empty if no prior activation)
// stamp: value of _TSUKU_STATE_STAMP (empty if no prior activation)
//
// Returns nil when no change is needed: the same directory AND unchanged
// installation state. On a parse failure it returns a non-nil result AND a
// non-nil error, so the caller can record the project whose file would not
// parse and thereby report only once per entry.
func ComputeActivation(
    cwd, prevPath, curDir, stamp string,
    cfg *config.Config,
    installed InstalledSet,
) (*ActivationResult, error)

// FormatExports renders the ActivationResult as shell-specific
// export/unset statements.
func FormatExports(result *ActivationResult, shell string) string
```

**New CLI commands:**

```go
// cmd/tsuku/shell.go
// tsuku shell -- outputs activation shell code for current directory
var shellCmd = &cobra.Command{Use: "shell", ...}

// cmd/tsuku/hook_env.go
// tsuku hook-env <shell> -- prompt hook entry point with early-exit
var hookEnvCmd = &cobra.Command{Use: "hook-env", Hidden: true, ...}
```

**Updated hook files:**

```
internal/hooks/tsuku-activate.bash  -- prompt hook for bash
internal/hooks/tsuku-activate.zsh   -- prompt hook for zsh
internal/hooks/tsuku-activate.fish  -- prompt hook for fish
```

### Data Flow

**Flow 1: Automatic activation (prompt hook)**

```
1. User cds into project directory
2. Next prompt triggers _tsuku_hook
3. Hook calls: tsuku hook-env bash
4. hook-env reads $_TSUKU_DIR, $_TSUKU_PREV_PATH and $_TSUKU_STATE_STAMP
5. ComputeActivation stats installation state and compares both the
   directory and the stamp
6. If both unchanged: exit (no output, <5ms)
7. If either changed: call LoadProjectConfig(cwd)
8. If .tsuku.toml found: read the recorded versions once, resolve each
   declaration, build PATH, collect the reasons for any that could not be
   honored, and return the result
9. If .tsuku.toml found but unparseable: return a result recording the
   directory alongside the error
10. If no .tsuku.toml and was active: return deactivation result
11. hook-env writes any reasons to stderr, then calls FormatExports(result, "bash")
12. Hook evals the output: export PATH="...", export _TSUKU_DIR="...",
    export _TSUKU_PREV_PATH="...", export _TSUKU_STATE_STAMP="..."
```

The full export block is emitted on every re-resolve, including when the
computed PATH is byte-identical to the current one. Emitting only when PATH
needs to change looks like an obvious saving and is not one: the stamp would
never be recorded, so a shell already running would re-resolve on every prompt,
forever.

**Flow 2: Explicit activation (tsuku shell)**

```
1. User runs: eval $(tsuku shell)
2. shell command calls ComputeActivation(cwd, prevPath, "", "")
3. Same resolution as above but always runs: passing an empty curDir and an
   empty stamp defeats the early exit, so an explicit invocation always
   resolves and always emits
4. Outputs shell code to stdout
5. Shell evals it
```

**Flow 3: Deactivation (leaving project)**

```
1. User cds out of project (no .tsuku.toml in new location)
2. Prompt hook fires, hook-env detects directory change
3. ComputeActivation finds no config, sees _TSUKU_PREV_PATH is set
4. Returns: PATH=_TSUKU_PREV_PATH, Active=false
5. Output: export PATH="$original"; unset _TSUKU_DIR _TSUKU_PREV_PATH _TSUKU_STATE_STAMP
```

## Implementation Approach

### Phase 1: Core Activation Logic (`internal/activation`)

Build `ComputeActivation` and `FormatExports`. Pure logic, no CLI integration.

Deliverables:
- `internal/activation/activate.go`: `ComputeActivation`, `FormatExports`, `ActivationResult`
- `internal/activation/resolve.go`: per-declaration resolution and classification
- `internal/activation/reason.go`: `Reason`, `Unhonorable`, `StateUnreadable`
- `internal/activation/stamp.go`: `StateStamp`
- Tests for activation, deactivation, project-to-project, each reason, and the early exit

### Phase 2: CLI Commands

Add `tsuku shell` and `tsuku hook-env` commands.

Deliverables:
- `cmd/tsuku/shell.go`: Cobra command, calls `ComputeActivation` + `FormatExports`
- `cmd/tsuku/hook_env.go`: Hidden Cobra command with early-exit optimization
- Tests for both commands

### Phase 3: Shell Hook Scripts

Create activation hook scripts and extend `hook install`.

Deliverables:
- `internal/hooks/tsuku-activate.bash`: `_tsuku_hook` function on `PROMPT_COMMAND`
- `internal/hooks/tsuku-activate.zsh`: `_tsuku_hook` on `precmd_functions`
- `internal/hooks/tsuku-activate.fish`: `_tsuku_hook` on `fish_prompt` event
- `internal/hooks/embed.go`: updated to embed new hook files
- Updated `hook install` with `--activate` flag
- `internal/hook/install.go`: new marker block for activation hooks

### Phase 4: Documentation

Update CLI help and user-facing docs.

Deliverables:
- Help text for `tsuku shell`, `tsuku hook-env`, updated `tsuku hook install`
- Getting started guide updates

### Reporting

A declaration that puts nothing on PATH produces one message on stderr naming
the tool and one of five reasons. They are distinct sentences rather than one
templated message, because they carry different information and lead to
different actions:

| Reason | Means | Carries |
|--------|-------|---------|
| `no-match` | Nothing installed satisfies the declaration | The tool name |
| `bad-form` | The key or version string is malformed | The declared string to edit |
| `channel` | The declaration is a channel pin (`@lts`), which activation does not resolve | The declared string |
| `missing-files` | A recorded version satisfies it, but its directory is gone | The version to reinstall |
| `unreadable` | Installation state could not be read at all | The tools it would have resolved |

An *absent* state file is `no-match`, not `unreadable`. The loader returns an
empty state and no error for a missing file, so a machine whose state was
deleted is indistinguishable from one that has never installed anything;
`unreadable` would be the different and false claim that a read failed.

`unreadable` is reported once per activation rather than once per declaration,
because it is a property of the read that would have classified all of them.
Messages go to stderr only -- stdout carries exclusively the shell code both
entry points are `eval`'d for -- and both commands exit 0 for all five, because
a prompt hook that exits non-zero gets wrapped in `|| true`, which would discard
the reporting entirely. `--quiet` suppresses all of them.

### Reading installation state

Activation reads the recorded versions once per invocation, through a
consumer-declared one-method interface, and **without taking the state file
lock**. Two constraints drive this. Decoding the whole state file per tool would
make a project declaring N tools pay N decodes on the hot path. And the shared
lock has no non-blocking variant here, so a prompt firing while an install held
the exclusive lock would block the shell.

The lock-free read is safe because `Save` publishes by atomic rename: a reader
either sees the whole previous file or the whole new one, never a torn write.

## Security Considerations

### PATH Manipulation Safety

The primary security surface is PATH modification. A malicious `.tsuku.toml` can reference tool names and versions that, when resolved to `$TSUKU_HOME/tools/{name}-{version}/bin`, prepend unexpected directories to PATH.

This section previously claimed the resolution "only produces paths within `$TSUKU_HOME/tools/`" and that traversal was "already guarded by the install pipeline's name validation". Both were false. `filepath.Join` calls `Clean`, so `..` in either component escaped the tools directory, and no name validation existed on the install path -- the validator the claim referred to lived in a neighbouring package and was called by nobody on this route. The residual-risk table below said as much, one column away from the claim that it was mitigated.

**Mitigations:**
- Both components of a declaration -- the tool name and the declared version -- are validated where `.tsuku.toml` becomes a configuration object, in `internal/project`. Every consumer reads its values through that point, so activation, `tsuku install`, `tsuku shim install` and the `tsuku run` fast path all inherit the guarantee.
- The name rule is an allowlist of lowercase letters, digits, `.`, `_` and `-`, rejecting `..` as a path segment, a leading `-` or `.`, and anything outside that set. An allowlist rather than a denylist because a colon is neither traversal nor a shell metacharacter but *is* the `PATH` separator, so `a:b` would split one entry into two with the second relative to the working directory -- which neither quoting nor a containment check catches.
- An org-scoped key's `owner/repo` half is validated separately and more permissively, since GitHub allows uppercase there.
- A declaration that fails is refused individually and reported on stderr naming the offending key; its siblings still activate.
- Activation only references already-installed tools; it doesn't trigger downloads.
The primary security surface is PATH modification. A malicious `.tsuku.toml` could reference tool names that, when resolved to `$TSUKU_HOME/tools/{name}-{version}/bin`, prepend unexpected directories to PATH. The resolution produces paths within `$TSUKU_HOME/tools/` because activation checks that it does.

This paragraph previously said the install pipeline's name validation already guarded this. It did not: that validation runs on names `tsuku install` is given, and a `.tsuku.toml` key never passes through it. Naming a control that does not cover the path in question is worse than naming none, because it ends the reader's inquiry. The control now exists and is described below.

**Mitigations:**
- Tool bin directories are always under `$TSUKU_HOME/tools/`, asserted on the composed path with a separator-appended prefix check so a sibling such as `tools-x` cannot match `tools`
- A declared key is split into its distributed source and bare recipe name, and the bare name -- which is what becomes a path component -- must be a well-formed recipe identifier: a single path segment, rejecting `/`, `\`, `..` and NUL
- A version read from installation state is validated at the same sink before it becomes a path component, because the declared string (`latest`) is not the string that ends up in the path (`26.8.1`)
- Activation only references already-installed tools; it doesn't trigger downloads

### Prompt Hook Safety

Shell hooks execute with user privileges on every prompt, and the shell **evaluates** what `tsuku hook-env` prints: the hook body is `eval "$(tsuku hook-env bash)"`. Calling tsuku as a subprocess does not change that -- its stdout becomes shell code.

This section previously said the output was "structured (export/unset statements with validated values)". The statements were structured; the values inside them were not validated and were not shell-quoted. They were rendered with Go's `%q`, which escapes `"` and `\` but leaves `$` and the backtick live inside the double quotes it produces, so a value containing `$( )` executed on evaluation. `_TSUKU_DIR` in particular carries the directory holding the project config, whose name is chosen by whoever authored the cloned repository -- no validator can help there, because the path is legitimate and merely contains metacharacters.

**Mitigations:**
- Every emitted value is quoted for the target dialect by `internal/shellquote`, with separate POSIX and fish functions. Fish's single quotes recognise `\'` and `\\` where POSIX's recognise nothing, so one shared function would be wrong for one of them.
- Emission goes through a helper that takes the value rather than a format string, so a variable added later cannot reintroduce the defect by omitting a call.
- Diagnostics are written to stderr, never stdout, precisely because stdout is evaluated -- and diagnostics quote the offending key, which is attacker-controlled.
- Hook installation uses marker blocks for clean install/uninstall.

### Untrusted Repository Config

Same concern as Block 4: cloning a repo with `.tsuku.toml` and running `tsuku hook install --activate` means the repo author influences your PATH. Unlike `tsuku install` (which requires explicit invocation), prompt hooks activate automatically on directory entry.

**Mitigations:**
- Prompt hooks are opt-in (`tsuku hook install --activate`), not default. Note that `tsuku shell` needs no hook at all, so anyone running it in an untrusted repository is exposed regardless of hook state.
- Declared names and versions are validated at config load, so a hostile declaration is refused before it reaches a path or the emitted output.
- Activation only references installed tools -- it can't install new ones.
- `TSUKU_CEILING_PATHS` adds ceilings to the discovery walk, but it is **opt-in and unset by default**, and the walk's only unconditional ceiling is `$HOME`. A repository checked out elsewhere walks to `/`, so a config in a world-writable directory applies beneath it. Tracked separately as tsukumogami/tsuku#2555.
- Tools that aren't installed are silently skipped, not fetched.
- Prompt hooks are opt-in (`tsuku hook install --activate`), not default
- Activation only references installed tools -- it can't install new ones
- `TSUKU_CEILING_PATHS` prevents traversal into untrusted parent directories
- A declaration with nothing installed to satisfy it is not fetched; it is reported on stderr with the reason, so the developer knows the repo asked for something they do not have

### Mitigations Summary

| Risk | Severity | Mitigation | Residual Risk |
|------|----------|------------|---------------|
| PATH injection via malicious .tsuku.toml | Was Critical, now Low | Names and versions validated at config load in `internal/project`; refused declarations reported on stderr | A name accepted by the allowlist still selects which installed tool activates |
| Auto-activation in cloned repos | Medium | Hooks are opt-in, only installed tools activate | User may not realize activation affects their PATH in untrusted repos; `tsuku shell` needs no hook |
| Prompt hook eval of hook-env output | Was Critical, now Low | Every emitted value quoted per dialect by `internal/shellquote`; emission takes values, not format strings | If the tsuku binary is compromised, hook-env output is arbitrary |
| Config discovered outside `$HOME` | Medium | None yet | A config in a world-writable directory applies to everyone working beneath it. tsukumogami/tsuku#2555 |
| _TSUKU_PREV_PATH tampering | Low | Value is shell-quoted on emission, so it cannot execute | A user can still put a misleading PATH in their own environment |

The first and third rows were rated Low against mitigations that did not exist.
Row one claimed "All paths constrained to $TSUKU_HOME/tools/, name validation"
while its own residual-risk cell, one column to the right, said "Tool names with
unusual characters could construct unexpected paths" -- the table recorded the
real behaviour and rated it Low anyway. Row three claimed structured output was
the control, when the structure was never in question and the values inside it
were unquoted. Both were reported as tsukumogami/tsuku#2553 and are fixed above;
the severities here now describe the state after that fix, with the pre-fix
rating shown so the correction is visible rather than silent.
| PATH injection via malicious .tsuku.toml | Low | Derived bare name must be a single safe path segment; state-derived version validated at the same sink; composed path checked to lie under $TSUKU_HOME/tools/ | A key rejected by those checks contributes no PATH entry and is reported as `bad-form`, so an unusual name cannot construct an unexpected path |
| Auto-activation in cloned repos | Medium | Hooks are opt-in, only installed tools activate | User may not realize activation affects their PATH in untrusted repos |
| Prompt hook eval of hook-env output | Low | Structured output (export/unset only), subprocess invocation | If tsuku binary is compromised, hook-env output is arbitrary |
| _TSUKU_PREV_PATH tampering | Low | Graceful handling when variable is missing | User could inject malicious PATH via env var manipulation |

## Consequences

### Positive

- **Automatic per-project tool versions**: Developers get the right tool versions by entering the project directory. No manual switching.
- **Zero overhead when unused**: Users who don't install hooks see no change. `shellenv` behavior is identical.
- **Clean deactivation**: Leaving a project restores the exact pre-activation PATH.
- **Incremental adoption**: Users can start with `tsuku shell` (explicit) and upgrade to hooks when comfortable.

### Negative

- **Per-prompt fork+exec cost**: ~2-4ms on every prompt for hook-env invocation, even when nothing has changed.
- **PATH changes lost on deactivation**: Other tools' PATH modifications during a project session are lost when deactivating.
- **A declaration can go unhonored**: If `.tsuku.toml` declares something no installed version satisfies, that tool is not activated. It is reported rather than skipped silently, but the project still does not get the tool it asked for.

### Mitigations

- **Per-prompt cost**: The 2-4ms cost is consistent with mise and direnv. Benchmarking should confirm this during implementation.
- **PATH changes lost**: Document this behavior. If demand arises, the surgical-removal approach can be added later.
- **Unhonored declarations**: A message on stderr naming the tool and the reason, once per entry into the project. This was written here as a planned mitigation and was not built for a long time, which is how the silent-skip behavior survived; it now exists. It does not affect the fast path, since a file whose declarations are all honored produces no messages.
