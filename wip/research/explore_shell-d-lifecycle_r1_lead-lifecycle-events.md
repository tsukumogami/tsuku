# Lead: What is the complete set of lifecycle events that change which version of a tool is active, and what does each do today with respect to `$TSUKU_HOME/share/shell.d`?

All line numbers are against this worktree's HEAD (`8a7c8908`, based on `origin/main` at
`4d470df9`).

## Findings

### 1. What "active version" concretely is

There is exactly one authoritative record and exactly three places in the whole tree that
write it.

`ToolState.ActiveVersion` (`internal/install/state.go:88`) is the field. It is a key into
`ToolState.Versions map[string]VersionState` (`internal/install/state.go:90`). The legacy
`ToolState.Version` (`internal/install/state.go:93`) is a mirror kept for migration;
`State.migrateToMultiVersion` (`internal/install/state_tool.go:199-218`) promotes a legacy
`Version` into `ActiveVersion` plus a one-entry `Versions` map on every load.

`grep -rn "ActiveVersion = "` over non-test Go finds three writers:

| Site | Context |
|---|---|
| `internal/install/manager.go:260` | `InstallWithOptions` — every successful install sets the installed version active |
| `internal/install/manager.go:463` | `Activate` |
| `internal/install/remove.go:179` | `RemoveVersion` — implicit promotion when the removed version was active |

`PreviousVersion` (`internal/install/state.go:108`) is snapshotted at `manager.go:246-248`
(install) and `manager.go:460-462` (activate). It is the rollback target and is one level
deep.

**`tools/current/` is not a per-tool version pointer.** `cfg.CurrentDir` is
`$TSUKU_HOME/tools/current` (`internal/config/config.go:322,356`) and
`cfg.CurrentSymlink(name)` is `tools/current/<binaryName>`
(`internal/config/config.go:415-418`). Each entry is one **binary**, not one tool, and it is
either a symlink to `tools/<tool>-<version>/<binaryPath>`
(`internal/install/manager.go:498-519`) or, when the tool has runtime dependencies, a
generated `#!/bin/sh` wrapper script written in place
(`internal/install/manager.go:559-628`). There is no `tools/<tool>/current` indirection
level anywhere.

This matters for the exploration: **the binary layer has a version-independent name pointing
at version-specific content, and it is re-pointed on every active-version change.**
`shell.d` has the identical shape — version-independent filename, version-specific content —
but is re-pointed on only a subset of those changes. shell.d is missing the equivalent of
`createSymlinksForBinaries`.

### 2. Who writes shell.d, and who rebuilds the cache

**Writers of `share/shell.d/*` on `origin/main` today: exactly one action.**
`InstallShellInitAction` (`internal/actions/shell_init.go:82-133`) writes
`share/shell.d/{target}.{shell}` for each configured shell, either by copying a file from
the install dir (`shell_init.go:153-179`) or by running a command and capturing stdout
(`shell_init.go:182-236`). Both paths hash the written bytes (`shell_init.go:136-139`) and
call `recordCleanup` (`shell_init.go:141-150`), which appends a `CleanupAction{Action:
"delete_file", Path: "share/shell.d/<target>.<shell>", ContentHash: hash}` onto
`ctx.CleanupActions`.

`install_completions` (`internal/actions/completions.go`) has the exact same shape for
`share/completions/{shell}/{target}` — version-independent path, version-specific content,
`ContentHash` recorded at `completions.go:106-115`. **Every finding below about shell.d
applies verbatim to `share/completions/`.** The only asymmetry is that
`shellFromCleanupPath` (`internal/install/remove.go:386-395`) returns `""` for a
`share/completions/...` path, so completions never trigger a cache rebuild — there is no
completions cache to rebuild.

**`shellenv.RebuildShellCache`** (`internal/shellenv/cache.go:29-169`) concatenates every
`*.{shell}` in `share/shell.d/` alphabetically, wraps each in a brace group, and atomically
writes `.init-cache.{shell}`. It never re-derives file *contents* — it is a concatenator,
not a renderer. Given an optional `contentHashes` map it *excludes* files whose on-disk
content does not match the stored hash (`cache.go:119-131`), printing a warning to stderr.

Exactly five call sites:

| Call site | Passes hashes? |
|---|---|
| `cmd/tsuku/install_deps.go:595` (after post-install phase) | no |
| `cmd/tsuku/plan_install.go:134` (after post-install phase) | no |
| `internal/install/remove.go:400` via `Manager.rebuildShellCaches` | no |
| `internal/install/update.go:58` via `Manager.ExecuteStaleCleanup` | no |
| `cmd/tsuku/doctor.go:66` (`doctor --fix`) | **yes** |

### 3. `Manager.Activate` and `Manager.Rollback`

`Manager.Activate` (`internal/install/manager.go:411-470`) does, in order: validate the
version string, load state, check the version exists in `Versions`, early-return if already
active (`manager.go:437-439`), stat the tool directory, `createSymlinksForBinaries`
(`manager.go:454`), and `UpdateTool` to set `PreviousVersion`/`ActiveVersion`
(`manager.go:459-464`).

**It does not read, write, delete, or rebuild anything under `share/shell.d`. It does not
look at `CleanupActions` at all.** The `install/shellenv` import that `remove.go` and
`update.go` carry is absent from `manager.go` entirely — there is no import of
`internal/shellenv` in that file.

`Manager.Rollback` (`internal/install/manager.go:353-387`) is a thin wrapper: it snapshots
`fromVersion`, calls `m.Activate(ctx, name, toVersion)` (`manager.go:364`), and publishes
`RolledBack` / `RollbackFailed`. **All of its state effect is `Activate`'s, so it inherits
the same gap exactly.**

### 4. `Manager.executeCleanupActions` and the multi-version skip

`Manager.executeCleanupActions` (`internal/install/remove.go:326-356`) is the *only* code
that acts on recorded `CleanupActions` during a remove. Its mechanism:

1. Build `otherPaths` — the set of every cleanup path recorded by **any other version of the
   same tool** (`remove.go:332-341`).
2. For each action of the version being removed: if its path is in `otherPaths`, print
   `"Cleanup: skipping <path> (still referenced by another version)"` and `continue`
   (`remove.go:345-349`). Otherwise run `executeSingleCleanup` and add the derived shell to
   `affectedShells`.
3. Return `affectedShells` for `rebuildShellCaches` (`remove.go:161`, `remove.go:398-404`).

**The skip is the mechanism that produces the reported bug.** Because
`recordCleanup` derives the path from `target` and `shell` only — never from the version —
two versions of the same tool always record *the same path*. So on `remove <tool>@<v>` with
two versions installed, the skip always fires, the file is never touched, and because the
`continue` happens before the `affectedShells[shell] = true` line, **the cache is not
rebuilt either**. The file and the cache both retain the removed version's content.

Note the direction is independent of which version was active. `remove` of the *non-active*
version skipping the delete is correct (the active version's file should survive). `remove`
of the *active* version skipping the delete is the bug — the file survives, but so does the
wrong content.

`executeSingleCleanup` (`remove.go:360-377`) only understands `delete_file` and
`delete_dir`. **There is no "rewrite" or "re-render" action verb in the vocabulary.**

`RemoveAllVersions` (`remove.go:213-258`) skips the cross-version check entirely
(`remove.go:235-243`, "no cross-version safety check is needed — everything goes") and
rebuilds affected caches at `remove.go:254`. That path is correct.

### 5. The event table

| # | Event | Entry point | Changes active version? | Re-renders shell.d today? | Broken? |
|---|---|---|---|---|---|
| 1 | `install` (fresh, no prior version) | `cmd/tsuku/install_deps.go:184` → `Manager.InstallWithOptions` (`manager.go:260`) | yes (`""` → v) | yes — post-install re-runs `install_shell_init` (`install_deps.go:579`), cache rebuilt (`install_deps.go:595`) | no |
| 2 | `install` of a second/newer version | same | yes (v1 → v2) | yes — file overwritten by post-install, cache rebuilt | no, but leaves v1's `CleanupActions` in state holding v1's now-wrong `ContentHash` for the same path (see #6) |
| 3 | `install <tool>@<already-installed-version>` | `cmd/tsuku/install_deps.go:509-516` | **no — short-circuits** | no | **yes, differently**: with v2 active, `tsuku install nvm@0.40.5` prints "already installed" and returns. It does not activate v1. The active version is silently not what the user asked for |
| 4 | `update <tool>` | `cmd/tsuku/update.go:136` → same install path | yes | yes — post-install rewrite + `install_deps.go:595`, plus `ExecuteStaleCleanup` (`update.go:183`, `internal/install/update.go:43-62`) for paths the new version dropped, plus `warnShellInitChanges` (`update.go:188,232-258`) | no |
| 5 | `update --all` | `cmd/tsuku/update.go:267-410` | yes | post-install rewrite + `install_deps.go:595` only | **partially** — `runUpdateAll` never calls `StaleCleanupActions` / `ExecuteStaleCleanup` / `warnShellInitChanges`. A tool that dropped a shell between versions leaks the old file under `--all` but not under single-tool update |
| 6 | `activate <tool> <version>` | `cmd/tsuku/activate.go:49` → `Manager.Activate` (`manager.go:411`) | **yes** | **no — zero shell.d code** | **yes, primary** |
| 7 | `rollback <tool>` | `cmd/tsuku/cmd_rollback.go:70` → `Manager.Rollback` (`manager.go:353`) → `Activate` | **yes** | **no** | **yes, primary** |
| 8 | `remove <tool>@<v>` with other versions remaining | `cmd/tsuku/remove.go:86` → `Manager.RemoveVersion` (`remove.go:102`) | **yes, implicitly** — `getMostRecentVersion` promotes the newest remaining by `InstalledAt` (`remove.go:174-186`, `remove.go:301-320`) | **no** — the shared path is skipped at `remove.go:345-349` before `affectedShells` is populated | **yes, primary — this is the reported reproduction** |
| 9 | `remove <tool>` (all versions) | `cmd/tsuku/remove.go:96` → `RemoveAllVersions` (`remove.go:213`) | yes (→ gone) | yes — no skip, cache rebuilt (`remove.go:254`) | no |
| 10 | orphan auto-removal after `remove` | `cmd/tsuku/remove.go:136-180` → `RemoveAllVersions` (`remove.go:160`) | yes (→ gone) | yes, same as #9 | no |
| 11 | `Manager.Remove` (deprecated) | `internal/install/remove.go:21-68` | leaves `ActiveVersion` **pointing at a deleted directory** — it deletes the dir and one symlink and never touches state | no | yes, but no non-test caller — dead code |
| 12 | background auto-apply, success | `internal/updates/apply.go:133` → `applyUpdate` → injected `installFn` (`cmd/tsuku/cmd_apply_updates.go:45-54`) → `runInstallWithReporter` | yes | post-install rewrite + `install_deps.go:595` | **partially** — like #5, no stale-cleanup pass. Also no `warnShellInitChanges`, so a silently changed init script lands with no user-visible signal |
| 13 | background auto-apply, **failure rollback** | `internal/updates/apply.go:173` → `mgr.Activate(previousVersion)` | **yes** | **no** | **yes** — and this one is silent by construction: `apply-updates` redirects stdout and stderr to `/dev/null` (`cmd_apply_updates.go:21-27`) |
| 14 | version GC after auto-apply | `internal/updates/apply.go:153` → `GarbageCollectVersions` (`internal/updates/gc.go:15-69`) | no — active and previous dirs are protected (`gc.go:38-46`) | no | **yes, adjacent** — it `os.RemoveAll`s tool directories without touching `state.json` at all. The GC'd version's `VersionState` (including its `CleanupActions` and `ContentHash`) survives in state and keeps feeding the `otherPaths` skip in `executeCleanupActions` for a directory that no longer exists |
| 15 | `tsuku run` autoinstall of a project-pinned version | `internal/autoinstall/run.go:192` → `r.Installer.Install` → `runInstall` | **yes, as a side effect** — `InstallWithOptions` unconditionally sets `ActiveVersion` (`manager.go:260`); there is no project-scoped install mode | yes — post-install rewrite + `install_deps.go:595` | no for shell.d, but the *global* active version and *global* shell.d are silently flipped by entering a project directory and running a command |
| 16 | `tsuku install` with no args in a project dir (`.tsuku.toml`) | `cmd/tsuku/install_project.go:230,256` → `runInstall` | yes, per tool | yes | no |
| 17 | `tsuku install --plan <file>` | `cmd/tsuku/plan_install.go:111` → `InstallWithOptions` | yes | rebuilds the cache (`plan_install.go:134`) | **yes, differently** — `plan_install.go` **never calls `mgr.RecordCleanup`**. `grep -rn RecordCleanup` finds exactly one non-test caller, `install_deps.go:613`. Plan installs write shell.d files that have no `CleanupAction` and no `ContentHash` in state: they are never deleted on remove and are invisible to `doctor`'s hash check |
| 18 | `tsuku eval <tool>` installing eval-time deps | `cmd/tsuku/eval.go:350-361` → `runInstall` | yes | yes | no |
| 19 | `ExposeHidden` | `internal/install/hidden.go:23-50`, called from `install_deps.go:226` | no — sets `IsHidden=false`, creates symlinks from the **legacy** `toolState.Version` field (`hidden.go:41`), never writes `ActiveVersion` | no | latent: reads `Version`, not `ActiveVersion`. Correct only while migration keeps them in sync |
| 20 | `self-update` | `cmd/tsuku/cmd_self_update.go` → `updates.ApplySelfUpdate` | no — replaces the `tsuku` binary only | no | no |
| 21 | `tsuku cleanup` | `cmd/tsuku/cache_cleanup.go:41` | no — registry recipe cache only | no | no |
| 22 | project PATH activation (`hook-env`, `shell`) | `internal/shellenv/activate.go:42-119` via `cmd/tsuku/hook_env.go:36`, `cmd/tsuku/shell.go:79` | **no** — prepends `cfg.ToolBinDir(name, version)` to `PATH` and sets `_TSUKU_DIR`/`_TSUKU_PREV_PATH`; never reads or writes `state.json` | no | see #7 below — this is a design divergence, not a bug in itself |
| 23 | `doctor --fix` | `cmd/tsuku/doctor.go:46-80` | no | rebuilds the cache **only when `CacheStale`** (`doctor.go:64-73`), passing hashes | **yes** — see #8 |

### 6. The `ContentHash` in state is per-version, and goes stale on install

`recordCleanup` stamps the hash of what was written *at that moment*
(`shell_init.go:173,175`). `Manager.RecordCleanup` (`internal/install/state_ops.go:79-94`)
attaches the actions to `ts.Versions[ts.ActiveVersion]` — only the active version's entry.

So after `install v1; install v2`:

- `Versions["0.40.5"].CleanupActions` = `[{delete_file, share/shell.d/nvm.bash, hashA}, ...]`
- `Versions["0.40.6"].CleanupActions` = `[{delete_file, share/shell.d/nvm.bash, hashB}, ...]`
- disk holds hashB, `ActiveVersion` is `0.40.6`

Two versions claim the same path with different hashes. Nothing reconciles them. This is
what makes the record simultaneously (a) the trigger for the `otherPaths` skip and (b) the
detector for the resulting corruption.

`doctor` builds its expected-hash map from **the active version of each tool only**
(`cmd/tsuku/doctor.go:198-215`). That is why the corruption is detectable: after the remove
promotes `0.40.5`, doctor expects hashA and finds hashB.

### 7. shell.d is strictly global; project activation is PATH-only

`internal/shellenv/activate.go` never mentions `shell.d` — its entire output is a new `PATH`
plus two tracking variables (`activate.go:104-118`, `FormatExports` at
`activate.go:123-156`). `internal/project/config.go` is read-only config loading.

So per-directory activation swaps which *binaries* resolve, but the shell.d init scripts and
the `00-env-*` exports that a future `set_env` would write remain whatever the last global
mutation left them as. A project pinning `nvm@0.40.5` while `0.40.6` is globally active gets
`0.40.6`'s `nvm.sh` in its shell.

The one bridge between the two worlds is #15: `tsuku run` autoinstalling a project-pinned
version calls the ordinary install path, which sets the global `ActiveVersion` and rewrites
global shell.d.

### 8. There is no repair path, and `doctor --fix` makes it worse

`shellenv.CheckShellD` (`internal/shellenv/doctor.go:52-142`) detects hash mismatches
(`doctor.go:98-110`) and reports them. `doctor` prints
`"<name>: content hash mismatch"` (`cmd/tsuku/doctor.go:243-245`) and sets `failed = true`.

**No `--fix` branch handles `HashMismatches`.** `doctor.go:63-73` only rebuilds when
`CacheStale[shell]` is true.

Worse, when a rebuild *does* run, it passes `contentHashes`
(`doctor.go:66`), and `RebuildShellCache` responds to a mismatch by **excluding the file
from the cache** (`internal/shellenv/cache.go:122-129`). The repair for "this file has the
wrong version's content" is "silently drop the tool's shell integration."

And that leaves the system wedged: `isCacheStale` (`internal/shellenv/doctor.go:146-174`)
recomputes the expected cache **without any hash filtering** — it concatenates every file it
finds. So once `--fix` has written a cache that omits the mismatched file, `isCacheStale`
compares against a concatenation that includes it, returns `true` forever, and every
subsequent `doctor` reports a stale cache that `--fix` cannot clear.

### 9. A second shell.d writer is in flight and not on main

The reproduction in the brief names `00-env-nvm.bash`. **That file does not exist on
`origin/main`.** `internal/actions/set_env.go:46-64` on HEAD writes
`{install_dir}/env.sh`, a file `grep` shows nothing ever reads.

`00-env-{tool}.{shell}` is introduced by commit `60683917` ("fix(actions): make set_env
actually export variables", closes #2439) on the unmerged branch
`origin/fix/2439-set-env-exports`. `git merge-base --is-ancestor 60683917 origin/main`
returns false. On that branch, `SetEnvAction.Execute` writes
`share/shell.d/00-env-{recipe.Metadata.Name}.{bash,zsh}`, moves itself to the post-install
phase via a new `DefaultPhase()`, and records cleanup with a content hash — the same shape
as `install_shell_init`, with one extra wrinkle: multiple `set_env` steps in one recipe
*append* to the file within a single install (`upsertCleanup` / `cleanupRecorded`), so the
file's content is accumulated across steps rather than written once.

The reproduction was therefore run against that branch, not main. The finding still holds on
main — the brief's own note that "`install_shell_init`'s `nvm.bash` is left holding 0.40.6's
whole script" reproduces on main with `recipes/n/nvm.toml`'s `install_shell_init` step alone
(`target = "nvm"`, `shells = ["bash", "zsh"]`).

## Implications

**The re-render hook has to fire on five events, not two.** The brief names `Activate` and
`Rollback`; `Rollback` is not independent (it is a wrapper around `Activate`, `manager.go:364`),
so the real list is:

1. `Manager.Activate` — covers `tsuku activate`, `tsuku rollback`, and auto-apply's
   defensive rollback (`updates/apply.go:173`). One insertion point, three user-visible paths.
2. `Manager.RemoveVersion`'s implicit promotion (`remove.go:174-205`) — the skip at
   `remove.go:345-349` is correct about *not deleting* and wrong about *not re-rendering*.
3. `runUpdateAll` (`cmd/tsuku/update.go:267`) — missing the stale-cleanup pass that the
   single-tool path has.
4. Background auto-apply (`cmd_apply_updates.go`) — same missing pass, and silent.
5. `plan_install.go` — missing `RecordCleanup` entirely, so its files are outside the
   bookkeeping the whole scheme depends on.

**Re-rendering is not the same operation as rebuilding the cache.** `RebuildShellCache`
concatenates whatever is on disk; it cannot produce `0.40.5`'s `nvm.sh` when disk holds
`0.40.6`'s. The content originates from either a file inside the tool directory
(`shell_init.go:154`, `copyFile`) or the *output of running a binary from the tool
directory* (`shell_init.go:203-207`). Re-rendering after an `Activate` therefore means
re-executing a post-install action against the newly-active tool directory — which for
`source_command` recipes means running a subprocess during `tsuku activate` and during
`tsuku rollback`. That is a materially larger change than "call RebuildShellCache in one
more place," and any design that assumes otherwise will only fix the `source_file` half.

**The `CleanupAction` vocabulary has no verb for this.** `executeSingleCleanup`
(`remove.go:360-377`) understands `delete_file` and `delete_dir`. Re-rendering needs either
a third verb, or a different record entirely — the recorded step (`install_shell_init` with
its params) rather than the recorded effect (a path to delete).

**The binary layer already solved this problem.** `createSymlinksForBinaries` is called from
both `InstallWithOptions` (`manager.go:212`) and `Activate` (`manager.go:454`) and from
`RemoveVersion`'s promotion (`remove.go:202`). Those are exactly the three
`ActiveVersion`-writing sites. A shell.d equivalent placed alongside each of those three
calls would be structurally symmetric and provably exhaustive against the
`grep "ActiveVersion = "` list.

**`share/completions/` is the same bug with no detector.** Same version-independent path,
same per-version `ContentHash`, same `otherPaths` skip. It differs only in having no cache
to go stale, so `doctor` never notices.

## Surprises

1. **`doctor --fix` is a corruption amplifier, not a repair.** Hash mismatch has no fix
   branch, and the fix branch that does exist responds to a mismatch by deleting the tool's
   integration from the cache — then wedges `isCacheStale` into permanent `true` because the
   staleness check and the rebuild disagree about whether hash filtering applies
   (`internal/shellenv/doctor.go:146-174` vs `internal/shellenv/cache.go:119-131`). This is
   the closest thing to a repair path that exists, and it is a second bug.

2. **`tsuku install <tool>@<older-installed-version>` does not activate that version.** The
   short-circuit at `cmd/tsuku/install_deps.go:509-516` returns before `InstallWithOptions`.
   With `0.40.6` active, `tsuku install nvm@0.40.5` prints "already installed" and leaves
   `0.40.6` active. Notably, `cmd_rollback.go:60` tells the user to run exactly this command
   to reach a version older than one level, and that advice does not work.

3. **`tsuku install --plan` never records cleanup actions.** `RecordCleanup` has one non-test
   caller (`install_deps.go:613`). Plan-based installs write shell.d files with no state
   record — permanently orphaned on remove and invisible to `doctor`.

4. **`update --all` and background auto-apply are missing the stale-cleanup pass that
   `update <tool>` has.** The lifecycle-aware cleanup at `cmd/tsuku/update.go:176-192` is
   inside the single-tool branch only. `runUpdateAll` (line 267 onward) has no equivalent.

5. **GC deletes tool directories without touching state.**
   `internal/updates/gc.go:15-69` never opens `state.json`. The GC'd `VersionState` — with
   its `CleanupActions` — remains and keeps participating in the `otherPaths` skip on behalf
   of a directory that no longer exists.

6. **`00-env-*` is not on main.** The reproduction was run against
   `origin/fix/2439-set-env-exports`. Anyone reproducing on main will not find that file, and
   any design that treats it as existing will not compile against main.

7. **`Manager.Remove` (`remove.go:21-68`) leaves `ActiveVersion` dangling.** It deletes the
   directory and one symlink and never calls `UpdateTool`. It is marked deprecated and has no
   non-test caller, but it is exported and reachable.

8. **nvm has no binaries.** `Activate` falls back to `bin/<name>`
   (`manager.go:449-452`) when `versionState.Binaries` is empty, and nvm ships no
   `bin/nvm`. `tsuku activate nvm 0.40.5` therefore creates a dangling
   `tools/current/nvm` symlink — its entire real effect *should* be the shell.d re-render
   that does not happen.

## Open Questions

- **Where does re-render content come from for `source_command` recipes?** Re-executing a
  binary from the newly-active tool directory during `tsuku activate` is a subprocess with
  arbitrary side effects. The alternative — caching the rendered bytes per version at install
  time — costs storage and changes what `ContentHash` means. I did not find any existing
  cached-content store in state; `VersionState` carries the `Plan`
  (`internal/install/state.go:29`) but plans record steps, not rendered output.

- **Does the stored `Plan` in `VersionState` contain enough to replay the post-install
  phase?** `Plan.Steps` (`state.go:49`) holds resolved action params. Whether
  `install_shell_init` steps survive into the stored plan with usable params, and whether
  `Executor` can be driven from a stored plan for a single phase without a recipe, I did not
  verify.

- **Should `RemoveVersion` re-render or defer to a shared promotion path?** Its promotion
  (`remove.go:174-205`) duplicates `Activate`'s symlink logic rather than calling `Activate`.
  Whether consolidating them is in scope, or whether the hook must simply be added in both
  places, is a design call.

- **Is `install <tool>@<installed-version>` intended to activate?** #3 above is either a
  separate bug or intended idempotence. `cmd_rollback.go:60`'s user-facing advice implies the
  former.

- **What happens to shell.d when `--no-shell-init` was used for the currently-active version
  but not for another?** `install_shell_init` skips both the write and the `CleanupAction`
  when `ctx.NoShellInit` is set (`shell_init.go:83-89`). A version installed with the flag has
  no cleanup record, so `otherPaths` does not protect the other version's file, and promoting
  to the flagged version leaves the other version's file in place with nothing in state
  claiming it. I did not trace this to a concrete failure.

## Summary

Three code sites write `ToolState.ActiveVersion` — `InstallWithOptions` (`manager.go:260`),
`Activate` (`manager.go:463`), and `RemoveVersion`'s implicit promotion (`remove.go:179`) —
and only the first re-renders shell.d, because only the install path re-runs the post-install
phase; `Activate` contains no shell.d code at all and `RemoveVersion` skips both the delete
and the cache rebuild whenever two versions claim the same path (`remove.go:345-349`), which
is always. The re-render hook must therefore cover `Activate` (which transitively covers
`tsuku rollback` and auto-apply's silent failure-rollback at `updates/apply.go:173`) and
`RemoveVersion`'s promotion, plus four adjacent gaps the brief did not name: `update --all`
and background auto-apply skip the stale-cleanup pass that single-tool `update` runs,
`plan_install.go` never records `CleanupActions` at all, and `doctor --fix` has no repair for
a hash mismatch and wedges the staleness check when it does rebuild. The biggest open question
is where re-rendered content comes from: `install_shell_init`'s `source_command` mode produces
its bytes by executing a binary from the tool directory, so re-rendering on `activate` means
either running a subprocess during a version switch or introducing a per-version cache of
rendered output that does not exist today.
