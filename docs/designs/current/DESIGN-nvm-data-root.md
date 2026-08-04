---
status: Current
problem: |
  recipes/n/nvm.toml exports NVM_DIR as {install_dir}, which resolves to
  $TSUKU_HOME/tools/nvm-<version>. nvm treats NVM_DIR as its data root, not its
  program directory: every Node version the user installs, every global npm package
  under those versions, the alias/ tree including default, the tarball cache, and
  default-packages all live inside it. The shell.d fragment carrying that export is
  version-keyed and only the active version's fragment reaches the shell, so
  upgrading nvm repoints NVM_DIR at a fresh empty extraction and the user's Node
  versions disappear at the next shell start. Garbage collection then deletes the
  old directory once retention expires. The recipe cannot express any path except
  the versioned tool directory, so fixing this requires deciding where a stable data
  root lives, how nvm exec survives a program/data split, and what tsuku remove nvm
  does to the data.
decision: |
  Give tsuku a new top-level $TSUKU_HOME/data/ tree and point NVM_DIR at
  $TSUKU_HOME/data/nvm through a new per-tool {data_dir} placeholder expanded
  Go-side. A new post-install action, install_program_files, copies nvm.sh and
  nvm-exec into that directory on every install, so nvm exec keeps working and
  placement and repointing are one idempotent operation. set_env itself merge-moves
  data from the two legacy locations immediately after it writes the export, so the
  data and the pointer change in one event and no recipe ordering can be got wrong;
  the same merge-move is called from the removal path as a rescue, and tsuku doctor
  reports a migration that did not complete and can retry it. tsuku remove nvm
  preserves the data root and prints its path; nothing tsuku ships ever deletes it.
rationale: |
  The data root has to leave tools/ entirely, because everything under it is
  version-scoped and tsuku deletes it on two clocks: garbage collection reclaims a
  superseded version once it ages past retention, and the install path removes a tool
  directory before its atomic rename. share/ was rejected as the destination because
  its defining property is that everything in it is regenerable, which is the opposite
  of the policy this data needs. Copies beat symlinks for the program files because
  activate, rollback, and removal-promotion never re-run post-install, so a
  recipe-placed symlink tracks the last-installed version and goes dangling, breaking
  nvm exec while every other subcommand stays green. Migration was built and then
  removed: the export is an absolute literal frozen into a fragment nothing else
  rewrites, so a migration must run in the same post-install execution as set_env — but
  that constrains when it runs, not whether it must be Go, and Go is the one place a
  tool's layout cannot be shipped alongside that tool's recipe.
---

# DESIGN: nvm data root

## Status

Current

## Context and Problem Statement

`recipes/n/nvm.toml` exports `NVM_DIR = "{install_dir}"`. That expands to
`$TSUKU_HOME/tools/nvm-<version>` — a tsuku-managed, versioned, garbage-collected
tool directory. nvm does not treat `NVM_DIR` as a program directory. It is nvm's
data root: `versions/node/` (every installed Node version and, under each, its global
npm packages), `alias/` (including `default`, which decides what a new shell gets),
`.cache/`, and `default-packages`. Upstream nvm keeps program and data in one stable
directory by design and upgrades by swapping the program half in place, precisely so
the data half survives.

This became reachable rather than theoretical when the shell.d lifecycle work landed.
Before it, `set_env` wrote an `env.sh` nothing read, so `NVM_DIR` was never actually
exported and `nvm.sh` self-located to the directory it was sourced from —
`$TSUKU_HOME/share/shell.d`. Node installs accumulated there: odd, but stable, and
never garbage-collected. Making the export work is what moved user data into a
directory tsuku deletes.

Three paths now bear on that data, on very different clocks:

- **Immediately, at the next shell start.** The `set_env` fragment is version-keyed
  (`<target>@<version>.<shell>`) and `ShellDSelection` serves only the active
  version's fragment. After `tsuku update nvm` the shell is told `NVM_DIR` is the new,
  empty directory. `nvm ls` is empty and the `default` alias is gone. This is the
  user-visible bug and it does not involve garbage collection at all.
- **On a plan-install of an already-present version.** `internal/install/manager.go:182`
  removes the existing tool directory before the atomic rename, with no retention
  period. On the normal path `mgr.IsVersionInstalled` short-circuits at
  `cmd/tsuku/install_deps.go:508` first, so this needs an external plan file naming an
  installed version — narrow, but it is a deletion with no timer at all.
- **Later, unattended.** `GarbageCollectVersions` (`internal/updates/gc.go`) asks state
  which versions it records for the tool, keeps only the active and previous, and
  deletes the rest past `DefaultVersionRetention` (7 days). Its single call site is the
  background auto-apply loop (`internal/updates/apply.go:153`), reached only after that
  same tool has successfully auto-updated, and it protects the just-superseded directory
  as `previousVersion`. In practice this needs two nvm releases plus seven days.

The issue frames this as a garbage-collection bug. That framing is the least urgent of
the three: a fix scoped to GC leaves the bug that fires in one second untouched while
addressing the one that fires in a week.

### What the exploration settled

The program/data split is legal. Every `${NVM_DIR}/...` reference in `nvm.sh` is data
nvm creates or reads at runtime, with exactly one exception:
`NODE_VERSION="${VERSION}" "${NVM_DIR}/nvm-exec" "$@"`.

That one exception costs `nvm exec` and `nvm run` (which delegates to `exec`), which
fail with `rc=127`. The naive repair — symlink `nvm-exec` into the data root — silently
does not work, because `nvm-exec` self-locates through an unresolved `BASH_SOURCE[0]`
and re-sources `$DIR/nvm.sh`. Both files must be present.

### Verified by running nvm, not by reading it

Settled empirically against two real nvm releases (v0.40.6 and v0.40.3, whose `nvm.sh`
and `nvm-exec` genuinely differ), extracted as `download_archive` with `strip_dirs = 1`
would leave them, every shell under `env -i`.

**nvm creates `$NVM_DIR` itself, unconditionally, including missing parents.** Every
directory creation in `nvm.sh` is `mkdir -p`, so the first write materializes the root
and everything above it — for `nvm install` and for non-install writes like `nvm alias`
alike. tsuku therefore needs no directory-creating action. It also means a
freshly-installed nvm has no data directory on disk until the user runs `nvm install`,
so anything reasoning about the root's presence must treat absence as normal.

**The full tsuku-shaped layout works, including `nvm exec` and `nvm run`.** With
`nvm.sh` copied into `share/shell.d`, `NVM_DIR` exported by a `00-env-` fragment, and
`nvm.sh` plus `nvm-exec` present in the data root, every command returns 0: `install`,
`alias default`, `ls`, `which`, `exec`, `run`, `use default`, `use system`,
`deactivate`. A brand-new shell auto-uses the default alias.

**Swapping the program underneath the data preserves everything,** and `rm -rf` on the
old tool directory afterwards is a no-op.

**Migration by `mv` is safe.** Across a pure-JS global (`cowsay`) and a native-binary
one (`esbuild`), zero files under a Node install embed the old root path: `bin/` entries
are relative symlinks, shebangs are `#!/usr/bin/env node`, and npm computes
`prefix`/`globalconfig` at runtime from node's location. After moving `versions/` and
`alias/`, `npm ls -g`, both global binaries, the `default` alias, and `exec`/`run` all
work, and a residual grep for the old path returns nothing.

**The one thing that regresses silently.** If the program files in the data root ever go
missing or stale, `install`, `ls`, `use`, `which`, and `alias` all keep working and only
`exec`/`run` break, with `rc=127`. Any regression test must run `nvm exec` *after* a
simulated upgrade, not only after a fresh install.

### Constraints that narrow the space

**No option is recipe-only.** `set_env` expands six placeholders — `{version}`, `{os}`,
`{arch}`, `{install_dir}`, `{work_dir}`, `{libs_dir}` (`internal/actions/util.go:28-37`)
— none stable, and it wraps values in single quotes (`set_env.go:245`) as a deliberate
injection defense, so `$HOME` and `${TSUKU_HOME}` reach the shell literally. The only
recipe-only escape, `{install_dir}/../../share/nvm`, resolves correctly but smuggles a
version string into a path nvm does string-prefix arithmetic on
(`nvm_tree_contains_path`, `nvm_change_path`, `nvm_sanitize_path`). "Smallest diff" is
not a differentiator.

**Nothing stable can live under `tools/`.** Every directory there is a
`<tool>-<version>` pair tsuku expects to reclaim, and it gets deleted on two independent
clocks: `GarbageCollectVersions` removes every version state records once it ages past
retention, and the install path removes an existing tool directory outright before its
atomic rename, with no timer at all. The only path a recipe can name under `tools/` is
`{install_dir}`, which is exactly the directory both of those delete. A directory no
version claims is spared only for as long as reclamation stays per-tool, since a per-tool
sweep cannot prove such a directory belongs to no installed tool at all; #2482 owns the
whole-state pass that can. Either side of that, `tools/` is no place for stable data.

**Dropping the export does not restore upstream behavior.** `install_shell_init`
*copies* `nvm.sh` into `$TSUKU_HOME/share/shell.d`, so self-location lands there, not at
`$HOME/.nvm`.

**A recipe-only change runs no unit or lint job.** `recipes/**` is absent from the
`code` paths-filter (`.github/workflows/test.yml:371-376`). A separate `recipes` filter
at `:389-391` does gate the integration jobs, but neither installs a Node version nor
upgrades nvm.

**Blast radius is one recipe.** nvm is the only recipe of 1,449 that uses `set_env` or
`install_shell_init`. Whatever lands defines the convention rather than following one —
the cheapest possible moment to set it, and the easiest moment to set it wrong.

## Decision Drivers

- The user-visible failure is immediate, not delayed. Candidates are judged against
  "what does the next shell see," not against retention policy.
- `nvm exec` and `nvm run` count as behavior. A fix that silently breaks two subcommands
  trades one bug for another, and would ship green without a test that runs them.
- Moving data without moving the pointer reproduces the bug. The export is an absolute
  literal frozen into a shell.d fragment at the moment the nvm recipe last ran, so the
  data and the export must change in the same event.
- `tsuku remove` must have a coherent, stated answer for the user's Node installs.
- Migration must reach installs that re-run no recipe steps: `mgr.IsVersionInstalled`
  short-circuits at `cmd/tsuku/install_deps.go:508` before `ExecutePlan`,
  `InstallWithOptions`, and the post-install phase, and `--force` does not affect it.
- The messenger has to actually deliver. `renderBackgroundSuccess`
  (`internal/updates/notify.go:155-171`) prints only `"<tool> -> <version>"` and drops
  `Messages`, so a bare `reporter.Warn` on the auto-apply path is persisted and never
  shown.
- `--no-shell-init` must not silently no-op where it matters.
- The fix needs a Go component for unit and lint coverage to run at all.
- Tests assert observable behavior, not intermediate state.

## Considered Options

### Decision 1 — where the data root lives, how the recipe names it, what removal does

**Chosen: a new top-level `$TSUKU_HOME/data/`, per-tool, so `$TSUKU_HOME/data/nvm`;
named by a per-tool `{data_dir}` placeholder; `tsuku remove nvm` preserves it and prints
its path, with no `delete_dir` cleanup action ever recorded and no tsuku command that
deletes it.**

- **`$HOME/.nvm`, exported explicitly.** Rejected. Because `nvm.sh` hardcodes
  `"${NVM_DIR}/nvm-exec"`, this is not "let nvm own its directory" — it is "tsuku writes
  program files into `$HOME/.nvm`". If the directory does not exist tsuku creates it, and
  a user who later runs upstream's installer hits `install_nvm_from_git()`, which
  `git init`s in place. If it is already an upstream git working tree, tsuku clobbers two
  tracked files and upstream's only upgrade path force-reverts them with the reflog
  expired immediately after. tsuku also cannot clean up after itself: cleanup actions
  resolve as `filepath.Join(m.config.HomeDir, ca.Path)` (`remove.go:418`), so a cleanup
  action structurally cannot name `$HOME/.nvm`. Its two headline arguments both reverse
  on inspection. The registry convention (pyenv, rbenv, rustup, asdf delegate to the
  tool's own root) holds because those tools write *nothing* into the data root — tsuku
  installs a binary and the tool bootstraps itself; nvm has no binary. And the Homebrew
  precedent, checked against the formula, writes nothing into `$HOME/.nvm` either: it
  writes a shim into Homebrew's own prefix that symlinks from `opt_libexec` — a
  package-manager-owned stable path — and exports `NVM_DIR` *conditionally*
  (`[ -z "$NVM_DIR" ] && export ...`). tsuku's `set_env` emits an unconditional export of
  a single-quoted literal and cannot express the conditional half, so this option is
  strictly more aggressive than the precedent invoked for it. The reachable form of it
  needs a version-independent tsuku-owned path to symlink at, which is the chosen option's
  machinery; the dichotomy was false.
- **`share/nvm`.** Rejected. `share/` is demonstrably safe today — nothing enumerates or
  prunes it — but the value of a separate tree is that it can carry a deletion policy, and
  `share/` cannot carry this one because its existing contents need the opposite.
  `shell.d`, `hooks`, and `completions` are tsuku-generated and regenerable by definition,
  and `RebuildShellCache` already rewrites inside that tree. "Nothing under `data/` is
  ever deleted except by an explicit user command" is a true statement about a new tree
  and a false one about `share/`. The cost argument that motivated `share/` evaporated on
  measurement: adding the tree is three lines in `internal/config/config.go`, following
  the pattern `ShareDir` already establishes, and none of the test `Config{}` literals
  need touching.
- **`state/nvm`.** Rejected on vocabulary: `state.json` already claims the word for
  tsuku's reconstructible install ledger. `data/` matches `XDG_DATA_HOME`'s sense against
  `XDG_STATE_HOME`'s.
- **A bare-root `{data_dir}` → `$TSUKU_HOME/data` with recipe-side composition.**
  Rejected: it re-permits `{data_dir}/nvm-{version}`, which is the same defect in a new
  spelling; it lets any recipe name a sibling tool's data; and it turns the validator
  guardrail from a string equality into a heuristic. The `{libs_dir}` precedent it appeals
  to is a bare root *because it exists for cross-tool reference*, which a data root never
  needs. The binding precedent is `{install_dir}`, already per-tool.
- **`{tsuku_home}` or `{share_dir}`.** Rejected: both leak the whole path namespace into
  recipes to solve one problem, and neither makes the guardrail enforceable.
- **Recording a `delete_dir` cleanup so `tsuku remove nvm` takes the data.** Rejected.
  `ReapVersion` (`remove.go:400`) executes cleanup actions from background GC, and the
  cross-version guard skips a path only on exact string equality against *another
  installed version's* recorded actions — which `--no-shell-init` guarantees will be
  absent (`set_env.go:100-104`), as will pre-existing installs. Install v1 normally,
  install v2 with `--no-shell-init`, let v1 age past retention, and GC reaps the user's
  entire Node estate. That is this design's own bug entering through a different door.
  `delete_dir` also has zero producers today; irreplaceable user data is the wrong place
  to break in an untested arm.
- **Preserving the data with no reclaim command.** Rejected as the honest complaint
  against the chosen option, and cheap to answer.

### Decision 2 — how `nvm.sh` and `nvm-exec` reach the data root

**Chosen: a new post-install action, `install_program_files`, that copies named files
from the tool's install directory into a declared stable directory on every install.**

The decisive mechanical fact: `Manager.Activate` (`internal/install/manager.go:417-481`)
repoints binary symlinks and rebuilds shell caches but **never runs the post-install
phase**; `Rollback` delegates to it, and removal-promotion re-symlinks only binaries.
Anything a recipe action materializes tracks the last-installed version, not the active
one.

- **Symlinks.** Verified working end to end and free of disk cost. Rejected because of
  the `Activate` gap: a recipe-placed symlink goes dangling once the version it points at
  is removed or reaped, producing `nvm exec` `rc=127` with every other subcommand still
  green — the exact silent partial regression this design is trying not to ship. With
  copies there is nothing to dangle, and placement and repointing become literally one
  idempotent operation, so there is no separate step a future change can forget. Copying
  is also not a new idiom: `install_shell_init` already copies `nvm.sh` rather than
  linking it, and `internal/shellenv/doctor.go:91-93` flags any symlink under `share/` as
  a security risk.
- **Hard links.** Rejected: they fail across filesystems, a stale link pins the old tool
  directory's inode so GC never reclaims the space, and they offer nothing a copy does not.
- **`run_command`.** Rejected on four counts: its `{install_dir}` expands from the
  *staging* directory, so the command would have to hardcode the tools path — exactly the
  pattern its own `Preflight` warns about, and only a warning, so it would ship;
  `RequiresNetwork() == true` upgrades every sandbox validation of nvm to the networked
  heavyweight image; it is `false` in `ActionEvaluability`; and it is unrestricted `sh -c`
  for a two-file copy. It also adds no Go file, which is a defect here rather than a
  virtue.
- **Extending `install_shell_init` or `set_env`.** Rejected as a conflation, and more
  expensive than it looks: `install_shell_init` does not implement `PhaseDeclarer` and
  runs in the install phase reading the staging directory, so teaching it to place program
  files would force it to `post-install` for every existing user. The two jobs also
  disagree about `--no-shell-init`, which is precisely where a merged action would branch.
- **Install-path Go code keyed off a `VersionState` field.** The right mechanism *if* the
  data root must track the active version, and it inherits repointing at all four sites
  for free. Rejected as disproportionate: with copies it buys only a cosmetic one
  point-release skew, at roughly ten files, for the single recipe that needs it. Recorded
  as the named upgrade path if activate/rollback fidelity ever matters.

### Decision 3 — how existing installs reach the new root

**Chosen: they do not, in this design. The migration is deferred to a design of its own
and users move their data by hand in the meantime.**

A migration was designed and built, then removed before merge. It is worth recording why,
because the reasoning is about tsuku's architecture rather than about nvm.

The mechanism worked: a merge with no deletion primitive, called from `set_env` so the
data and the export changed in one event, with a removal-path rescue and a `doctor`
check. What it cost was a file of nvm's on-disk layout — the names of the directories
nvm keeps under its root — compiled into the tsuku binary. nvm is not an embedded recipe.
It is fetched from a registry that updates on its own cadence. So the arrangement put a
tool's layout knowledge in the one artifact that cannot be updated with that tool's
recipe: if nvm adds a data path, the recipe can be corrected by a registry push while the
compiled list needs a CLI release.

The timing argument that justified putting it in Go does not survive inspection either.
The export is an absolute path frozen into a version-keyed fragment, and nothing rewrites
a fragment outside an install of the tool — so the migration must run in the same
post-install execution as `set_env`. That forces *when*, not *where*. A recipe-declared
step in the same phase satisfies it equally.

What a correct version needs is for a recipe to describe its own data layout and carry
its own migration, so the knowledge ships and versions with the recipe. That is a
recipe-schema design, and it deserves to be worked out against more than one example
rather than inferred from nvm. Tracked in #2472.

Deferring it costs a manual `mv`. For the population every released tsuku created that is
all it costs: their data is in `share/shell.d`, which nothing reclaims, so it sits there
until moved. The population whose data is in a directory that *is* reclaimed — on the
7-day retention timer — exists only on unreleased `main`, a window hours wide, but their
data is genuinely on a clock and the docs call that case out separately rather than
folding it into the harmless one.

- **A general migration hook on the install path (`manager.go:149`), the `EnsureEnvFile`
  shape.** Rejected on correctness before the migration was dropped at all, and the
  finding stands for whatever replaces it: every event in its extra coverage is an event
  at which the tool's recipe has not run, so the export still names the old location and
  moving the data breaks a working install. Both validators who advocated it verified the
  mechanism and withdrew it.
- **`doctor` alone.** Nothing in tsuku's non-test Go ever mentions `tsuku doctor` outside
  `doctor.go`, so it is a pull surface nothing points at.
- **A one-shot `tsuku migrate` command.** No precedent: every maintenance surface in the
  tree is either permanently idempotent (`doctor --fix`, `cache cleanup`) or fully
  automatic and shape-detecting.
- **Using the new release's file list as a program manifest to move "everything else".**
  Applies the *new* release's manifest to the *old* release's directory, so a file
  dropped between releases is misclassified as user data — emptying the directory
  garbage collection keeps as the rollback target.

### Decision 4 — the guardrail

**Chosen: ship it — a variable-name denylist at error severity in
`internal/recipe/validator.go`.**

- **Do not ship.** Considered seriously: the rule has exactly one subject today, so it
  cannot be validated against real usage, and a denylist of variable names is a guess
  about the future. Rejected because the timing argument is stronger — nvm is the only
  `set_env` consumer of 1,449 recipes, so the rule costs nobody anything now and there
  will never be a cheaper moment. Its failure mode is also benign: a variable omitted
  from the list gets the same outcome as not shipping, with no new harm.
- **A structural rule instead of a denylist** (flag any `set_env` value containing
  `{install_dir}`). Rejected on false positives: exporting a version-scoped `*_HOME` that
  genuinely is the program directory is legitimate.
- **Warning severity.** Rejected as a distinction without a difference: `--strict` sets
  `Valid = false` on any warning (`cmd/tsuku/validate.go:139-141`) and the Validate
  Recipes job runs `--strict`, so a warning turns that job red exactly as an error does.
  Error severity states the intent honestly.

## Decision Outcome

`NVM_DIR` names `$TSUKU_HOME/data/nvm`, a per-tool subdirectory of a new top-level tree
whose stated policy is that nothing in it is ever deleted except by an explicit
user-initiated command. The recipe names it with `{data_dir}`, a per-tool placeholder
expanded Go-side. `nvm.sh` and `nvm-exec` are copied into that directory by a new
`install_program_files` post-install action on every install, so `nvm exec` and `nvm run`
keep working and an upgrade repoints them by construction. Data that an earlier tsuku left
somewhere else is not moved: that needs recipes to carry their own migrations, which is a
separate design.

`tsuku remove nvm` deletes the tool and the two program-file copies and leaves the
estate, printing where it is. No tsuku command deletes it; the documented reclaim is
`rm -rf` on the printed path. That is the explicit answer to what removal does, and it is
deliberately asymmetric — tsuku will put user data somewhere it can guarantee, and will
not take it away.

The two pieces cohere because they answer different failure modes. `{data_dir}` answers
the upgrade, which is what destroys the data. The copies answer `Activate`, which never
re-runs post-install and would otherwise leave `nvm exec` pointing at a version that no
longer exists.

## Solution Architecture

### Paths

| Path | Contents | Lifetime |
|---|---|---|
| `$TSUKU_HOME/tools/nvm-<version>/` | the nvm release, extracted | versioned; GC-eligible |
| `$TSUKU_HOME/share/shell.d/nvm@<version>.<shell>` | a copy of `nvm.sh` | version-keyed; cleaned with the version |
| `$TSUKU_HOME/share/shell.d/00-env-nvm@<version>.<shell>` | `export NVM_DIR='<data root>'` | version-keyed |
| `$TSUKU_HOME/data/nvm/nvm.sh`, `nvm-exec` | copies, refreshed every install | tsuku-owned; removed with the tool |
| `$TSUKU_HOME/data/nvm/{versions,alias,.cache,default-packages,current}` | the user's Node estate | **never deleted by any tsuku command** |

### Components

- **`internal/config`** — a `DataDir` field alongside `ShareDir`, one entry in the
  `DefaultConfig` literal, and one in `EnsureDirectories` behind the same `if != ""`
  guard `ShareDir` uses, created `0700`.
- **`internal/actions` shared helper** — `dataDir(ctx)` returning
  `filepath.Join(filepath.Dir(ctx.ToolsDir), "data", <recipe name>)`, reusing
  `envTargetName`'s existing single-path-segment validation (it already rejects `.`,
  `..`, separators, and `@`). Expansion stays local to the actions that consume it rather
  than widening `GetStandardVars`, which would push `{data_dir}` into all nine callers to
  serve one recipe; promoting it later is mechanical.
- **`install_program_files`** — post-install via `PhaseDeclarer`, sourcing from
  `ctx.ToolInstallDir` (populated only after the atomic rename) and hard-erroring when it
  is empty. It takes **only** a `files` list; the destination is always the tool's own
  data directory, computed Go-side. There is deliberately no `dir` parameter — see
  Security Considerations, where dropping it removes an entire class of finding. Each
  entry is resolved with `filepath.EvalSymlinks` and required to stay inside the resolved
  tool directory, opened `O_RDONLY|O_NOFOLLOW` with the type check done on the file
  handle, and written to a temp name opened `O_CREATE|O_EXCL|O_NOFOLLOW` before the
  rename, so a concurrent `nvm exec` never sees a partial file. Mode is `0755` if the
  source has any execute bit and `0644` otherwise — `nvm-exec` must stay executable, and
  a mode derived from tar-header bytes is needless attack surface. Records a
  `delete_file` cleanup per placed file. Ignores `--no-shell-init` (a data root is not a
  shell.d file, and a user wiring nvm up by hand still needs `nvm-exec`), which also
  keeps GC's cross-version guard effective. An `ActionEvaluability` entry is mandatory —
  `install_completions` is already missing from that map, which silently makes every
  recipe using it non-reproducible.

### The `Activate` gap this does not close

`Manager.Activate` flips `ActiveVersion` and re-selects shell fragments without ever
rewriting one, and `Rollback` delegates to it. So rolling back to a version installed
under the old recipe repoints `NVM_DIR` at that version's baked path while the data sits
in `data/nvm`. The data is not destroyed and rolling forward restores the view. Closing it
means making `Activate` re-run `set_env`, which is the same gap
`DESIGN-shell-d-lifecycle.md` reasoned about and is wider than this issue.

## Implementation Approach

1. **`internal/config`** — add `DataDir`, its `DefaultConfig` entry, and its
   `EnsureDirectories` entry **behind the same `if != ""` guard `ShareDir` has**
   (`config.go:390-394`). Without the guard, a partial `Config{}` literal makes
   `filepath.Join("", "data")` = `"data"` and `os.MkdirAll` creates a stray relative
   directory in the test process's working directory; such literals exist at
   `cmd/tsuku/doctor_test.go:48` and `post_install_test.go:26`. A shared constant for
   the `"data"` segment, so the config field and the action-side helper cannot drift.
2. **`{data_dir}` and the shared helper** — `dataDir(ctx)` in `internal/actions` plus
   `set_env` expansion. The unexpanded-placeholder guard is a **runtime check in the
   expansion path**, mirroring `CheckUnexpandedDepVars` (`internal/actions/util.go:56-69`),
   not a list of expanding actions inside `validator.go`: a literal list there drifts
   from the actions that actually expand, and the failure mode is a hard `--strict`
   error against a correct recipe.
3. **`install_program_files`** — action, registration, `ActionEvaluability` entry,
   preflight, unit tests. No `dir` parameter.
4. **The recipe** — `set_env` value, the `install_program_files` step, and a header
   comment describing the model actually in force: where the data lives, that no tsuku
   command deletes it, that data from an earlier tsuku is not moved automatically, and
   that a pre-existing `$HOME/.nvm` is left alone.
5. **`docs/` note** — where `data/` lives, that `rm -rf $TSUKU_HOME` now destroys user
   data, the manual reclaim command, and the manual `mv` for data an earlier tsuku left
   elsewhere. This is the only mitigation offered for those consequences, so it is a step
   rather than a claim.
6. **Plugin skills** — `tsuku-recipe-author` (the new action, the new placeholder) and
   `tsuku-user` (the `data/` tree, what removal does), per the repo's plugin-maintenance
   table.

### Testing

The fast guard goes in `cmd/tsuku/shelld_lifecycle_test.go` — the only place install,
actions, and shellenv meet. Its harness already stands up a real `$TSUKU_HOME` under
`t.TempDir()` with a real `Manager` and a bash reader, needs no network, runs in about
1.7 seconds, and carries no `testing.Short()` guard, so it executes under
`go test -short ./...` on every PR that touches Go. Seed a user file under the path the
shell was told to use, install a second version, remove the first, and assert the file
survives and the shell still names the same root — a test that fails today.

The slow real-nvm test — install nvm, install a Node version, set a `default` alias,
upgrade nvm, then assert in a **fresh shell** that `nvm ls` still lists the version, the
`default` alias survives, and `nvm exec <v> node -v` still works — goes in a shell script
under `test/scripts/` invoked from `integration-tests.yml`, matching
`test-checksum-pinning.sh` and `test-homebrew-recipe.sh`. The alias assertion is not
optional decoration: `nvm alias default` is named explicitly in the issue's acceptance
criteria. **Adding the job is its own step**, with `TSUKU_REGISTRY_URL` pointed at the
head ref the way the existing jobs do — otherwise the modified recipe is never fetched
and the test passes against the old one. It does not belong in a
`@critical` Gherkin scenario, which would put a multi-minute network download on every PR
in the repo; and an untagged scenario runs only on PRs touching `test/functional/**`, so
it would pass green once and lie dormant.

Every guard added is mutation-tested by hand: apply a one-line defect, confirm the test
fails, revert. The repo has no mutation-testing tooling, so this is a manual discipline
and the applied defects are recorded in the PR body.

## Security Considerations

Reviewed against the code, not the prose. Two findings changed the design; one claim the
design originally made turned out to be false.

**A recipe-controlled destination is a write-anywhere primitive, so there is no
destination parameter.** `install_program_files` originally took a `dir` the recipe
named, validated to be inside `$TSUKU_HOME` and not under `tools/`. That predicate admits
`$TSUKU_HOME/share/shell.d`, which `RebuildShellCache` concatenates into the init cache
the user's shell sources — and `ShellDSelection.excludes` returns *false* for a path it
does not know (`internal/shellenv/selection.go:35-40`), so an unrecorded fragment is not
filtered out, it is sourced. It also admits `$TSUKU_HOME/bin`, which tsuku's own check
treats as a PATH location. Since the file contents come from the release tarball, that is
a route for a recipe to put attacker-controlled code into the user's shell startup — and
because this action deliberately ignores `--no-shell-init`, it would bypass the one flag
a user has for saying "do not touch my shell". The destination is therefore computed
Go-side from `ctx.ToolsDir` and the recipe name, reusing `envTargetName`'s existing
single-path-segment validation. No recipe-controlled path survives to validate. A future
tool that genuinely needs a second location can argue for a parameter then.

**The archive extractor does not contain tarball symlinks, so the new actions defend
themselves.** An earlier draft of this section claimed a symlink planted in a release
tarball could not read outside the tool directory. That is false.
`isPathWithinDirectory` (`internal/actions/extract.go:19-35`) compares cleaned strings
with `strings.HasPrefix` and `validateSymlinkTarget` (`:39-55`) resolves with
`filepath.Join`; both are purely lexical, so a chain whose lexical resolution stays inside
the destination while its real resolution leaves it passes both. A three-entry tarball —
symlink `a -> "."`, symlink `b -> "a/.."`, then a regular file `b/pwned` — writes outside
the destination directory, verified by driving it through `extractTarGz`. That is
pre-existing, wider than this feature, and filed separately; the consequence here is that
`install_program_files` cannot inherit containment from extraction. Each `files` entry is
resolved with `filepath.EvalSymlinks` and required to stay inside the resolved tool
directory — a lexical `..` rejection says nothing about a symlinked directory component —
then opened `O_RDONLY|O_NOFOLLOW` with the regular-file check done on the *file handle*,
so the type check and the read see the same object and the TOCTOU window closes rather
than narrowing. The temp destination is opened `O_CREATE|O_EXCL|O_NOFOLLOW`. The shared
`copyFile` helper cannot be reused: it uses plain `os.Create`, which follows a symlink at
the destination and preserves no mode.

**File modes are not derived from tarball bytes.** `0755` when the source has any execute
bit, `0644` otherwise. `data/` itself is created `0700`, matching `share/shell.d` and
`share/completions` — the two places in the tree holding things tsuku treats as sensitive
— rather than `tools/`'s `0755`. This affects only the root; nvm creates `versions/` and
the rest itself under the user's umask.

**No new destructive surface.** Dropping `--purge` removes the only one this design
proposed. It would have built an `os.RemoveAll` target from `args[0]`, which
`cmd/tsuku/remove.go:30-39` splits on `@` with no path-segment validation, and its
"tolerate an absent state entry" requirement would have removed the state lookup that is
currently the only thing standing between a traversal name and that call —
`filepath.Join` cleans before it returns, so `../../../etc/passwd` escapes silently. The
absence of a `ValidateToolName` in the tree is pre-existing and filed separately.

**No new privilege, no new network, no new secret.** The action is deterministic, reads
only the tool's own install directory, and declares `RequiresNetwork() == false`. The
single-quoting of `set_env` values is preserved: `{data_dir}` is expanded Go-side and the
result still passes through `shellQuote`, so adding the placeholder does not widen the
injection surface. The action asserts `filepath.Dir(ctx.ToolsDir) == config.HomeDir`
rather than assuming it, since the cleanup path resolves recorded paths against
`config.HomeDir` and directly-constructed `Config` literals exist in the test suite.

## Consequences

**What becomes true.** Upgrading nvm preserves Node versions by construction rather than
by machinery firing at the right moment. A plan-install of an already-present version and
GC both become no-ops with respect to user data. `nvm exec` and `nvm run` keep working,
verified empirically for this layout. A user who updates nvm has their estate follow
`NVM_DIR` in the same process, with no window in which the two disagree. `tsuku remove nvm`
preserves what it promises to preserve.

**What becomes easier.** The next recipe with a data root has `{data_dir}` waiting, and
the validator rule makes the original bug unrepresentable in recipe form. tsuku gains a
directory it can name in `doctor` output and size reporting.

**What becomes harder, honestly.**

- **`rm -rf $TSUKU_HOME` is now destructive to user data, and there is no code-level
  fix.** `tsuku remove <tool>` uninstalls a tool, but nothing removes the tsuku
  installation itself — `self-update` has no counterpart — which is why wiping the
  directory became the folk answer.
  This is the strongest surviving argument against the choice, and it is now sharper
  because there is no `--purge` to point at. It gets a documentation step and a
  BREAKING CHANGE note in the commit body rather than a promise. Seeing, sizing and
  reclaiming tool data — and what "reset tsuku" should mean now — is tracked in #2477.
- **`make clean` destroys a contributor's dogfooded data root,** since dev builds default
  `TSUKU_HOME` to `.tsuku-dev`. Contributor-facing; worth a note in CONTRIBUTING.
- **Nobody's existing Node installs are moved for them.** The next `tsuku update nvm`
  repoints `NVM_DIR` at `data/nvm` and `nvm ls` comes up empty until the user moves what
  an earlier tsuku left in `share/shell.d`. Nothing is destroyed — that tree is not
  garbage-collected — but nothing announces it either, beyond the recipe header comment
  and the docs note. This is the accepted cost of not compiling a tool's layout into the
  CLI, and it is the thing recipe-carried migration is meant to fix.
- **`tsuku rollback nvm` to a version installed under the old recipe repoints `NVM_DIR` at
  the old path while the data sits at `data/nvm`.** `Activate` flips `ActiveVersion` and
  re-selects fragments without ever rewriting one, so the rolled-back version's fragment
  still carries its own baked path. The data is not destroyed and rolling forward restores
  the view. Fixing it properly means
  making `Activate` re-run `set_env`, which is the same gap `DESIGN-shell-d-lifecycle.md`
  reasoned about, wider than this issue, and a change to the activate path this PR should
  not make. Recorded as a follow-up.
- **A `--no-shell-init` user gets program files but no export.** Consistent with what
  the flag means, and stated rather than discovered.
- **tsuku now owns a directory that can be the largest thing on the machine, and ships no
  command to delete it.** That is the deliberate shape of the answer to "what does
  `tsuku remove nvm` do": nothing tsuku ships ever removes the estate. Removal prints the
  path, and the documented reclaim is `rm -rf` on that path. A `--purge` flag was designed
  and cut — it served no acceptance criterion, and it would have built an `os.RemoveAll`
  target out of an unvalidated command-line argument.
- **A user who runs upstream's `install.sh` with tsuku's export live** gets a stray `.git`
  in `data/nvm` and tsuku's two files replaced, which the next `tsuku update nvm`
  overwrites back. `checkout -f` carries no `-d` and there is no `git clean`, so untracked
  `versions/` survives — untidy, not destructive.

## Deliberately out of scope

- **Recipe-carried migration (#2472).** The reason this design ships no migration: a
  recipe needs to be able to describe its own data layout and carry its own migration
  steps, so the knowledge ships and versions with the recipe rather than being compiled
  into the CLI. nvm is the motivating example, not the whole requirement.
- **Making `Activate` re-run `set_env`** so activate and rollback rewrite fragments. The
  general fix for the gap above.
- **`set_env` gaining `if_unset` semantics** so a user-set `NVM_DIR` wins regardless of
  sourcing order. Raised during the location decision and deferred deliberately: it changes
  `set_env`'s emitted form for every future consumer and stands on its own.
- **`GarbageCollectVersions` deleting other tools' directories on recipe-name prefix
  collisions (#2474)** (`git`/`git-lfs`, `docker`/`docker-compose` — 59 pairs, verified
  empirically). A real bug, independent of this one. Fixed since, in #2491, by sourcing
  the candidate versions from state instead of from the directory listing.
- **The archive extractor's containment is purely lexical (#2473).**
  `isPathWithinDirectory` and `validateSymlinkTarget` (`internal/actions/extract.go:19-55`)
  compare cleaned strings and resolve with `filepath.Join`, so a symlink chain in a
  release tarball escapes the destination directory — demonstrated with a three-entry
  archive. Pre-existing, wider than this feature, and the most serious thing the security
  review found.
- **There is no `ValidateToolName`.** `cmd/tsuku/remove.go:30-39` splits `args[0]` on `@`
  with no path-segment validation; today only the state lookup stops a traversal name
  from reaching a path construction. `envTargetName` (`internal/actions/set_env.go:209-217`)
  already has the check to lift.
- **A recipe-validator rule rejecting a known data-root variable exported at a
  version-scoped value.** Designed and deferred: it would have exactly one subject today,
  and that subject is fixed here, so the risk of a rule bug turning the Validate Recipes
  job red across every recipe outweighs prospective value. The cheapest moment argument
  is about *when*, not *whether*.
- **`install_completions` missing from `ActionEvaluability`**, which silently makes every
  recipe using it non-reproducible.
- **Post-install failures being silently discarded on the background path**, and
  `tsuku notices` printing no `Messages`. Pre-existing and wider than this feature.
- **`nix_portable.go` and `util.go` hardcoding `~/.tsuku`** from `os.UserHomeDir()`,
  escaping `TSUKU_HOME` isolation.
- Sibling recipe-review follow-ups, and reverting or reworking the shell.d lifecycle
  mechanism.
