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
- **How `nvm exec` survives.** Both `nvm.sh` and `nvm-exec` must be materialized in
  the data root, and no tsuku action can create a directory, symlink, or copy a file
  to an arbitrary path today. Either a narrow new action or `run_command` — which
  works but is non-evaluable, declares `RequiresNetwork()`, and trips a
  hardcoded-path preflight warning.
- **What the new template variable is.** `{tsuku_home}`, `{share_dir}`, or a
  per-tool `{data_dir}` expanding to `$TSUKU_HOME/share/<tool>`. The third is the
  most reusable and the only one that makes a validator guardrail enforceable, and
  it is also a schema commitment.
- **How existing installs are migrated.** Two populations exist with different
  on-disk signatures and different urgency, and an already-installed nvm runs no
  steps on reinstall (`--force` does not bypass the short-circuit), so a recipe
  change alone reaches neither of them.

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
