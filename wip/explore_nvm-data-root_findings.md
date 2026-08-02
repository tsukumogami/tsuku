# Exploration Findings: nvm-data-root

Round 1. Seven leads, all returned. Research files under
`wip/research/explore_nvm-data-root_r1_lead-*.md`.

## The problem statement in the issue is slightly wrong, and it matters

The issue frames this as a garbage-collection bug. It is not, or not only.
`tsuku update nvm` makes the user's Node versions invisible **at the next shell
start**, not seven days later: the `set_env` fragment is version-keyed and
`ShellDSelection` serves only the active version's fragment, so the new shell
gets `NVM_DIR=$TSUKU_HOME/tools/nvm-<new>` and `nvm ls` is empty immediately.
`GarbageCollectVersions` only makes it permanent later.

Two consequences. First, any fix that only constrains GC (per-recipe exemption,
teaching GC about tool data, longer retention) leaves the user-visible bug fully
intact and just delays the deletion. Second, the deletion deadline is much softer
than the brief assumed: GC has exactly one call site — the background auto-apply
loop at `internal/updates/apply.go:153` — and it is reached only after that same
tool has successfully auto-updated, with the victim directory's own mtime 7+ days
stale. That is typically weeks. Migration is still required; it is not a fire.

There is a **worse** deletion path nobody had named. `internal/install/manager.go:183`
removes the whole tool directory before every atomic rename, so reinstalling the
*same* nvm version destroys `NVM_DIR` today with no retention period at all. Any
fix that keeps the data inside `{install_dir}` fails on this path regardless of
what GC does.

## What is settled by evidence

**nvm.sh does not need to live inside `$NVM_DIR`.** Every `${NVM_DIR}/...`
reference in nvm.sh is data nvm creates or reads at runtime, with exactly one
exception: `NODE_VERSION="${VERSION}" "${NVM_DIR}/nvm-exec" "$@"` (nvm.sh:4083 in
one release, :4379 in another). The program/data split is legal. It was verified
empirically, not just read: a real `nvm install 22`, `use`, `which`, `alias`, and
`ls` all work with nvm.sh sourced from one directory and `NVM_DIR` pointing
elsewhere.

**That one line breaks `nvm exec` and `nvm run`** (`run` delegates to `exec`) with
`rc=127`. No acceptance criterion in the issue names them, but they are how people
run one-off commands under a pinned version in scripts and CI, so silently
breaking them is a real regression.

**The obvious one-symlink fix silently fails.** `nvm-exec` self-locates through an
unresolved `BASH_SOURCE[0]` and re-sources `$DIR/nvm.sh`, so symlinking only
`nvm-exec` into the data root makes it look for a `nvm.sh` that is not there. Both
files have to be present. This is the kind of thing that ships green and breaks in
the field, and it argues for a test that actually runs `nvm exec <v> node -v`.

**nvm creates its own data tree wherever it is pointed.** `nvm alias` does
`mkdir -p "${NVM_ALIAS_DIR}"`, `nvm ls` does `mkdir -p "$(nvm_alias_path)/lts"`,
`nvm_install_binary` does `mkdir -p "${VERSION_PATH}"`, and `nvm_ls` guards missing
directories. A fresh, empty, or nonexistent `$NVM_DIR` is fine. There is no
bootstrap file nvm needs to find there. (Empirical confirmation of the
*nonexistent* case is pending a follow-up run.)

**`install_shell_init` copies nvm.sh; it does not source it from the tool
directory.** The fragment at `$TSUKU_HOME/share/shell.d/nvm@<version>.bash` is a
copy. This is load-bearing and the issue does not realize it: "drop the export and
let nvm use its default" does **not** yield `$HOME/.nvm`. nvm.sh self-locates to
the directory it was sourced from, which under tsuku is `$TSUKU_HOME/share/shell.d`
— a `0700` directory that `RebuildShellCache` and `tsuku doctor` both walk. That is
the pre-#2465 behavior and it is not a fix.

**No option here is recipe-only.** `set_env` expands exactly six placeholders —
`{version} {os} {arch} {install_dir} {work_dir} {libs_dir}` (`internal/actions/util.go:28-37`)
— none of which yields a stable path, and it wraps values in single quotes
(`set_env.go:245`) as a deliberate injection defense, so `$HOME` and `${TSUKU_HOME}`
in a recipe value reach the shell literally. The only recipe-only escape is
`{install_dir}/../../share/nvm`, which resolves correctly but smuggles a version
string into a path nvm does string-prefix arithmetic on
(`nvm_tree_contains_path`, `nvm_change_path`, `nvm_sanitize_path`). That is a booby
trap, not a fix. So "smallest diff" is not a differentiator — every viable option
needs a Go change.

**Anything under `tools/` is disqualified.** `GarbageCollectVersions` matches on a
raw string prefix `toolName + "-"` (`gc.go:34,45`) and `ReapVersion` returns nil for
an unrecognized version rather than objecting (`remove.go:394-397`), so
`os.RemoveAll` runs anyway. `tools/nvm-data` would be collected.

**`$TSUKU_HOME/share/` is safe as written.** It already exists
(`config.go:333,367,392-393`), already holds durable non-tool-versioned state
(`hooks/`, `shell.d/`, `completions/`), and no code path enumerates it for
deletion. A data root there needs zero new protection code. `$TSUKU_HOME/data/<tool>`
is equally safe but needs a new top-level directory in `Config` and
`EnsureDirectories`.

**Blast radius is one recipe.** nvm is the only recipe of 1,449 that uses `set_env`
or `install_shell_init`. Whatever convention this lands is defining the convention
rather than following one — which makes now the cheapest possible moment to set it,
and also means there is no second data point to validate it against.

**Migration has a precedent to copy, not invent.** `Config.EnsureEnvFile` +
`migrateEnvExports` (`internal/config/config.go:477-556`) solved the same shape
— managed location changed, user content must survive — with shape-detection rather
than a version counter, delivered through two paths: automatically from
`InstallWithOptions` (`manager.go:149`) and manually via `tsuku doctor --fix`
(`doctor.go:54`). `state.json` has no schema version and two production migrations
that both work this way.

**There are two affected populations, and the larger one is not on a deletion
clock.** Population A (pre-#2465) has data in `$TSUKU_HOME/share/shell.d`, which
nothing garbage-collects — it is stranded, not doomed. Population B (post-#2465)
has data in `tools/nvm-<version>/`. Since #2465 merged today, almost everyone is
Population A. That reframes A's requirement from "rescue before deletion" to
"relocate before abandonment," which is strictly easier.

## Traps that would bite an unwary implementation

- **`--no-shell-init`**: `set_env` writes nothing and records no `CleanupActions`
  when `ctx.NoShellInit` is set (`set_env.go:100-104`). A mechanism built purely on
  `set_env` silently does nothing for those users.
- **The local plan cache**: `getOrGeneratePlanWith` (`install_deps.go:95-108`)
  returns a cached plan keyed by `tool@version` before consulting the recipe, and
  the `set_env` value is carried into the plan verbatim. A user with a cached
  `nvm@<version>` plan keeps the old literal even after the recipe is fixed.
- **Already-installed short-circuit**: an install that short-circuits runs no steps,
  and `--force` does not bypass it. Existing nvm installs cannot pick up a recipe
  fix on their own.
- **Background warnings are discarded**: `renderBackgroundSuccess`
  (`internal/updates/notify.go:155-171`) prints only `"<tool> -> <version>"` and
  drops `Messages`, so a bare `reporter.Warn` on the auto-apply path is written to
  disk and never displayed. Any "tell the user" path needs a notice `Kind` whose
  renderer prints `Messages`.
- **`delete_dir` is implemented and has no producer** (`remove.go:425`). Anyone
  adding one inherits an untested path — and cleanup actions are also run by GC's
  `ReapVersion`, so a `delete_dir` on a stable path is protected only while another
  installed version records the same path.
- **`nvm cache clear`** does `rm -rf "${DIR}" && mkdir -p "${DIR}"` on
  `$NVM_DIR/.cache`, which removes a symlink and recreates a real directory. This
  single line kills the "leave `NVM_DIR` versioned, symlink the data subtrees out"
  family.
- **Recipe-only changes run no Go jobs in CI** — `recipes/**` is absent from the
  `code` paths-filter, so a recipe-only fix ships with an install-succeeds check
  and nothing else.

## Where the decision actually sits

Twelve options were generated and ranked. The families that keep the data in the
versioned directory with compensating machinery (inverse symlinks, per-recipe GC
exemption, teaching GC about tool data, migrating data forward on every upgrade)
are uniformly worse on the evidence: they either leave the next-shell-start bug
intact, or they break on a specific nvm line, or they were already rejected in
`DESIGN-shell-d-lifecycle.md` for driving behavior off `VersionState.Plan`.

That leaves two real answers, and the tiebreaker is a product question rather than
a technical one:

> **Should `tsuku remove nvm` be able to delete the user's Node installs?**

- **Yes** → the data belongs inside `$TSUKU_HOME` (a stable `share/nvm` or
  `data/nvm` root, plus `nvm.sh` and `nvm-exec` materialized there so `nvm exec`
  survives, plus a new template variable). tsuku then owns a directory it currently
  has no lifecycle for, and needs a `delete_dir`-emitting cleanup or a `--purge`
  flag, with an explicit GC exclusion so `ReapVersion` cannot fire it.
- **No** → `$HOME/.nvm` is right, exported explicitly (not by dropping the export,
  which self-locates to `share/shell.d`). This matches what every other version
  manager in the registry does — pyenv, rustup, and asdf all delegate to the tool's
  own default root — and matches the issue's own criterion that nvm "behave the way
  it does with an upstream nvm install." It also means an existing upstream nvm
  install is picked up seamlessly. The cost is that tsuku writes user state outside
  `$TSUKU_HOME`, which sits awkwardly against the self-contained philosophy, and
  uninstalling tsuku leaves it behind.

Two companions are cheap, additive, and worth shipping with whichever wins: a
validator rule rejecting `set_env` of a known data-root variable
(`NVM_DIR`, `PYENV_ROOT`, `RBENV_ROOT`, `VOLTA_HOME`, `SDKMAN_DIR`, `CARGO_HOME`,
`GOPATH`) at a value containing `{install_dir}`, which makes the class of bug
unrepresentable at zero cost today; and honoring a pre-existing user-set value.

## Where the tests go

The fast guard belongs in `cmd/tsuku/shelld_lifecycle_test.go` — the only place
install, actions, and shellenv meet. Its harness already stands up two versions of
a synthetic tool with no network in 1.7 seconds and reads results out of a real
bash subshell, and it is not `testing.Short()`-guarded, so it runs under
`go test -short ./...` on every PR that touches Go. The missing assertion is three
lines from what the harness already supports: seed a user file under the path the
shell was told to use, install the second version, remove the first, assert the file
survives. That test fails today.

The slow end-to-end (real nvm, real Node, real upgrade) does not fit a `@critical`
Gherkin scenario — that would put a multi-minute network download on every PR in the
repo — and an untagged scenario runs only on PRs touching `test/functional/**`, so
it would pass green once and lie dormant forever. A shell script under
`test/scripts/` invoked from `integration-tests.yml` matches how that workflow
already works (`test-checksum-pinning.sh`, `test-homebrew-recipe.sh`).

## Out-of-scope findings worth filing separately

`GarbageCollectVersions` deletes **other tools'** directories when one recipe name
is a prefix of another, verified empirically against the real function:
`GarbageCollectVersions(..., "git", ...)` deleted `git-lfs-3.5.0`. The registry has
59 such pairs — `git`/`git-lfs`, `docker`/`docker-compose`, `kubectl`/`kubectl-ai`,
`helm`/`helm-docs`, `bat`/`bat-extras`. The victim's `state.json` entry survives, so
`tsuku list` still shows it while its `bin/` symlinks dangle. The existing test
`TestGarbageCollectVersions_IgnoresOtherTools` uses `ripgrep` vs `node`, which cannot
collide. This is a real bug independent of #2464 and deserves its own issue.

Also noted: `Manager.Remove` (`remove.go:21-68`) is dead code; `KindShellInitChange`
has no producer; `install_completions` is missing from `ActionEvaluability` so it
silently evaluates as non-evaluable.

## Decision: Crystallize

Coverage is sufficient. The option space is enumerated and attacked, the decisive
technical questions are answered with evidence, and what remains is a genuine
architectural choice between two viable families plus a product question — which is
design work, not more research.
