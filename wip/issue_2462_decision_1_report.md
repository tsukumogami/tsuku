# Decision: where a dependency's cleanup actions get persisted

**Prefix**: `issue_2462_decision_1`
**Tier**: 3 (standard), fast path
**Status**: COMPLETE
**Chosen**: A — the dependency gets a hidden state entry of its own
**Confidence**: high

## Question

Issue #2462's first acceptance criterion asks that cleanup actions recorded while
installing a dependency "end up in `state.json` against the dependency's version state."
A dependency installed by `Executor.installSingleDependency` has no `state.json` entry of
any kind, so the criterion presupposes a record that does not exist. Where do the cleanup
actions go, and what has to be created to hold them?

## Alternatives

**A — the dependency gets a hidden state entry of its own.** After the dependency is
copied into `$TSUKU_HOME/tools/<name>-<version>`, write a minimal `ToolState`:
`IsHidden`, `IsExecutionDependency`, a `RequiredBy` edge back to the tool that pulled it
in, and a `Versions[version]` entry carrying `InstalledAt` and the cleanup actions. The
directory mechanics are untouched — no symlinks, no wrappers, no binaries registered.

**B — attach the cleanup actions to the parent tool.** No state entry for the dependency.
The executor merges the dependency's cleanup actions into what `GetCleanupActions()`
returns, and the existing `finishPostInstall` call records them against the parent's
`VersionState`.

**C — route dependency installs through the normal install path.** Stop having the
executor install dependencies. Move the dependency loop out of `ExecutePlan` and up to
the cmd layer, and install each dependency through `install.Manager.InstallWithOptions`
with `HiddenInstallOptions()` plus the full
ExecutePlan/SetToolInstallDir/ExecutePhase/finishPostInstall sequence.

## Why A

**B is not merely weaker, it is incorrect.** `BuildShellDSelection`
(`internal/install/shelld.go:21`) projects a recorded cleanup path into `Active` only when
the version that recorded it is that tool's active version. Under B, dependency D's
fragment `D@1.0.bash` is recorded under parent X's `VersionState` for X@2.0. When the
user upgrades X to 3.0, X's `ActiveVersion` becomes 3.0. D is already installed, so the
`os.Stat(finalDir)` dedup at `internal/executor/executor.go:794` short-circuits, D's steps
never run, and X@3.0 records nothing for D. `D@1.0.bash` is then in `Known` but not in
`Active`, so `ShellDSelection.excludes` (`internal/shellenv/selection.go:35`) returns true
and the fragment is dropped from the rebuilt cache. D's shell integration silently
disappears while D is installed and unchanged — the exact silent-breakage failure the
design doc narrowed the exclusion rule to avoid. B would reintroduce it through a
different door.

B also cannot satisfy the second and fifth acceptance criteria, which both say "remove the
dependency." Under B the dependency still has no state entry, so `tsuku remove <dep>`
fails with `tool %q is not installed` (`internal/install/remove.go:240`).

**A is correct by construction on the same point.** The fragment is named after the
dependency and keyed on the dependency's version — `install_shell_init` builds the
filename from its own `target` param and `ctx.Version`
(`internal/actions/shell_init.go:179`), and `set_env` from `ctx.Recipe.Metadata.Name`
(`internal/actions/set_env.go:204`), which `installSingleDependency` sets to the
dependency. So the state entry that decides whether the fragment belongs in the cache has
to be the dependency's. Nothing about the parent's lifecycle can then strand it.

**C is the right long-term shape and the wrong size for a bug fix.**
`InstallWithOptions` does staging, symlinks, wrappers, binary checksums, and event-bus
publishing; the dependency loop is recursive and lives inside `ExecutePlan`, which
`cmd/tsuku/plan_install.go` calls with no recipe loader at all; and library dependencies
go through a different manager entry point entirely. That is a refactor of the install
path, not a fix to a discarded slice, and it would put a broad regression surface under a
one-line defect. Recorded as the direction of travel.

## What A costs

`install.HiddenInstallOptions()` (`internal/install/hidden.go:11`) already describes this
exact shape — `CreateSymlinks: false, IsHidden: true` — and its read side is fully wired
(`IsHidden`, `ExposeHidden`, `CheckAndExposeHidden`, and the hidden filter in
`ListWithOptions`). It simply had no production writer. A supplies one.

Blast radius on `state.Installed` consumers:

- `tsuku list` — a hidden entry is filtered out by default (`internal/install/list.go:42`)
  and shown under `--all` with the `* ` execution-dependency prefix
  (`cmd/tsuku/list.go:142`). This is the intended display for exactly this kind of tool.
- `tsuku remove <dep>` — now works, executing the dependency's own cleanup actions via
  `RemoveAllVersions` (`internal/install/remove.go:223`). The `RequiredBy` edge makes it
  warn and demand `--force` first (`cmd/tsuku/remove.go:58`), which is the desired
  behavior for a tool something else depends on.
- The shell.d projection — the dependency's fragment is now `Active` while the
  dependency's own version is active, independent of the parent.

Two guards keep A from overreaching, both mutation-tested:

- The active version is claimed only when the tool has no state yet. A plan can pin a
  dependency version that is not the one the user has active, and installing it must not
  switch what the user's shell sources.
- `IsHidden` / `IsExecutionDependency` are set only on a fresh entry. A tool the user
  installed explicitly must not become hidden because some later recipe depended on it.

## Assumptions

- Library dependencies (`RecipeType == "library"`) are out of scope. They land in
  `$TSUKU_HOME/libs` and are tracked in `state.Libs`, whose `LibraryVersionState`
  (`internal/install/state.go:121`) has no cleanup-action field. `set_env` is rejected in
  library recipes by the validator (`internal/recipe/validator.go:503`), so the gap is
  narrow. The implementation warns rather than silently orphaning, and leaves the schema
  change to issue #2461.
- Existing dependency directories installed before this change keep no state entry. The
  `os.Stat(finalDir)` dedup returns before any recording, so they are grandfathered.
  Their fragments remain unrecorded, which — per the deliberately narrow exclusion rule —
  means they keep being sourced exactly as they are today.

## Rejected

- **B (attach to the parent)** — silently drops the dependency's shell fragment from the
  cache on the next parent upgrade, and leaves `tsuku remove <dep>` unable to work at all.
- **C (reroute through the normal install path)** — right shape, wrong size. A refactor
  of the install path carrying a broad regression surface, for a defect that is a
  discarded slice and a missing phase filter.

## Process note

Three parallel investigations were commissioned, one per alternative, to check blast
radius against the actual code. They did not return before the implementation was
verified by other means. Every claim above is instead grounded in a file:line reading
done directly, and the decision's load-bearing guards are each covered by a mutation test
that was confirmed to fail when the guard is removed (nine mutations applied, nine
killed). Should a late report contradict anything here, it is worth re-reading before
merge.
