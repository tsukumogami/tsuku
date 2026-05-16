# Lead: What is the actual current surface of install state operations?

## Findings

### 1. The Manager surface (state-mutating methods)

The state-mutating surface of `install.Manager` is split across five files in
`internal/install/`. The table below captures every method that, directly or
transitively, ends up writing `state.json` or moving symlinks in `current/`.

| Method | File:line | Signature | Takes `Source`? | Mutations performed |
|---|---|---|---|---|
| `Install` | `manager.go:90` | `Install(name, version, workDir string, src installevents.Source) error` | yes | Delegates to `InstallWithOptions` (sets `opts.Source = src`). |
| `InstallWithOptions` | `manager.go:114` | `InstallWithOptions(name, version, workDir string, opts InstallOptions) (err error)` | yes (via `opts.Source`) | Copy work dir to `tools/{name}/{version}`, create symlinks or wrapper scripts in `current/`, call `state.UpdateTool` to set `ActiveVersion`, snapshot `PreviousVersion`, mark `IsHidden`/`IsExecutionDependency`. Publishes `Installed`/`Updated`/`InstallFailed`/`UpdateFailed` via deferred closure. |
| `Activate` | `manager.go:388` | `Activate(name, version string) error` | **NO** | Validates version string, rebuilds binary symlinks via `createSymlinksForBinaries`, calls `state.UpdateTool` to flip `ActiveVersion` and snapshot `PreviousVersion`. Used both as a public command and internally by `updates/apply.go` as a defensive rollback. **No event is published.** |
| `Rollback` | `manager.go:335` | `Rollback(name, toVersion string, src installevents.Source) error` | yes | Reads prior `ActiveVersion`, calls `Activate(name, toVersion)`, then publishes `RolledBack` or `RollbackFailed`. |
| `Remove` (deprecated) | `remove.go:17` | `Remove(name string, src installevents.Source) error` | yes | Lists tools, removes the active tool directory, removes the single legacy symlink, publishes `Removed`/`RemoveFailed`. **Does not call `state.RemoveTool` itself** — that is done at the caller (see external call sites). |
| `RemoveVersion` | `remove.go:90` | `RemoveVersion(name, version string, src installevents.Source) (err error)` | yes | Validates version, executes per-version cleanup actions, removes the version directory, calls `state.UpdateTool` to drop the version and possibly re-pick `ActiveVersion`, rebuilds symlinks; if last version, calls `removeToolEntirely` -> `state.RemoveTool`. Deferred publish of `Removed`/`RemoveFailed`. |
| `RemoveAllVersions` | `remove.go:194` | `RemoveAllVersions(name string, src installevents.Source) (err error)` | yes | Executes cleanup actions for every version, removes every version directory, calls `removeToolEntirely` -> `state.RemoveTool`. Deferred publish. |
| `InstallLibrary` | `library.go:23` | `InstallLibrary(name, version, workDir string, opts LibraryInstallOptions) error` | **NO** | Copies to `libs/{name}-{version}/`, calls `state.UpdateLibrary` (checksums, `used_by`). **No event is published** (libraries are out of the bus's vocabulary today). |
| `AddLibraryUsedBy` | `library.go:108` | `AddLibraryUsedBy(libName, libVersion, toolNameVersion string) error` | NO | Pure wrapper that delegates to `state.AddLibraryUsedBy`. |
| `ExecuteStaleCleanup` | `update.go:43` | `ExecuteStaleCleanup(staleActions []CleanupAction)` | NO | Filesystem-only side effects (delete files, rebuild shell init caches). No state.json write, no event. |

The `Manager.Remove`-deprecated path is a partial duplicate of
`RemoveVersion`/`RemoveAllVersions` and stays around because
`cmd/tsuku/remove.go:156` (orphan auto-removal) still uses it.

#### Indirect state mutators on `StateManager`

These live in `internal/install/state_tool.go` and `internal/install/state_lib.go`.
None of them know about `installevents.Source`; the event publish is the
caller's responsibility.

| Method | File:line | Purpose |
|---|---|---|
| `UpdateTool` | `state_tool.go:7` | Acquire file lock, read state.json, run user mutation, write atomically. Single read-modify-write entry point. |
| `UpdateToolWithoutLock` | `state_tool.go:89` | Same as `UpdateTool` but caller holds the file lock. **Currently unused outside its definition** — added per `DESIGN-auto-apply-rollback.md` for an apply path that ultimately took a different shape (probe-then-release lock; see `updates/apply.go:109-118`). |
| `RemoveTool` | `state_tool.go:39` | Read-modify-write to delete from `state.Installed[name]`. |
| `AddRequiredBy` / `RemoveRequiredBy` | `state_tool.go:61,73` | Convenience wrappers over `UpdateTool` to mutate the `RequiredBy` slice. |
| `Save` | `state.go:225` | Full state write. **No production callers** — only used in tests and migration code; production paths go through `UpdateTool` / `RemoveTool`. |
| `UpdateLibrary` | `state_lib.go:7` | Library equivalent of `UpdateTool`. |
| `RemoveLibraryVersion` | `state_lib.go:80` | Library equivalent of `RemoveTool`. **No production callers found outside tests.** |
| `AddLibraryUsedBy` / `RemoveLibraryUsedBy` / `SetLibraryChecksums` / `SetLibrarySonames` | `state_lib.go:40,52,59,66` | Library-specific convenience wrappers over `UpdateLibrary`. |
| `RecordGeneration` | `state_llm.go:15` | Writes the `LLMUsage` block. Distinct lifecycle (LLM rate limiting); unrelated to install events. |

#### Hidden-flag manipulation

`internal/install/hidden.go:42` — `ExposeHidden` (called by `bootstrap.go:9`'s
`CheckAndExposeHidden`, which is in turn called from
`cmd/tsuku/install_deps.go:200`) flips `IsHidden=false`, `IsExplicit=true`,
**and creates symlinks**, but does it through `state.UpdateTool` directly, not
through `Manager.Install`. So a tool can be "exposed" without any
`installevents.Installed` event firing — this is intentional (the tool was
already installed), but it means there is a state transition users observe
that isn't on the lifecycle bus.

### 2. The wrapper chain between CLI and Manager

The four wrappers named in the issue all live in `cmd/tsuku/install_deps.go`
and `cmd/tsuku/install_lib.go`. Their depth and purpose:

```
CLI command (install.go / update.go / install_project.go / cmd_apply_updates.go / cmd_run.go / eval.go / create.go)
  -> runInstall                (install_deps.go:167)         constructs reporter, delegates
  -> runInstallWithReporter    (install_deps.go:181)         caller owns reporter lifecycle
  -> installWithDependencies   (install_deps.go:185)         recursive: deps walk, recipe load, plan gen, executor setup, state writes, post-install
       -> installLibrary       (install_lib.go:22)           variant taken when recipe.IsLibrary() == true
       -> mgr.InstallWithOptions (manager.go:114)            terminal state write
       -> mgr.InstallLibrary     (library.go:23)             terminal library state write
```

| Wrapper | File:line | Role | Carries `Source`? |
|---|---|---|---|
| `runInstall` | `install_deps.go:167` | Constructs a `progress.NewTTYReporter`, schedules its `Stop`/`FlushDeferred`, calls `installWithDependencies` with a fresh visited map. | yes |
| `runInstallWithReporter` | `install_deps.go:181` | Same as `runInstall` but the caller owns the reporter (so spinner can be replaced by an outcome line, per issue #2280). | yes |
| `installWithDependencies` | `install_deps.go:185` | The actual install workhorse: hidden-tool short-circuit (line 200), already-installed short-circuit + `mgr.GetState().UpdateTool` to refresh `IsExplicit`/`RequiredBy` (line 223), recipe load and validation (lines 263-299), library branch (line 303), recursive dependency walk for `Metadata.Dependencies` and `Metadata.RuntimeDependencies` (lines 335-370), executor setup, plan generation, plan execution, `mgr.InstallWithOptions` (line 551), post-install phase, post-install state update (line 584), `AddLibraryUsedBy` for library deps (line 630), verification, telemetry. **Roughly 500 lines of orchestration.** | yes (threads `src` to `mgr.InstallWithOptions`, recursive calls, and `installLibrary`) |
| `installLibrary` | `install_lib.go:22` | Library equivalent: recursive dep walk via `installWithDependencies`, executor setup, plan generation, `mgr.InstallLibrary`, sonames extraction, `verifyLibrary`. | yes (threads `src` to the recursive `installWithDependencies` only — `Manager.InstallLibrary` itself doesn't take `Source`) |

Three observations on this chain:

1. `runInstall` and `runInstallWithReporter` exist purely because of progress-
   reporter lifecycle ownership. They add no install logic.
2. `installWithDependencies` is the layer that owns the recursive dependency
   walk and threads `src` into both the recursive call and the terminal
   `mgr.InstallWithOptions` call. Three `mgr.GetState().UpdateTool` calls
   happen here directly, **outside** the Manager's published events
   (lines 223, 475, 584) — they update bookkeeping (`IsExplicit`,
   `RequiredBy`, `InstallDependencies`, `RuntimeDependencies`, cleanup
   actions) after the Manager has already published its install event.
3. `installLibrary` is the only place `installWithDependencies` is **not**
   the parent — it is invoked from inside `installWithDependencies` itself
   when the recipe is a library. The recursive walk inside `installLibrary`
   loops back into `installWithDependencies` for the dep recipes.

### 3. External callers (non-test, outside `internal/install/`)

Re-checking the grep-confirmed list from the scope; two of the listed files
do not call Manager lifecycle methods at all in production code. The actual
external state-mutation surface is:

| File:line | Verb | Method called | What it does |
|---|---|---|---|
| `cmd/tsuku/install_deps.go:551` | install | `mgr.InstallWithOptions(toolName, version, exec.WorkDir(), installOpts)` | Terminal install for the standard pipeline. `installOpts.Source = src` is set on line 550 immediately before the call. |
| `cmd/tsuku/install_deps.go:223` | install (bookkeeping) | `mgr.GetState().UpdateTool(toolName, ...)` | "Already installed" branch: refresh `IsExplicit`, `RequiredBy` without an install event. |
| `cmd/tsuku/install_deps.go:475` | install (bookkeeping) | `mgr.GetState().UpdateTool(toolName, ...)` | "Resolved version already installed" branch: same purpose. |
| `cmd/tsuku/install_deps.go:584` | install (post-install bookkeeping) | `mgr.GetState().UpdateTool(toolName, ...)` | Records `InstallDependencies`, `RuntimeDependencies`, and `CleanupActions` after `InstallWithOptions` already committed and published. |
| `cmd/tsuku/install_deps.go:630` | install (library link) | `mgr.AddLibraryUsedBy(dep, libVersion, toolNameVersion)` | Records reverse-edge from library to consumer tool. |
| `cmd/tsuku/plan_install.go:107` | install (from external plan) | `mgr.InstallWithOptions(...)` | Uses `DefaultInstallOptions()` — `Source` defaults to `SourceManual`. Bypasses `runInstall`/`installWithDependencies`. |
| `cmd/tsuku/plan_install.go:137` | install (bookkeeping) | `mgr.GetState().UpdateTool(...)` | Sets `IsExplicit = true` after the plan-based install. |
| `cmd/tsuku/install_distributed.go:219` | install (post-install bookkeeping) | `mgr.GetState().UpdateTool(toolName, ...)` | Records `Source` and `RecipeHash` after a distributed install; runs from `cmd/tsuku/install.go:283`. |
| `cmd/tsuku/remove.go:84` | remove | `mgr.RemoveVersion(toolName, targetVersion, installevents.SourceManual)` | Specific-version remove. |
| `cmd/tsuku/remove.go:94` | remove | `mgr.RemoveAllVersions(toolName, installevents.SourceManual)` | Tool-wide remove. |
| `cmd/tsuku/remove.go:117,170` | remove (orphan cleanup) | `mgr.GetState().RemoveRequiredBy(dep, toolName)` | Drops reverse-edge from a tool to its deps; runs after each removal. |
| `cmd/tsuku/remove.go:156` | remove (orphan auto) | `mgr.Remove(toolName, installevents.SourceManual)` | Recursive orphan dependency cleanup (deprecated `Remove`). |
| `cmd/tsuku/remove.go:163` | remove (orphan state cleanup) | `mgr.GetState().RemoveTool(toolName)` | **Direct state mutation** after the deprecated `Remove`, because `Remove` itself doesn't call `RemoveTool`. This is the only direct `RemoveTool` call outside `internal/install/`. |
| `cmd/tsuku/cmd_rollback.go:69` | rollback | `mgr.Rollback(toolName, ts.PreviousVersion, installevents.SourceManual)` | Public rollback verb. |
| `cmd/tsuku/activate.go:46` | activate | `mgr.Activate(toolName, version)` | Public activate verb. No `Source` parameter. |
| `cmd/tsuku/install_lib.go:160` | install library (post-install bookkeeping) | `mgr.GetState().SetLibrarySonames(libName, version, sonames)` | Records auto-discovered sonames after the library install. |
| `cmd/tsuku/install_deps.go:744` (via `mgr.AddLibraryUsedBy` at line 630) | install library link | as above |
| `internal/updates/apply.go:167` | auto-apply (defensive rollback) | `mgr.Activate(entry.Tool, previousVersion)` | After a failed auto-update, falls back to the prior active version's symlinks. **No event published** — comment at lines 162-165 documents this is intentional. |
| `internal/updates/self.go:161,177` | self-update | `bus.Publish(installevents.Updated{...})` / `Publish(UpdateFailed{...})` | The tsuku binary self-update path publishes directly on the bus (no Manager involvement). The "tool" for these events is `SelfToolName = "tsuku"`. |

Two items the scope listed as external callers are **not** state-mutating call
sites in production code:

- `cmd/tsuku/cmd_shim.go:85` — the `mgr.Install(recipeName)` here is on a
  `shim.Manager` (constructed at line 52: `mgr := shim.NewManager(cfg, loader)`),
  not `install.Manager`.
- `cmd/tsuku/cmd_hook.go` — no `install` package imports or Manager calls at all.
- `internal/index/rebuild.go` — reads installed state via a `StateReader`
  interface but does not mutate. It is not a state-mutation caller.
- `internal/autoinstall/run.go` — calls `r.Installer.Install(ctx, ...)`, where
  `Installer` is an interface (`autoinstall.go:69`). The implementation lives
  at `cmd/tsuku/cmd_run.go:152` (`runInstaller.Install`) and delegates to
  `runInstall(... SourceProjectAuto)`. So autoinstall is a CLI-side caller of
  the wrapper chain, not a direct caller of Manager.

### 4. Entry-point count and longest call-chain length

There are nine entry points in the foreground binary that ultimately reach a
state-mutating Manager method. The longest chain is six layers deep
(CLI -> `runInstall` -> `installWithDependencies` -> `installWithDependencies`
recursive dep walk -> `installLibrary` -> `Manager.InstallLibrary`).

| Entry point | First-call file:line | Chain length | Chain (top to terminal) |
|---|---|---|---|
| `tsuku install <tool>` | `install.go:344` | 4 | `runE` -> `runInstall` -> `installWithDependencies` -> `mgr.InstallWithOptions` |
| `tsuku install <tool>` with deps | `install.go:344` | 5+ (recursion depth) | same chain, with `installWithDependencies` recursing into itself for each dep |
| `tsuku install <tool>` library branch | `install_deps.go:303` | 6 | `runE` -> `runInstall` -> `installWithDependencies` -> dep recursion into `installWithDependencies` -> `installLibrary` -> `mgr.InstallLibrary` |
| `tsuku install --plan <file>` | `install.go:155` -> `runPlanBasedInstall` | 3 | `runE` -> `runPlanBasedInstall` -> `mgr.InstallWithOptions` |
| `tsuku update <tool>` / `--all` | `update.go:136,329` | 4 | `runE` -> `runInstallWithReporter` -> `installWithDependencies` -> `mgr.InstallWithOptions` |
| `tsuku rollback <tool>` | `cmd_rollback.go:69` | 2 | `runE` -> `mgr.Rollback` |
| `tsuku activate <tool> <version>` | `activate.go:46` | 2 | `runE` -> `mgr.Activate` |
| `tsuku remove <tool>[@v]` | `remove.go:84,94` | 2 | `runE` -> `mgr.RemoveVersion` or `mgr.RemoveAllVersions` |
| `tsuku remove` orphan recursion | `remove.go:156` | 3 | `runE` -> `cleanupOrphans` -> `mgr.Remove` |
| `tsuku run <cmd>` (auto-install) | `cmd_run.go:154` via `runInstaller.Install` | 5 | `runE` -> `Runner.Run` -> `runInstaller.Install` -> `runInstall` -> `installWithDependencies` -> `mgr.InstallWithOptions` |
| `tsuku install` (project, no-args) | `install_project.go:230,250` | 4 | `runE` -> `runInstall` -> `installWithDependencies` -> `mgr.InstallWithOptions` |
| `tsuku apply-updates` (hidden subprocess) | `cmd_apply_updates.go:45` | 4 | `runE` -> `runInstallWithReporter` -> `installWithDependencies` -> `mgr.InstallWithOptions` |
| `tsuku check-updates` (hidden subprocess, self-update branch) | `cmd_check_updates.go:64` | 3 | `runE` -> `RunUpdateCheck` -> `CheckAndApplySelf` -> `bus.Publish` (bypasses Manager entirely) |
| `tsuku self-update` (manual) | `cmd_self_update.go` | n/a | Does not touch `install.Manager`; replaces the tsuku binary in place. |
| `tsuku create --from <eco>` (auto-install) | `create.go:411` | 4 | as standard install |
| `tsuku eval --install` | `eval.go:359` via `runInstallTool` | 5 | as standard install |

`Source` propagation summary across the entry-point set: every wrapper
call site explicitly passes a literal `installevents.SourceManual`,
`SourceAuto`, or `SourceProjectAuto`. Today's mapping:

- `SourceManual`: `tsuku install`, `tsuku update`, `tsuku rollback`, `tsuku remove`, `tsuku create --from`, `tsuku eval --install`, `tsuku install --plan`.
- `SourceAuto`: `tsuku apply-updates` subprocess; the tool-update part of `RunUpdateCheck`.
- `SourceProjectAuto`: `tsuku run` autoinstall (from `cmd_run.go:154`).

## Implications

1. **The actual call surface is small but the wrappers are heavy.** Only
   eight non-test files outside `internal/install/` call Manager lifecycle
   methods, but `installWithDependencies` (1 function, ~500 lines) does
   recursive dependency walking inside itself, so a single CLI verb can
   produce dozens of Manager calls and three or four direct
   `state.UpdateTool` bookkeeping calls per tool installed.

2. **`Source` threading is uniform across CLI -> wrapper chain.** Every
   wrapper takes `installevents.Source` as a trailing parameter; every CLI
   call site passes a literal constant. The threading was mechanical
   (touch every wrapper signature once) rather than semantically rich (the
   wrappers don't choose between sources, they just pass it through).

3. **The Manager surface is not uniform with respect to events.**
   `Activate`, `InstallLibrary`, `ExposeHidden`, and `ExecuteStaleCleanup`
   are state-mutating but emit no event. `Install`, `Rollback`, `Remove*`
   do. The split is real (rollback's internal `Activate` call shouldn't
   double-emit) but it means "every state mutation publishes an event"
   is already not true.

4. **The wrapper chain is roughly two layers deep on top of Manager** for
   most verbs (CLI -> reporter wrapper -> dep walker -> Manager). The
   library branch adds one extra layer; the orphan-cleanup branch in
   `remove.go` adds one. Recursion through `installWithDependencies` for
   transitive deps multiplies call-site work but doesn't add depth.

5. **There are three `mgr.GetState().UpdateTool` bookkeeping calls inside
   `installWithDependencies` that happen outside the Manager's event
   contract.** They update `IsExplicit`, `RequiredBy`, `InstallDependencies`,
   `RuntimeDependencies`, and `CleanupActions` after the Manager has already
   committed and published. Today this is harmless (the bookkeeping fields
   are not on the event vocabulary), but any future "every state mutation
   produces an event" rule would have to reconcile these.

## Surprises

1. **`UpdateToolWithoutLock` has no production callers.** It was added in
   `DESIGN-auto-apply-rollback.md` to support the auto-apply path holding
   a single file lock across the apply cycle, but the actual implementation
   in `internal/updates/apply.go:108-118` uses a probe-then-release pattern
   and lets the install pipeline acquire its own locks. The dead method
   stays around as a documented escape hatch.

2. **`Manager.Remove` doesn't fully remove.** The deprecated `Manager.Remove`
   method removes the tool directory and symlink but does NOT remove the
   tool entry from `state.json` — the caller at `cmd/tsuku/remove.go:163`
   does `mgr.GetState().RemoveTool(toolName)` explicitly. This is the only
   place in the codebase outside `internal/install/` where `RemoveTool` is
   called directly, and it exists because `Manager.Remove` is intentionally
   incomplete (legacy single-version contract).

3. **`Manager.Activate` does not take `Source`.** It's both a public verb
   (`tsuku activate`) and an internal helper (called by `Manager.Rollback`
   and by `updates/apply.go` for defensive rollback). The Rollback callers
   carry `Source` themselves; the apply.go caller intentionally skips an
   event. Activate's `state.UpdateTool` at `manager.go:432` is the one path
   where state changes without any event flowing through the bus.

4. **`Manager.InstallLibrary` doesn't take `Source` either.** The library
   install path is wholly outside the install event vocabulary. The scope
   document notes libraries are out of scope but they share
   `internal/install/state_lib.go` plumbing with tools, and they are
   reachable through the same wrapper chain (`installWithDependencies`
   recurses into `installLibrary` when the recipe is a library).

5. **The "external callers" list in the scope is partially wrong.**
   `cmd_shim.go:85` uses `shim.Manager`, not `install.Manager`.
   `cmd_hook.go` doesn't call Manager. `internal/index/rebuild.go` reads
   state but doesn't mutate. `internal/autoinstall/run.go` calls an
   `Installer` interface whose only implementation
   (`cmd/tsuku/cmd_run.go:152`) delegates to `runInstall`. The actual
   external state-mutation footprint is smaller than the scope suggests:
   five files in `cmd/tsuku/` plus one in `internal/updates/`.

6. **Three of the "bookkeeping" `UpdateTool` calls happen in the same
   function** (`installWithDependencies` at install_deps.go:223, :475, :584).
   They handle three semantically different cases (already-installed
   refresh; resolved-version-already-installed refresh; post-install
   metadata recording), but they share the pattern "we just need to set
   `IsExplicit`/`RequiredBy` without publishing". This concentration
   matters for any refactor that wants the wrapper to be slimmer.

7. **CLI-level `Source` is a literal constant in every call site.** No
   call site computes `Source` from runtime state; every site picks one
   of three constants at compile time. This argues that `Source` is more
   like a process-level attribute than a per-call argument.

## Open Questions

1. The scope's PR #2412 "file count" claim cannot be verified from this
   lead alone — that's lead 5's cost-model work. The data this lead produced
   shows the wrapper chain has four signatures that needed editing
   (`runInstall`, `runInstallWithReporter`, `installWithDependencies`,
   `installLibrary`) plus six Manager method signatures (`Install`,
   `InstallWithOptions` via `InstallOptions.Source`, `Rollback`, `Remove`,
   `RemoveVersion`, `RemoveAllVersions`). That's ten signature edits before
   counting non-test call sites. Lead 5 should validate this against
   the actual PR diff.

2. Is the `Activate`-without-event asymmetry deliberate forever, or was it
   the pragmatic compromise from DESIGN-notices-install-event-bus's
   Decision 3 (rejecting the state.UpdateTool shim)? If the latter, any
   future restructure has to decide whether `Activate` joins the event
   vocabulary or stays the explicit exception.

3. `cmd_run.go`'s `runInstaller` is a one-method type that exists purely
   to satisfy the `autoinstall.Installer` interface. Is this interface
   pattern (already in the codebase) a hint about the shape that would
   reduce the wrapper chain, or is it specific to the autoinstall security
   gating?

4. `UpdateToolWithoutLock` is dead code; removing it is unrelated to this
   exploration but worth flagging for a separate cleanup.

5. The library install path's lack of events isn't reflected in the
   `installevents` vocabulary at all. If a future restructure wants every
   state-mutating method to flow through the abstraction, libraries either
   need a parallel set of events or have to be treated as a special case
   from day one.

## Summary

The state-mutation surface of `install.Manager` is six lifecycle methods
(`Install`, `InstallWithOptions`, `Rollback`, `Remove`, `RemoveVersion`,
`RemoveAllVersions`) plus three asymmetric ones (`Activate`,
`InstallLibrary`, `ExposeHidden`) that mutate state without taking
`Source` or publishing an event, called from roughly five non-test files
in `cmd/tsuku/` plus one in `internal/updates/`; the CLI-to-Manager
distance is two to six layers deep, with all the meaty work concentrated
in a single ~500-line `installWithDependencies` function that recursively
walks dependencies and makes both Manager calls and three direct
`state.UpdateTool` bookkeeping calls per install. The main implication is
that the wrapper chain is wider than it is deep: ten function signatures
in the CLI -> Manager path had to learn about `Source`, but each one
just passes the value through (every call site uses a literal constant),
which argues `Source` behaves more like a process-level attribute than a
per-call argument. The biggest open question is whether the existing
asymmetries (`Activate` and `InstallLibrary` not on the event bus,
three `UpdateTool` bookkeeping calls happening outside Manager's event
contract) are intentional invariants that any abstraction must preserve
or accidents of the current design that a restructure should heal.
