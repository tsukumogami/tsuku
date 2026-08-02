<!-- decision:start id="program-file-placement" status="assumed" -->
### Decision: Program-file placement into nvm's data root

**Context**

`nvm exec` and `nvm run` invoke `"${NVM_DIR}/nvm-exec"`. Once `NVM_DIR` names a stable
data root rather than the versioned tool directory, that file is no longer there, and
those two subcommands fail with `rc=127` while `install`, `ls`, `use`, `which`, and
`alias` all keep working. A lone `nvm-exec` does not fix it either: `nvm-exec`
self-locates through an unresolved `BASH_SOURCE[0]` and re-sources `$DIR/nvm.sh`, so
**both** `nvm.sh` and `nvm-exec` must be present in the data root. tsuku has no action
today that writes a file to a path outside the staging directory on a recipe's
instruction.

The decisive mechanical fact, found in the source rather than assumed: `Manager.Activate`
(`internal/install/manager.go:417-481`) repoints binary symlinks via
`createSymlinksForBinaries` (`:460`) and rebuilds shell caches (`:479`), but **never runs
the post-install phase**. `Rollback` delegates to `Activate` (`:370`), and
removal-promotion re-symlinks only binaries (`internal/install/remove.go:206`). Anything a
recipe *action* materializes is therefore refreshed on install only — it tracks the
last-installed version, not the active one.

That fact is what makes the "symlink versus copy" question decisive rather than
cosmetic, and the two questions have to be answered together.

**Assumptions**

- The data root's *location* is decided elsewhere and is treated here as a parameter.
  The TOML below writes it as `{data_dir}`; substitute whatever placeholder that decision
  mints. **The placeholder must be reachable from this action's expansion, not only from
  `set_env.Execute`** — if the location decision adds it as a local variable inside
  `SetEnvAction.Execute`, it needs to be factored into a shared helper instead.
- A one point-release skew between the nvm the interactive shell sources and the nvm
  `nvm exec` spawns is acceptable. nvm's on-disk data format is stable across point
  releases and the copied `nvm.sh`/`nvm-exec` pair always comes from a single release, so
  both halves are internally consistent. *If wrong:* the mechanism must move to
  install-path Go code (see Alternatives), roughly a ten-file change.
- Placing program files is **not** shell integration, so the action ignores
  `--no-shell-init`. *If wrong:* a `--no-shell-init` install records no cleanup for the
  data-root paths, which reopens the GC hazard described under Consequences.
- `recipes/README.md` and the two recipe-author skill files are maintained by convention
  rather than enforced by CI (`lint_test.go` carries no action-registry assertions).

**Chosen: A new post-install action, `install_program_files`, that COPIES**

Add one registered action to the `install_*` family. It copies named files out of the
tool's final install directory into a declared stable directory, and it does so on every
install, so placement and repointing are the same idempotent operation.

TOML surface as it would appear in `recipes/n/nvm.toml`:

```toml
[[steps]]
action = "set_env"
vars = [{name = "NVM_DIR", value = "{data_dir}"}]

[[steps]]
action = "install_program_files"
dir = "{data_dir}"
files = ["nvm.sh", "nvm-exec"]
```

Semantics:

- **Phase: `post-install`, declared via `PhaseDeclarer`.** The source is
  `ctx.ToolInstallDir`, which is only populated after the atomic rename, by
  `Executor.SetToolInstallDir` (`cmd/tsuku/install_deps.go:577`,
  `cmd/tsuku/plan_install.go:117`). Copying from the *staging* directory in the install
  phase would produce identical bytes but would mutate a stable path during an install
  that can still fail. `Execute` hard-errors when `ctx.ToolInstallDir == ""`, matching
  `set_env.go:126-136`. Declaring the phase also makes
  `internal/recipe/validator.go:508-516` reject any recipe that tries to override it.
- **`files` are relative to the tool install directory**; destination basename is
  `filepath.Base(file)`. Preflight rejects absolute paths and any `..` segment; Execute
  rejects a source that is not a regular file, so a symlink in the tarball cannot be used
  to read outside the tool directory.
- **`dir` is expanded, then validated.** After `filepath.Clean`, it must be absolute,
  must be inside `$TSUKU_HOME` (`filepath.Dir(ctx.ToolsDir)`) or inside the user's home,
  and must **not** be under `$TSUKU_HOME/tools` — that last check catches a recipe that
  mistakenly writes `{install_dir}` and re-creates the bug this design exists to fix.
  The validation matters beyond correctness: `Manager.executeSingleCleanup`
  (`remove.go:417-434`) does `filepath.Join(m.config.HomeDir, ca.Path)` with **no
  traversal check**, so an unvalidated recipe-controlled path would become a
  delete-anything primitive on `tsuku remove`.
- **`os.MkdirAll(dir, 0755)`.** This does not contradict the exploration's finding that
  no directory-creating action is needed — that finding was about nvm bootstrapping its
  own data tree, which it still does. It is one line inside this action because we now
  write a file into the directory before nvm ever touches it.
- **Copy, then chmod from the source's mode**, masked to drop group/other write.
  `nvm-exec` must stay executable; the shared `copyFile`
  (`internal/actions/download_cache.go:319-337`) does not preserve mode, so this is
  explicit. Write to a temp name in the destination directory and rename, so a concurrent
  `nvm exec` never observes a partial file.
- **Records a `delete_file` `CleanupAction` per placed file**, with content hash, path
  relative to `$TSUKU_HOME` — but **only when the destination is inside `$TSUKU_HOME`**.
  If the location decision puts the data root at `$HOME/.nvm`, `CleanupAction.Path`
  cannot address it (it is joined onto `config.HomeDir`), and tsuku correctly holds no
  opinion about a directory it does not own. Use `upsertCleanup`
  (`shell_init.go:229-242`) rather than `recordCleanup` for idempotence.
- **Does not check `ctx.NoShellInit`.** This diverges from its two siblings
  (`shell_init.go:112-116`, `set_env.go:100-104`) and follows `install_completions`
  (`completions.go:62-95`), which already ignores the flag. Two reasons: the flag's help
  text is "Skip shell integration setup (shell.d files)" and a data root is not a shell.d
  file; and under `--no-shell-init` a user who wires nvm up by hand still needs
  `nvm-exec` present, so skipping placement would break `nvm exec` for exactly the users
  most likely to pass the flag. With both files copied in, such a user can simply source
  `$TSUKU_HOME/share/nvm/nvm.sh` and get self-location for free. This choice also closes
  the largest GC hazard (below), so it should carry an explaining comment in the action.
- **`ActionEvaluability["install_program_files"] = true`** and `IsDeterministic() → true`.
  The action reads only the tool's own install directory; there is no network and no
  external state. The entry is mandatory — omitting it silently makes every recipe using
  the action non-reproducible, and `install_completions` is already missing from that map
  (`internal/executor/plan.go:174-213`).
- **The action is general, not nvm-specific.** pyenv, rbenv, asdf and sdkman have the
  same "`$TOOL_ROOT` holds both program and data" shape. The name says what it does —
  place a tool's own program files — without naming nvm or a mechanism.

Files an implementer touches:

| File | Change |
|---|---|
| `internal/actions/install_program_files.go` | new — the action |
| `internal/actions/install_program_files_test.go` | new — unit tests |
| `internal/actions/action.go` | one `Register(&InstallProgramFilesAction{})` line in `init()` (`:202-272`) |
| `internal/executor/plan.go` | one `ActionEvaluability` entry (`:174-213`) |
| `recipes/n/nvm.toml` | the new step, plus the `set_env` value and the header comment |
| `cmd/tsuku/shelld_lifecycle_test.go` | the upgrade regression test (below) |
| `recipes/README.md` | action-table row |
| `plugins/tsuku-recipes/skills/recipe-author/references/action-reference.md` | action docs |
| `plugins/tsuku-recipes/skills/recipe-author/SKILL.md` | action list |

The Go files satisfy the constraint that `recipes/**` alone arms no Go CI job.

**Rationale**

Copying is what turns a recipe action from insufficient into sufficient. With symlinks,
the `Activate` gap is fatal: the link tracks the last-installed version, so a
`tsuku rollback nvm` followed eventually by GC of the newer version leaves a dangling
`$NVM_DIR/nvm-exec`, and `nvm exec` returns 127 while everything else still works. That is
the same silent partial regression the exploration identified as the thing most likely to
ship unnoticed. With copies there is nothing to dangle — deleting the tool directory,
whether by GC, by `manager.go:182`'s pre-rename `RemoveAll`, or by `tsuku remove nvm@<v>`,
leaves the data root untouched. The residual consequence of the `Activate` gap shrinks to
a one point-release skew in which both halves keep working.

That reframing is what disqualifies the install-path alternative on cost. Wiring a
`VersionState` field through `InstallOptions`, a plan extractor, and all four
`createSymlinksForBinaries` call sites is the correct mechanism *if* the data root must
track the active version — it is exactly how binary symlinks work, and
`createBinarySymlink` (`manager.go:507-531`) already uses `ValidateSymlinkTarget` and
`AtomicSymlink`. But with copies it buys only the cosmetic skew fix, at roughly ten files,
for the single recipe in 1,449 that needs it.

Copying is also not a new idiom here. `install_shell_init` already *copies* `nvm.sh` into
`share/shell.d` (`shell_init.go:245-271`) rather than linking it, and
`internal/shellenv/doctor.go:91-93` flags any symlink under `share/` as a security risk.
A copy is what this tree already does with this exact file.

On the exploration's guidance that repointing must live inside the placement action rather
than in a separate step: **confirmed, and copies make it self-enforcing.** With copies,
placement and repointing are literally one operation — an idempotent overwrite from the
current tool directory — so there is no separate step a future change could forget. With
symlinks they would remain two conceptually distinct operations (create versus update)
even inside one action, and the `Activate` path would still be a third uncovered case.

The `--no-shell-init` answer follows from the same reasoning and pays a second dividend:
placing unconditionally means every installed version records the data-root cleanup, which
is what keeps GC's cross-version guard effective.

**Alternatives Considered**

- **Symlinks instead of copies.** Verified working end to end by the exploration, and
  free of disk cost. Rejected because `Activate`/`Rollback`/removal-promotion never re-run
  post-install (`manager.go:417-481`, `:370`, `remove.go:206`), so a recipe-placed symlink
  tracks the last-installed version and goes dangling once that version is removed or
  reaped — producing `nvm exec` `rc=127` with every other subcommand still green.

- **Hard links.** Survive tool-directory deletion via inode refcount. Rejected: they fail
  across filesystems, which matters if the data root lands outside `$TSUKU_HOME`; a stale
  link pins the old tool directory's inode so GC never reclaims its space; and they offer
  nothing a copy does not.

- **`run_command`.** Works today with no new Go code — which is itself a defect here,
  since `recipes/**` arms no Go CI job. Rejected on four counts: `{install_dir}` expands
  from the *staging* directory (`run_command.go:79`), so the command would have to
  hardcode `$TSUKU_HOME/tools/nvm-{version}`, exactly the pattern its own `Preflight`
  warns about (`:31-44` — a warning, not an error, so it would ship); `RequiresNetwork()
  == true` (`:15`) upgrades every sandbox validation of nvm to the networked heavyweight
  image (`internal/sandbox/requirements.go:122-127`); it is `false` in
  `ActionEvaluability` (`plan.go:212`); and it is unrestricted `sh -c` for a two-file copy.

- **Extending `install_shell_init` (or `set_env`).** Rejected as a conflation, and it is
  more expensive than it looks: `install_shell_init` does **not** implement
  `PhaseDeclarer` and today runs in the *install* phase reading the staging directory
  (`shell_init.go:246`; `recipes/n/nvm.toml` sets no phase). Teaching it to place program
  files into a data root would force it to `post-install` for every existing user, and
  `internal/recipe/validator.go:511-516` would then reject any explicit
  `phase = "install"` override. The two jobs also disagree about `--no-shell-init`, which
  is precisely where a merged action would have to branch. `set_env` is worse still.

- **Install-path Go code keyed off a `VersionState` field.** The right mechanism if the
  data root must track the active version, and it inherits repointing at all four sites
  for free. Rejected as disproportionate — see Rationale. Recorded as the named upgrade
  path if activate/rollback fidelity ever matters, or if a second tool adopts the action.

**Consequences**

*Cleanup and removal.* Two `delete_file` records, one per placed file. `tsuku remove nvm`
goes through `RemoveAllVersions` (`remove.go:223-271`), which applies no cross-version
guard, so both files are deleted — the program files go with the tool. The user's Node
installs under `versions/` are untouched, because this action never recorded them; what
`tsuku remove` should do about *those* is the location decision's call, not this one's.

*Upgrade.* Safe twice over. `StaleCleanupActions` (`internal/install/update.go:14-36`)
keys on `(action, path)` and filters the identical record out as not-stale, and
`ExecuteStaleCleanup`'s global `recordedPaths()` guard (`update.go:65`) retains it anyway.
The content hash cannot interfere: `executeSingleCleanup` never reads it, and all four
`ContentHash` consumers are prefix-scoped to `share/shell.d/`. That last point cuts both
ways — the hash recorded for a data-root path is inert, so it is not usable as an
integrity signal without widening `BuildShellDSelection`, `CheckShellD`, and
`warnShellInitChanges`.

*Same-version reinstall and downgrade.* `manager.go:182` removes the tool directory before
the rename at `:197`, and post-install runs strictly afterwards
(`install_deps.go:576-580`). The copies are untouched throughout and are then overwritten
with the correct content. `RecordCleanup` replaces a version's slice rather than appending
(`state_ops.go:79-94`), so records do not accumulate. A downgrade via
`tsuku install nvm@<older>` runs the full install including post-install, so the copies
correctly revert.

*The residual GC hazard, narrowed but not closed.* `ReapVersion` (`remove.go:384-413`)
does execute cleanup actions, guarded by `otherPaths` (`:346-368`), which protects the
shared path only if the surviving active version also recorded it. Placing unconditionally
under `--no-shell-init` closes the largest hole. What remains: if the active version's
post-install phase *failed*, it only warns (`install_deps.go:578-581`) and records
nothing, after which reaping an older version deletes the data-root files out from under
it. This hole is pre-existing and affects `install_completions` identically today. It
should be filed separately rather than fixed here.

*The test that has to exist.* `cmd/tsuku/shelld_lifecycle_test.go` needs no network, is
not `testing.Short()`-guarded, and already installs v1 then v2. Add a scenario: install
v1, install v2, `os.RemoveAll(cfg.ToolDir(tool, v1))` to simulate GC, then assert the
data-root files exist, are executable, and match v2's content. The harness invokes
post-install actions by hand (`:167-183`), so this needs a third explicit `Execute` call
there. Without this test the regression ships green — a test that only covers a fresh
install passes while the upgrade path is broken.

*What becomes easier.* Any future tool with a `$TOOL_ROOT` that mixes program and data —
pyenv, rbenv, asdf, sdkman — gets a declarative mechanism instead of a `run_command`
workaround. *What becomes harder:* the data root now holds tsuku-written files, so
anything that reasons about it (a `doctor` check, disk accounting) must distinguish
tsuku's copies from nvm's own data.

*Follow-ups worth filing separately, not fixing here:* `install_completions` is missing
from `ActionEvaluability`; `executeSingleCleanup` performs no path-traversal validation
before joining onto `$TSUKU_HOME`; and post-install failures warn rather than fail, which
is what leaves the reap hazard open.
<!-- decision:end -->
