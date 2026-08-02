# Decision Report: migrating existing nvm installs to the data root

Decision question: *How do existing nvm installs reach the new data root — what code
runs, where does it run so it beats every deletion path, and how is the user told when it
cannot run silently?*

Complexity: critical. Full path — research, five adversarial validators, peer revision,
cross-examination, and a third round on the crux. Run in `--auto` mode.

<!-- decision:start id="nvm-data-migration" status="assumed" -->
### Decision: migrating existing nvm installs to `$TSUKU_HOME/data/nvm`

**Context**

Decision 1 settled the destination (`$TSUKU_HOME/data/nvm`) and decision 2 settled how
`nvm.sh` and `nvm-exec` get there. Neither moves a single byte of anyone's existing Node
estate. Two populations hold that data in places the new design does not look, and the
issue's acceptance criterion is that they are *migrated, or told where their data is and
what to do about it, before retention can delete it*.

The framing this decision inherited said a recipe change reaches neither population.
That framing is wrong in one important way and it changed the answer. The plan cache is
not a separate store keyed by `tool@version` — it is `VersionState.Plan` inside
`state.json` (`internal/install/state_tool.go:142-159`), so it can only hit for a version
already installed and can never block a *new* version's recipe. The real short-circuit is
`mgr.IsVersionInstalled` at `cmd/tsuku/install_deps.go:508`, which returns before
`ExecutePlan` (`:521`), `InstallWithOptions` (`:568`) and the post-install phase (`:578`).
`--force` is unrelated to it (`cmd/tsuku/install.go:373` — security prompts only) and
`--fresh` only skips the plan-cache read (`install_deps.go:96`). So a recipe post-install
step **does** reach every user who ever gets a new nvm version, foreground or background.
It reaches nobody who stays on their current version.

**Assumptions**

- `$TSUKU_HOME/data` and `$TSUKU_HOME/share` are on one filesystem, so `os.Rename` between
  them succeeds. The install pipeline already bets on this at
  `internal/install/manager.go:198`, with the assumption written into the comment at
  `:397`. *If wrong:* the migration reports the failure and moves nothing — see the
  no-copy-fallback rule below.
- nvm's on-disk data set under `NVM_DIR` is `versions/`, `alias/`, `.cache/`,
  `default-packages`, and `current`. Carried from the exploration's empirical run, not
  re-derived here. *If wrong:* a future nvm data path is left behind rather than moved,
  which degrades gracefully — the exploration's own run left `.cache/` behind and nvm
  silently re-downloaded it.
- Population B is `main`-only. The shell.d fix (`d396aeec`) landed 2026-08-02 and is not
  in a tagged release. *If wrong:* Population B is larger and the removal-path rescue
  matters more, not less.

**Chosen: one merge-move in a new leaf package, invoked from three call sites that each
earn their place for a different reason, plus a failure-only notice**

**1. The populations, and the exact predicates.**

*Population A — pre-shell.d-fix, the large one.* `set_env` wrote an `env.sh` nothing
sourced, so `NVM_DIR` was never exported and `nvm.sh` self-located to
`$TSUKU_HOME/share/shell.d`. Predicate: **any of `versions`, `alias`, `.cache` exists as a
directory directly under `$TSUKU_HOME/share/shell.d`.**

The move rule for A is **every entry under `share/shell.d` that is a directory**, and it
is provable rather than heuristic: tsuku's only writers there write named *files*
(`internal/actions/shell_init.go:252`, `:318`; `internal/actions/set_env.go:175`, `:190`),
and both enumerators `continue` on `IsDir()` (`internal/shellenv/cache.go:65-67`,
`internal/shellenv/doctor.go:75-77`). A directory under `share/shell.d` is therefore not
tsuku's, by construction.

*Population B — post-shell.d-fix, tiny.* Data under `$TSUKU_HOME/tools/nvm-<version>/`.
Predicate: **for each version in `state.Installed["nvm"].Versions`, any of `versions`,
`alias`, `.cache` exists as a directory under `$TSUKU_HOME/tools/nvm-<version>/`.**

The move rule for B is the **enumerated data set** (`versions/`, `alias/`, `.cache/`,
`default-packages`, `current`), not an inverse rule. The source directory is *also* a
program directory, and an inverse rule would classify a file that exists in the old
release but not the new one as user data — gutting a directory GC deliberately preserves
as the rollback target (`internal/updates/gc.go:53-56`) and depositing a stale nvm program
file into the one tree decision 1 declares tsuku never deletes.

*Population C — a pre-existing upstream install.* `$HOME/.nvm/versions/node` exists as a
directory. **In scope, as a `doctor` note only.** tsuku never reads, writes, moves, or
deletes anything under `$HOME/.nvm` on any path. Guarded three ways: emit only when
`$HOME/.nvm/versions/node` exists, `$TSUKU_HOME/data/nvm/versions` does **not** exist, and
`state.Installed["nvm"] != nil`. The middle guard is what stops a user who legitimately
runs both roots from being nagged. Rendered as an `ok` verdict with a note, because
nothing is wrong — it answers "why is `nvm ls` different from what I remember" and states
the non-interference guarantee out loud.

*Order:* Population B before Population A. B is on a deletion clock and A is not, so B
should win any collision and A's leftovers are safe where they sit. Within B, the active
version first.

**2. What runs, and where. The crux: moving the data without repointing `NVM_DIR` inflicts
the exact bug this design exists to fix.**

`set_env` expands its placeholder to an **absolute literal** before writing —
`value := ExpandVars(...)` (`set_env.go:165`), `fmt.Fprintf(&buf, "export %s=%s\n", ...)`
(`:169`) with `shellQuote` single-quoting so nothing re-expands at shell time (`:245-247`)
— into `share/shell.d/00-env-nvm@<version>.<shell>` (`envTargetName`, `set_env.go:204-219`,
returning `EnvFilePrefix + name` with `EnvFilePrefix = "00-env-"` at `:27`; filename via
`shellDFileName`, `shell_init.go:179-181`). `RebuildShellCache` concatenates that content
verbatim. **Nothing in the tree rewrites a fragment's body outside an install of nvm
itself.**

Note that nvm has **two** fragments and only one of them carries the export: `set_env`
writes `00-env-nvm@<v>.<shell>` (the `export NVM_DIR=` line), while `install_shell_init`
writes `nvm@<v>.<shell>` (the line that sources `nvm.sh`). The `00-env-` prefix exists so
the export sorts into the cache ahead of the sourcing. Every predicate below reads the
`00-env-` file.

So any code that reaches `internal/install/manager.go:149` but not nvm's post-install
phase runs at a moment when the nvm recipe has not run and `NVM_DIR` still names the old
location. A Population B user with `NVM_DIR='$TSUKU_HOME/tools/nvm-v1'` live in their
shell who runs `tsuku install jq` would have their data relocated to `data/nvm` while the
fragment still says `tools/nvm-v1`: next shell, `nvm ls` empty, `default` alias gone.
Population A is worse — `NVM_DIR` was never exported at all, which is *why* the data is in
`share/shell.d`, so moving it out strands them with no export to compensate. **The set a
general install hook uniquely covers is precisely the set in which migrating is unsafe**,
and gating it to be safe collapses its trigger set onto the recipe step's. Both validators
who advocated that hook verified the mechanism themselves and withdrew it as primary.

The three call sites that survive:

- **Primary mover — a `migrate_data_dir` step in `recipes/n/nvm.toml`, `post-install`
  phase, ordered after `set_env`.** Its safety is not a gate but co-location: `set_env`
  and the move are consecutive steps of one `exec.ExecutePhase(globalCtx, plan,
  "post-install")` call (`install_deps.go:578`), followed by `finishPostInstall` (`:585`).
  The export and the data change together, in one process, before any shell sees either.
  This fires on every nvm version bump — fresh install, `tsuku update nvm`, or background
  auto-apply — which is exactly the event that would otherwise orphan the data, and it
  runs well before `MaybeAutoApply` reaches GC at `internal/updates/apply.go:153`.

  It **must return `nil` on every runtime failure.** `ExecutePhase` aborts the remaining
  steps of the phase on the first error (`internal/executor/executor.go:657-659`), and a
  post-install error would cost `install_program_files` its cleanup records. Failures are
  reported, never returned.

- **Rescue — the same merge-move called from `RemoveVersion` and `RemoveAllVersions`,
  immediately before `os.RemoveAll(toolDir)` at `internal/install/remove.go:151` and
  `:258`.** This is not the migration and needs no gate: `executeCleanupActions` at
  `remove.go:147` has already torn the fragment down, so there is no export left to
  invalidate, and the destination is the root decision 1 promises to preserve. Without
  it, `tsuku remove nvm` destroys an unmigrated Population B user's entire Node estate —
  which makes decision 1's "remove preserves the data root, only `--purge` deletes it"
  vacuous for exactly the users it was written to protect. It also covers
  `cmd/tsuku/remove.go:160`, the orphaned-dependency auto-removal, where nvm is reaped
  without the user ever naming it. **Skip it under `--purge`**, which deletes the data
  root anyway.

- **Pull surface — a `doctor` check and a third `--fix` stanza.** The check runs on the
  *defect* predicate, not the data-exists predicate: **the active nvm export fragment
  `share/shell.d/00-env-nvm@<activeVersion>.<shell>` contains `export NVM_DIR='<home>/data/nvm'`,
  AND nvm data still exists in an old location.** Two verdicts:

  | Condition | Verdict | `--fix` |
  |---|---|---|
  | Fragment names `data/nvm`, data still in an old location | **FAIL** — the shell points at an empty root right now | yes, retry the merge-move |
  | Fragment names an old path, data is there | **WARN** — working, at risk | **no**, must not touch it |

  The second row is the one that matters. A Population B user on the old recipe is not
  broken; they are fine until they update, and moving their data would break them. The
  fragment must be read rather than inferred: an `os.Stat($TSUKU_HOME/data/nvm/nvm.sh)`
  shortcut is unsound because `Manager.Activate` (`manager.go:417-482`) re-selects
  fragments at `:479` without ever running post-install, so after a rollback that file
  exists while the active fragment names the old path. `--no-shell-init` is handled for
  free: `set_env` returns early writing nothing (`set_env.go:101-104`), no fragment
  exists, the predicate is false, and nothing moves.

  `doctor --fix` is the only user-invokable actuator that exists, because the
  `IsVersionInstalled` short-circuit at `install_deps.go:508` blocks every
  `tsuku install`/`update` a broken user could type.

**Package placement: a new leaf package `internal/nvmdata`**, importing stdlib and
`internal/config` only. `internal/actions` and `internal/install` do not import each
other today; routing the action through `internal/install` would create an
`actions → install` edge — acyclic and legal, but a new architectural edge for a
one-tool feature. A leaf package all three callers import adds no edge between existing
packages and is one deletable unit when the migration outlives its usefulness.

**3. Move, copy, or leave-and-warn: a merge-move that contains no deletion primitive.**

`mergeMove(src, dst)`: for each entry in `src`, if the corresponding path under `dst` does
not exist, `os.Rename` it; if it exists and both sides are directories, recurse; if it
exists and either side is not a directory, **leave the source in place and record a
conflict**. Finish with `os.Remove(src)`, which fails harmlessly on a non-empty directory.

- **No `os.RemoveAll`. No overwrite. Ever.** This is the invariant that makes the
  operation safe to run without a human watching, and it deserves a comment and a test.
  The worst outcome is data in two places, both intact.
- **Atomicity:** not atomic overall, atomic per entry at every depth. A crash leaves every
  entry wholly in one place or the other, never split and never torn.
- **Re-runnable** by shape detection, with no state flag — matching
  `State.migrateToMultiVersion` and `migrateSourceTracking`
  (`internal/install/state_tool.go:199`, `:172`), which is how this codebase already does
  migrations. `state.json` has no schema version and does not need one here.
- **Conflict policy is explicit, not emergent:** the destination always wins, the source
  is never destroyed, and every conflict is reported by path.
- **No `copyDir` fallback, ever — this belongs in the doc comment as a prohibition, not as
  an omission a future contributor helpfully fills in.** On `EXDEV`, `EACCES`, or anything
  else, record the source path, report it, and stop. `copyDir`
  (`internal/install/manager.go:724-771`) has no hard-link tracking, so npm's
  content-addressable store becomes N independent full copies and a fits-on-disk migration
  can become ENOSPC; a single unreadable file aborts mid-tree leaving a partial destination
  *and* the original with nothing saying which is authoritative; and modes are umask-masked.
  It does get symlinks right (`copySymlink`, `:774-786` copies the raw target string), but
  `mv(1)` handles the cross-device case correctly including hard links, so the honest
  remedy is to print the path and let the user run it.

**4. How the user is told: a failure-only notice, delivered by `WriteNotice` rather than
by the reporter.**

Nothing is said on success. A successful migration has nothing to report — Population A's
`nvm ls` lists the same versions it always did, and Population B's alternative outcome was
"your Node versions vanish", which now does not happen. The acceptance criterion is a
disjunction, and a successful migration discharges it by the first clause.

On failure, a notice: `Tool: "nvm-data-migration"`, a new `notices.KindDataMigration`, and
a **non-empty `Error`**. Three consequences follow from that one field, and together they
are why this is cheap:

- `isBackgroundSuccess` (`internal/updates/notify.go:145-150`) is false, so the notice
  escapes `renderBackgroundSuccess` — the trap that drops `Messages` entirely — and lands
  in the `rest` loop at `notify.go:91`.
- `cmd/tsuku/cmd_notices.go` needs **no change**: `:70-74` deletes only notices with an
  empty `Error`, and `:53-57` counts this one correctly.
- It takes the `MarkShown` branch rather than single-view removal, so it persists.

The notice **must not** be keyed on `nvm`. `WriteNotice` writes
`filepath.Join(noticesDir, notice.Tool+".json")` (`internal/notices/notices.go:104`), so a
notice named `nvm` is overwritten twice in the same tick — by `Subscriber.Handle` via
`publishInstallOutcome`, and by `InboxReporter.Stop`
(`internal/progress/inbox_reporter.go:112-117`). `nvm-data-migration` passes
`validateNoticeName` untouched; a `lib--`-style sentinel would **not** —
`notices.go:231-233` rejects any name containing `--` unless it starts with `lib--`.

The new `Kind` is worth its nine lines for one concrete reason: without it the notice
falls through to `renderToolNotice`'s failure branch, which prints
`"Run 'tsuku notices' for details, 'tsuku rollback %s' to revert."` (`notify.go:251`) with
`n.Tool` — telling a user whose Node estate is currently half in two places to run
`tsuku rollback nvm-data-migration`, a command that cannot succeed against a tool that
does not exist.

The notice text names the two paths and `tsuku doctor --fix`. That is what closes the
discoverability gap from the other side: **nothing in tsuku's non-test Go ever prints the
string `tsuku doctor` outside `doctor.go` itself**, so the notice is the only thing that
can point a user at the remedy.

The action never calls `reporter.Warn`, and it does not need a foreground/background
discriminator. `install_deps.go:578` passes **`globalCtx`**, not the source-tagged `ctx`
that `:568` hands `InstallWithOptions`, so `installevents.SourceFromContext` returns empty
inside a post-install action — the discriminator is simply unavailable there. It is also
unnecessary: `reporter.Status` prints inline on a `TTYReporter` and is a hard no-op on
`InboxReporter` (`inbox_reporter.go:42`), which is exactly the desired behavior on each
path, while `WriteNotice` carries durability on both. `reporter.Warn` would be actively
harmful, since one `Warn` flips `InboxReporter.Stop` into overwriting `notices/nvm.json`
with a stub that renders as `"nvm -> "` with an empty version.

**Is it acceptable for this to run unattended?** Yes, and the reason is structural rather
than a judgement call: `mergeMove` contains no deletion primitive, so there is no state in
which running it is worse than not running it. It either succeeds, or it fails leaving the
source exactly where a detect-only design would have left it, with the same notice written
either way. The incoherence of the alternative is decisive — `GarbageCollectVersions`
already `os.RemoveAll`s this data unattended from the same subprocess
(`internal/updates/gc.go:82`, called from `apply.go:153`), a few lines after the install
that performs the move. A position that permits that deletion but forbids an `os.Rename`
earlier in the same process is not defensible.

**5. Files an implementer touches.**

| File | Change |
|---|---|
| `internal/nvmdata/nvmdata.go` | new — predicates, `mergeMove`, `Detect`, `Migrate`, `Rescue`, `Report` |
| `internal/nvmdata/nvmdata_test.go` | new — unit tests |
| `internal/actions/migrate_data_dir.go` | new — the post-install action |
| `internal/actions/migrate_data_dir_test.go` | new |
| `internal/actions/action.go` | one `Register(...)` line in `init()` |
| `internal/executor/plan.go` | one `ActionEvaluability` entry |
| `internal/install/remove.go` | rescue call before `:151` and `:258`, skipped under `--purge` |
| `internal/notices/notices.go` | `KindDataMigration` constant plus its doc sentence |
| `internal/updates/notify.go` | one `case` in `renderUnshownNotices`, plus the doc line |
| `cmd/tsuku/doctor.go` | Check 9 (two verdicts) and a third `--fix` stanza |
| `cmd/tsuku/doctor_test.go` | modeled on `TestDoctorFix_EnvRewrite` (`:136-178`) |
| `cmd/tsuku/shelld_lifecycle_test.go` | end-to-end migration case |
| `recipes/n/nvm.toml` | the `migrate_data_dir` step, after `set_env` |
| `recipes/README.md`, recipe-author skill files | action docs |

`cmd/tsuku/cmd_notices.go` is **not** touched.

**Tests.** `cmd/tsuku/shelld_lifecycle_test.go` is the end-to-end home: no
`testing.Short()` anywhere in `cmd/tsuku/*_test.go`, no network, and it already stands up
a real `$TSUKU_HOME` under `t.TempDir()` with a real `Manager` and a bash reader
(`:73-114`, `:196-200`). Seed `share/shell.d/versions/node/v20.0.0/...`, run the trigger,
then assert the tree moved **and** that `NVM_DIR` read out of a real bash subshell names
the new root — observable behavior, not intermediate state. Unit tests go beside the
`TestEnsureEnvFile_*` quartet's pattern (`internal/config/config_test.go:733-940`):
happy path, idempotent, migrates, and idempotent-after-migration, that last being the one
that matters for a re-runnable file move. Seed calls need explicit error checks
(`errcheck` covers test files with no `os.WriteFile`/`os.MkdirAll` exclusion), which the
existing files already do throughout.

**Rationale**

The decision turns on one fact that none of the background research surfaced and that
inverted the bakeoff: **the exported `NVM_DIR` is a literal absolute path frozen into a
shell.d fragment at the moment the nvm recipe last ran.** Once that is on the table, the
question "where does the migration run so it beats every deletion path" is the wrong
question. Beating the deletion paths is easy — GC has one call site, reached only after
nvm itself successfully auto-updates, and it protects the just-superseded directory as
`previousVersion` (`apply.go:126-131`), so deletion needs two nvm releases plus seven
days. The hard constraint is the *invalidation* path, which fires in one second: the data
and the export must move together or the migration causes the bug.

Only one place guarantees that, and it is nvm's own post-install phase. That is not a
layering preference; it is the only moment in the system where "the data moved" and
"`NVM_DIR` now names the new place" are the same event.

The two call sites that survive alongside it survive because they are *not* migrations.
The removal rescue runs where the export is being torn down, so there is no export to
gate on; it exists because decision 1's promise is otherwise false for exactly the users
it protects. The `doctor` check runs on a failure-shaped predicate that is true only after
the recipe already committed to the new path and the data did not arrive.

The general install hook at `manager.go:149` is rejected on correctness, not cost. Its
advocates' strongest argument was coverage-per-elapsed-time — days rather than an nvm
release — and those extra firings are exactly the unsafe ones. The `EnsureEnvFile`
precedent that motivated it does not transfer: `EnsureEnvFile` repairs a file tsuku owns
outright, with no external state that has to agree with it. This migration has a second
half — the fragment — that only nvm's own install rewrites.

The delivery answer follows from the same discipline. The notice is failure-only because
success has nothing to say; it carries a non-empty `Error` because that one field routes
it around every trap in the notice system for free; and it is delivered by `WriteNotice`
rather than `reporter.Warn` because the reporter is the trap.

**Alternatives Considered**

- **A general migration hook on the install path (`manager.go:149`), the `EnsureEnvFile`
  shape.** Rejected on correctness: every event in its extra coverage is an event at
  which the nvm recipe has not run, so `NVM_DIR` still names the old location and the
  move breaks a working install. Both validators who advocated it verified the mechanism
  and withdrew it. Recorded below as an upgrade path, with its gate.
- **`doctor` alone.** Rejected: nothing in tsuku's non-test Go ever mentions
  `tsuku doctor` outside `doctor.go`, so it is a pull surface nothing points at. Its
  advocate conceded this without being pushed. Its *repair* half was right and is adopted.
- **Detect automatically, never move (or move only in the foreground).** Withdrawn by its
  own validator under cross-examination. The irreversibility premise is false against an
  algorithm with no deletion primitive, and the position's own consequence — that
  detected-but-unmoved Population B directories then need GC protection via an mtime touch
  or a `gc.go` skip — is new machinery in the deletion path whose entire purpose is to
  stop the background from deleting data, versus one `os.Rename` earlier in the same
  process that makes the problem disappear.
- **A one-shot `tsuku migrate` command.** Rejected: no precedent exists. Every maintenance
  surface in the tree is either permanently idempotent (`doctor --fix`, `cache cleanup`)
  or fully automatic and shape-detecting. A one-shot command would invent a category.
- **Using `ctx.ToolInstallDir` as a program manifest to move "everything that is not a
  program file".** Rejected and withdrawn by the validator who proposed it: it is
  irrelevant to Population A, and for Population B it applies the *new* release's file
  list to the *old* release's directory, so a file dropped between releases is
  misclassified as user data — gutting the rollback target and depositing a stale program
  file into `data/`. Survives as an optional *detector* for unrecognized leftovers.
- **A notice keyed on `nvm`, or written through `reporter.Warn`.** Rejected: the first is
  overwritten twice in the same tick by `Subscriber.Handle` and `InboxReporter.Stop`; the
  second corrupts an unrelated tool's background success line into `"nvm -> "`.

**Consequences**

What becomes true: a user who updates nvm has their Node estate follow `NVM_DIR` in the
same process, with no window in which the two disagree. `tsuku remove nvm` preserves the
data it promises to preserve even for users who never migrated. A stuck migration is
visible in `doctor` and names its own remedy in a notice. And the migration is a
disposable unit — one leaf package and one action — that can be deleted outright once the
affected releases are old enough, with shape detection making the deletion a no-op for
anyone already migrated.

What becomes harder, honestly:

- **A user who never updates nvm is never migrated.** This is the real coverage gap and it
  is bounded by nvm's release cadence rather than by days. It is survivable because that
  user is not broken: their fragment and their data agree, `nvm ls` works, and Population
  A's data is on no deletion clock at all. The residual risk is `tsuku remove nvm`, which
  the rescue call site closes, and GC, which cannot fire without an nvm install running
  the recipe step first in the same process.
- **A partial removal leaves rescued data invisible.** Removing the active version with
  others remaining promotes another version whose fragment names its own directory
  (`remove.go:262-264`), so the rescued data is preserved but not pointed at until the
  next nvm install. Strictly better than destruction, and the message must say so rather
  than claim success.
- **Conflicts and `EXDEV` are reported, not resolved.** By design. The remedy printed is
  a literal `mv`, because the shell one-liner a user would otherwise improvise is unsafe:
  `mv src/* dst/` skips dotfiles, so `.cache` is silently lost, and it nests
  `versions/versions` when the destination already exists.

**Cross-decision items this decision surfaced and does not own:**

- **`Activate`/`rollback` repoints `NVM_DIR` across the old-path/new-path boundary without
  running post-install.** `Manager.Activate` (`manager.go:417-482`) flips `ActiveVersion`
  and calls `rebuildShellCachesForTool` at `:479`, which *selects* a fragment and never
  rewrites one. So once the fixed recipe ships, `tsuku rollback nvm` to a pre-design
  version repoints `NVM_DIR` at `tools/nvm-<old>` while the data sits at `data/nvm`, and
  the user's Node versions vanish. This belongs to decision 2 — it is the same `Activate`
  gap that decision already reasoned about for program files — and it is unaddressed.
- **`tsuku remove nvm` on an unmigrated Population B user contradicts decision 1's settled
  semantics.** The rescue call site above is the fix; decision 4 should record that its
  guarantee is conditional on the migration having run, and that the rescue is what makes
  it unconditional.
- **Every post-install failure in tsuku is already silently discarded on the background
  path.** `install_deps.go:580` reports it with `reporter.Warn`, which on `InboxReporter`
  becomes a `KindAutoApplyResult` notice with no `AttemptedVersion` whose `Messages`
  `renderBackgroundSuccess` never reads. Pre-existing, wider than this feature, worth its
  own issue.
- **`tsuku notices` prints no `Messages` and deletes every notice with an empty `Error`**
  (`cmd_notices.go:47-52`, `:70-74`). This already loses `KindVersionFallback` content
  today. Separable and deliberately not fixed here.

**Two corrections to `DESIGN-nvm-data-root.md`:**

- The driver *"`recipes/**` is absent from the `code` paths-filter, so a recipe-only change
  runs no Go jobs"* is false as written. A separate `recipes` filter exists
  (`.github/workflows/test.yml:388-391`) and gates `integration-linux` (`:471-475`) and
  `integration-macos`; `platform-integration.yml:6-9` triggers on `recipes/**` outright.
  What a recipe-only change does *not* arm is `unit-tests`, `lint`, and `vet`. The
  conclusion survives; the premise does not, and a reviewer will find it.
- The claim that `manager.go:183`'s pre-rename `RemoveAll` deletes the data "immediately,
  on a same-version reinstall" overstates its reach. `IsVersionInstalled`
  (`install_deps.go:508`) returns first on every ordinary path, so `:183` is reachable
  only via `tsuku install --plan <file>` naming an already-installed version
  (`plan_install.go` has no such guard: `ExecutePlan` `:82` → `InstallWithOptions` `:110`)
  or a state/disk divergence. It is real and it is niche; it should not be cited as a
  clock any user is on.
<!-- decision:end -->

---

## Deliberately deferred, with the gate specified

**The safety-gated retry hook at `internal/install/manager.go:149`.** Its only unique
coverage is automatically retrying a migration that already ran and failed — and of the
three failure modes, conflict and `EXDEV` both require a human anyway, so it self-heals
only a transient `EACCES`. Meanwhile the notice already names `tsuku doctor --fix`, which
is the retry a broken user will actually reach, and the hook is the one component
requiring `internal/install` to read shell.d fragment *content*, a coupling nothing has
today.

If it is ever added, its gate is not negotiable and the cheap version of it is unsound:

1. `os.Stat($TSUKU_HOME/data/nvm)` — ENOENT means the new recipe has never run on this
   machine. One syscall, and it is the universal negative case for every user who does not
   have nvm. Everything else sits behind it.
2. `GetToolState("nvm")` → `ActiveVersion`.
3. `os.ReadFile($TSUKU_HOME/share/shell.d/00-env-nvm@<ActiveVersion>.bash)` — the
   `set_env` fragment, **not** the `nvm@<ActiveVersion>.bash` that `install_shell_init`
   writes. Only the `00-env-` one carries the export.
4. `strings.Contains(content, "export NVM_DIR="+shellQuote(dataDir))` — the exact quoted
   form `set_env.go:169` emits, not a bare path substring, which would false-positive on a
   fragment naming a longer path.
5. Only then probe the old locations.

Step 3 cannot be shortcut to `os.Stat(data/nvm/nvm.sh)`. After a rollback to a pre-design
version that file still exists while the active fragment names the old path
(`manager.go:479` selects, never rewrites), and the stat-only gate would migrate and break
the rolled-back install — the same bug one level down.
