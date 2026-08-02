---
status: Proposed
problem: |
  recipes/n/nvm.toml exports NVM_DIR as {install_dir}, which resolves to
  $TSUKU_HOME/tools/nvm-<version>. nvm treats NVM_DIR as its data root, not its
  program directory: every Node version the user installs, every global npm package
  under those versions, the alias/ tree including default, the tarball cache, and
  default-packages all live inside it. Because the shell.d fragment carrying that
  export is version-keyed and only the active version's fragment reaches the shell,
  upgrading nvm repoints NVM_DIR at a fresh empty extraction and the user's Node
  versions disappear at the next shell start. GarbageCollectVersions then deletes
  the old directory once retention expires, and manager.go's pre-rename cleanup
  deletes it immediately on a same-version reinstall. The recipe cannot express any
  path except the versioned tool directory, so the fix requires deciding where a
  stable data root lives, how nvm exec survives a program/data split, what
  tsuku remove nvm does to the user's Node installs, and how existing installs are
  migrated.
---

# DESIGN: nvm data root

## Status

Proposed

## Context and Problem Statement

`recipes/n/nvm.toml` exports `NVM_DIR = "{install_dir}"`. That expands to
`$TSUKU_HOME/tools/nvm-<version>` — a tsuku-managed, versioned, garbage-collected
tool directory. nvm does not treat `NVM_DIR` as a program directory. It is nvm's
data root: `versions/node/` (every installed Node version and, under each,
its global npm packages), `alias/` (including `default`, which decides what a new
shell gets), `.cache/`, and `default-packages`. Upstream nvm keeps program and data
in one stable directory by design and upgrades by swapping the program half in
place, precisely so the data half survives.

This became reachable rather than theoretical when the shell.d lifecycle work
landed. Before it, `set_env` wrote an `env.sh` nothing read, so `NVM_DIR` was never
actually exported and `nvm.sh` self-located to the directory it was sourced from —
`$TSUKU_HOME/share/shell.d`. Node installs accumulated there: odd, but stable, and
never garbage-collected. Making the export work is what moved user data into a
directory tsuku deletes.

Three deletion or invalidation paths now bear on that data, and they fire on
different clocks:

- **Immediately, at the next shell start.** The `set_env` fragment is version-keyed
  (`<target>@<version>.<shell>`) and `ShellDSelection` serves only the active
  version's fragment. After `tsuku update nvm`, the shell is told `NVM_DIR` is the
  new, empty directory. `nvm ls` is empty and the `default` alias is gone. This is
  the user-visible bug, and it does not involve garbage collection at all.
- **Immediately, on a same-version reinstall.** `internal/install/manager.go:183`
  removes the existing tool directory before the atomic rename. There is no
  retention period on this path.
- **Later, unattended.** `GarbageCollectVersions` (`internal/updates/gc.go`) keeps
  only the active and previous versions and deletes the rest past
  `DefaultVersionRetention` (7 days). Its single call site is the background
  auto-apply loop (`internal/updates/apply.go:153`), reached only after that same
  tool has successfully auto-updated, so in practice this is weeks rather than days.

The issue frames this as a garbage-collection bug. The exploration found that
framing is the least important of the three, and that fixes scoped to GC leave the
bug that fires in one second untouched while addressing the one that fires in a
week.

### What the exploration settled

The program/data split is legal. Every `${NVM_DIR}/...` reference in `nvm.sh` is
data nvm creates or reads at runtime, with exactly one exception:
`NODE_VERSION="${VERSION}" "${NVM_DIR}/nvm-exec" "$@"`. This was confirmed
empirically — a real `nvm install`, `use`, `which`, `alias`, and `ls` all work with
`nvm.sh` sourced from one directory and `NVM_DIR` pointing elsewhere. nvm creates
its own data tree wherever it is pointed (`mkdir -p` in `nvm alias`, `nvm ls`, and
`nvm_install_binary`), so a fresh or empty root needs no bootstrap.

That one exception costs `nvm exec` and `nvm run` (which delegates to `exec`), which
fail with `rc=127`. The naive repair — symlink `nvm-exec` into the data root —
silently does not work, because `nvm-exec` self-locates through an unresolved
`BASH_SOURCE[0]` and re-sources `$DIR/nvm.sh`. Both files must be present.

### Verified by running nvm, not by reading it

Three questions were settled empirically against two real nvm releases (v0.40.6 and
v0.40.3, whose `nvm.sh` and `nvm-exec` genuinely differ), extracted exactly as
`download_archive` with `strip_dirs = 1` would leave them, with every shell run under
`env -i`.

**nvm creates `$NVM_DIR` itself, unconditionally, including missing parents.** Every
directory creation in `nvm.sh` is `mkdir -p`, so the first write materializes the root
and everything above it. This holds for `nvm install` and for non-install writes like
`nvm alias`. It removes a whole branch of work: **tsuku needs no directory-creating
action** — no new action, registry entry, plan-evaluability entry, preflight, or
tests. It also means a freshly-installed nvm has no data directory on disk at all
until the user runs `nvm install`, so anything reasoning about the data root's
presence must treat absence as normal rather than as corruption. (Note this is the
opposite of `install.sh`, which refuses to create a non-default root — tsuku extracts
the tarball itself and never runs it, so `nvm.sh`'s permissive behavior governs.)

**The full tsuku-shaped layout works, including `nvm exec` and `nvm run`.** With
`nvm.sh` copied into `share/shell.d` (as `install_shell_init` does), `NVM_DIR`
exported by a `00-env-` fragment, and `nvm.sh` plus `nvm-exec` symlinked into the data
root, every command returns 0: `install`, `alias default`, `ls`, `which`, `exec`,
`run`, `use default`, `use system`, `deactivate`. A brand-new shell auto-uses the
default alias and `node -v` resolves to the nvm-managed version.

**Swapping the program underneath the data preserves everything.** Repointing both
symlinks from v0.40.6 to v0.40.3 and rewriting the version-keyed fragments — precisely
what a tsuku upgrade plus its shell.d cleanup does — leaves the Node version, the
`default` alias, and `exec`/`run` intact, with `nvm --version` correctly reporting the
new program version. Then `rm -rf` on the old tool directory, the exact event this
issue is about, is a **no-op**.

**Migration by `mv` is safe.** Across a pure-JS global (`cowsay`) and a
native-binary one (`esbuild`), zero files under a Node install embed the old root
path; the `bin/` entries are relative symlinks, shebangs are `#!/usr/bin/env node`,
and npm computes `prefix`/`globalconfig` at runtime from node's location rather than
storing them. After moving `versions/` and `alias/` to a new root, `npm ls -g`, both
global binaries, the `default` alias, and `exec`/`run` all work, and a residual grep
for the old path returns nothing.

**The one thing that regresses silently.** If an upgrade deletes the old tool
directory *without* repointing the symlinks, `install`, `ls`, `use`, `which`, and
`alias` all keep working and only `exec`/`run` break, with `rc=127`. The two symlinks
are load-bearing upgrade state, and because the breakage hides behind the common
commands, an upgrade that forgets to repoint them would very plausibly ship unnoticed.
Repointing belongs inside the placement action rather than in a separate step, and the
regression test must run `nvm exec` *after* a simulated upgrade, not only after a
fresh install.

Three constraints narrow the solution space sharply:

1. **No option is recipe-only.** `set_env` expands six placeholders — `{version}`,
   `{os}`, `{arch}`, `{install_dir}`, `{work_dir}`, `{libs_dir}`
   (`internal/actions/util.go:28-37`) — none of which yields a stable path, and it
   wraps values in single quotes (`set_env.go:245`) as a deliberate injection
   defense, so `$HOME` and `${TSUKU_HOME}` reach the shell literally. The only
   recipe-only escape, `{install_dir}/../../share/nvm`, resolves correctly but
   smuggles a version string into a path nvm does string-prefix arithmetic on
   (`nvm_tree_contains_path`, `nvm_change_path`, `nvm_sanitize_path`). "Smallest
   diff" is therefore not a differentiator.
2. **Nothing stable can live under `tools/`.** `GarbageCollectVersions` matches a
   raw `strings.HasPrefix(name, toolName+"-")` and `ReapVersion` returns nil for an
   unrecognized version rather than objecting, so `os.RemoveAll` runs anyway.
3. **Dropping the export does not restore upstream behavior.**
   `install_shell_init` *copies* `nvm.sh` into `$TSUKU_HOME/share/shell.d`, so
   self-location lands there, not at `$HOME/.nvm`. Using nvm's own default root is
   viable only as an explicit export of that path.

`$TSUKU_HOME/share/` is safe as written: it already exists, already holds durable
non-tool-versioned state (`hooks/`, `shell.d/`, `completions/`), and no code path
enumerates it for deletion.

### What remains open

- **Where the data root lives.** A tsuku-owned stable path, or nvm's own default
  root at `$HOME/.nvm`. This is the primary decision and it is gated on a product
  question: *should `tsuku remove nvm` be able to delete the user's Node installs?*
  If yes, the data must live inside `$TSUKU_HOME` and tsuku must grow its first
  `delete_dir`-emitting cleanup (with an explicit GC exclusion, since cleanup
  actions are also run by `ReapVersion`). If no, `$HOME/.nvm` is right and tsuku
  should hold no opinion about it.
- **How `nvm.sh` and `nvm-exec` get placed in the data root.** No tsuku action can
  create a symlink or copy a file to a path outside the staging directory today. The
  candidates are a narrow new action — which could use the existing `AtomicSymlink`
  and `ValidateSymlinkTarget` in `internal/install/symlink.go`, both already generic
  rather than binary-specific — or `run_command`, which works but is non-evaluable,
  declares `RequiresNetwork()`, and trips a hardcoded-path preflight warning. Note
  this requirement is **neutral between the two location candidates**: pointing
  `NVM_DIR` at `$HOME/.nvm` needs `nvm-exec` there just as much, unless the user
  happens to already have an upstream nvm install at that path. Whichever location
  wins, the same placement mechanism is needed.
- **What the new template variable is.** `{tsuku_home}`, `{share_dir}`, or a
  per-tool `{data_dir}` expanding to `$TSUKU_HOME/share/<tool>`. The third is the
  most reusable and the only one that makes a validator guardrail enforceable, and
  it is also a schema commitment.
- **How existing installs are migrated.** Two populations exist with different
  on-disk signatures and different urgency, and an already-installed nvm runs no
  steps on reinstall (`--force` does not bypass the short-circuit), so a recipe
  change alone reaches neither of them. The move itself is verified safe; what is
  open is where the migration code runs and what it enumerates. Moving *everything*
  under the old root except the extracted program files is safer than listing known
  data paths, since a future nvm release adding a data path would otherwise be
  silently dropped — the empirical run left `.cache/` behind and nvm silently
  re-downloaded it.

## Decision Drivers

- **The user-visible failure is immediate, not delayed.** Any candidate must be
  evaluated against "what does the next shell see," not against retention policy.
- **`nvm exec` and `nvm run` count as behavior.** The issue's criterion is that nvm
  subcommands behave as they do with an upstream install. A fix that silently
  breaks two subcommands trades one bug for another, and it would ship green
  without a test that actually runs `nvm exec <v> node -v`.
- **Whatever lands defines the convention.** nvm is the only recipe of 1,449 that
  uses `set_env` or `install_shell_init`. There is no precedent to follow and no
  second data point to validate against — which makes this the cheapest moment to
  set the convention and the easiest moment to set it wrong.
- **`tsuku remove` must have a coherent answer.** Today removal takes the data with
  the tool. A stable tsuku-owned root inverts that: the data outlives removal with
  no command to reclaim it. `delete_dir` exists in the schema
  (`internal/install/remove.go:425`) and has never had a producer, so any option
  that uses it inherits an untested path.
- **Migration must reach installs that will not re-run recipe steps.** The
  short-circuit and the local plan cache (`getOrGeneratePlanWith` returns a cached
  plan keyed by `tool@version` before consulting the recipe) both block a
  recipe-only fix from reaching existing users.
- **The messenger has to actually deliver.** `renderBackgroundSuccess`
  (`internal/updates/notify.go:155-171`) prints only `"<tool> -> <version>"` and
  drops `Messages`, so a bare `reporter.Warn` on the auto-apply path is persisted
  and never shown. A "tell the user" path needs a notice `Kind` whose renderer
  prints `Messages`.
- **`--no-shell-init` must not silently no-op.** `set_env` writes nothing and
  records no cleanup actions when `ctx.NoShellInit` is set (`set_env.go:100-104`).
- **The fix needs a Go component to get any CI coverage at all.** `recipes/**` is
  absent from the `code` paths-filter, so a recipe-only change runs no Go jobs.
- **Tests must assert observable behavior.** The fast guard belongs in
  `cmd/tsuku/shelld_lifecycle_test.go`, whose harness already installs two versions
  of a synthetic tool with no network in 1.7s and reads results out of a real bash
  subshell, and which is not `testing.Short()`-guarded so it runs on every PR.

## Decisions Already Made

These were settled during exploration with concrete evidence. The design should
treat them as constraints rather than reopen them.

- **The problem statement is re-framed away from garbage collection.** The
  version-keyed fragment plus active-version selection hides the data at the next
  shell start; GC only makes it permanent.
- **The "keep data in the versioned directory and compensate" family is
  eliminated.** Inverse symlinks die on `nvm cache clear`, which runs
  `rm -rf "${DIR}" && mkdir -p "${DIR}"` on `$NVM_DIR/.cache` — removing the symlink
  and recreating a real directory inside the versioned tree. A per-recipe GC
  exemption and teaching GC about tool data both leave the next-shell-start bug
  intact and make disk usage unbounded. Migrating data forward on every upgrade was
  already rejected in `DESIGN-shell-d-lifecycle.md` for driving behavior off
  `VersionState.Plan`, and would copy a multi-gigabyte Node tree on every point
  release.
- **"Drop the `set_env` step" as literally stated is eliminated** — it reinstates
  the pre-fix layout under `share/shell.d` rather than yielding `$HOME/.nvm`.
- **Any stable path under `tools/` is eliminated** by GC's prefix matching.
- **`nvm exec` and `nvm run` are in scope**, which promotes the stable-root option
  from its naive form to one that materializes both `nvm-exec` and `nvm.sh`.
- **Version-keyed fragment names and active-version cache selection are settled
  upstream** by `DESIGN-shell-d-lifecycle.md` and are built on, not revisited.

## Out of Scope

- Reverting or reworking the shell.d lifecycle mechanism.
- Sibling recipe-review follow-ups filed separately from the same review.
- `GarbageCollectVersions` deleting *other tools'* directories when one recipe name
  is a prefix of another (`git`/`git-lfs`, `docker`/`docker-compose` — 59 pairs in
  the registry, verified empirically). This is a real bug, independent of this one,
  and belongs in its own issue.
