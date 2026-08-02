# Lead: Where could a one-time migration (or, failing that, a warning) for existing nvm installs live, and how would existing affected installs be detected?

## Findings

### 1. Migration concept today: no framework, one strong precedent, and state.json is unversioned

**`state.json` has no schema version field.** `State` (`internal/install/state.go:141-146`) is
`{installed, libs, llm_usage}` — no `version`, no `schema_version`. The only schema-versioned
artifact in the repo is the *registry manifest* (`internal/registry/manifest.go:25-29`,
`MinManifestSchemaVersion`/`MaxManifestSchemaVersion` = 1), and `Plan.FormatVersion`
(`internal/install/state.go:39`), neither of which describes state.

**There *is* a load-time migration hook, and it is used twice.** `loadWithLock`
(`internal/install/state.go:216-219`) and `loadWithoutLock` (`internal/install/state.go:300-303`)
both call, unconditionally, on every single load:

```go
state.migrateToMultiVersion()
state.migrateSourceTracking()
```

Both live in `internal/install/state_tool.go` (`:199` and `:172`). Both are *shape-detecting and
idempotent* rather than version-gated: `migrateToMultiVersion` fires when `tool.Version != "" &&
tool.ActiveVersion == ""`; `migrateSourceTracking` fires when `tool.Source == ""`. Neither writes
to disk — they mutate the in-memory `State`, and the change only persists if some later `Save`
happens to run. That is the established idiom in this codebase: **detect the old shape, fix it in
place, stay idempotent, no version counter.**

This is a genuine hook point for "run once on load when the old shape is seen." But it is the
wrong altitude for nvm: these run inside `StateManager.Load()`, which is called by nearly
everything including read-only commands, holds a shared file lock, has no reporter, no config
beyond `HomeDir`, and no way to surface a message. Moving a multi-gigabyte Node tree from inside
`Load()` would be a serious layering violation.

**Precedent for an actual data-relocating migration: `EnsureEnvFile`.** `docs/designs/current/
DESIGN-shell-env-integration.md` (lines 11, 57-59, 153-160, 265-271) designed and shipped a
one-time migration that *moves user content between files*. The implementation is
`Config.EnsureEnvFile` + `Config.migrateEnvExports` (`internal/config/config.go:477-556`):
`EnsureEnvFile` compares the on-disk `$TSUKU_HOME/env` against the `EnvFileContent` constant; on
mismatch it extracts every `export` line not present in the managed content, appends it to
`$TSUKU_HOME/env.local` (dedup-safe via substring check), then rewrites `env`. Two call sites,
both stated in the design as the migration's delivery vehicle:

- `internal/install/manager.go:149` — every `InstallWithOptions`, so every install *and* every
  update, foreground or background.
- `cmd/tsuku/doctor.go:54` — `tsuku doctor --fix`, the explicit repair path for users who never
  re-install.

The design (line 212) is explicit that install-triggered migration alone is insufficient and that
the doctor path exists for exactly that gap. That two-path pattern — automatic on the next
install/update, plus a `doctor` route — is the closest precedent available and is directly
transferable.

There is no `docs/decisions/` directory in this repo; ADRs as a category do not exist here.

### 2. `internal/notices/` — can carry the message, but the background path silently drops it

`Notice` (`internal/notices/notices.go:32-55`) is a per-tool JSON file at
`$TSUKU_HOME/notices/<tool>.json`, with `Tool`, `Verb`, `Kind`, `Error`, `Messages []string`,
`Shown`. Renderers live in `internal/updates/notify.go`.

**When the user sees them.** `DisplayNotifications` (`internal/updates/notify.go:25`) is called
from `rootCmd.PersistentPreRun` (`cmd/tsuku/main.go:78`) at the head of *every* command except
the skip list (`check-updates`, `apply-updates`, `hook-env`, `run`, `help`, `version`,
`completion`, `self-update` — `cmd/tsuku/main.go:63-72`). So an unshown notice surfaces on the
next `tsuku <anything>`. `tsuku notices` (`cmd/tsuku/cmd_notices.go`) lists them on demand.

**What can write one.** Three producers:
- The event subscriber (`internal/notices/subscriber.go:33`) — reacts to install lifecycle
  events; a recipe cannot influence it.
- `progress.InboxReporter.Stop` (`internal/progress/inbox_reporter.go:91-118`) — the background
  auto-apply reporter. Any `reporter.Warn(...)` issued by *an action during a background install*
  accumulates and is flushed to a Notice with `Messages` populated.
- Direct `notices.WriteNotice` from anywhere with the notices dir path.

So yes, an action can emit one, via the reporter, with no new plumbing. In the foreground path the
reporter is a `TTYReporter` (`cmd/tsuku/update.go:134`) and `Warn` prints inline immediately.

**The trap.** `InboxReporter.Stop` (`inbox_reporter.go:104-110`) tags the notice
`KindAutoApplyResult` *unless* some message literally starts with the string `"version_fallback:"`,
in which case it becomes `KindVersionFallback`. And in `renderUnshownNotices`
(`internal/updates/notify.go:88-113`), only `KindVersionFallback`, `KindShellInitChange`, and
`KindCheckFailure` render their `Messages`. A `KindAutoApplyResult` notice with `Error == ""`
matches `isBackgroundSuccess` (`notify.go:145-150`) and gets routed to `renderBackgroundSuccess`
(`notify.go:155-171`), which prints **only** `"  <tool> -> <version>"` and discards `Messages`
entirely.

**Concrete consequence: a plain `reporter.Warn("your nvm data is at X")` from the nvm recipe or
an action, during background auto-apply, is written to disk and then never shown to the user.**
It would only surface via `tsuku notices` if that command printed `Messages` (it prints the
notice list; worth confirming separately). Two workarounds exist: prefix the message with
`version_fallback:` (abuse of an unrelated kind), or add a new `Kind` with a render branch —
noting that `kindFor` in `subscriber.go:120` deliberately refuses to let publishers select
single-view kinds, and there is a `kindForSubset` invariant test enforcing it.

Also worth flagging: `InboxReporter.Stop` *overwrites* the success notice the subscriber wrote,
and its `Notice` literal sets no `AttemptedVersion` and no `Verb`, so the background-success line
renders as `"nvm -> "` with an empty version whenever any warning was emitted. Pre-existing, not
caused by this issue, but it means the warning path visibly degrades the success line.

`KindShellInitChange` has **no non-test writer** anywhere in the tree — the constant, the render
branch, and the single-view removal logic all exist but nothing produces it. It is a ready-made,
unclaimed kind whose renderer prints `"Note: shell init changed for <tool>"` followed by every
`Messages` line. Repurposing or copying it is the cheapest path to a rendered multi-line message.

### 3. Update / auto-apply sequence, and where GC actually sits

**GC has exactly one call site.** `GarbageCollectVersions` is called only from
`updates.MaybeAutoApply` at `internal/updates/apply.go:153`, inside the per-entry loop, **only on
`applyErr == nil`**, immediately after the successful install of that tool:

```go
retention := userCfg.UpdatesVersionRetention()   // apply.go:152, default 7 * 24h
_ = GarbageCollectVersions(mgr, cfg.ToolsDir, entry.Tool, entry.LatestWithinPin, previousVersion, retention, time.Now())
```

`tsuku update <tool>` (`cmd/tsuku/update.go:35-203`) does **not** call GC. Neither does
`runUpdateAll`, `tsuku install`, or `tsuku doctor`. There is no `tsuku prune` / `tsuku gc`
command. Verified by grep: the only non-test references to `GarbageCollectVersions` are its
definition and `apply.go:153`.

This is the single most important scheduling fact for this issue:

> **Old nvm version directories are deleted only by the background auto-apply subprocess, and only
> in the tick that successfully auto-updates nvm itself.**

`MaybeAutoApply` runs inside `tsuku apply-updates` (`cmd/tsuku/cmd_apply_updates.go`), a hidden
command spawned detached from `PersistentPreRun` via `updates.MaybeSpawnAutoApply`
(`cmd/tsuku/main.go:77`), with stdout/stderr redirected to `/dev/null`
(`cmd_apply_updates.go:22-27`). Auto-apply is **on by default** (`UpdatesAutoApplyEnabled`,
`internal/userconfig/userconfig.go:396-398` returns `true` when unset; disabled only under CI or
explicit config). Check interval defaults to 24h (`DefaultCheckInterval`,
`userconfig.go:145`). So the deletion path is fully unattended.

**GC's own guards** (`internal/updates/gc.go:38-87`): skips the active version dir, skips the
`previousVersion` rollback target, and skips anything whose **directory mtime** is younger than
retention. That mtime is the tool directory's own mtime — it changes only when a *direct* child of
`nvm-<version>/` is created or removed. nvm creates `versions/`, `alias/`, `.cache/` once and
then writes deep inside them, so a heavily-used NVM_DIR can easily have a months-old top-level
mtime. Active use does **not** reliably refresh the retention clock.

**Upgrade sequence in `InstallWithOptions`** (`internal/install/manager.go:114-282`), which both
foreground update and auto-apply run through:

1. snapshot `priorActiveVersion` (`:126`)
2. `EnsureDirectories()` (`:142`), `EnsureEnvFile()` (`:149`) ← the precedent migration hook
3. stage into `.staging-<tool>-<version>`, `copyDir`, atomic `os.Rename` to
   `tools/<tool>-<version>` (`:154-201`)
4. symlinks/wrappers (`:205-223`)
5. `state.UpdateTool` — sets `PreviousVersion = old ActiveVersion`, adds `Versions[version]`,
   sets `ActiveVersion` (`:238-270`)
6. `rebuildShellCachesForTool` (`:279`)
7. **caller** then runs `ExecutePhase(ctx, plan, "post-install")` (`cmd/tsuku/install_deps.go:578`,
   `cmd/tsuku/plan_install.go:118`) — this is where `set_env` and `install_shell_init` run
8. **caller** then runs `finishPostInstall` (`cmd/tsuku/post_install.go:26`) to record cleanup
   actions and rebuild caches
9. only in the background path: back in `MaybeAutoApply`, GC for that tool (`apply.go:153`)

**Per-tool hook points in that sequence:** there is no lifecycle-hook registry. `internal/hook/`
is shell-rc integration (bashrc marker blocks), not install hooks; `internal/hooks/` is the
embedded `tsuku.bash`/`tsuku.zsh` shell scripts. The only per-tool extension surface is **the
recipe's own steps**, executed at step 7 for `post-install` and earlier for `install`.

The gap between step 7 (recipe steps, per-tool) and step 9 (GC) is within the same process, so a
recipe step *does* run before GC can delete anything, in both foreground and background.

### 4. Recipe-level migration: viable, with real constraints

`run_command` exists (`internal/actions/run_command.go`). It is used today by exactly two recipes
(`recipes/a/awscli.toml`, `recipes/p/pipx.toml`). No validation forbids it. Constraints:

- **Executes `sh -c <command>`** (`run_command.go:96`) with `cmd.Dir = workingDir`. No sandbox, no
  env scrubbing, no `cmd.Env` override — it inherits the parent process environment.
- **Not cross-platform.** `sh -c` is POSIX-only. `RequiresNetwork()` returns `true`
  (`run_command.go:15`) as a conservative default, which forces `network=host` in sandbox recipe
  testing — a testing-surface cost, not a correctness one.
- **Available variables** are `GetStandardVars` (`internal/actions/util.go:28-37`): `version`,
  `os`, `arch`, `install_dir`, `work_dir`, `libs_dir`, plus `binary` and optionally `PYTHON`
  (`run_command.go:79-85`). **There is no `{tsuku_home}` variable.** `$TSUKU_HOME` can only be
  derived — e.g. `dirname $(dirname {install_dir})` — or read from the inherited env, which is not
  guaranteed to be set.
- **Phase matters.** `run_command` does not implement `DefaultPhase`, so it defaults to the
  `install` phase, where `ctx.InstallDir` is the *staging* directory, not the final tool dir. A
  recipe must write `phase = "post-install"` on the step (supported: `recipe.Step.Phase`,
  `internal/recipe/types.go:392-395`; documented in `recipes/README.md:48`; validator at
  `internal/recipe/validator.go:511` only constrains phases for actions that declare a
  `DefaultPhase`, which `run_command` does not — so `post-install` is accepted).
- **Runs on both fresh install and update**, because an update is a full recipe execution of the
  new version. That is exactly what a migration needs: it fires on the very upgrade that would
  otherwise orphan the data.
- **Output handling**: `CombinedOutput` is captured and echoed via `reporter.Log`
  (`run_command.go:100-108`). `Log` is a **no-op on `InboxReporter`**
  (`inbox_reporter.go:45`) — so anything a migration script prints during background auto-apply is
  discarded. A failure returns an error, which fails the step and therefore the install.
- **Idempotency is the recipe author's problem.** A migration expressed as
  `[ -d old ] && mv old new` runs on every install of every version forever, with no state to say
  "already done." Acceptable if the guard is a plain existence test.

An alternative to `run_command` is a **new dedicated action** (e.g. `migrate_data_dir`) in
`internal/actions/`, which gets a real `ExecutionContext`, a reporter whose `Warn` is not a no-op,
cross-platform Go file operations, and the ability to record state. That is more code but removes
every constraint above except the notices-rendering trap in finding 2.

### 5. Detection signatures on disk

Two populations, two locations. `$TSUKU_HOME` defaults to `~/.tsuku`.

**Population A — pre-#2465 (`set_env` was a no-op).** Before commit `d396aeec`, `SetEnvAction.Execute`
wrote `export NVM_DIR=...` into `{install_dir}/env.sh` (confirmed by
`git show d396aeec^:internal/actions/set_env.go`, line ~44: `envFilePath := filepath.Join(ctx.InstallDir, "env.sh")`).
Nothing sources `env.sh` — `EnvFileContent` (`internal/config/config.go:444-465`) sources only
`share/shell.d/.init-cache.{bash,zsh}` and `env.local`. So `NVM_DIR` was never exported and
`nvm.sh` self-located to the directory the init cache was sourced from:
`$TSUKU_HOME/share/shell.d`. Signatures:

- `$TSUKU_HOME/share/shell.d/versions/node/` non-empty (installed Node versions)
- `$TSUKU_HOME/share/shell.d/alias/` (contains `default`, and per-name alias files)
- `$TSUKU_HOME/share/shell.d/.cache/` (nvm download cache)
- `$TSUKU_HOME/tools/nvm-<version>/env.sh` containing `export NVM_DIR=` (the dead file — a clean
  marker that this install predates #2465)
- Pre-#2465 shell.d fragment naming was `<target>.<shell>`
  (`git show d396aeec^:internal/actions/shell_init.go:160`), i.e. `share/shell.d/nvm.bash` and
  `nvm.zsh` — no `@<version>` key. Post-#2465 it is `nvm@<version>.bash`
  (`shellDFileName`, `internal/actions/shell_init.go:179-181`).

Note that `RebuildShellCache` (`internal/shellenv/cache.go:60-80`) skips directories and only
globs `*.bash`/`*.zsh` at the top level, so nvm's data subdirectories under `share/shell.d` have
been sitting there invisibly, never concatenated into the init cache. Nothing in `tsuku doctor`'s
`CheckShellD` looks at subdirectories either.

**Population B — post-#2465 (`NVM_DIR` genuinely resolves to the versioned tool dir).**
Signatures under `$TSUKU_HOME/tools/nvm-<version>/`:

- `versions/node/` non-empty
- `alias/`
- `.cache/`
- and, distinguishing "at risk" from "current": **more than one** `tools/nvm-*` directory, or a
  `tools/nvm-<v>` whose `<v>` is not `state.Installed["nvm"].ActiveVersion`. That non-active
  directory holding a non-empty `versions/node/` is precisely the orphaned-data condition.

**Population B is currently tiny.** #2465 merged today (commit `d396aeec`, 2026-08-02), and per
`git log` it is not yet in a tagged release. Unless a release has shipped since, essentially the
entire installed base is Population A, and Population B consists only of people running from
`main`. That materially changes the cost/benefit of a code migration for B.

**A third, easily-missed case:** a user who installed nvm pre-#2465 and *then* upgraded tsuku past
#2465 will, on their next `tsuku update nvm`, get a correctly-exported `NVM_DIR` pointing at the
new versioned tool dir — and their existing Node tree stays behind in `share/shell.d`. From their
point of view every Node version silently disappears at that moment. That transition is the most
urgent detection target, and it is a *tsuku upgrade*, not an nvm upgrade, that triggers it.

**State-based detection is not sufficient.** `state.json` records versions, binaries, plans, and
cleanup actions, but nothing about data directories. `CleanupAction`
(`internal/install/state.go:17-21`) is `{action, path, content_hash}` with paths relative to
`$TSUKU_HOME`, used only for `share/shell.d` fragments today. Detection has to stat the
filesystem.

### 6. Ranking the candidate homes by "runs before GC can delete the data"

GC runs at `internal/updates/apply.go:153`, unattended, in the background subprocess, immediately
after a successful auto-update of nvm. Anything that runs *earlier in that same subprocess* is
safe; anything that requires the user to type a command is not.

1. **A recipe step (`post-install` phase) in `recipes/n/nvm.toml`** — strongest on this criterion.
   Runs at step 7 of the sequence above, in-process, before `MaybeAutoApply` reaches its GC call
   at step 9, on *both* the foreground and background paths. It fires on exactly the upgrade that
   creates the orphan. Weaknesses: `sh -c` only (no Windows), no `{tsuku_home}`, and its output is
   swallowed in the background path (`InboxReporter.Log` is a no-op).
2. **A new Go action invoked from the recipe** — same timing guarantee, none of `run_command`'s
   constraints, and its `Warn` reaches the notice store. Costs a new action + validator entry +
   tests.
3. **`Config.EnsureEnvFile`-style hook in `InstallWithOptions`** (i.e. a global one-time migration
   called near `manager.go:149`) — runs before GC on every install of *any* tool, so it fires much
   sooner than a nvm-specific step (the next install of anything at all, not the next nvm update).
   Best coverage-per-elapsed-time. Weakness: puts tool-specific knowledge in the install manager,
   which is exactly the layering the recipe system exists to avoid. Mitigated if scoped as
   "relocate any stray data directories under `share/shell.d`", which is arguably a shell.d
   integrity concern rather than an nvm concern.
4. **A `tsuku doctor` check (+ `--fix`)** — the precedent's second half. Detects both populations
   reliably and can report precisely. But `doctor` is user-invoked; it never runs unattended, so
   on its own it cannot beat GC.
5. **A notice written by whichever of the above detects the condition** — the delivery mechanism,
   not a detection site. Surfaces at the head of the next foreground command
   (`cmd/tsuku/main.go:78`). Requires the finding-2 workaround to render `Messages` at all.
6. **`State.migrate*` at load time** (`internal/install/state.go:216-219`) — runs on essentially
   every command including read-only ones, so it beats GC easily on timing, but it is the wrong
   layer: no reporter, no config beyond `HomeDir`, holds the state lock, and moving large trees
   from inside `Load()` would be indefensible. Useful only if the migration needs a *state* flag
   (e.g. `nvm_data_migrated: true`) to stay idempotent.

The realistic combination is (2 or 3) for the automatic path, plus (4) for users who never
re-install, plus (5) to tell them what happened — mirroring exactly what
`DESIGN-shell-env-integration.md` did.

## Implications

The acceptance criterion "existing installs must be migrated, OR the user must be told" is
achievable, and the deadline pressure is lower than it first appears: **GC only runs during
background auto-apply, and only for a tool that just successfully auto-updated.** Orphaned
`nvm-<old>` directories therefore survive until the *next* nvm release is auto-applied and the old
directory's own mtime is 7+ days stale. That is typically weeks, not days. It is not a reason to
skip the migration, but it means a migration shipped in the same release as the data-root fix will
comfortably beat the reaper for nearly everyone.

The bigger practical finding is that **the population at risk is almost entirely Population A**
(data in `share/shell.d`), because #2465 landed today. Data in `share/shell.d` is *not* subject to
version GC at all — nothing garbage-collects `share/shell.d` subdirectories. So Population A's
data is not on a deletion clock; it is merely stranded and will be silently abandoned the moment
the user's tsuku crosses #2465 and they next update nvm. That reframes the requirement for A from
"rescue before deletion" to "relocate before abandonment," which is a strictly easier problem and
argues for handling A in the same migration step that establishes the new stable root.

Whatever emits the user-facing message must not rely on a bare `reporter.Warn` in the background
path. Either the message is emitted from a foreground-only surface (`doctor`, the inline install
reporter), or the notice must carry a `Kind` whose renderer prints `Messages` — which today means
`KindVersionFallback`, `KindCheckFailure`, or the unused `KindShellInitChange`. Adding a proper
kind is the honest fix and is a small, self-contained change to
`internal/notices/notices.go` + `internal/updates/notify.go`.

The `EnsureEnvFile` precedent should be cited directly in the design: same problem shape (managed
location changed, user content must survive), same two-path delivery (automatic on next
install/update at `manager.go:149`, manual repair at `doctor.go:54`), same shape-detecting
idempotency rather than a version counter. Reusing that pattern is a much easier sell than
introducing a migration framework or versioning `state.json`.

## Surprises

- **`tsuku update <tool>` never garbage-collects.** GC has exactly one call site, in the
  background subprocess. I expected the foreground update path to prune too.
- **A `reporter.Warn` during background auto-apply is written to disk and then never displayed.**
  `renderBackgroundSuccess` (`notify.go:155-171`) prints only `"<tool> -> <version>"` and drops
  `Messages`. The only escape hatch is the string-prefix hack `"version_fallback:"` in
  `InboxReporter.Stop` (`inbox_reporter.go:106`). That is a genuinely load-bearing trap for any
  design that assumes "just warn the user."
- Relatedly, `InboxReporter.Stop` overwrites the subscriber's success notice with a `Notice` that
  has no `AttemptedVersion` and no `Verb`, so any background install that emits a warning renders
  as `"  nvm -> "` with an empty version.
- **`KindShellInitChange` has no producer.** The constant, the render branch, and the single-view
  removal path all exist; nothing writes it. Either it was designed and never wired, or its writer
  was removed.
- **GC's retention clock is the tool directory's own mtime,** which nvm's usage pattern does not
  refresh (nvm writes into `versions/node/...`, several levels down). A daily-driver NVM_DIR can
  read as untouched for months.
- **There is no `{tsuku_home}` recipe variable** and no `docs/decisions/` directory. Both were
  assumed by the lead's framing.
- `state.json` carries no schema version at all, yet has two production migrations running on
  every load. The project's answer to "how do we migrate" is "detect the old shape," not "bump a
  number."

## Open Questions

- Has any tagged release shipped with #2465? `git log` shows it as the fourth commit from HEAD
  with no tag between. If not, Population B is only `main` users and a code migration for B may be
  unnecessary — a release note plus the doctor check could suffice, and the engineering should
  concentrate on Population A.
- Exact nvm on-disk layout under NVM_DIR needs confirmation from the nvm-upstream lead. I asserted
  `versions/node/`, `alias/`, `.cache/` from general knowledge; the recipe vendors nvm's source but
  I did not read `nvm.sh` to confirm the directory names, nor whether `versions/io.js/` matters.
- Does `tsuku notices` print `Messages`? I confirmed it calls `ReadAllNotices`
  (`cmd/tsuku/cmd_notices.go:29`) but did not read its rendering. If it does, that is a partial
  mitigation for the dropped-messages trap.
- Should the migration be a nvm-specific recipe step or a generic "stray data under
  `share/shell.d`" repair? The generic framing runs sooner (any install, not just nvm) and is
  defensible as shell.d hygiene, but it is a bigger behavioral claim. This is a design decision,
  not a research one.
- If the new stable root lives under `$TSUKU_HOME`, does anything else in the tree assume
  `tools/<tool>-<version>` is the only per-tool storage? I did not audit `tsuku remove` /
  `RemoveAllVersions` (`internal/install/remove.go:223`) for whether a stable data root would be
  correctly *preserved* on `tsuku remove nvm` — or correctly *deleted*. That needs a deliberate
  answer either way.

## Summary

tsuku has no migration framework and no versioned `state.json`, but it has a directly transferable
precedent — `Config.EnsureEnvFile` + `migrateEnvExports` (`internal/config/config.go:477-556`),
designed in `DESIGN-shell-env-integration.md` and delivered through two paths: automatically from
`InstallWithOptions` (`internal/install/manager.go:149`) and manually via `tsuku doctor --fix`
(`cmd/tsuku/doctor.go:54`). The deletion pressure is lower than assumed because
`GarbageCollectVersions` has exactly one call site — the background auto-apply loop at
`internal/updates/apply.go:153`, reached only after that same tool successfully auto-updates — so
a recipe `post-install` step or a new Go action runs in the same process well before GC, and the
much larger pre-#2465 population's data in `$TSUKU_HOME/share/shell.d` is not garbage-collected at
all, only stranded. The biggest open risk is delivery, not detection: a plain `reporter.Warn`
during background auto-apply is persisted and then silently discarded by
`renderBackgroundSuccess` (`internal/updates/notify.go:155-171`), so any "tell the user" path
needs a notice `Kind` whose renderer actually prints `Messages`.
