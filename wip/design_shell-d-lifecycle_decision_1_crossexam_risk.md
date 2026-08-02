# Cross-examination: landing risk

Lens: which option lands as one review-ready PR with green CI, and which ships a new
bug. Not judging elegance. All verification done in this worktree against
`docs/shell-d-lifecycle` (based on `origin/main`); `origin/fix/2439-set-env-exports` was
fetched and read, not modified.

## Blast-radius counts

Counted as files, non-test. "Forced collateral" means: omit it and the option introduces
a *new* bug or does not compile. "Optional" means the option works without it.

| Option | Core to the fix | Forced collateral | Optional | Total non-test | Test files touched |
|---|---|---|---|---|---|
| A | 8 | 8 | 2 | **18** | ~5 |
| B (sidecar) | 5 | 1 | 3 | **9** | ~3 |
| C | 7 | 5 | 0 | **12** | ~8 |

**Option A — 18.** Core (8): `internal/actions/renderer.go` (new), `shell_init.go`,
`set_env.go`, `internal/install/shelld.go` (new), `manager.go`, `remove.go`,
`state_ops.go`, `internal/shellenv/lock.go` (new). Forced collateral (8) — the whole
import sever, which A cannot skip because the driver lives in `internal/install` and
`internal/install` cannot import `internal/actions` today: `internal/pin/pin.go` (new),
`internal/pin/pin_test.go` (moved), `internal/install/pin.go` (deleted),
`internal/version/resolve.go`, `internal/updates/apply.go`, `internal/updates/checker.go`,
`cmd/tsuku/outdated.go`, `cmd/tsuku/update.go`. Optional (2): the `plan_install.go` /
`install_deps.go` post-install helper extraction.

A's own doc says 16. It undercounts by folding `plan_install.go` + `install_deps.go`
into one table row and by not counting the deletion. The sever count is otherwise
accurate: I verified `internal/version/resolve.go` is the only file in `internal/version`
importing `internal/install` (lines 18, 28, 43 — `ValidateRequested`,
`PinLevelFromRequested`/`PinChannel`, `VersionMatchesPin`), and that exactly five
non-test files outside `internal/install/pin*.go` use those symbols.

**Option B — 9, and only if you ship the sidecar variant.** Core (5):
`internal/install/state_ops.go` (capture), `internal/install/restore.go` (new),
`manager.go`, `remove.go`, plus the blob store helper. Forced collateral (1):
`internal/install/state.go` — the inline variant's `Content []byte` field and the
`0644 → 0600` permission fix that field forces; the sidecar variant needs neither, which
is why I count it at 1 (the `CleanupAction` doc comment) rather than 2. Optional (3):
`cmd/tsuku/doctor.go` (`--fix` repair), `plan_install.go` (RecordCleanup),
`internal/updates/gc.go` (orphan-blob sweep).

B is the only option whose optional column is genuinely optional. Omitting all three
leaves B correct, just narrower.

**Option C — 12, all of them load-bearing.** Core (7): `internal/actions/shell_init.go`
(rename + target charset preflight), `internal/shellenv/cache.go` (manifest gate),
`internal/install/shelld.go` or `state_ops.go` (manifest builder),
`internal/install/manager.go` (Activate hook), `internal/install/remove.go` (promotion
hook + rebuild reorder), `cmd/tsuku/install_deps.go`, `cmd/tsuku/doctor.go`. Forced
collateral (5), every one of which is "omit it and you ship a regression":

- `internal/shellenv/doctor.go` — `isCacheStale` must filter identically to the builder
  or `doctor` fails permanently on every multi-version install; `HasShellIntegration`
  probes `{tool}.bash` by name and returns wrong answers under the new scheme.
- `internal/install/update.go` — `StaleCleanupActions` deletes the rollback target's
  shell.d file. See below; this is the sharpest one.
- `internal/updates/gc.go` — GC'd versions leak ~200 KB of shell.d per version forever.
- `cmd/tsuku/plan_install.go` — must gain the `RecordCleanup` it has never made, or
  `tsuku install --plan` produces a tool with no shell integration.
- `cmd/tsuku/info.go` — `HasShellIntegration`'s only caller has to move.

C's own doc lists all twelve honestly. My count matches its list. The difference between
C and A is not the file count — it's that C's twelve are spread across six packages and
five of them are "fix this or regress," where A's eight collateral files are a mechanical
identifier rename that the compiler checks for you.

## Silent-breakage analysis

**Option C is the answer to "which one hides a working shell integration."** It is not
close, and the mechanism is structural rather than incidental: C changes the cache from
"include every file, and include unrecorded ones unverified" to "include only what
`state.json` names." Every existing behavior that depends on the permissive default
becomes a silent deletion of function. Three concrete paths:

1. **Unrecorded writers.** Anything that writes shell.d without a `RecordCleanup` stops
   being sourced the moment C lands. I found three, not one — see the next section.
2. **`StaleCleanupActions` deletes the rollback target.** This is confirmed, not
   theorized. `internal/install/update.go:14-36` computes old-minus-new by
   `(action, path)`, and `ExecuteStaleCleanup` (`update.go:43-62`) calls
   `executeSingleCleanup` directly with **no `otherPaths` guard** — unlike
   `executeCleanupActions` in `remove.go:332-349`, which has one. Under C, upgrading nvm
   0.40.5 → 0.40.6 produces old = `[nvm@0.40.5.bash, nvm@0.40.5.zsh]` and new =
   `[nvm@0.40.6.bash, nvm@0.40.6.zsh]`, disjoint, so all four old actions are "stale" and
   the previous version's files are deleted while that version is still installed and is
   the rollback target. `tsuku rollback` then hands the user a version with no shell
   integration and nothing to restore it from — C has no re-render. `cmd/tsuku/update.go:183`
   is the live call site. No test on `main` catches this: `internal/install/update_test.go`
   builds every case from version-independent paths.
3. **The migration residue is a permanent split-brain, not a transient one.** C
   grandfathers legacy records as valid manifest entries, which works — but a user with
   two pre-upgrade versions sharing `nvm.bash` keeps a hash mismatch forever, and hash
   mismatch under C means *excluded from the cache*. On `main` that user gets wrong
   content; after C they get no content. That is the "we got it subtly wrong" case
   turning a silent wrongness into a silent absence, which for a tool that exists only as
   a shell function is not obviously the better failure.

**Option A's silent-breakage mode is a delete, and it is new.** The driver's step 5
deletes every path in `ownedEver \ ownedNow`. When `ownedNow` is empty — a version with
no recorded `CleanupActions`, i.e. a `--plan` install, a pre-plan migrated entry, or a
`--no-shell-init` install — the driver skips rendering and goes straight to the delete.
So `tsuku activate nvm <plan-installed-version>` removes the other version's shell.d file.
On `main`, activate does nothing and the user keeps a stale-but-working file. A's doc
frames this as the intentional `--no-shell-init` demotion case and it is correct that
demotion should delete; what it does not say is that "installed with `--no-shell-init`"
and "installed by a path that forgot to record" are indistinguishable from `ownedNow`, so
the same rule fires for both. A's `plan_install.go` fix converts future `--plan` installs
but cannot backfill existing ones. Severity: lower than C's, because it needs a
`--plan`-installed or pre-plan version to be the activate target, but it is a fresh
deletion of a working file that `main` does not perform.

A's second-order risk is the driver writing files during `remove`. Bounded well —
temp+rename per file, warn-never-fail, idempotent — but the recorded-hash rewrite means a
render that disagrees with the record silently *wins*. A names the pipx-shebang case as
the one place `renderSourceRoot` is not provably a no-op. Low probability, and A discloses
it.

**Option B's failure mode is performance, not correctness, and the author already
measured it out of the running.** Inline: 240 µs → 5.0 ms state load on a clean install
(12x gzipped, 20x raw), on a file read by effectively every command. That is a real
regression and B's own doc says the inline variant should not ship. The sidecar variant
has no hot-path cost and no schema change. Its residual failure mode is a missing blob,
which is indistinguishable from absent content and takes the same warn-and-leave-alone
path — it never deletes. B is the only option of the three that cannot remove a file the
user is depending on.

## Unrecorded shell.d writers

The question that decides C. Enumerated by walking every `ExecutePlan` caller and every
context in which `install_shell_init.Execute` (and the prototype's `set_env`) can run.

Recording is not done by the action — the action appends to
`ExecutionContext.CleanupActions` (`shell_init.go:145`, `completions.go:110`) and the
*caller* persists it via `exec.GetCleanupActions()` → `convertCleanupActions` →
`mgr.RecordCleanup`. So the enumeration is of callers, and there are four.

| # | Writer path | Records? | Verdict |
|---|---|---|---|
| 1 | `cmd/tsuku/install_deps.go:522` (`ExecutePlan`) + `:579` (`ExecutePhase`) | **Yes**, `RecordCleanup` at `:613` | Safe |
| 2 | `cmd/tsuku/plan_install.go:83` + `:119` | **No** — `grep -rn "RecordCleanup("` minus tests yields only `install_deps.go:613` | Known; C must fix |
| 3 | `internal/executor/executor.go` `installSingleDependency` (~`:838`) | **No** — builds its own `execCtx`, never assigned to `e.ctx`, so `GetCleanupActions()` cannot see it; writes no state at all | **New find** |
| 4 | `cmd/tsuku/install_lib.go:138` (`ExecutePlan`) | **No** — the file contains no `GetCleanupActions`, no `RecordCleanup`, and no `RebuildShellCache` at all | **New find** |

Two notes that matter for reading this table:

- `install_shell_init` runs in the **install** phase, not post-install, because no recipe
  declares `phase` (`executor.go:553` skips non-install steps in `ExecutePlan`;
  `ExecutePhase("post-install")` then finds zero steps). Path 1 still records, because
  `ExecutePlan` sets `e.ctx = execCtx` and `GetCleanupActions()` returns that same
  context's accumulated actions. So the `RecordCleanup` at `install_deps.go:613` does
  capture install-phase writes. Good — but it means the recording is incidental to a
  shared-context detail, not to the phase design.
- Path 3 is worse than "does not record": `installSingleDependency`'s `execCtx` sets no
  `NoShellInit`, and its execution loop does not filter by phase at all, so it runs
  *every* step of the dependency's plan including post-install ones. A dependency recipe
  with `install_shell_init` would write shell.d, ignore `--no-shell-init`, and record
  nothing.
- `tsuku update` and background auto-apply both route through `runInstall` /
  `runInstallWithReporter` (`cmd/tsuku/update.go:136`, `:335`;
  `internal/updates/apply.go:189` via the injected `InstallFunc`), i.e. path 1. Those are
  safe. I checked this specifically because an unrecorded auto-update writer would be the
  worst possible finding for C — a tool silently vanishing from shells after an unattended
  background update. It does not happen.

**Verdict on C: not safe as scoped, but the hazard is latent rather than live.** No
shipped recipe reaches paths 3 or 4 today — `recipes/n/nvm.toml` is the only
`install_shell_init` consumer and it is installed as a top-level tool. So C would not
break a real user on merge. What C would do is convert two dormant gaps into a
class of bug where the symptom is "the tool is gone from my shell and nothing says why,"
with no error, no log line, and a `doctor` that reports the file as unrecorded rather
than as broken. C's own falsifier #3 says exactly this: *"If there is a supported path
that writes shell.d without recording — one that cannot be fixed the way `plan_install.go`
can — then a manifest-gated cache silently drops a working integration, and the whole
approach is a breaking change dressed as a bug fix. I found exactly one such path and it
is fixable."* There are three, and path 3 is not fixable the way `plan_install.go` is: it
needs the dependency installer to acquire a state-writing seam it does not have, which is
scope C did not budget.

## CI reality

### Golden allowlist

Read from `.github/workflows/validate-golden-code.yml:6-45`. The `paths:` list is:
`cmd/tsuku/eval.go`; `internal/executor/plan_generator.go`, `plan.go`,
`plan_conversion.go`; `internal/actions/decomposable.go`, `action.go`, `composites.go`,
`download.go`; `internal/recipe/types.go`, `loader.go`, `platform.go`; the nine package-
manager decomposers; **`internal/version/*.go`** (with `!internal/version/*_test.go`);
the workflow file; and `testdata/golden/code-validation-exclusions.json`.

| Option | Arms it? | Which file |
|---|---|---|
| A | **Yes** | `internal/version/resolve.go`, matched by `internal/version/*.go` |
| B | No | Nothing in `internal/actions`, `internal/executor`, `internal/recipe`, or `internal/version` |
| C | No | `shell_init.go`, `cache.go`, `shellenv/doctor.go`, `gc.go` — none listed |

**Both advocates' claims are confirmed.** A's is unavoidable in its primary form: the
cycle edge runs *into* `internal/install` *from* `internal/version`, so severing it means
editing a file under `internal/version`, and a `paths:` filter counts an edit as a change.
A's fallback (a leaf `internal/shellrender` package with an `init()`-registered parallel
registry) does avoid the allowlist, at the cost of a second registry and a link-order-
dependent silent no-op. C touches nothing listed — also confirmed by direct comparison of
its twelve files against the list.

### What arming it actually costs

The drift is real and it is genuine upstream Homebrew bottle drift, not an artifact of the
prototype's schema change. `git diff origin/main...origin/fix/2439-set-env-exports --
testdata/golden/plans/embedded/gcc-libs/` shows four files (arch, debian, rhel, suse,
all linux/amd64), each changing a bottle sha256 in three places plus a `size` field —
`606c8f50…` → `eebab973…`, 161,998,997 → 161,971,540 bytes. Nothing about `Phase` or any
new field. So `main` carries stale goldens that a triggered run would fail on.

`gcc-libs` is **not** in `testdata/golden/code-validation-exclusions.json` (seven entries:
ack, curl, fpm, jekyll, ninja, readline, sqlite, all pointing at issue #988). The workflow
runs `validate-all-golden.sh --os linux --category embedded`, which walks
`testdata/golden/plans/embedded/*/`, so all four gcc-libs files are validated. An
Option A PR gets a red check on day one.

**Is regenerating a real unblock?** Partially, and it is a moving target. It does not need
special credentials: `scripts/regenerate-golden.sh:23-32` auto-detects `GITHUB_TOKEN` from
`gh auth token` and fails fast if absent — a normal `gh auth login` is sufficient, no R2
or AWS secrets involved. But bottle shas are whatever Homebrew currently serves. The
prototype regenerated these at some point and they have presumably drifted again since;
regenerating on the PR branch fixes the check *at that moment* and can go red again before
merge if Homebrew re-bottles gcc. The alternative — adding gcc-libs to the exclusions file
— requires an open issue link, because the workflow runs
`validate-golden-exclusions.sh --check-issues` before validating, and editing that file
also arms the workflow. So A's options are: regenerate and hope the merge window is short,
file an issue and add an exclusion (which is itself scope and an admission of drift), or
take the fallback registry design and lose most of A's claim to being the idiomatic choice.

### Broken tests

Counted as test functions that fail to compile or fail an assertion, from
`awk '/^func Test/{f=$2} /<symbol>/{print f}' | sort -u`.

| Option | Count | Breakdown |
|---|---|---|
| A | **~5** | `internal/install/pin_test.go` 4 functions move package (rename, not break); `remove_test.go` ~3 for the `affectedShells` change and cache-rebuild reorder (`TestRemoveVersion_CleanupMultiVersionSafety`, `TestRemoveVersion_ExecutesCleanupActions`, `TestRemoveAllVersions_ExecutesCleanupActions`); `shell_init_test.go` should survive intact since `Execute` is behavior-preserving |
| B | **~0–2** | `internal/install/state_test.go` has 2 shell.d references; a `0644` permission assertion if one exists. Sidecar variant: closer to 0 |
| C | **~50** | See below |

C's breakdown, measured:

- `internal/shellenv/cache_test.go`: **19 of 19** test functions call `RebuildShellCache`.
  Every one is a compile break from the variadic-to-required signature change, and most
  are also *semantic* breaks: `TestRebuildShellCache_ConcatenatesFiles`,
  `_SortedAlphabetically`, `_OnlyMatchesCorrectShell`, `_LegacyTolerance_NilHashMapIncludesAll`
  and friends all construct files on disk and assert they appear in the cache. Under a
  manifest gate, files with no manifest entry are excluded, so these do not just need a
  new argument — they need a manifest fabricated for every fixture.
  `_LegacyTolerance_NilHashMapIncludesAll` tests the exact behavior C deletes.
- `internal/shellenv/doctor_test.go`: **8** call `CheckShellD` (signature change), **2**
  call `RebuildShellCache`, **3** are `TestHasShellIntegration_*` and get deleted with the
  function. Effectively 11 of 11.
- `internal/install/update_test.go` + `cmd/tsuku/update_test.go`: **7** —
  `TestStaleCleanupActions`, `TestUpdateStaleCleanup_EndToEnd`, and five
  `TestWarnShellInitChanges_*` (all of which lose their premise, since under C no path is
  present in both old and new).
- `internal/actions/shell_init_test.go`: **6** of 16 assert on filenames.
- `internal/install/remove_test.go`: **~3** with shell.d paths.
- `cmd/tsuku/doctor_test.go`: **~2**.

C's advocate predicted it would break more. It breaks an order of magnitude more, and
critically the shellenv breakage is not mechanical — you cannot fix 19 cache tests with
`sed`, because each needs a manifest that encodes what the test is actually asserting.
That is real work and it is where a rushed implementation introduces the next bug.

## Reviewability

The repo constraints check out: `dupl` at threshold 250 (`.golangci.yaml:81-82`), and the
unit-test job fails on non-empty `git status --porcelain` (`.github/workflows/test.yml:80`),
with `-short` on both the race and plain runs (`:70`, `:72`).

**B produces the diff a reviewer can actually evaluate.** Five core files, one new
~70-line file, three-line insertions at two call sites, and one invariant the reviewer can
check by reading two functions: capture verifies the hash before storing, restore verifies
it after decompressing, and restore never deletes. Everything lives in `internal/install`,
which already imports `shellenv` (`remove.go:13`), so the tests live next to the code
instead of being exiled to `cmd/tsuku` (which has no unit-test infrastructure). No
signature changes, so no call-site churn diluting the diff.

**A is reviewable but front-loads eight files of noise.** The sever is mechanically
verifiable — the compiler proves it — but a reviewer opening the PR sees an import-graph
change, a new capability interface, a new lock export, and a driver, before reaching the
fix. The eight-file rename is the kind of collateral that gets waved through, and one of
those eight is the file that turns the golden workflow red. A's own instinct to put the
capability in a new `renderer.go` rather than `action.go` is right and does keep the
allowlist clean for the actions half.

**C is the hardest to review and the easiest to approve wrongly.** Twelve files across six
packages, two public signature changes with call-site churn everywhere, ~50 touched tests,
and — the reviewability problem specifically — the five forced-collateral changes are
*absences*: the reviewer has to notice that `StaleCleanupActions` now needs an
`otherPaths`-style guard, that `isCacheStale` now needs the same filter as the builder,
that GC now needs to run cleanup actions. Nothing in the diff points at them. C's advocate
found them by walking every consumer; a reviewer will not repeat that walk. And the
failure mode for a missed one is a deleted file, not a compile error. `dupl` is a
secondary concern for all three; the real duplication risk is C's rewritten cache tests.

## Recommendation

**Ship Option B, in its sidecar form.** It is the only one of the three that lands as a
single self-contained PR with green CI and cannot delete or hide a working integration.
Nine files, ~two touched tests, no allowlisted path, no signature changes, no import-graph
surgery, no dependency on the prototype branch.

Cut from it, in this order:

- **The inline `Content []byte` field.** B's own measurements disqualify it: 12–20x on
  clean-install state load, on a file read by every command. Ship the content-addressed
  sidecar (`$TSUKU_HOME/share/rendered/<sha256>`, 0600, dir 0700) keyed off the
  `ContentHash` state.json already carries. This also removes the `state.json`
  permission question, the `Content`/`ContentHash` disagreement check, and the schema
  change — three sections of the design doc become unnecessary.
- **The `doctor --fix` repair branch.** Separate PR. It is the most valuable follow-up
  and it is not needed to fix the reproduction.
- **The orphan-blob sweep in `gc.go`.** An unreferenced blob is inert and small. Defer.
- **The `plan_install.go` `RecordCleanup` fix.** Keep it *out* of B specifically because B
  does not need it — B without it is inert for `--plan` installs, which is no worse than
  today. It belongs in the follow-up PR with the doctor repair. (Under C it would be
  mandatory; that asymmetry is itself informative.)

Keep the hash-threading containment at the four `RebuildShellCache` call sites that pass
none (`remove.go:400`, `update.go:58`, `install_deps.go:595`, `plan_install.go:134`). It is
a handful of lines, every option needs it, and it is what turns "wrong exports, silently"
into "no exports, and `doctor` says why" for the versions B cannot restore.

**Splitting.** B does not need to be split, which is the whole reason it wins under the
single-PR constraint. The other two do:

- **A splits at the import sever.** PR 1 is the `pin.go` move and five identifier renames
  — mechanical, compiler-verified, independently correct (`pin.go` does not belong in
  `internal/install` and nothing in that package uses it), and it takes the golden-workflow
  hit on its own where a red check is easy to explain. PR 2 is the renderer plus driver
  and touches nothing allowlisted. Two clean PRs, but the task says one.
- **C splits at "prerequisites" versus "rename."** PR 1: the `plan_install.go`
  `RecordCleanup` extraction, the `isCacheStale`/builder alignment, GC dropping
  `VersionState`, and the `StaleCleanupActions` `otherPaths` guard. All four are bug fixes
  on `main` today, independently valuable, and reviewable on their own merits. PR 2: the
  filename rename and the manifest gate, which is now a much smaller and much safer diff
  because its landmines are already defused. Shipping C as one PR means shipping the
  landmines and the thing that steps on them together, which is precisely how the
  `StaleCleanupActions` regression gets merged.

**The strongest argument against my own recommendation** is that B does not fix the
reported bug for anyone who already has it. Every version in the 2.6 MB `state.json` on
the author's own machine would restore nothing; the reproduction still reproduces on an
existing machine after this PR merges. A reviewer can reasonably say "this prevents future
instances of a bug we currently have" and reject it. Worse, A's falsifier #1 for B is
correct and cheap: `install_shell_init` hardcodes `ctx.InstallDir`
(`shell_init.go:154`), the deleted staging directory, and that is the *only* reason the
bytes look unrecoverable — the versioned tool directory holds a byte-identical copy
(`manager.go:168-201`). Redirect the read at `ToolInstallDir` and the 158 KB blob becomes
a `copyFile` with a ten-byte reference, at which point B's blob store is storing a third
copy of a file that already exists twice. If someone is willing to make that redirection,
the right answer is A's render capability with B's per-version restore semantics, and B as
designed is dead weight. I still recommend B for *this week* because that combination is
the 18-file, golden-workflow-arming PR, and the question was landing risk.

**Confidence: medium-high.** High on the mechanical findings — the allowlist comparison,
the golden drift being genuine bottle drift, the missing `otherPaths` guard in
`ExecuteStaleCleanup`, the two additional unrecorded writers, and the test counts are all
directly verified in this tree. Medium on the recommendation itself, because it turns on a
scoping judgement I do not own: if the acceptance criteria include repairing installs that
are already broken, B does not meet them and the honest answer becomes "A, split into two
PRs, and accept that it does not land this week."
