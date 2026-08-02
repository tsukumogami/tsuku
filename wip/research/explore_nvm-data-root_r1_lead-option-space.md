# Lead: widen the option space, adversarially

Everything below is grounded in this worktree's code and in `nvm.sh` at tag
`v0.40.3` (fetched from `raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/nvm.sh`,
4661 lines). Line numbers for nvm.sh refer to that file.

## Findings

### 0. Four facts that reshape the whole option space

Before steelmanning anything, four things I verified that the issue does not say
and that several of the candidate options quietly assume away.

**Fact 1 — `nvm.sh` needs almost nothing from `$NVM_DIR` except data.** I
enumerated every `${NVM_DIR}/...` reference in nvm.sh:

```
4  ${NVM_DIR}/            3  ${NVM_DIR}/versions/     2  ${NVM_DIR}/current
2  ${NVM_DIR}/*/bin       2  ${NVM_DIR}/*/share/man   1  ${NVM_DIR}/versions/node
1  ${NVM_DIR}/versions/io.js   1  ${NVM_DIR}/.cache    1  ${NVM_DIR}/default-packages
1  ${NVM_DIR}/*/lib/node_modules   1  ${NVM_DIR}/v*
1  ${NVM_DIR}/nvm-exec
```

Every one of those is data nvm creates or reads at runtime — *except*
`${NVM_DIR}/nvm-exec`, at nvm.sh:4083, inside the `nvm exec` command:

```sh
NODE_VERSION="${VERSION}" "${NVM_DIR}/nvm-exec" "$@"
```

`nvm-exec` is a repo-shipped file, and it in turn does
`\. "$DIR/nvm.sh" --no-use` where `$DIR` is its own directory. So the answer to
the decisive question — *does `nvm.sh` work when it lives outside `$NVM_DIR`?* —
is **yes, with exactly one exception: `nvm exec` breaks**, and it breaks by
running a nonexistent file, not by corrupting anything. `nvm install`,
`nvm use`, `nvm ls`, `nvm alias default`, `nvm current`, `nvm which`,
`nvm uninstall`, `nvm reinstall-packages`, `nvm cache dir|clear` are all pure
`$NVM_DIR`-as-data operations. Option A is **legal**.

nvm also creates its own data tree wherever you point it: `nvm alias` does
`command mkdir -p "${NVM_ALIAS_DIR}"` (nvm.sh:4296) before writing, `nvm ls`
does `mkdir -p "$(nvm_alias_path)/lts"` (nvm.sh:1230, 1665), and
`nvm_install_binary` does `command mkdir -p "${VERSION_PATH}"` (nvm.sh:2236).
`nvm_ls` guards missing dirs with `! [ -d ... ]` at nvm.sh:1517-1524. A fresh,
empty, or nonexistent `$NVM_DIR` is fine. There is no bootstrap file nvm needs
to find there.

There is no git dependency either. `nvm.sh` at v0.40.3 contains zero `git`
invocations — the git-clone-and-`git checkout` upgrade path lives entirely in
`install.sh` (install.sh:166 `command git clone "$(nvm_source)" --depth=1
"${INSTALL_DIR}"`), which tsuku does not run.

**Fact 2 — the recipe cannot currently express any path except the versioned
tool directory.** `set_env` expands values through
`GetStandardVars` (`internal/actions/util.go:28-37`), whose entire vocabulary is
`{version} {os} {arch} {install_dir} {work_dir} {libs_dir}`. There is no
`{tsuku_home}`, no `{share_dir}`, no `{home}`. And the value is written through
`shellQuote` (`internal/actions/set_env.go:245`), which wraps it in single
quotes:

```go
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
```

so `$HOME` and `${TSUKU_HOME}` in a recipe value reach the shell **literally**
and never expand. **Options A and B both require a Go change** — a new template
variable, or `set_env` gaining an expandable-value mode — unless you resort to
the traversal hack in Fact 3.

**Fact 3 — there is exactly one recipe-only escape hatch, and it is bad.**
`{install_dir}` is `$TSUKU_HOME/tools/nvm-<version>`, so
`{install_dir}/../../share/nvm` resolves to `$TSUKU_HOME/share/nvm` and
`{install_dir}/../../../.nvm` resolves to `$HOME/.nvm` *when and only when*
`$TSUKU_HOME` happens to be `$HOME/.tsuku`. The second breaks under any
`TSUKU_HOME` override, including tsuku's own validation container, which sets
`TSUKU_HOME=/workspace/tsuku` and `HOME=/workspace`
(`internal/validate/executor.go:243-244`). The first survives that, but the
literal string still contains `nvm-<version>`, so the *string* changes on every
upgrade even though the resolved path does not — and nvm does string-prefix
arithmetic on `$NVM_DIR` in `nvm_tree_contains_path` (nvm.sh:972-974,
`index($0, NVM_DIR) == 1`), `nvm_change_path` (nvm.sh:987-1002), and
`nvm_sanitize_path` (nvm.sh:2938-2939). A `..`-bearing, version-bearing
`NVM_DIR` is a booby trap, not a fix.

I also checked whether CI blocks hardcoded paths in recipes (recipes/README.md
line 71 claims "No hardcoded paths or secrets"). I could find no such check in
`internal/recipe/validator.go` — grep for `HOME`, `hardcoded` returns nothing.
So the traversal hack would pass validation. That is a finding about the
validator, not a licence to use it.

**Fact 4 — anything named `tools/nvm-*` is inside the blast radius by
construction.** `GarbageCollectVersions` matches on a *string prefix*, not on
state:

```go
prefix := toolName + "-"                       // gc.go:34
...
if !strings.HasPrefix(name, prefix) { continue } // gc.go:45
```

A stable data directory at `$TSUKU_HOME/tools/nvm-data` matches `nvm-`, is
neither `activeDir` nor `previousDir`, ages past retention, gets
`ReapVersion("nvm", "data")` — which no-ops, because `toolState.Versions["data"]`
does not exist and `ReapVersion` returns `nil` for unknown versions
(`internal/install/remove.go:394-397`) — and is then deleted by
`os.RemoveAll(dirPath)` at gc.go:82. **Any option that parks stable data under
`tools/` with the tool's name prefix is self-defeating.** `share/` is outside
`toolsDir` and is never scanned by GC; nothing in `internal/` deletes from
`share/` except recorded `CleanupAction` paths
(`internal/install/remove.go:412-427`).

### 1. Option A — stable tsuku-owned path (`$TSUKU_HOME/share/nvm`)

**Steelman.** It is the only named option that keeps both halves of the tool
honest: the program (nvm.sh, nvm-exec, bash_completion) stays versioned and
GC-able exactly like every other tsuku tool, and the data gets a root whose
lifetime is the *user's*, not the version's. It matches how every mature tool of
this shape separates itself — `$XDG_DATA_HOME/<tool>` vs. the binary — and it is
the only option that makes the acceptance criterion "upgrade preserves data"
true by construction rather than by machinery that has to run at the right
moment. Fact 1 says it is legal: nvm.sh does not care where it lives. And
because `share/` is outside `toolsDir`, the fix is immune to the GC prefix rule
that caused the bug (Fact 4).

**Attacks.**

1. **`nvm exec` breaks — silently, at the worst moment.** nvm.sh:4083 runs
   `"${NVM_DIR}/nvm-exec"`. Under A that path does not exist, so `nvm exec 20
   npm test` fails with a bare `command not found`-shaped error from the shell,
   not an nvm diagnostic. This is the one concrete functional regression in A,
   and the issue's acceptance criterion "`nvm install`/`alias default`/`ls`
   behave like upstream" does not name it, which is precisely how it would ship
   unnoticed. It is fixable (see Option D) but it means **A as literally stated
   fails the "behaves like upstream" criterion**.
2. **A is not recipe-only.** Per Fact 2 there is no `{tsuku_home}` and single
   quoting kills `${TSUKU_HOME}`. A needs a Go change in `internal/actions/` —
   which the issue's framing ("point NVM_DIR at ...") reads as a one-line recipe
   edit. Anyone who scopes A as recipe-only will reach for the Fact 3 traversal
   hack.
3. **It invents an unowned directory.** Nothing in tsuku creates, tracks,
   `doctor`-checks, or deletes `$TSUKU_HOME/share/nvm`. `EnsureDirectories`
   (`internal/config/config.go:373-401`) creates `share` and `share/hooks` and
   nothing else. `CleanupAction` supports `delete_dir`
   (`internal/install/remove.go:424`) but **no action in the codebase ever emits
   one** — grep for `delete_dir` outside tests returns only the two schema
   comments and the switch arm. So `tsuku remove nvm` leaves the whole Node
   install behind forever, with no command to reclaim it.
4. **The migration is the hard part and A does not describe it.** Existing
   installs have data under `tools/nvm-<old>/`. Moving it requires knowing the
   old `NVM_DIR`, and the only record of it is the *content* of
   `share/shell.d/00-env-nvm@<version>.bash`. Copying it changes
   `ContentHash` expectations in state.json and would trip
   `CheckShellD`'s `HashMismatches` (`internal/shellenv/doctor.go:114`) if done
   carelessly.
5. **`.cache` shared across nvm versions.** `nvm_cache_dir` is
   `${NVM_DIR}/.cache` (nvm.sh:2997) and `nvm cache clear` does
   `rm -rf "${DIR}" && mkdir -p "${DIR}"` (nvm.sh:3177). Two tsuku-installed nvm
   versions now share one cache. Benign in practice — the cache is
   content-addressed Node tarballs — but it means the versions are no longer
   independent, which is a property tsuku's multi-version model otherwise holds.

### 2. Option B — drop the export, use nvm's default

**Steelman.** It is the smallest possible diff: delete a four-line step from
`recipes/n/nvm.toml`. It gives the user *exactly* upstream behavior, which makes
the "behaves like upstream" criterion trivially true and makes tsuku's nvm
interoperable with an `nvm` the user installed by hand or via a dotfiles repo.
It puts the data outside `$TSUKU_HOME` entirely, so no future GC, cleanup, or
`tsuku uninstall` change can ever touch it. And it stops tsuku from having an
opinion about a data root that is really the user's.

**Attack — and this is the one that kills it as stated.** *Dropping the export
does not restore upstream behavior.* nvm.sh's self-location (nvm.sh:445-453) is:

```sh
# Auto detect the NVM_DIR when not set
if [ -z "${NVM_DIR-}" ]; then
  if [ -n "${BASH_SOURCE-}" ]; then
    NVM_SCRIPT_SOURCE="${BASH_SOURCE}"
  fi
  NVM_DIR="$(nvm_cd ${NVM_CD_FLAGS} "$(dirname "${NVM_SCRIPT_SOURCE:-$0}")" >/dev/null && \pwd)"
  export NVM_DIR
```

It resolves to *the directory of the file being sourced*. And tsuku does not
source nvm.sh from the tool directory — `install_shell_init` with `source_file`
**copies** it:

```go
destPath := filepath.Join(shellDDir, shellDFileName(target, version, shell))
if err := copyFile(srcPath, destPath); err != nil {   // shell_init.go:253
```

so `share/shell.d/nvm@<version>.bash` is a byte copy of nvm.sh
(`internal/actions/shell_init.go:245-270`). `RebuildShellCache` then
concatenates it into `share/shell.d/.init-cache.bash`
(`internal/shellenv/cache.go`), and `$TSUKU_HOME/env` sources *that*:

```sh
_tsuku_init_cache="${TSUKU_HOME:-$HOME/.tsuku}/share/shell.d/.init-cache.bash"
... . "$_tsuku_init_cache"          # internal/config/config.go:452-457
```

`$BASH_SOURCE` inside the cache is the cache file. So with the export dropped,
**`NVM_DIR` becomes `$TSUKU_HOME/share/shell.d`** — not `$HOME/.nvm`. That is
the pre-#2465 behavior the issue describes as the old bug, and it is worse than
the current bug in one respect: nvm would create `versions/`, `alias/`,
`.cache/`, and `default-packages` inside a `0700` directory that tsuku's cache
builder scans on every install. (`RebuildShellCache` skips directories and
non-`.bash`/`.zsh` files, so it would not *corrupt* the cache — but `tsuku
doctor`'s `CheckShellD` walks the same directory, and Node version trees living
inside shell.d is not a state anyone wants to debug.)

Second attack: B *done properly* — export `NVM_DIR="$HOME/.nvm"` — hits Fact 2.
Single-quoting means the recipe cannot say `$HOME`. So "the smallest possible
diff" is in fact a Go change plus a recipe change, the same blast radius as A,
for a worse outcome (tsuku now writes gigabytes into a directory it does not own
and cannot report on, and `tsuku doctor` can say nothing about it).

Third attack: interop cuts both ways. If the user already has a real nvm at
`$HOME/.nvm`, tsuku's copied nvm.sh in the init cache and the user's own
`. "$NVM_DIR/nvm.sh"` in `.bashrc` both define `nvm()`, at two different
versions, sharing one data root. Last one sourced wins, silently.

### 3. Option C — nvm does not belong in the registry as a versioned tool

**Steelman.** The whole bug is a category error: tsuku's model is
`name-version` directories that are interchangeable, disposable, and
garbage-collected, and nvm is none of those things — it is a stateful manager
whose directory *is* the state. Every other option is machinery bolted on to
keep a bad fit alive. Deprecating the recipe (or reducing it to a bootstrapper
that runs upstream's `install.sh` once and then gets out of the way) is the only
option that removes the failure mode instead of routing around it, and it costs
nothing to maintain. There is precedent in the registry: `rustup`'s recipe
(`recipes/r/rustup.toml`) installs only `bin/rustup-init` — the bootstrapper —
and lets rustup own `~/.rustup` and `~/.cargo` itself. `asdf`
(`recipes/a/asdf.toml`) ships a single static binary and keeps its data in
`~/.asdf`. **nvm is the outlier, not the pattern.**

**Attacks.**

1. **It is a non-answer for the ~N users who already have it installed.** The
   issue's acceptance criteria include "existing installs migrated or warned".
   Deprecating the recipe does not migrate anyone; it strands them on a recipe
   version whose `NVM_DIR` still points into a directory GC will delete. C has
   to be paired with a migration anyway, at which point you have already built
   most of A or B.
2. **`curated = true`.** `recipes/n/nvm.toml` is a handcrafted, curated recipe.
   Removing a curated tool from the registry is a product decision with a
   deprecation surface (search results, `satisfies` aliases, telemetry), not a
   bug fix, and the issue is scoped as a bug.
3. **The "run upstream install.sh" variant reintroduces the same problem.**
   install.sh:28-29 `nvm_install_dir()` honors `$NVM_DIR` if set, and
   install.sh:425 writes `export NVM_DIR="..."` into the user's *profile* —
   tsuku would now be writing to `.bashrc` outside its own shell.d discipline,
   which the entire `DESIGN-shell-d-lifecycle.md` architecture exists to avoid.
   It also needs `git` at install time (install.sh:166) and does network I/O
   outside tsuku's download cache.
4. **It generalizes badly in the wrong direction.** "Tools with user data don't
   belong here" would eventually exclude pyenv, rbenv, sdkman, volta, and
   anything else people actually want. The registry has `pyenv`
   (`recipes/p/pyenv.toml`) already — installed as a plain binary today, but
   pyenv writes to `$PYENV_ROOT` and the day someone adds `set_env` to it, this
   exact bug recurs.

### 4. Options nobody named

I am numbering these D through M. D, E, G, H, I, J, K were requested; L and M
are additions.

---

**D. Split: stable data root + materialize `nvm-exec` into it.**
*The corrected form of A.* `NVM_DIR = $TSUKU_HOME/share/nvm`; at install time,
copy (or hard-link) `nvm-exec` from the versioned tool dir into
`$TSUKU_HOME/share/nvm/nvm-exec`, overwriting on every install so it tracks the
active version. `nvm.sh` continues to be delivered by `install_shell_init` from
the versioned tool dir.

*Concrete change:* `internal/actions/util.go` (add `{tsuku_home}` or
`{share_dir}` to `GetStandardVars` — note `SetEnvAction.Execute` already derives
`tsukuHome := filepath.Dir(ctx.ToolsDir)` at set_env.go:151, so the value is
already in hand); `recipes/n/nvm.toml` (`set_env` value, plus one step to place
`nvm-exec`); the placement step needs either `run_command`
(`internal/actions/run_command.go:96` runs `sh -c`, full expansion, so
`mkdir -p` + `cp` works today) or a new narrow action.

*Failure mode:* `nvm-exec` in the stable dir is a *third* copy of a versioned
artifact, and nothing removes it when nvm is removed. Downgrading leaves the
newer `nvm-exec` in place, and because `nvm-exec` sources `$DIR/nvm.sh` — which
does not exist in the stable dir — `nvm exec` would then fail *differently*
(sourcing nothing, then calling an undefined `nvm use`). To make `nvm exec`
genuinely work you must place **both** `nvm-exec` and `nvm.sh` in the stable
dir, at which point the "versioned tool dir is the program half" story is half
fiction. That is D's real cost, and it is the honest one.

---

**E. Inverse symlinks: `NVM_DIR` stays `{install_dir}`; the versioned dir holds
symlinks into a stable data dir.** `tools/nvm-<v>/versions -> share/nvm/versions`,
`alias -> share/nvm/alias`, `.cache -> share/nvm/.cache`, `default-packages ->
share/nvm/default-packages`.

*Concrete change:* `recipes/n/nvm.toml` plus a new action (or `run_command`) to
create four symlinks post-install. No `set_env` change at all — the export keeps
saying `{install_dir}`, so the recipe comment stays true and no template
variable is needed.

*Failure mode — decisive:* `nvm cache clear` runs
`command rm -rf "${DIR}" && command mkdir -p "${DIR}"` (nvm.sh:3177) where
`$DIR` is `$NVM_DIR/.cache`. `rm -rf` on a symlink **removes the link**, and the
`mkdir -p` then recreates `.cache` as a real directory inside the versioned tool
dir. One `nvm cache clear` silently un-does a quarter of the fix, with no
error. The same shape of hazard applies anywhere nvm replaces rather than
mutates a path. I checked the one I expected to break and did not: `nvm ls`
globs `"$NVM_DIR/versions/node"/*` and seds `s#^${NVM_DIR}/##` (nvm.sh:1535-1538)
— glob expansion does not canonicalize symlinks, so `find`'s output still
carries the `$NVM_DIR` prefix and the sed strips correctly. E survives `nvm ls`.
It does not survive `nvm cache clear`. Secondary failure: GC deletes the
versioned dir and the symlinks with it, so the *data* survives but every
upgrade must recreate four links, and `--no-shell-init` installs would skip
that.

---

**F. Stable path is a symlink to the active versioned dir**
(`$TSUKU_HOME/share/nvm -> tools/nvm-<active>`), `NVM_DIR = share/nvm`.

*Concrete change:* `internal/install/` — a fourth thing to update at the three
`ActiveVersion` assignment sites named in `DESIGN-shell-d-lifecycle.md`
(`InstallWithOptions`, `Activate`, `RemoveVersion`'s implicit promotion), using
`AtomicSymlink` (`internal/install/symlink.go:11`).

*Failure mode — dead on arrival.* The data still physically lives in
`tools/nvm-<version>/`, which is exactly what GC deletes at gc.go:82. This
option changes the *name* the user's shell sees and nothing about where bytes
land. Worse, it upgrades the failure from "data is invisible" to "data is
invisible **and** the symlink dangles", because `ValidateSymlinkTarget`
(`symlink.go:44`) would happily point it at a directory GC removed. I include it
only because "symlink-based stable path" is a phrase that sounds like a solution
and this is the reading of it that does not work.

---

**G. Upstream-style: git clone into a stable dir, upgrade in place; tsuku's
"version" becomes a thin pointer.** `$TSUKU_HOME/share/nvm` is a git checkout;
`tools/nvm-<v>/` contains only a marker plus the shell.d fragment source.

*Concrete change:* a new `git_clone`-shaped action (none exists — the action
registry at `internal/actions/action.go:202-271` has `fossil_archive` and
`github_archive`, no git clone), plus recipe rewrite, plus a story for
`tsuku update nvm` doing `git fetch && git checkout v<new>`.

*Failure mode:* it imports `git` as an install-time system dependency for a tool
whose entire selling point in tsuku is "pure shell, no system deps" (the recipe
comment at nvm.toml line 15 says exactly that: "nvm source archive is pure shell
— works on both libc variants"). It also breaks tsuku's rollback model: a git
checkout has one working tree, so `tsuku rollback nvm` has nothing to point at,
and `GarbageCollectVersions`'s "keep active and previous" invariant becomes
meaningless. And upgrade-in-place is not atomic, so a failed `git checkout`
leaves a half-version. This is a rewrite of what "version" means for one recipe.

---

**H. Pin nvm to a single non-upgradable version.** Recipe declares an exact pin;
auto-apply never selects a newer nvm; GC never has a second version to collect.

*Concrete change:* `internal/recipe/types.go:171-200` (`MetadataSection` gains a
field), plus the resolver, plus `internal/updates/` to honor it.
`PinLevelFromRequested` (`internal/install/pin.go:44-68`) derives pin level from
the *user's* request string only; there is no recipe-declared pin today.

*Failure mode:* it does not fix the bug, it disarms the trigger. The data still
lives at a path that encodes a version, so `tsuku remove nvm@0.40.6 && tsuku
install nvm` — or any manual version change, or a user who typed
`tsuku install nvm@0.40.7` explicitly — still loses everything. It also freezes
nvm at whatever version we pin, in a registry whose whole premise is
version-awareness, and it fails the acceptance criterion "upgrade preserves
data" by making upgrade impossible rather than safe. Reviewers will read it as
what it is.

---

**I. Per-recipe GC exemption.** `MetadataSection` gains e.g.
`holds_user_data = true`; `GarbageCollectVersions` skips the tool entirely.

*Concrete change:* `internal/recipe/types.go`, `internal/updates/gc.go`
(the caller at `internal/updates/apply.go:153` would need the recipe, which it
does not currently load — it has `entry.Tool` and a `*install.Manager`, so this
is a real plumbing change, not a one-liner).

*Failure mode — the deep one:* GC is not what makes the data invisible. The
*export* is. `tsuku update nvm` writes a new
`share/shell.d/00-env-nvm@<new>.bash` pointing `NVM_DIR` at the new directory,
and `RebuildShellCache`'s active-version selection (`ShellDSelection.excludes`,
`internal/shellenv/selection.go:36-43`) deliberately drops the old version's
fragment. The user's Node versions become invisible **the moment the new shell
starts** — GC only makes it permanent seven days later. I is a fix for the
symptom that takes a week; the one that takes a second is untouched. It also
makes disk usage unbounded (`nvm-0.40.1` through `nvm-0.40.9`, each holding a
full Node tree, forever) and it generalizes a per-tool escape valve into the
GC's contract, which is exactly the kind of flag that accumulates.

---

**J. `NVM_DIR` respects a pre-existing value.** `set_env` gains per-var
`if_unset` / `default` semantics, emitting `: "${NVM_DIR:=<path>}"; export
NVM_DIR` instead of `export NVM_DIR='<path>'`.

*Concrete change:* `internal/actions/set_env.go:158-166` (the export-line
writer), plus the recipe, plus `recipe.EnvVar` gaining a field.

*Failure mode:* it is at best half a fix and never a whole one. It solves
"tsuku stomped my existing nvm" — a real and currently-unaddressed complaint —
and solves nothing about upgrade, because tsuku's own value is still versioned.
It is also **order-dependent in a way most users will get wrong**: the fragment
runs when `$TSUKU_HOME/env` is sourced from `.bashrc`, so a user who sets
`NVM_DIR` *after* that line still gets stomped, and a user who sets it before
does not. Two users doing "the same thing" get different results with no
diagnostic. J is a good companion to D or A. It is not a candidate on its own.

---

**K. Fix it in GC — teach GC about tool data.** Leave the recipe alone; make
`GarbageCollectVersions` refuse to delete a directory that has grown beyond what
the install put there, or that a `set_env` step named as an exported root.

*Concrete change:* `internal/updates/gc.go` plus a way to know the install
manifest. `VersionState` records `CleanupActions` and `Binaries`; it does not
record a file manifest of the tool dir, so "grown beyond" would need either a
manifest (state.json schema change) or a heuristic (size, mtime, presence of a
`versions/` subdirectory) — heuristics here delete user data when wrong, which
is the worst possible failure direction.

*Failure mode:* same fatal flaw as I — GC is not the mechanism that hides the
data, the export is (see I above). Plus: the "refuse to delete" branch turns a
disk-reclamation feature into a silent no-op with no user-visible signal, on the
unattended background path (`apply.go:153`, whose return value is discarded:
`_ = GarbageCollectVersions(...)`). Nobody would ever find out. A *variant* of K
that does work is L.

---

**L. Migrate data forward on upgrade.** When installing a new version of a tool
whose previous version exported a data root at `{install_dir}`, copy or move
that data into the new version's directory before the export flips.

*Concrete change:* `internal/install/` install path, driven off the previous
version's recorded `set_env` params in `VersionState.Plan`.

*Failure mode:* it is the option `DESIGN-shell-d-lifecycle.md` already rejected
in a different guise. That design's Option A was rejected because
`VersionState.Plan` is unreliable — `ToStoragePlan` silently drops fields, plans
can be nil for older installs, and driving behavior off stored plans "adds
machinery that stays correct only while every future lifecycle event remembers
to call it." Copying a multi-gigabyte Node tree on every `nvm` point release is
also absurd on its face, and moving it makes rollback destroy the data instead
of hiding it — strictly worse than today.

---

**M. Guardrail only: make the class of bug unrepresentable.** Add a recipe
validator rule that rejects `set_env` exporting a known data-root variable
(`NVM_DIR`, `PYENV_ROOT`, `RBENV_ROOT`, `VOLTA_HOME`, `SDKMAN_DIR`, `GOPATH`,
`CARGO_HOME`) at a value containing `{install_dir}`.

*Concrete change:* `internal/recipe/validator.go`, `lint_test.go`.

*Failure mode:* it fixes nothing that exists — it prevents the *next* one. It is
worth shipping alongside whichever real fix wins, and worthless alone. It is
also the only option that generalizes at zero cost, because the list is data,
not code. Note the timing: nvm is currently the **only** recipe in the registry
using `set_env` at all (`grep -rl set_env recipes/` returns exactly
`recipes/n/nvm.toml`), so the rule costs nobody anything today.

### 5. Ranking

Acceptance criteria abbreviated: **U** upgrade preserves data; **G** nothing
GC'd contains user data; **B** `install`/`alias default`/`ls` behave like
upstream; **M** existing installs migrated or warned; **C** recipe comment
accurate; **T** testable.

| | Option | U | G | B | M | C | T | Blast radius | Generalizes? | `tsuku remove nvm` | Reversible? |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | **D** split: stable root + `nvm-exec`+`nvm.sh` placed there | yes | yes | yes | needs a step | yes | yes, with a shell harness | recipe + `{tsuku_home}` var in `actions/util.go` + one placement step | yes — `share/<tool>` becomes the convention | tool dir goes; `share/nvm` **survives**, unowned | high — path is a recipe value |
| 2 | **A** stable root, nothing else | yes | yes | **no** (`nvm exec`) | needs a step | yes | yes | recipe + one Go template var | yes | same as D | high |
| 3 | **J** respect user-set `NVM_DIR` (as a *companion*) | no | no | yes | n/a | partly | yes | `set_env.go` + `EnvVar` field | yes, cheaply | unchanged | high |
| 4 | **M** validator guardrail (as a *companion*) | no | no | n/a | no | n/a | yes | `validator.go` only | **best** — pure data | unchanged | high |
| 5 | **E** inverse symlinks | yes | yes | **no** (`nvm cache clear`) | needs a step | yes | yes | recipe + symlink step | poorly — per-tool path list | data survives, orphaned | medium |
| 6 | **C** deprecate/bootstrap-only | n/a | yes | yes | **no** | n/a | hard | product decision + registry | yes but excludes real tools | n/a | **low** |
| 7 | **B** drop the export | yes* | yes | yes* | no | needs rewrite | yes | recipe + Go (Fact 2) | yes | data outside `$TSUKU_HOME`, untouched | high |
| 8 | **I** per-recipe GC exemption | **no** | yes | yes | no | no | yes | schema + gc + apply plumbing | escape-valve flag | unchanged | medium |
| 9 | **H** pin to one version | **no** | yes | yes | no | no | yes | schema + resolver + updates | no | unchanged | medium |
| 10 | **K** teach GC about tool data | **no** | yes | yes | no | no | hard | gc + manifest/heuristic | no | unchanged | medium |
| 11 | **L** migrate data forward | yes | yes | yes | yes | no | hard | install path + stored plans | no | unchanged | **low** |
| 12 | **F** stable path → symlink to versioned dir | **no** | **no** | yes | no | no | yes | 3 install sites | no | dangling symlink | medium |

\* B's `yes` entries are for B *done properly* (an explicit `$HOME/.nvm`
export). B **as literally stated** — delete the `set_env` step — scores `no` on
U, G, and B, because `NVM_DIR` self-locates to `$TSUKU_HOME/share/shell.d`
(section 2).

Two notes on the table. **"Generalizes?"** is where D/A pull ahead: `pyenv`,
`rustup`, and `asdf` are all in the registry today and none of them uses
`set_env` — the moment one does, `share/<tool>` is a convention that already
exists, whereas E's symlink list and I's exemption flag are per-tool. **"`tsuku
remove nvm`"** is where D/A are weakest and it is not close: today removal takes
the data with the tool, which is at least coherent; under D/A the data outlives
removal with no command to reclaim it, because no action in tsuku emits a
`delete_dir` cleanup (verified — `grep delete_dir` outside tests hits only
`internal/install/state.go:18`, `internal/actions/action.go:57`, and the switch
arm at `internal/install/remove.go:424`).

## Implications

The exploration should stop treating this as a recipe bug. **Fact 2 means every
serious option requires a Go change**, so "recipe-only" is not a
differentiator — it is a property no viable option has. That reframing kills the
main argument for B-as-stated (smallest diff) and removes the main argument
against A/D (needs code).

Second, **the GC is a red herring**, and options I and K exist only because the
issue's framing points at it. `tsuku update nvm` hides the data in the next
shell, via the `set_env` fragment plus `ShellDSelection`'s active-version
filtering, not in seven days via `GarbageCollectVersions`. Any option that only
constrains GC leaves a bug that fires a week earlier and is harder to explain.
The exploration should re-word the problem statement accordingly before it
reaches a design.

Third, the decision reduces to **where the data root lives**, and there are only
three real answers: a tsuku-owned stable path (A/D), the user's home (B done
properly), or the versioned dir with compensating machinery (E/I/K/L). The third
family is uniformly worse on the evidence above. So the real question for round
two is A/D versus B-done-properly, and the tiebreaker is a product question, not
a technical one: **should `tsuku remove nvm` be able to delete a user's Node
installs?** If yes, the data must live inside `$TSUKU_HOME` (A/D) and tsuku must
grow a `delete_dir`-emitting cleanup or a `--purge` flag. If no, `$HOME/.nvm` is
right and tsuku should never have opinions about it.

Fourth, whatever wins should ship with **M** (the validator rule) and probably
**J** (respect a user-set value). Both are cheap, both are additive, and M is
free right now because nvm is the only `set_env` consumer in the registry.

## Surprises

- **`nvm.sh` needs exactly one repo-shipped file** (`nvm-exec`, nvm.sh:4083).
  I expected several, and that expectation is what would make option A look
  illegal. It is legal. But that one file is enough to break `nvm exec`, and no
  acceptance criterion in the issue names `nvm exec`.
- **`install_shell_init` copies nvm.sh rather than sourcing it.** This is
  load-bearing for option B and I do not think the issue realizes it: dropping
  the export puts nvm's data root in `$TSUKU_HOME/share/shell.d`, a `0700`
  directory that `RebuildShellCache` and `tsuku doctor` both walk.
- **`set_env` single-quotes its values** (`set_env.go:245`), so no recipe can
  ever reference `$HOME` or `$TSUKU_HOME`. This is a deliberate injection
  defense and it is correct — but it means the recipe-only framing of A and B
  is impossible, and the only workaround is `{install_dir}/../../...`, which
  smuggles a version string into a path that must be stable.
- **A stable directory at `tools/nvm-data` would be garbage-collected.** GC
  matches `strings.HasPrefix(name, "nvm-")` (gc.go:45) and `ReapVersion`
  no-ops on an unknown version rather than objecting (remove.go:394-397), so
  the delete goes through. Any "put it next to the tool" proposal must be
  checked against this.
- **`nvm cache clear` deletes symlinks** (`rm -rf` + `mkdir -p`, nvm.sh:3177).
  This single line is what makes option E unworkable, and it is invisible from
  the outside.
- **No action in tsuku ever emits a `delete_dir` cleanup**, despite the schema
  supporting it since whenever. Any option that creates a stable directory is
  creating tsuku's first piece of state with no removal path.
- **nvm is the only recipe in the registry using `set_env`.** The generalization
  question is therefore entirely prospective — and that makes now the cheapest
  possible moment to set the convention.

## Open Questions

1. **Does `tsuku remove nvm` delete the user's Node versions?** This is the
   product decision the whole ranking hinges on and I cannot answer it. It
   determines A/D versus B and it determines whether tsuku needs a `--purge`
   flag or a `delete_dir`-emitting action.
2. **Is `nvm exec` in scope for "behaves like upstream"?** If yes, A is out and
   D is in, and D must place both `nvm-exec` *and* `nvm.sh` in the stable dir.
   If no, A is materially cheaper than D.
3. **What does migration actually do to existing installs?** Move, copy, or
   warn-and-leave? A move is not atomic across filesystems and can be
   multi-gigabyte; a copy doubles disk; a warning is the honest minimum. Someone
   should check how many installs exist before assuming migration is required.
4. **Should the new template variable be `{tsuku_home}` or `{share_dir}` or a
   per-tool `{data_dir}` that expands to `$TSUKU_HOME/share/<tool>`?** The third
   is the most opinionated and the most reusable, and it is the one that makes M
   enforceable ("data roots must use `{data_dir}`"). It is also a schema
   commitment.
5. **Does `$TSUKU_HOME/share/` want a subdirectory convention at all**, or
   should tool data go somewhere new (`$TSUKU_HOME/data/<tool>`)? `share/`
   currently holds `shell.d`, `hooks`, and `completions` — all tsuku-generated,
   all disposable, all rebuilt from state. Putting irreplaceable user data in
   the same tree as three directories tsuku freely regenerates is an
   invitation to a future `rm -rf $TSUKU_HOME/share`.
6. **Is there prior art in the repo for a recipe that owns durable state?** I
   found none — `pyenv`, `rustup`, `asdf` all delegate to the tool's own default
   root. That absence is itself evidence for B, and I want a second reader to
   confirm it rather than take my grep at face value.

## Summary

`nvm.sh` works fine outside `$NVM_DIR` — the only repo-shipped file it needs is
`${NVM_DIR}/nvm-exec` at nvm.sh:4083, so option A is legal but breaks `nvm exec`,
while option B as stated does not restore upstream behavior at all: because
`install_shell_init` *copies* nvm.sh into `share/shell.d` and `$TSUKU_HOME/env`
sources the concatenated cache, dropping the export self-locates `NVM_DIR` to
`$TSUKU_HOME/share/shell.d`, not `$HOME/.nvm`. The deeper implication is that
`set_env` single-quotes its values and `GetStandardVars` has no `{tsuku_home}`,
so **no** option here is recipe-only, and the GC is a red herring — the data
goes invisible at the next shell start via the version-keyed fragment, not seven
days later via `GarbageCollectVersions` — which disqualifies options I, K, and H
outright. I would pick **D** (stable `$TSUKU_HOME/share/nvm` data root with
`nvm-exec` and `nvm.sh` materialized there, a new `{tsuku_home}`/`{data_dir}`
template variable, plus companions J and M); the strongest argument against my
own pick is that it makes tsuku the owner of a directory it has no lifecycle
for — no action in the codebase emits a `delete_dir` cleanup, so `tsuku remove
nvm` would leave gigabytes of Node installs behind permanently with no command
to reclaim them, and the biggest open question is whether `tsuku remove` should
be allowed to delete that data at all.
