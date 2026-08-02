# Decision Context: where dependency cleanup actions get persisted

**Prefix**: `issue_2462_decision_1`
**Tier**: 3 (standard) — fast path, Phases 0, 1, 2, 6
**Parent**: issue #2462, `/work-on` analysis phase

## Question

Issue #2462's first acceptance criterion says cleanup actions recorded while installing a
dependency must "end up in `state.json` against the dependency's version state." A
dependency installed by `Executor.installSingleDependency` has no `state.json` entry of
any kind, so the criterion presupposes a record that does not exist. Where should those
cleanup actions actually be persisted, and what has to be created to hold them?

## Why this is contested rather than plumbing

The three candidate homes differ in blast radius by roughly an order of magnitude each,
and two of them change what `tsuku list`, `tsuku remove`, and the GC see.

## Verified facts

1. `ExecutePlan` (executor.go:433) calls `installDependencies` at line 450 but assigns
   `e.ctx` at line 531. Any write into `e.ctx` from dependency installation is clobbered.
   A separate accumulator on the `Executor` is required no matter which home wins.

2. `installSingleDependency` (executor.go:780) never touches `StateManager`. It creates a
   temp work dir, runs every step inline, `copyDir`s the result into
   `$TSUKU_HOME/tools/<dep>-<version>/` (or `libs/<dep>-<version>/`), appends the dep's
   `bin/` to `e.execPaths`, and returns. No `ToolState`, no `VersionState`, no symlink,
   no binary registration, no event.

3. `install.Manager.ListWithOptions` (list.go:30) iterates `state.Installed`. A dependency
   installed by this path is invisible to `tsuku list`. `Manager.Remove` and
   `RemoveAllVersions` both resolve the version through state, so `tsuku remove <dep>`
   fails today with `tool %q is not installed`.

4. `Manager.RecordCleanup` (state_ops.go:79) returns early when the tool has no
   `ActiveVersion` or no `Versions` map. Recording against a dependency that has no state
   entry silently does nothing — it would look wired but change nothing.

5. Which dependencies actually reach `installSingleDependency`:
   - The `tsuku install <tool>` path recurses over `r.Metadata.Dependencies`
     (install_deps.go:342), installing each through the full path with proper state. By
     the time `ExecutePlan` runs, those are already installed and
     `installSingleDependency` short-circuits on the `os.Stat(finalDir)` check.
   - `plan.Dependencies` is built from `actions.ResolveDependenciesForTarget`
     (plan_generator.go:754), which *also* includes action-inherited install-time deps
     that never appear in `Metadata.Dependencies`. Those are installed by
     `installSingleDependency` with no state.
   - Every dependency on the `tsuku install --plan <file>` path
     (`cmd/tsuku/plan_install.go`) is installed by `installSingleDependency`, since that
     path has no recipe recursion at all.

6. `install.HiddenInstallOptions()` (hidden.go:11) already models exactly this shape:
   `CreateSymlinks: false`, `IsHidden: true`. The read side is wired (`IsHidden`,
   `ExposeHidden`, `CheckAndExposeHidden`, and `ListWithOptions`'s hidden filter). The
   write side has **no production caller** — the concept exists but nothing creates one.

7. `Manager.RemoveAllVersions` (remove.go:223) executes every version's `CleanupActions`,
   deletes the version directories, clears state, and rebuilds affected shell caches. A
   dependency with a proper `ToolState` + `VersionState.CleanupActions` would be removed
   correctly with no further change to the remove path.

8. `internal/executor` already imports `internal/install` (plan_conversion.go);
   `internal/install` does not import `internal/executor`. Either direction of helper is
   available without a cycle.

9. A shared post-install helper exists at `cmd/tsuku/post_install.go` (`finishPostInstall`),
   extracted by PR #2465. Issue #2461 is sequenced next and wants the same helper for
   library recipes, so whatever shape this decision produces has to stay reusable.

## Constraints

- Must not tighten `internal/shellenv/selection.go`'s exclusion rule. Its narrowness is
  deliberate: it excludes a fragment only when state records it for an installed-but-
  inactive version, precisely because unrecorded writers like this one exist. A stricter
  rule would silently drop working shell integrations.
- Must not add a fourth divergent copy of the post-install block.
- Must leave the helper reusable for #2461 without implementing #2461.
- The repo has no state.json schema migration story in scope here; whatever is written
  must be readable by, and harmless to, existing consumers.

## Pre-identified options (from the dispatch brief)

- **A** — give the dependency a proper `VersionState` the way the normal install path does
- **B** — attach the dependency's cleanup actions to the parent tool that pulled it in
- **C** — route dependency installs through the normal install path entirely
