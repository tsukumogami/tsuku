# Decision Report: nvm data root

Decision question: *Where does nvm's data root live, what template surface
expresses it in the recipe, and what does `tsuku remove nvm` do to the data that
lives there?*

Complexity: critical. Full path — research, two adversarial validators, peer
revision, cross-examination, synthesis. Run in `--auto` mode.

<!-- decision:start id="nvm-data-root" status="assumed" -->
### Decision: nvm data root — location, template surface, and removal semantics

**Context**

`recipes/n/nvm.toml` exports `NVM_DIR = "{install_dir}"`, which resolves to
`$TSUKU_HOME/tools/nvm-<version>`. nvm treats `NVM_DIR` as its data root, so every
Node version, every global npm package, the `alias/default` that decides what a new
shell gets, and the tarball cache all live in a versioned, garbage-collected tool
directory. Because the `set_env` fragment carrying the export is version-keyed and
`ShellDSelection` serves only the active version's fragment, `tsuku update nvm`
repoints `NVM_DIR` at a fresh empty directory and the user's Node versions vanish at
the next shell start. Two other paths then delete the directory outright.

Ten of the twelve options enumerated during exploration are eliminated on evidence
and recorded as settled in `DESIGN-nvm-data-root.md`. Two survived into this
decision: a tsuku-owned stable root under `$TSUKU_HOME` (**A**), or nvm's own default
root `$HOME/.nvm` exported explicitly (**B**). Under both, tsuku must materialize
`nvm.sh` **and** `nvm-exec` inside the data root, or `nvm exec` and `nvm run` fail
with `rc=127` — nvm.sh runs `"${NVM_DIR}/nvm-exec"`, and `nvm-exec` self-locates via
an unresolved `BASH_SOURCE[0]` and re-sources `$DIR/nvm.sh`, so both files are
required. That requirement is what makes this decision hard: it means tsuku writes
program files into whichever directory wins.

**Assumptions**

- `nvm exec` and `nvm run` are in scope for "behaves like an upstream install"
  (stated as a decision driver in the design; not independently confirmed with the
  issue author).
- A user's Node estate is irreplaceable-in-practice data — reinstallable in
  principle, but multi-gigabyte and slow, and losing it silently is the failure this
  design exists to prevent.
- Deferring to a `NVM_DIR` the user set themselves is desirable. The exploration
  already ranked this as a cheap companion worth shipping; this decision assumes it
  and shapes the template surface around it.
- No `tsuku uninstall` command exists today and none is assumed to arrive as part of
  this work.

**Chosen: A — a tsuku-owned stable data root at `$TSUKU_HOME/data/nvm`**

Three welded answers.

**1. The tree: a new top-level `$TSUKU_HOME/data/`, per-tool subdirectory, so
`$TSUKU_HOME/data/nvm`.** Not `share/nvm`, and not `state/nvm`.

`share/` is demonstrably safe *today* — nothing in the codebase enumerates, prunes,
or `RemoveAll`s it or any subtree. That is not the point. The value of a separate
tree is that it can carry a **deletion policy**, and `share/` cannot carry the one
this data needs because its existing contents need the opposite one. `share/shell.d`,
`share/hooks`, and `share/completions` are tsuku-generated and regenerable by
definition; `RebuildShellCache` already rewrites inside that tree and
`rebuildShellCachesForTool` runs on the removal path. The invariant you want to be
able to state — *nothing under `data/` is ever deleted except by an explicit
user-initiated command* — is a true statement about a new `data/` and a false
statement about `share/`. Mixing irreplaceable user data into a tree whose defining
property is "safe to regenerate" is a category error that a future cleanup routine
will eventually cash in.

The historical argument for `share/` was cost, and that argument has evaporated
under measurement. Adding the tree is three lines in `internal/config/config.go` —
a struct field alongside `ShareDir` (`config.go:333`), one entry in the
`DefaultConfig` literal (`config.go:367`), and one entry in the `EnsureDirectories`
slice (`config.go:375-388`), following the optional-guard pattern `ShareDir` already
establishes at `config.go:390-394`. None of the 92 test `Config{}` literals need
touching: they use named fields, `NewTestConfig` (`internal/testutil/testutil.go:27-42`)
does not even set `ShareDir`, and production actions derive
`tsukuHome := filepath.Dir(ctx.ToolsDir)` and join rather than reading the config
field. The sandbox needs no change either — `EnsureDirectories` runs inside the
container.

`state/` is rejected on vocabulary: `state.json` already sits at
`$TSUKU_HOME/state.json`, and "state" in this codebase means tsuku's install ledger,
which is conceptually reconstructible. `data/` says user data, matches
`XDG_DATA_HOME`'s sense against `XDG_STATE_HOME`'s, and reads correctly to a user
browsing `~/.tsuku`. The name also buys standing to tell people that `rm -rf ~/.tsuku`
is the wrong way to uninstall.

**2. The template surface: a per-tool `{data_dir}`, expanding to
`$TSUKU_HOME/data/<tool>`.** The recipe line becomes:

```toml
[[steps]]
action = "set_env"
vars = [{name = "NVM_DIR", value = "{data_dir}"}]
```

Per-tool, not a bare root. The composed form (`{data_dir}` → `$TSUKU_HOME/data`, with
the recipe writing `{data_dir}/nvm`) mirrors `{libs_dir}`, but that precedent does
not bind: `{libs_dir}` is a bare root *because it exists for cross-tool reference* —
`{libs_dir}/openssl-{deps.openssl.version}/lib` reaches into another tool's libs, with
the version supplied by `{deps.*}`. A data root is never cross-tool; a recipe's data
root is its own by definition. The binding precedent is `{install_dir}`, which is
already per-tool-per-version. `{data_dir}` is to `{install_dir}` as stable is to
versioned, and that is the whole shape of the fix.

Per-tool also buys the two things a bare root cannot. It makes the bug
unrepresentable: a bare `{data_dir}` leaves nothing stopping a recipe from writing
`{data_dir}/nvm-{version}`, which is precisely the defect being fixed wearing a new
spelling, and it lets any of 1,449 recipes name a sibling tool's data. And it makes
the validator guardrail a string equality rather than a heuristic — a rule in
`validateSteps` (`internal/recipe/validator.go:484-559`, in the shape of the existing
set_env-in-library rule at `:503-506`) can require that a `set_env` step naming a
known data-root variable (`NVM_DIR`, `PYENV_ROOT`, `RBENV_ROOT`, `VOLTA_HOME`,
`SDKMAN_DIR`, `CARGO_HOME`, `GOPATH`) has value exactly `{data_dir}`.
`SetEnvAction.Preflight` (`set_env.go:58-87`) sees pre-expansion literals, which is
what such a rule needs.

The per-tool form costs almost nothing to thread, contrary to the initial
cost estimate. `ExecutionContext` has no tool-name field, but it does not need one:
`ctx.Recipe.Metadata.Name` is already in hand, and `envTargetName`
(`set_env.go:204-219`) already validates it as a safe single path segment —
rejecting `.`, `..`, separators, and `@`. That is exactly the validation a per-tool
data path requires, and it can be reused directly.

Expansion is **Go-side and local**, not added to `GetStandardVars`. `set_env.go:149-151`
already derives `tsukuHome := filepath.Dir(ctx.ToolsDir)`, so the value is available
at the point of use. Factor it as a small helper in `internal/actions` —
`dataDir(ctx)` returning `filepath.Join(filepath.Dir(ctx.ToolsDir), "data", <name>)`
— shared by `set_env` and by whatever action places `nvm.sh`/`nvm-exec` into the
root, since that is the second day-one consumer. Widening
`GetStandardVars(version, installDir, workDir, libsDir)` would push `{data_dir}` into
all nine actions that call it (`run_command.go:79`, `download.go:330`,
`extract.go:93`, `install_binaries.go:167`, `chmod.go:71`, `text_replace.go:58`,
`set_rpath.go:43`, `configure_make.go:105`, `set_env.go:147`) to serve one recipe;
promoting it later is a mechanical change if a real second consumer appears. Pair
this with a validator rule rejecting `{data_dir}` in any action that does not expand
it, so nobody writes it into a download URL and gets a literal brace.

**3. Removal: `tsuku remove nvm` preserves `$TSUKU_HOME/data/nvm`. `tsuku remove nvm
--purge` deletes it. No `delete_dir` cleanup action is recorded — ever.**

The structural rule, which matters more than the flag: **the data root's deletion
path must be reachable only from an explicit foreground user command, never from a
recorded cleanup action.** Cleanup actions are executed by `ReapVersion`
(`internal/install/remove.go:400`), which garbage collection calls from a detached
background process. The cross-version guard (`remove.go:339-372`) skips a path only
on exact `ca.Path` string equality against *another installed version's* recorded
actions — and `set_env` records no cleanup actions at all under `--no-shell-init`
(`set_env.go:100-104`), and pre-existing installs recorded none either. So there are
real sequences that end in silent data loss: install v1 normally (records the path),
install v2 with `--no-shell-init` (records nothing), v1 ages past retention,
auto-apply GC reaps v1, the guard finds no other version recording the path, and
`delete_dir` fires on the user's entire Node estate. That is the same shape as the
bug this design exists to fix, entering through a different door. Independently,
`delete_dir` (`remove.go:424`) has zero producers today; making irreplaceable user
data the first exercise of an untested arm is the wrong place to break it in.

`--purge` therefore computes its target from config (`data/<tool>`), not from a
recorded cleanup action. Concretely:

- One `Flags().Bool("purge", ...)` in `cmd/tsuku/remove.go:182-184` alongside the
  existing `--force`, read at `:31`; the bool threads into `RemoveVersion` /
  `RemoveAllVersions` (`internal/install/remove.go:102,223`), called from three sites
  (`cmd/tsuku/remove.go:86,96,160`).
- `--purge` is meaningful **only on whole-tool removal**. `tsuku remove nvm@X` with
  another version still installed must never purge — the data root is shared across
  versions by construction.
- Interactive `--purge` confirms via `confirmWithUser` (`cmd/tsuku/create.go:166-180`),
  printing the resolved path and its size first.
- Non-interactive `--purge` **errors** with `ExitNotInteractive`
  (`cmd/tsuku/exitcodes.go:47`). It must not silently proceed and it must not
  silently skip — `confirmWithUser` returns false on a non-TTY, so gating on it
  alone would make `tsuku remove nvm --purge` a silent no-op in scripts, which is
  the same silent-no-op failure the drivers already condemn for `--no-shell-init`. A
  script that genuinely wants this removes the printed path itself, which is safe
  precisely because `~/.tsuku/data/nvm` names the thing exactly.
- `--purge` must tolerate an absent state entry, so a user who already ran
  `tsuku remove nvm` can still reclaim the directory. Otherwise the reclaim path is
  unreachable exactly when it is needed.
- Bare `tsuku remove nvm` prints the retained path and the `--purge` hint. This is a
  foreground command, so `renderBackgroundSuccess`'s dropping of `Messages`
  (`internal/updates/notify.go:155-171`) does not apply.

**Rationale**

Option B does not survive the requirement that put both options in the same boat.
Because `nvm.sh` hardcodes `"${NVM_DIR}/nvm-exec"`, B is not "let nvm own its own
directory" — it is "tsuku writes program files into `$HOME/.nvm`." From there, every
path is bad:

- If `$HOME/.nvm` does not exist, tsuku creates it as a plain directory. A user who
  later runs upstream's installer hits `install_nvm_from_git()`: the directory exists
  and is non-empty with no `.git`, so `install.sh` runs `git init` **in place**, adds
  the nvm-sh remote, fetches, and runs `git checkout -f --quiet FETCH_HEAD` — then
  `reflog expire --expire=now --all` and `gc --prune=now`.
- If `$HOME/.nvm` is already an upstream git working tree, tsuku clobbers two tracked
  files, and upstream's *only* upgrade path (re-running `install.sh` — there is no
  `nvm upgrade` subcommand) force-reverts them with the reflog expired immediately
  after. The two package managers ping-pong over the same file, and `state.json`
  claims a version the shell does not run.
- tsuku cannot clean up after itself. Cleanup actions resolve as
  `filepath.Join(m.config.HomeDir, ca.Path)` (`remove.go:418`) with `HomeDir` =
  `$TSUKU_HOME`, so a cleanup action structurally cannot name `$HOME/.nvm`. B would
  need an entirely new absolute-path cleanup channel, and the only precedent —
  `ApplicationSymlink` (`internal/install/state.go:31`, `remove.go:159-160,292-294`)
  — is a bare unverified `os.Remove` on a symlink tsuku exclusively owns.

So B's honest options are clobber a file tsuku does not own, or guard and silently
no-op while reporting success. Both decouple what `tsuku list` says from what the
shell runs. For a package manager whose stated philosophy is reproducibility, that
is disqualifying.

The two strongest arguments for B both invert under inspection.

*"Every other version manager in the registry delegates to the tool's own root."*
pyenv, rbenv, rustup, and asdf delegate by writing **nothing** into the data root —
tsuku installs a binary and the tool bootstraps its own directory. nvm has no
binary; nvm *is* `nvm.sh`, and `nvm exec` demands `nvm-exec` beside the data. The
analogy fails at exactly the point that matters, and the B validator conceded this
plainly.

*"Homebrew hit this exact bug and answered `$HOME/.nvm`."* This was the B case's
load-bearing claim and it was the one place the two validators asserted opposite
facts, so I verified the formula directly. Homebrew's `install` block writes nothing
into `$HOME/.nvm`. It writes a **shim into Homebrew's own prefix** which the user
then sources by hand, per a caveat that tells the user to `mkdir ~/.nvm` themselves.
That shim symlinks into the data root from `opt_libexec` — Homebrew's
*version-independent, package-manager-owned* stable path, not the versioned Cellar
path, which is why `[ -e ] || ln -s` (create once, never refresh) is safe across
upgrades. The export is **conditional**: `[ -z "$NVM_DIR" ] && export
NVM_DIR="$HOME/.nvm"`, with an inline comment saying it exists to avoid destroying
user-installed nodes. And the caveat opens: *"Please note that upstream has asked us
to make explicit managing nvm via Homebrew is unsupported by them."*

Read properly, Homebrew endorses a package-manager-owned stable program path with
conditional deference on the data root — and tsuku's `set_env` cannot express the
conditional half at all (`set_env.go:169` emits an unconditional `export %s=%s` of a
single-quoted literal). Option B as framed is therefore *strictly more aggressive
than the precedent it invokes*: it clobbers a deliberate user `NVM_DIR` where
Homebrew defers to it, inside a tree Homebrew declines to touch, under an
arrangement upstream calls unsupported.

The B validator's final position is the most useful output of the bakeoff. It
concedes B-as-framed is unreachable and identifies the only form of B that never
writes outside `$TSUKU_HOME` — emit a Homebrew-style shim as the shell.d fragment,
with the conditional export and `[ -e ] || ln -s` executing in the user's shell at
source time. But that shim needs a version-independent tsuku-owned path for its
symlinks to target, which is Option A's stable path. **The reachable version of B is
built on A. The dichotomy was false**, and A is the part both branches need.

Finally, the design's own argument that A "gets isolation for free" from
`$TSUKU_HOME` overrides is **overstated and should not be carried into the design
doc**. `internal/sandbox/executor.go:333-334` hardcodes both `TSUKU_HOME` and
`HOME=/workspace` (with `HOME` in `protectedEnvKeys` at `:48`),
`internal/validate/executor.go:243-244` and `internal/validate/source_build.go:157-158`
do the same, and `cmd/tsuku/shelld_lifecycle_test.go:200` already sets
`HOME=` its own tempdir. `$HOME` is isolated in CI. The surviving, weaker version of
the argument: `Config.HomeDir` is isolated *by parameter* — a struct field the
compiler makes you thread — while `$HOME` is isolated *by discipline*, and
`internal/actions/nix_portable.go:39-43` and `internal/actions/util.go:653-658`
already hardcode `~/.tsuku` paths from `os.UserHomeDir()` and escape `TSUKU_HOME`
isolation, which is standing proof that discipline fails.

**Alternatives Considered**

- **B — `$HOME/.nvm`, exported explicitly.** Rejected because nvm's `nvm-exec`
  requirement forces tsuku to write program files into a directory upstream manages
  as a git working tree, where upstream's only upgrade path force-reverts them and
  where tsuku's cleanup mechanism structurally cannot reach
  (`remove.go:418` joins under `$TSUKU_HOME`). Its two headline arguments — the
  registry's delegate-to-upstream convention, and the Homebrew precedent — both
  reverse on inspection.
- **A rooted at `share/nvm`.** Rejected because `share/` holds only tsuku-generated,
  regenerable state, so it cannot carry the never-delete-implicitly policy this data
  needs. The cost advantage that motivated it is three lines.
- **A rooted at `state/nvm`.** Rejected on vocabulary: `state.json` already claims
  the word for tsuku's reconstructible install ledger.
- **A bare-root `{data_dir}` → `$TSUKU_HOME/data` with recipe-side composition.**
  Rejected: it re-permits `{data_dir}/nvm-{version}`, lets a recipe name a sibling
  tool's data, and turns the validator guardrail from a string equality into a
  heuristic. The `{libs_dir}` precedent it appeals to exists for cross-tool
  reference, which a data root never needs.
- **`{tsuku_home}` or `{share_dir}`.** Rejected: both leak the whole path namespace
  into recipes to solve one problem, and neither makes the guardrail enforceable.
- **Recording a `delete_dir` cleanup so `tsuku remove nvm` takes the data.**
  Rejected: `ReapVersion` (`remove.go:400`) runs cleanup actions from background GC,
  and the cross-version guard fails open when the other installed version recorded
  nothing — which `--no-shell-init` guarantees (`set_env.go:100-104`). It would
  reintroduce silent unattended deletion of user data.
- **Preserving the data with no reclaim command at all.** Rejected as the honest
  complaint against A that is cheap to answer; leaving a growing directory with no
  way to name or remove it is not a coherent answer to "what does removal do."

**Consequences**

What becomes true: upgrading nvm preserves Node versions by construction rather than
by machinery firing at the right moment; a same-version reinstall
(`internal/install/manager.go:183`) and GC both become no-ops with respect to user
data; `nvm exec` and `nvm run` keep working, verified empirically for this layout;
and `share/<tool>` never becomes a convention that later has to be walked back.

What becomes easier: the next recipe with a data root has `{data_dir}` waiting, and
the validator rule makes the original bug unrepresentable in recipe form — worth
noting that nvm is the only recipe of 1,449 using `set_env`, so this is the cheapest
moment this convention will ever be set. `tsuku` also gains a directory it can name
in `doctor` output and size reporting.

What becomes harder, honestly:

- **`rm -rf ~/.tsuku` is now destructive to user data, and there is no code-level
  fix.** No `tsuku uninstall` exists (32 commands, `cmd/tsuku/main.go:159-190`),
  which is exactly why the folk wisdom exists. Both validators independently refused
  to wave this off and I will not either. It needs a documentation answer, a
  `data/README`, and — as a follow-up outside this decision — a real uninstall
  command that warns. This is the strongest surviving argument against the choice.
- **`make clean` (`Makefile:15`, `rm -rf .tsuku-dev .tsuku-test`) destroys a
  contributor's dogfooded data root**, since dev builds default `TSUKU_HOME` to
  `.tsuku-dev`. Contributor-facing, worth a note in CONTRIBUTING, not a blocker.
- **A user who already has `~/.nvm` gets an empty root and a confusing moment.**
  Detecting `$HOME/.nvm/versions/node` and emitting a migration hint turns this into
  a recoverable prompt. The counterfactual under B is worse — silent no-op or
  clobber — so this is a downgrade of the failure, not a new one.
- **`tsuku` now owns a directory that can be the largest thing on the machine.**
  That is the price of the guarantee, and `--purge` plus size reporting is the
  accounting for it.
- **A user who runs upstream's `install.sh` with tsuku's export live** gets a stray
  `.git` in `data/nvm` and tsuku's two files replaced, which the next
  `tsuku update nvm` overwrites back. `checkout -f` carries no `-d` and there is no
  `git clean`, so untracked `versions/` survives — untidy, not destructive. The
  unmarked `export NVM_DIR=...` line `install.sh` appends to the user's rc file is a
  wash between the options, since it echoes whatever tsuku exported either way.

Constraints this decision hands to the still-open placement question (how `nvm.sh`
and `nvm-exec` get into the data root — deliberately not decided here):

- The placement target is `$TSUKU_HOME/data/<tool>` and it is tsuku-owned, so
  placement can overwrite unconditionally rather than guarding. Under B it could
  not, and that asymmetry is a large part of why A wins.
- **Prefer symlinks to copies.** Both validators converged here, and the B
  validator reversed its own earlier recommendation on a good argument: a dangling
  symlink is *self-identifying*, because `[ -e ]` follows links and is false on a
  dangling one, so a `[ -e ] || ln -s` placement correctly re-creates tsuku's own
  litter while correctly deferring to a real file. A stale copy is indistinguishable
  from a user's install, so any guard against overwriting a user file also silently
  refuses to refresh tsuku's own.
- **Repointing on upgrade is load-bearing state.** `remove.go:150-152` `os.RemoveAll`s
  the tool directory, so a link into `tools/nvm-<version>/` dangles the moment that
  version goes, returning `nvm exec` to `rc=127` while every common command keeps
  working — a regression that would ship green. The regression test must run
  `nvm exec` *after* a simulated upgrade, not only after a fresh install.
- **tsuku has no `opt/`-equivalent.** Homebrew's placement works because
  `opt_libexec` is a version-independent per-tool program prefix, which is why its
  create-once `[ -e ] || ln -s` never needs refreshing. tsuku's nearest thing,
  `CurrentDir` (`config.go:322,356`), is a flat directory of binary symlinks
  (`app_bundle.go:186`), not a per-tool prefix. Placement therefore either points
  into the versioned tool directory and repoints on upgrade, or the design invents a
  stable program path too. That choice belongs to the placement question, but it
  should be made knowingly rather than defaulted into.

One scoping note on the Homebrew evidence, conceded to the B validator: `opt_libexec`
is Homebrew's answer to referencing the *program* without version-keying, not to
where the *data* lives, and on the data root Homebrew does name `$HOME/.nvm`. The
formula is therefore cited above as precedent for A's machinery and against B's
write-into-a-foreign-tree posture — not as precedent for A's data-root location.
Nothing in the chosen decision rests on it being the latter.
<!-- decision:end -->

---

## Adjacent recommendation (separable, flagged rather than folded in)

Both validators converged independently on something the decision question did not
ask about, and it changes the template surface enough to state: **`set_env` should
grow a way to express "default, do not override."**

Under the chosen design the emitted line is an unconditional
`export NVM_DIR='/home/u/.tsuku/data/nvm'` (`set_env.go:169`). A user who sets
`NVM_DIR` in their own rc file wins or loses purely on sourcing order, which is a bad
property. Homebrew's formula solves the identical problem with
`[ -z "$NVM_DIR" ] && export NVM_DIR="$HOME/.nvm"`, and the exploration already
ranked "respect a user-set value" as a cheap, additive companion worth shipping with
whichever option won.

The concrete shape: an `if_unset = true` (or `default = true`) flag on the `set_env`
var entry, emitting the conditional form. It is bounded, it is directly responsive to
"what template surface expresses the data root," and it is the mechanism the B
validator identified as the only way its own option could have been reachable.

This can be deferred without invalidating anything above — the location, the
`{data_dir}` spelling, and the removal semantics all stand on their own.

## Also worth carrying into the design doc

- The design's claim that A "gets isolation for free" because `$TSUKU_HOME` is what
  isolation and validation containers override is **overstated** and should be
  corrected: `HOME` is overridden too (`internal/sandbox/executor.go:333-334`,
  `internal/validate/executor.go:243-244`, `internal/validate/source_build.go:157-158`,
  `cmd/tsuku/shelld_lifecycle_test.go:200`). The decision does not rest on it.
- `internal/actions/nix_portable.go:39-43` and `internal/actions/util.go:653-658`
  hardcode `~/.tsuku` paths from `os.UserHomeDir()` and ignore `TSUKU_HOME`,
  silently escaping dev-home isolation. Unrelated to this issue; worth its own issue
  alongside the GC prefix-matching bug already filed out of scope.
