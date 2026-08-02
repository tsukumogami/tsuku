# Exploration Findings: shell-d-lifecycle

Round 1 investigated seven leads. Every claim below is sourced to a research file
under `wip/research/`; file and line citations live there.

## What the evidence confirmed

**The core defect is real and the brief describes it accurately.** shell.d filenames
are a pure function of `(recipe's literal target, shell)` with no version component,
while contents are whatever that version produced. Nothing anywhere re-renders an
existing shell.d file — the only writes are at install time and the only other
operation is deletion.

**The trigger set is provable rather than guessed.** Exactly three non-test sites
assign `ToolState.ActiveVersion`: `Manager.InstallWithOptions`, `Manager.Activate`,
and `Manager.RemoveVersion`'s implicit promotion. `createSymlinksForBinaries` is
already called from all three. Only the install path re-renders shell.d, because only
it re-runs the install steps. shell.d is missing the exact equivalent of the binary
symlink layer's re-point — the codebase already solved this problem one artifact over.
`Rollback` is not a fourth site; it delegates to `Activate`. Auto-apply's failure
rollback also calls `Activate` directly from `internal/updates`, and does so with
stdout and stderr redirected to `/dev/null`.

**The content needed to re-render is on disk.** `Manager.InstallWithOptions` copies
`workDir/.install` wholesale into the versioned tool directory, so for any installed
version, every path under the staging `InstallDir` has a byte-identical twin under
`ToolInstallDir`. `install_shell_init`'s `source_file` mode is hardwired to
`ctx.InstallDir` — the deleted staging directory — which is the only reason its
content looks unrecoverable. Redirecting it at the versioned tool directory is
behavior-preserving during install and is what makes re-render possible outside one.

**The stored plan carries the params but not the phase.** `VersionState.Plan` is
populated on the normal install path and its `Params` survive the JSON round trip
(the `GetInt`/`GetStringSlice` accessors already normalize `float64` and
`[]interface{}`). But `executor.ToStoragePlan` silently drops `Phase`, `Dependencies`,
`Verify`, `RecipeType`, and `Binaries`, and no test notices. Any replay must therefore
select steps by action name, not by phase. The drop is independently a latent bug: the
stored plan doubles as the install-time plan cache, so a cache-hit reinstall already
re-executes a plan whose nested dependencies and verify blocks are gone.

**The import cycle is real but shallow, and it is not the one anyone assumed.** It runs
`actions -> version -> install`, closed by a single edge: `internal/version/resolve.go`
importing `internal/install` to call `install.ValidateRequested` — seventeen lines of
pure string validation over `unicode` and `strings` with no package-level dependencies.
Moving that one function to a leaf package severs the edge and lets `internal/install`
import `internal/actions`. `install -> executor` has a second independent edge through
`plan_conversion.go`.

**`doctor` already detects this exact state, and `--fix` cannot repair it — in one
variant it makes things worse.** Verified empirically by two independent agents. When
the cache matches disk but the hash does not match the active version's record, `--fix`
finds no stale shell and does nothing; `doctor` fails permanently. When the cache is
also stale, `--fix` rebuilds with hashes, `RebuildShellCache` *excludes* the mismatched
file, and if it was the only file for that shell the cache is deleted outright —
silently removing the tool from new shells. The re-check then reports stale again,
because `isCacheStale` recomputes the expected concatenation *without* hash filtering.
`--fix` never converges. The detection half of a repair path is already built.

**Hash verification is inert everywhere except `doctor --fix`.** Four of five
`RebuildShellCache` call sites pass no hashes at all, so every install and remove
concatenates whatever bytes are on disk, unverified.

## What the evidence corrected

**The brief's reproduction is narrower than stated in one direction and wider in
another.** Only removing the *active* version is broken; removing the inactive version
already leaves correct content. But there is a third failure mode the brief did not
name: if the surviving version predates cleanup recording and has no `CleanupActions`,
`otherPaths` is empty, the skip does not fire, the file is deleted outright, and the
active version ends up with no shell.d at all.

**The skipped cleanup also skips the cache rebuild.** `affectedShells[shell] = true`
sits *after* the `continue`, so the reproduction leaves `.init-cache.{shell}` holding
the removed version's content too. The cache is not merely stale — it is never touched.

**`00-env-nvm.bash` does not exist on `main`.** It is introduced only by the unmerged
prototype branch. The `nvm.bash` half of the reproduction is fully reproducible on
`main` from `recipes/n/nvm.toml`'s `install_shell_init` step alone, which is the
evidence that this is a lifecycle defect rather than a `set_env` defect.

**The post-install phase is dead code in production.** No recipe in the registry
declares a `phase`, so `ExecutePhase("post-install")` finds zero steps on every real
install. `install_shell_init` runs inside the *install* phase against the staging
directory. The prototype's `PhaseDeclarer` does not route within an established phase —
it introduces the post-install phase to real installs for the first time.

**Ruling out stable indirection was half right and led to the wrong conclusion.** Both
supporting observations are true: `{install_dir}` is versioned by construction, and
`tools/current/` is a *binary* namespace on `PATH`, not a tool-directory namespace. But
the conclusion does not follow. `AtomicSymlink` and `ValidateSymlinkTarget` are already
generic, the activation sites are already centralized, and adding an `opt/<tool>`
directory follows the existing `LibsDir`/`AppsDir` pattern. An opt-style link is cheap.
It just does not solve the whole problem — see the prior-art finding below.

**Candidate "store the content in state" was ruled out for the wrong reason.** "Only a
hash is stored" argues against inverting the hash, not against storing the bytes. And
because `CleanupActions` already live per-version, stored content would cover activate
and rollback by construction. Its real objection is different and unmeasured:
`nvm.sh` is ~100 KB, `state.json` is loaded in full on effectively every command, and
inflating it into a blob store taxes every invocation.

**Only one gap of the four blocks anything, and not the one expected.** All four
adjacent gaps are real and all four are pre-existing on `main`. None blocks the stated
acceptance criteria. Gap 1 (`--plan` never calls `RecordCleanup`) is worth folding in
anyway, because the two post-install blocks in `install_deps.go` and `plan_install.go`
are the same 25 lines copy-pasted and already drifted apart once, the re-render work
adds a third consumer, and the project's own documented sandbox test command routes
through the broken copy. Gap 4 (the already-installed short-circuit, unbypassable —
`--force` only suppresses security prompts, and the `--reinstall` flag that
`tsuku verify` recommends does not exist) forecloses any "just reinstall it" design
and must be settled before the mechanism is chosen, not after.

## What prior art says

Of eight mechanisms surveyed, tsuku's `share/shell.d/` most closely resembles
`/etc/profile.d` — version-specific content under a version-independent filename,
written once and never re-rendered. That is the one model in the survey with a
documented reputation for rotting; Debian Policy explicitly tells packagers not to
depend on it. tsuku is already better on three counts (deterministic byte-order
sorting rather than locale-dependent glob, recorded content hashes, and a health
check), and the remaining gap is precisely the re-render trigger.

The two mechanisms with the best track record — Homebrew's `opt_prefix` and Nix
generations — both combine a stable indirection path *and* a re-render step rather
than choosing between them. Homebrew has `opt/<formula>` and `inreplace`s built
artifacts to reference it; Nix has `~/.nix-profile` and rebuilds the whole generation
on activation. The two address disjoint halves: indirection fixes content that *names
a path*, re-render fixes content that *is a copy of the version's own bytes*. Neither
alone closes this reproduction, because `install_shell_init` copies all of `nvm.sh`
and no path substitution makes a byte copy version-agnostic.

Homebrew avoids the copy problem structurally: its `inreplace`d artifacts live *inside*
the keg, so they are per-version by location and a stale copy is impossible. mise does
the same thing for its cached per-tool env — the cache path is keyed by tool *and*
version, so a wrong-version file is unreachable rather than sourced. tsuku's
version-independent filename is what converts "cache miss" into "wrong content."

Separately, and independent of staleness: pointing `NVM_DIR` at a tsuku-versioned
directory is a recipe-level modeling error. `NVM_DIR` is nvm's data root as well as its
program directory — it holds `versions/node/`, `alias/`, `.cache/`, and
`default-packages`. Every node version the user installs and every global npm package
would live inside a directory tsuku versions and garbage-collects. A correctly
re-rendered `NVM_DIR` would still migrate the user's node installs to a fresh empty
tree on every nvm upgrade, and `GarbageCollectVersions` would eventually delete them.

## Constraints the implementation must respect

- The test cannot live in `internal/install` — all thirteen test files there are
  `package install`, and importing `internal/actions` is a cycle. `cmd/tsuku`
  (`package main`) is the only package where `install`, `actions`, `executor`, and
  `shellenv` already meet.
- CI always passes `-short`, so a `testing.Short()`-gated test never runs.
- The unit-test job fails if `git status --porcelain` is non-empty after the run.
- Functional tests run only `@critical`-tagged scenarios on PRs, and the existing
  "source home file and run" step discards command output.
- `errcheck` runs on test files and does not exclude `os.WriteFile` or `os.MkdirAll`.
  `dupl` fires at 250 tokens, so repeated multi-version scenario bodies need
  table-driven subtests.
- The golden-plan workflow triggers on a path allowlist that includes
  `internal/actions/action.go`, `internal/executor/plan.go`, `plan_conversion.go`, and
  `internal/recipe/types.go` — but *not* `set_env.go` or `shell_init.go`. Touching an
  allowlisted file arms a workflow that regenerates plans against live upstream and
  currently fails on unrelated Homebrew bottle drift. `main` still carries that drift;
  the prototype's regeneration never landed.
- There is no mutation-testing tooling in the repo. "Mutation-test the guards" means
  the manual discipline, and the applied defects should be enumerated in the PR body.
- `GarbageCollectVersions` deletes tool directories without touching `state.json`, so
  a version whose directory is already gone can still hold the `otherPaths` skip open.
  Any mechanism keyed on "another version still references this path" inherits that.

## Open design questions carried into scoping

These are decisions, not research gaps. Another discovery round would not settle them.

1. **What is the re-render mechanism?** Six candidates were characterized with their
   strongest objections: replay the stored plan's steps; a render capability interface
   separate from `Execute`; regenerate from the recipe; store rendered content in state;
   a stable per-tool indirection path; delete-plus-lazy-regenerate. The prior art
   favors combining indirection with re-render rather than choosing. This is the
   contested choice and warrants the decision framework.
2. **Where does the driver live, and does the `version -> install` edge get severed?**
   Moving `install.ValidateRequested` to a leaf package unlocks `install -> actions` and
   allows the hook to sit exactly where `createSymlinksForBinaries` already is. The
   alternative is a callback seam on `Manager` with the driver in `cmd/tsuku` — but
   `internal/updates` calls `Manager.Activate` directly, so a `cmd`-only driver would
   miss auto-apply's rollback.
3. **How wide is the writer scope?** `install_completions` has the identical defect in
   `share/completions/` with no detector at all. In, out, or its own issue — but stated
   either way rather than implied.
4. **Does `doctor --fix` gain a repair path?** The detection is already built and
   `--fix` currently cannot converge. Fixing the non-convergence is arguably a separate
   bug from adding the repair.
5. **Is `--no-shell-init` persisted?** It is not recorded anywhere today, so absent
   `cleanup_actions` is ambiguous between "user opted out", "installed via `--plan`",
   and "installed before cleanup tracking". A re-render driven off the plan alone would
   resurrect files a user deliberately declined.
6. **What happens to `recipes/n/nvm.toml`?** The lifecycle fix and the recipe's
   `NVM_DIR` model are separable, but shipping the first while leaving the second makes
   the fragment correct and still points nvm's data root at a disposable directory.

## Decision: Crystallize
