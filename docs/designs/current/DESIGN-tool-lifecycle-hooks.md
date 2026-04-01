---
status: Proposed
problem: |
  Tsuku installs tools by downloading binaries and symlinking them, but tools
  that need shell integration (eval-init wrappers, completions, env files) or
  cleanup on removal get a bland, incomplete installation. At least 8-12 tools
  in the registry (direnv, zoxide, mise, niwa) cannot provide their core
  functionality without post-install shell setup. The recipe system has no
  lifecycle phases beyond the install step sequence, the remove flow doesn't
  consult recipes, and the update flow has no pre/post hooks.
decision: |
  Add a phase field to recipe steps (post-install, pre-remove, pre-update) with
  new declarative actions (install_shell_init, install_completions) that write
  per-tool init scripts to a shell.d directory. Extend tsuku shellenv to source
  a cached concatenation of these scripts. Track cleanup instructions in state
  so removal works without loading recipes. Security hardening includes exec-based
  command invocation, content hashing, symlink validation, and update-time diffs.
rationale: |
  Extends the proven action system (phase field on Step, new actions in the
  registry) rather than redesigning the recipe schema. State-tracked cleanup
  makes removal reliable offline. The shell.d cache keeps shell startup under
  5ms while giving tsuku control over ordering and lifecycle. Declarative-only
  hooks preserve tsuku's trust model advantage over npm-style arbitrary scripts.
---

# DESIGN: Tool Lifecycle Hooks

## Status

Proposed

## Context and Problem Statement

Niwa's shell-integration design (tsukumogami/niwa#39) identified a gap: when
niwa is installed via tsuku, it gets a generic binary-and-symlink installation
that misses its shell function wrapper, completions, and env file setup. The
design explicitly deferred tsuku generalization, noting "tsuku has no post-install
shell mechanism" and that adding one would cost 200+ LOC across two repos with
only one consumer.

Exploration found that generalization is now warranted. A survey of 1,400 recipes
identified 8-12 tools that can't function without post-install shell integration
(direnv, zoxide, asdf, mise) and 200+ that would benefit from completion
registration. The current action system has the building blocks (set_env,
run_command) but no lifecycle phase concept -- all steps execute during a single
install pass. The remove flow deletes directories without consulting recipes,
and the update flow is a plain reinstall with no pre/post hooks.

The design must answer: how should recipes declare lifecycle behavior, how should
per-tool shell integration compose with tsuku's existing shellenv, what security
model applies to tool-defined hooks, and how does cleanup state persist across
install/remove cycles?

## Decision Drivers

- Tsuku's current trust model (no post-install code execution) is a genuine
  security advantage over npm-style arbitrary scripts -- preserve it where possible
- The existing action system's WhenClause infrastructure is proven and extensible
- Shell startup time budget is tight (~50ms total); per-tool init adds 10-30ms each
- Tools must work when installed standalone (not just via tsuku)
- Cleanup must be reliable even without registry access at remove time
- Recipe authors should be able to declare lifecycle behavior in familiar TOML format

## Decisions Already Made

These choices were settled during exploration and should be treated as constraints:

- **Start with declarative hooks (Level 1), not imperative scripts**: Security
  research shows tsuku's current trust model is a strength. A limited vocabulary
  of lifecycle actions (install_shell_init, install_completions, cleanup_paths)
  preserves this while enabling the key use cases. Imperative hooks (Level 2) can
  be added later if declarative proves insufficient.

- **shell.d directory model for composition**: Ecosystem patterns (mise, asdf) and
  tsuku's existing hook machinery support this. Post-install hooks write init
  scripts to $TSUKU_HOME/share/shell.d/{tool}.{shell}. Cached combined scripts
  keep startup under 5ms.

- **Extend existing action system rather than new recipe sections**: The
  WhenClause/Step infrastructure is proven. Adding a phase qualifier is lower
  risk than a recipe schema redesign with separate [lifecycle] sections.

- **Post-install shell integration is the priority**: 8-12 tools can't function
  without it. Completions and service registration are secondary.

- **Store cleanup instructions in state at install time**: The remove flow doesn't
  load recipes today. Storing what was installed (which shell.d files, which
  completions) in state ensures reliable cleanup without registry access.

- **Hooks fail gracefully, not fatally**: Hook failure should warn but not block
  installation or removal. The tool is still installed and usable.

## Considered Options

### Decision 1: Recipe schema for lifecycle phases

How should recipes declare steps that run at different lifecycle points (post-install,
pre-remove, pre-update) rather than during the main install sequence?

Key assumptions:
- Recipe authors use 1-2 lifecycle hooks per recipe at most
- Backward compatibility is non-negotiable -- all existing recipes must work unchanged
- The executor gains an `ExecutePhase(phase)` method that filters and runs steps

#### Chosen: Phase field on Step struct

Add a `phase` string field to the Step struct. Values: `"install"` (default),
`"post-install"`, `"pre-remove"`, `"pre-update"`. Steps without a `phase` field
default to `"install"`, so every existing recipe works unchanged. The executor
filters steps by the current phase using a one-line check in the step loop.

New declarative actions for the initial release:

- **`install_shell_init`** -- Writes a shell initialization script to
  `$TSUKU_HOME/share/shell.d/{target}.{shell}`. Params: `source_command` (tool
  command that generates init, e.g., `niwa shell-init bash`) or `source_file`
  (path relative to tool install dir), `target` (name for shell.d files).
  Generates per-shell variants (bash, zsh, fish where supported).

- **`install_completions`** -- Installs shell completions to
  `$TSUKU_HOME/share/completions/{shell}/{tool}`. Params: `command` (to generate
  completions) or `source_file`, `shells` (target shells). Lower priority than
  shell-init.

Both actions record cleanup instructions in state (see Decision 3).

Recipe example for niwa:

```toml
[[steps]]
action = "github_archive"
repo = "tsukumogami/niwa"
asset_pattern = "niwa-v{version}-{os}-{arch}.tar.gz"
archive_format = "tar.gz"
strip_dirs = 1
binaries = ["niwa"]

[[steps]]
action = "install_shell_init"
phase = "post-install"
source_command = "niwa shell-init {shell}"
target = "niwa"
```

#### Alternatives Considered

- **Phase field on WhenClause**: Embed `phase` in the existing `when` condition
  block alongside `os`/`arch`/`libc`. Rejected because it conflates lifecycle
  semantics with platform filtering. WhenClause's matching logic (AND across
  dimensions, empty matches all) doesn't fit phase selection. Every step without
  an explicit `when.phase` would implicitly run during install, breaking the
  `IsEmpty()` shortcut and creating special cases in every code path that handles
  WhenClause.

- **Separate step arrays per phase**: Add `[[post_install_steps]]`,
  `[[pre_remove_steps]]` as new top-level TOML arrays. Cleanest separation, but
  each new phase requires a new Recipe struct field, TOML array, loader path, and
  validator. Doesn't scale -- the schema grows linearly with phases. Also breaks
  the "one steps array" mental model recipe authors know.

### Decision 2: Shell integration delivery mechanism

How do per-tool shell init scripts reach the user's shell at startup, and how is
the 5ms startup budget met?

Key assumptions:
- Shell.d scripts are small (under 50 lines each); if a tool's init is expensive,
  the tool is responsible for lazy-loading
- The number of tools needing shell init stays in the 5-15 range
- Users already have `eval "$(tsuku shellenv)"` in their rc files
- Extending shellenv's output is backward-compatible (additive)

#### Chosen: Extend shellenv to source cached shell.d/

`tsuku shellenv` gains a second output section. After the PATH export, it emits a
source command for a cached combined init file:

```bash
export PATH="$TSUKU_HOME/bin:$TSUKU_HOME/tools/current:$PATH"
. "$TSUKU_HOME/share/shell.d/.init-cache.bash"
```

The cache file (`.init-cache.{bash,zsh,fish}`) concatenates all per-tool scripts
in `$TSUKU_HOME/share/shell.d/`. It's regenerated by tsuku whenever the tool list
changes (install, remove, update) using atomic writes. If no tools need shell init,
the source line is omitted.

Directory structure:

```
$TSUKU_HOME/share/shell.d/
  niwa.bash
  niwa.zsh
  direnv.bash
  direnv.zsh
  .init-cache.bash    # concatenation of *.bash
  .init-cache.zsh     # concatenation of *.zsh
```

Startup cost: `eval "$(tsuku shellenv)"` is ~3ms (Go binary startup). Sourcing the
cache file adds <1ms (file read) plus 1-5ms executing shell functions (no
subprocesses). Total incremental cost: under 5ms.

Cache rebuild triggers on `tsuku install`, `tsuku remove`, and `tsuku update` when
any tool with shell.d scripts is modified. Uses the existing `atomicWrite` helper.

#### Alternatives Considered

- **Separate `tsuku shell-init` command**: Users add a second eval line. Same
  caching approach, but doubles user setup friction (two eval lines instead of one),
  adds a second Go process spawn (~3ms), and splits a conceptually unified operation.
  Rejected because the user-facing cost doesn't justify the internal separation.

- **Direct sourcing without tsuku mediation**: Users add a glob-source loop to their
  rc files. No tsuku command at startup, no cache. Rejected because the loop syntax
  varies per shell and is error-prone, there's no ordering control (glob is
  alphabetical), and startup cost grows linearly with tool count.

### Decision 3: Remove and update lifecycle awareness

How do remove and update flows execute cleanup hooks, and how are cleanup instructions
persisted across install/remove cycles?

Key assumptions:
- `VersionState` already stores enough context to locate cleanup targets
- Tools installed before lifecycle hooks have no cleanup state (graceful degradation)
- `tsuku reinstall` can populate cleanup state for legacy installs

#### Chosen: State-driven cleanup

Install records cleanup actions as a new `CleanupActions` field on `VersionState`:

```go
type CleanupAction struct {
    Action string `json:"action"`  // "delete_file", "delete_dir"
    Path   string `json:"path"`    // absolute or $TSUKU_HOME-relative
}
```

When `install_shell_init` writes `$TSUKU_HOME/share/shell.d/niwa.bash`, it records
`CleanupAction{Action: "delete_file", Path: "share/shell.d/niwa.bash"}`. During
`tsuku remove`, the remove flow reads cleanup actions from state and executes them
before deleting the tool directory.

Multi-version handling: before deleting a shared file, check whether another installed
version of the same tool has a matching cleanup path. If so, skip (resource still in
use).

Update ordering: (1) install new version with fresh hooks, (2) compute stale cleanup
(actions in old version not in new), (3) execute stale cleanup, (4) delete old
version directory. This ensures no gap in shell.d coverage during updates.

Failure handling: each cleanup action executes independently. Failures log warnings
and continue. Cleanup errors never block removal.

#### Alternatives Considered

- **Recipe-consulted cleanup**: Remove loads the recipe and runs pre-remove steps.
  Rejected because it introduces version skew (recipe at remove time may differ from
  recipe at install time), requires registry availability, and violates the exploration
  decision to store cleanup in state.

- **Hybrid (state + recipe fallback)**: State handles common patterns, recipe handles
  complex cleanup. Rejected for unnecessary complexity -- two code paths for cleanup
  in a single operation, conflict resolution between sources, and the declarative
  action vocabulary already covers the identified use cases.

## Decision Outcome

Lifecycle hooks work through three coordinated mechanisms: recipe-level declaration,
shell-level delivery, and state-tracked cleanup.

Recipes declare lifecycle steps in the same `[[steps]]` array they use today, with a
new `phase` field that defaults to `"install"`. A recipe for niwa adds two steps: an
`install_shell_init` step with `phase = "post-install"` that generates shell function
wrappers to `$TSUKU_HOME/share/shell.d/niwa.{bash,zsh}`, and the existing install
steps that download and symlink the binary. The executor gains an `ExecutePhase`
method that filters steps by phase -- during install it runs `"install"` steps as
today, then `"post-install"` steps afterward.

Shell delivery extends `tsuku shellenv` to source a cached combined init file after
the PATH export. The cache concatenates all per-tool scripts from the shell.d
directory into a single `.init-cache.{shell}` file, rebuilt atomically on every
install, remove, or update that touches shell.d. Users keep their existing
`eval "$(tsuku shellenv)"` line unchanged -- the new behavior is automatic.

Cleanup is state-driven. When post-install actions create files outside the tool
directory (shell.d scripts, completions), they record `CleanupAction` entries in
`VersionState`. The remove flow reads these entries and deletes the files before
removing the tool directory. No recipe loading at remove time. Multi-version
installs are handled by cross-referencing cleanup paths across versions before
deletion. Update flow installs the new version first (no gap in shell.d coverage),
then computes and executes stale cleanup for paths the new version no longer creates.

The decisions reinforce each other: the phase field on steps keeps recipe authoring
simple (one array, one new field), the shell.d cache keeps startup fast (under 5ms
incremental), and state-tracked cleanup keeps removal reliable (no registry
dependency). The main trade-off is that cleanup is limited to declarative actions
(file deletion). Complex cleanup patterns that can't be expressed as
`CleanupAction` entries would need the state schema extended -- but the exploration
scoped this to Level 1 (declarative only), and the identified use cases (shell.d
files, completions) are all file operations.

## Solution Architecture

### Overview

Lifecycle hooks extend tsuku's install-only recipe system with post-install, pre-remove,
and pre-update phases. The design adds three things: a `phase` field on recipe steps that
controls when they execute, new declarative actions for shell integration and completions,
and state-tracked cleanup that makes removal self-contained.

### Components

```
Recipe (TOML)
  |
  +-- [[steps]] phase="install"        (existing, unchanged)
  +-- [[steps]] phase="post-install"   (new: install_shell_init, install_completions)
  +-- [[steps]] phase="pre-remove"     (new: reserved for future use)
  +-- [[steps]] phase="pre-update"     (new: reserved for future use)
  |
Executor
  |
  +-- ExecutePhase("install")          (existing step loop, filtered)
  +-- ExecutePhase("post-install")     (new: runs after install, records CleanupActions)
  |
State (state.json)
  |
  +-- VersionState.CleanupActions[]    (new: recorded by post-install actions)
  |
Remove Flow
  |
  +-- Execute CleanupActions           (new: before directory deletion)
  +-- Rebuild shell.d cache            (new: after cleanup)
  |
shellenv
  |
  +-- PATH export                      (existing, unchanged)
  +-- source .init-cache.{shell}       (new: sources cached shell.d scripts)

Shell.d Directory ($TSUKU_HOME/share/shell.d/)
  |
  +-- {tool}.{bash,zsh}               (per-tool init scripts, written by install_shell_init)
  +-- .init-cache.{bash,zsh}          (concatenated cache, rebuilt on tool list change)
```

### Key Interfaces

**Step.Phase field** (`internal/recipe/types.go`)

```go
type Step struct {
    Action       string            `toml:"action"`
    Phase        string            `toml:"phase"`        // new: "install" (default), "post-install", "pre-remove", "pre-update"
    When         *WhenClause       `toml:"when"`
    // ... existing fields unchanged
}
```

Missing `phase` defaults to `"install"`. The TOML parser handles this naturally --
omitted fields get the zero value, and the executor treats `""` as `"install"`.

**ResolvedStep.Phase** (`internal/executor/plan.go`)

The executor operates on `InstallationPlan.Steps` (type `[]ResolvedStep`), not raw
recipe steps. `ResolvedStep` must also carry the `Phase` field so that
`ExecutePhase` can filter resolved steps. `GeneratePlan` propagates `Phase` from
`recipe.Step` to `ResolvedStep` during plan generation. Post-install steps appear
in the plan but are marked non-evaluable during dry-run/validation (they depend on
the installed binary being available).

```go
type ResolvedStep struct {
    // ... existing fields
    Phase string `json:"phase,omitempty"`
}
```

**ToolInstallDir availability during post-install.** The `ExecutionContext.ToolInstallDir`
is set after the install phase copies files to their final location. Post-install
steps run after this copy, so `ToolInstallDir` is populated and `source_command` can
resolve the tool binary via `{install_dir}/bin/{tool}`.

**Executor.ExecutePhase** (`internal/executor/executor.go`)

```go
func (e *Executor) ExecutePhase(ctx *ExecutionContext, phase string) ([]CleanupAction, error)
```

Filters `recipe.Steps` to those matching the given phase (and passing WhenClause
evaluation), executes them in array order, and returns any cleanup actions recorded
by the actions. The existing `ExecutePlan` method becomes a wrapper that calls
`ExecutePhase("install")`.

**install_shell_init action** (`internal/actions/shell_init.go`)

Params:
- `source_command` (string): command template to generate init script (e.g.,
  `niwa shell-init {shell}`). `{shell}` is substituted with `bash`, `zsh`, etc.
- `source_file` (string): alternative -- path relative to tool install dir containing
  the init script. One of `source_command` or `source_file` required.
- `target` (string): name for the shell.d file (e.g., `niwa`). Required.
- `shells` ([]string): which shells to generate for. Defaults to `["bash", "zsh"]`.

Behavior: for each shell in `shells`, either runs `source_command` (with `{shell}`
substituted) or copies `source_file` to `$TSUKU_HOME/share/shell.d/{target}.{shell}`.
Returns `CleanupAction{Action: "delete_file", Path: "share/shell.d/{target}.{shell}"}`
for each file created.

Security constraints on the action:
- `shells` values are validated against a hardcoded allowlist: `["bash", "zsh", "fish"]`.
  Arbitrary strings are rejected to prevent template injection into `source_command`.
- `source_command` is invoked via `exec` (not `sh -c`) to prevent shell metacharacter
  injection. The command template is split into argv, not passed through a shell.
- `source_file` is validated to resolve within the tool's install directory after
  symlink resolution (prevents path traversal and symlink-to-arbitrary-file attacks).

**CleanupAction** (`internal/state/types.go`)

```go
type CleanupAction struct {
    Action string `json:"action"`  // "delete_file", "delete_dir"
    Path   string `json:"path"`    // relative to $TSUKU_HOME
}
```

Stored in `VersionState.CleanupActions`. Paths are $TSUKU_HOME-relative for
portability.

**RebuildShellCache** (`internal/shellenv/cache.go`)

```go
func RebuildShellCache(tsukuHome string, shell string) error
```

Reads all `*.{shell}` files from `$TSUKU_HOME/share/shell.d/`, concatenates them
(sorted alphabetically or by configured order), and atomically writes
`.init-cache.{shell}`. Called by install, remove, and update after any shell.d
modification.

### Data Flow

**Install with shell init:**

```
1. tsuku install niwa
2. Executor.ExecutePhase("install")
   - download, extract, symlink (existing flow)
3. Executor.ExecutePhase("post-install")
   - install_shell_init runs: niwa shell-init bash > share/shell.d/niwa.bash
   - install_shell_init runs: niwa shell-init zsh  > share/shell.d/niwa.zsh
   - Returns CleanupActions: [{delete_file, share/shell.d/niwa.bash}, ...]
4. CleanupActions stored in VersionState
5. RebuildShellCache("bash"), RebuildShellCache("zsh")
6. User opens new shell -> eval "$(tsuku shellenv)" -> niwa shell function active
```

**Remove with cleanup:**

```
1. tsuku remove niwa
2. Load VersionState from state.json
3. Read CleanupActions: [{delete_file, share/shell.d/niwa.bash}, ...]
4. For each action: check no other version references same path, then delete
5. RebuildShellCache("bash"), RebuildShellCache("zsh")
6. Delete tool directory, remove symlinks, remove state entry (existing flow)
```

**Update:**

```
1. tsuku update niwa
2. Install new version (ExecutePhase "install" + "post-install", new CleanupActions)
3. Compute stale: old CleanupActions - new CleanupActions
4. Execute stale cleanup (delete files new version no longer creates)
5. RebuildShellCache for affected shells
6. Delete old version directory
```

## Implementation Approach

### Phase 1: Recipe schema and executor

Add the `phase` field to the Step struct. Implement `ExecutePhase` in the executor.
Ensure all existing recipes work unchanged (empty phase = "install"). Add the
`install_shell_init` action with `source_command` support.

Deliverables:
- `internal/recipe/types.go` -- Phase field on Step
- `internal/executor/executor.go` -- ExecutePhase method
- `internal/actions/shell_init.go` -- install_shell_init action
- Tests for phase filtering and backward compatibility

### Phase 2: Shell.d, shellenv, and state integration

Create the shell.d directory structure. Implement `RebuildShellCache`. Extend
`tsuku shellenv` to source the cache file. Add `CleanupActions` to `VersionState`
and wire post-install actions to record cleanup entries. Cache rebuild and cleanup
recording are co-dependent (knowing which shells to rebuild requires the cleanup
entries that track which files were written), so they ship together.

Deliverables:
- `internal/shellenv/cache.go` -- RebuildShellCache with symlink checks and file locking
- `cmd/tsuku/shellenv.go` -- extended output with source line
- `internal/actions/shell_init.go` -- file writing to shell.d with input validation
- `internal/state/types.go` -- CleanupAction struct, VersionState field
- Tests for cache generation, shellenv output, and cleanup recording

### Phase 3: Remove integration

Modify the remove flow to read and execute cleanup actions from state. Wire cache
rebuild into the remove flow.

Deliverables:
- `internal/state/types.go` -- CleanupAction struct, VersionState field
- `internal/install/remove.go` -- cleanup execution before deletion
- Tests for cleanup recording, execution, and multi-version handling

### Phase 4: Update flow and completions

Wire cleanup into the update flow (install new, clean stale, delete old). Add
`install_completions` action. Update first recipe (niwa) to use lifecycle hooks.

Deliverables:
- `cmd/tsuku/update.go` -- lifecycle-aware update
- `internal/actions/completions.go` -- install_completions action
- `recipes/n/niwa.toml` -- updated with post-install hooks
- Integration tests for the full install/remove/update cycle

## Security Considerations

This design introduces a trust model shift. Before lifecycle hooks, tsuku's security
posture was: we trust recipes to declare correct download-and-symlink steps for
pre-built binaries. After: we trust recipes to declare commands whose output is safe
to source in every shell session. This is a qualitative change in what trust means.

Four security dimensions apply:

**External artifact handling.** The `install_shell_init` action has two paths:
`source_command` (runs the installed binary to generate init scripts) and
`source_file` (copies a file from the downloaded archive). Both write to
`$TSUKU_HOME/share/shell.d/`, where content is sourced in every new shell.
`source_command` is higher risk because its output is dynamic -- a compromised
binary can vary its output based on environment detection, and recipe review
can't catch this.

Mitigations:
- Restrict `source_command` to invoke only the tool's own installed binary. Parse
  the command template and verify the executable resolves to a binary in the tool's
  install directory. Reject recipes where `source_command` invokes arbitrary commands.
- Scan `source_command` output for high-risk patterns before writing to shell.d:
  network commands (`curl`, `wget`, `nc`), credential file paths (`~/.ssh`,
  `~/.aws`), and shell built-in overrides. This is defense-in-depth alongside PR
  review, not a complete barrier.
- Consider shipping `source_file` first. It's lower risk (content is static and
  inspectable in the archive) and covers use cases where the tool ships pre-built
  init scripts. `source_command` follows once the validation infrastructure is in
  place.

**Permission scope.** Shell.d scripts run with the user's full interactive shell
privileges -- access to SSH keys, API tokens, cloud credentials, and the ability to
alias or wrap security-sensitive commands. The cache file
(`.init-cache.{bash,zsh}`) is a single point of compromise: modifying it gives
persistent access to every new shell session.

Mitigations:
- Set restrictive file permissions: 0700 on the shell.d directory, 0600 on cache
  and individual script files.
- Store SHA-256 content hashes for each shell.d file in `VersionState` at write
  time. During cache rebuild, verify all shell.d files match their stored hashes.
  This detects tampering between install and sourcing, and prevents TOCTOU races
  with concurrent installs.
- Use file locking during cache rebuild to prevent concurrent installations from
  racing on shell.d writes.

**Supply chain trust.** A compromised upstream tool can inject malicious shell code
via `source_command` without any recipe change triggering re-review. This is the
most significant new risk. The recipe PR review model protects the TOML declaration,
but the runtime output of the declared command is unreviewed.

Mitigations:
- Recipe metadata should make shell integration visible: a field like
  `shell_integration = true` that `tsuku info` displays, so users know which tools
  influence their shell environment.
- Content hashing in state (described above) detects when a tool update changes its
  init output without the recipe changing -- a signal worth surfacing to the user.

**Input validation.** Three injection vectors require hardening at the action level:
- The `shells` array must be validated against a hardcoded allowlist (`bash`, `zsh`,
  `fish`). Arbitrary values could inject template variables into `source_command`.
- `source_command` must be invoked via `exec` (argv splitting), not `sh -c`. Shell
  metacharacters (pipes, semicolons, backticks) in the command template would
  otherwise enable arbitrary execution beyond the declared tool binary.
- `source_file` must resolve within the tool's install directory after symlink
  resolution. Without this, a recipe could use `../../share/shell.d/.init-cache.bash`
  to overwrite the cache directly.

**Symlink following.** Cache rebuild must verify that shell.d files are regular
files, not symlinks. A compromised tool's post-install hook could create a symlink
from `shell.d/tool.bash` to an arbitrary file, causing the cache to include
unrelated content.

**Denial of shell.** A syntax error in any tool's init script breaks all new shell
sessions (the cache concatenates everything). The cache file should wrap each tool's
content in a subshell or error-trapped block so one tool's failure doesn't prevent
others from loading. `tsuku doctor` should syntax-check shell.d files.

**Update visibility.** The highest residual supply-chain risk is a compromised
upstream binary silently changing its `source_command` output. When `tsuku update`
runs `install_shell_init` and the output differs from the stored hash, tsuku should
display the diff and log a warning. This makes silent changes visible without
blocking the update.

**Content scanning limitations.** Pattern-based scanning of shell.d output (checking
for `curl`, credential paths, etc.) catches accidental mistakes but is trivially
evaded by deliberate attackers (variable construction, base64 encoding). It should
be framed as an accident-catcher in recipe CI, not a security barrier.

**User visibility.** The cache file is opaque by design (concatenated for
performance). Users need a way to inspect what's being sourced.

Mitigations:
- `tsuku doctor` gains shell.d verification: cache freshness, content hash
  integrity, symlink detection, syntax checking, and listing of active shell init
  scripts with their source tools.
- `tsuku install --no-shell-init` allows installing a tool without its shell
  integration, for users who want to inspect and configure manually.
- File locking during cache rebuild prevents TOCTOU races with concurrent installs.

## Consequences

### Positive

- Tools that need shell integration (niwa, direnv, zoxide) work properly when
  installed via tsuku, without manual user setup
- Existing users get the feature automatically -- their `eval "$(tsuku shellenv)"`
  line starts sourcing tool init after upgrading tsuku
- Recipe authors add 3-5 lines to get lifecycle hooks, using the same `[[steps]]`
  format they already know
- Removal is reliable: state-tracked cleanup works offline, without registry access
- Shell startup stays fast: cached init file keeps incremental cost under 5ms

### Negative

- shellenv's output is no longer a single static line; debugging requires checking
  both PATH export and cache file content
- Cache staleness is a new failure mode -- if `RebuildShellCache` fails silently,
  tool init doesn't update until the next tool list change
- The `source_command` param in `install_shell_init` executes the installed tool's
  binary during post-install, which is a form of code execution (though it's the
  tool's own binary, already trusted by installing it)
- Tools installed before this feature have no cleanup state -- removal works as
  today (directory deletion only), with no shell.d cleanup

### Mitigations

- `tsuku doctor` gains a cache freshness check (compare shell.d file list against
  cache contents) and offers to rebuild
- `tsuku reinstall <tool>` populates cleanup state for tools installed before the
  feature existed
- The `source_command` execution uses the same binary the user explicitly chose to
  install; this is equivalent to the user running the command manually
