# Lead: What re-render mechanisms are available in this codebase, and what does each cost?

All line references are against `origin/main` at `8a7c8908` unless a claim explicitly
names `origin/fix/2439-set-env-exports` (the prototype).

## Findings

### Ground truth established before evaluating candidates

Four facts constrain every candidate. Each was verified against code, not assumed.

**1. `internal/install` cannot import `internal/actions` or `internal/executor`. The cycle is
real, and it is shallow.**

Empirically probed by adding a throwaway import file and building:

```
package github.com/tsukumogami/tsuku/internal/install
	imports github.com/tsukumogami/tsuku/internal/actions from zz_cycle_probe.go
	imports github.com/tsukumogami/tsuku/internal/version from action.go
	imports github.com/tsukumogami/tsuku/internal/install from resolve.go: import cycle not allowed
```

The cycle does *not* run through `executor -> install`. It runs
`actions -> version -> install`, and the single edge that closes it is
`internal/version/resolve.go:7` importing `internal/install` to call
`install.ValidateRequested`. That function is `internal/install/pin.go:89-106`: seventeen
lines of pure string validation over `unicode` / `strings`, with no package-level
dependencies. Moving it to a leaf package severs `version -> install` and unlocks
`install -> actions`.

`install -> executor` has a *second*, independent cycle edge:
`internal/executor/plan_conversion.go:3` imports `install` (a 75-line type converter,
`ToStoragePlan` / `FromStoragePlan`). Both edges would have to go.

`internal/shellenv` is equally blocked, and for a longer reason —
`actions -> version -> install -> shellenv` (via `internal/install/remove.go:13`).

What *is* allowed today, probed and confirmed: `internal/updates` may import
`internal/executor`. So may any new package, and so may `cmd/tsuku`. A re-render driver
does not have to live in `internal/install`.

**2. The plan stored in `state.json` drops the `Phase` field.**

`install.PlanStep` (`internal/install/state.go:59-67`) carries Action, Params, Evaluable,
Deterministic, URL, Checksum, Size — and no Phase. `executor.ToStoragePlan`
(`internal/executor/plan_conversion.go:14-24`) does not copy it; `FromStoragePlan`
(`:49-60`) cannot restore it. It also drops Verify, Dependencies, RecipeType, and
Binaries. Anything that replays the stored plan sees every step as phase `""`, which
`executor.StepPhase` (`internal/executor/executor.go:605-610`) resolves to `"install"`.

This is not only a re-render problem. `cmd/tsuku/install_deps.go:98-106` loads the
stored plan as an install-time plan cache (`StateManager.GetCachedPlan`,
`internal/install/state_tool.go:142`), so a cache-hit reinstall of the same version also
loses phase routing.

**3. No recipe in the registry declares `phase`, so `ExecutePhase("post-install")` is
currently a no-op for every real install.**

`grep -rn "phase" recipes/` returns only `recipes/README.md` (documentation) and two
false positives (`hdr_writer_reader_phaser.h`, `vcf-phased-join`). `nvm.toml` — the only
recipe using `install_shell_init`, and the only one using `set_env` — sets no phase on
either step. Phase propagation exists at `internal/executor/plan_generator.go:264-269`
but has nothing to propagate.

Consequence: `install_shell_init` runs inside the *install* loop of `ExecutePlan`
(`executor.go:554` skips only steps whose phase is not `"install"`), against the staging
directory. `ExecutePhase(globalCtx, plan, "post-install")` at
`cmd/tsuku/install_deps.go:579` and `cmd/tsuku/plan_install.go:119` finds zero steps and
returns nil. The entire post-install phase machinery is, today, dead code exercised only
by tests.

The prototype changes this in an interesting way: its `StepPhase`
(`executor.go`, branch) resolves an *empty* phase through the action registry via
`actions.DefaultPhase(step.Action)`, and its `set_env` declares `DefaultPhase() ->
"post-install"`. Because the lookup is by action name rather than a stored value, this
sidesteps the Phase-drop in fact 2 — for `set_env` only. `install_shell_init` does not
implement `PhaseDeclarer` on the branch and stays in the install phase.

**4. `install_completions` has the same bug shape and nobody has named it.**

`internal/actions/completions.go:109` records cleanup for
`share/completions/{shell}/{target}` — a version-independent path holding
version-specific content, recorded with a ContentHash, cleaned up by the same
`executeCleanupActions` path. Any mechanism scoped to "the two shell.d writers" leaves a
third writer with the identical defect.

---

### Candidate A — replay the surviving version's post-install steps

**What exists.** More than the brief assumes. `Executor.ExecutePhase(ctx, plan, phase)`
(`internal/executor/executor.go:615-657`) is exactly "execute a subset of steps,
filtered by phase". `SetToolInstallDir` (`:347`) points the context at the permanent
directory, and `GetCleanupActions` (`:356`) harvests the resulting hashes. Both
production callers already use the three together
(`cmd/tsuku/install_deps.go:578-599`, `cmd/tsuku/plan_install.go:115-140`). There is a
second subset filter, `executor.FilterStepsByTarget` (`internal/executor/filter.go:18`),
but it filters by platform, not phase.

**What would have to be built.**

- A `Phase` field on `install.PlanStep` plus both sides of the conversion — a `state.json`
  schema change. Existing installs have no Phase in their stored plan, so replay would
  have to fall back to the prototype's registry-lookup trick (`actions.DefaultPhase`) or
  to re-deriving phase from the recipe.
- A way to build the `ExecutionContext` without running an install. `ExecutePhase`
  refuses when `e.ctx == nil` (`:616-618`), and `e.ctx` is only assigned inside
  `ExecutePlan` (`:531`) — after dependencies are installed (`:450`) and all install-phase
  steps have run (`:547-598`). A context-only constructor is required.
- A driver outside `internal/install` (see fact 1), plus a callback seam so
  `RemoveVersion` / `Activate` / `Rollback` can reach it.

**Side effects beyond writing shell.d.** This is the strongest objection. `ExecutePhase`
calls `action.Execute` unconditionally. For `install_shell_init` in `source_command` mode
that means `execCommandFunc(args[0], args[1:]...)` (`internal/actions/shell_init.go:203`)
— spawning a binary out of the tool directory during `tsuku remove`. If any recipe ever
places a `run_command` or a download in the post-install phase, replay runs that too;
`ExecutePhase` has no allowlist. There is also a correctness wrinkle: `executeSourceFile`
reads `filepath.Join(ctx.InstallDir, sourceFile)` (`shell_init.go:154`) — the *staging*
directory, which no longer exists at remove time — so `InstallDir` would have to be
repointed at `ToolInstallDir` for replay, a subtle behavioral fork between install and
replay.

**Coverage.** Both writers, once Phase survives — and `install_completions` for free,
since it is phase-agnostic. All five lifecycle events, if the callers are wired.

**Testability.** `internal/executor/phase_test.go` already exercises phase filtering, and
`execCommandFunc` is an overridable package var for tests. But the end-to-end path spans
`install` + `executor` + `cmd`, and nothing in `cmd/tsuku` is unit-tested today.

**Strongest objection.** It makes `tsuku remove` execute code from the tool being kept.
Users reasonably expect removal to delete things, not to run a subprocess and re-derive
state. A failure mid-replay leaves shell.d in a third, worse condition.

---

### Candidate B — a dedicated render pass decoupled from `Execute`

**What exists.** The pattern is well established. `internal/actions` already carries four
opt-in capability interfaces resolved by type assertion against the registry:

- `Decomposable.Decompose(ctx *EvalContext, params) ([]Step, error)`
  (`internal/actions/decomposable.go:19-24`) — explicitly "called during plan generation,
  not execution". 13 implementors. This *is* the codebase's compute-without-apply
  precedent.
- `Preflight(params) *PreflightResult` — "validates parameters without side effects".
  25 implementors.
- `ActionDescriber.StatusMessage(params)` (`internal/actions/describer.go`).
- `NetworkValidator.RequiresNetwork()` (`action.go:127-129`).

The prototype adds a fifth, `PhaseDeclarer` (`action.go` on branch), with the same shape
and a package-level `DefaultPhase(name string)` helper that does the registry lookup. So
adding a sixth is idiomatic, not novel.

**What would have to be built.** A `Renderer` interface plus a `RenderContext` narrower
than `ExecutionContext`. The inputs `set_env` actually needs are Version, ToolInstallDir,
WorkDir, LibsDir (for `GetStandardVars`, `set_env.go:40`) and `Recipe.Metadata.Name` (for
the target basename on the branch). `install_shell_init` additionally needs ToolsDir (to
derive `$TSUKU_HOME`, `shell_init.go:108`) and the `shells` param. Then a driver, which
again cannot live in `internal/install`.

**How many actions implement it.** Two on the branch, one on main (`install_shell_init`),
three if `install_completions` is included. A capability interface with two or three
implementors is thin but not unprecedented here.

**The honest problem.** "Pure render" is achievable for `set_env` — it is string
concatenation into a `strings.Builder` on the branch. It is *not* achievable for
`install_shell_init`. `source_file` mode copies bytes off disk (fine — the surviving
version's directory exists). `source_command` mode *executes a binary and captures
stdout* (`shell_init.go:190-233`). A `Render` that shells out is not a pure function
separated from apply; it is `Execute` minus the final `os.WriteFile`. Calling it "render"
oversells what the seam buys over candidate A.

There is also a duplication hazard: unless `Execute` is rewritten as
`Render` + write, the two paths drift. Only `set_env` on the branch is structured to make
that refactor clean (it already builds the full content in a buffer before writing,
`set_env.go` branch lines ~150-165). `install_shell_init` writes per-shell inside the
loop and hashes by reading the file back (`shell_init.go:169-175`).

**Coverage.** Both writers, all events, independent of the stored plan and the Phase
problem entirely — that is its real advantage over A. It needs only `Params`, which
*does* survive the storage round-trip.

**Testability.** Best of the set. A `Render` returning `[]RenderedFile` is table-testable
in `internal/actions` with no filesystem and no manager.

**Strongest objection.** It introduces an interface whose contract ("no side effects")
one of its two implementors cannot honor.

---

### Candidate C — regenerate from the recipe

**What exists.** `recipe.Loader.Get(name, opts)` (`internal/recipe/loader.go:57`) with an
embedded-registry fallback (`internal/recipe/embedded.go:58`). `internal/recipe` does not
import `internal/install`, so `install -> recipe` is legal today. `ToolState.RecipeHash`
(`internal/install/state.go:102`) records the SHA256 of the recipe TOML at install time,
so drift is *detectable* — but the recipe bytes themselves are not stored, so drift is
not *recoverable*.

**What would have to be built.** Recipe load at remove/activate time, step filtering, and
then — because a recipe is not a plan — either plan regeneration (network, version
resolution, checksum computation via `Decompose`) or a recipe-level executor. Both drag
in `internal/actions`, so the import-cycle constraint applies unchanged.

**Cost.** Makes `tsuku remove` and `tsuku activate` depend on registry availability.
Every other operation on those paths is offline today. If the recipe has changed upstream
since install, re-rendering produces content the installed tool never had — a silent
correctness break rather than a visible failure.

**Coverage.** Both writers, all events — in principle. In practice it is candidate A with
a worse input source: strictly more work, strictly more failure modes, and it does not
avoid the actions dependency.

**Strongest objection.** It trades a solvable schema problem (Phase in the stored plan)
for an unsolvable trust problem (the recipe on disk is not provably the one that produced
the installed bytes).

---

### Candidate D — store the rendered content in `state.json`

**The brief's stated reason for ruling this out does not apply.** The exploration context
says "restoring content from state" is ruled out because `CleanupAction` stores only a
non-invertible SHA-256. That argues against reconstructing content *from the hash*. It
does not argue against *storing the content*. These are different mechanisms.

**What exists.** The structural slot is already right. `CleanupAction` lives on
`VersionState`, not `ToolState` (`internal/install/state.go:20, 32`), so it is
**per-version**. That is precisely the property activate and rollback need: switching from
0.40.6 to 0.40.5 can read `toolState.Versions["0.40.5"].CleanupActions` and restore *that
version's* content. Removal of the active version can do the same for the survivor. A
`Content` field alongside `ContentHash` needs no schema restructuring, only a widened
struct.

**What would have to be built.** Populate `Content` at record time (the bytes are already
in hand at `shell_init.go:169-175` and `:230`, where the hash is computed). Then a
restore step at each lifecycle event — which is a small amount of code in
`internal/install` itself, with **no import-cycle problem at all**, because restoring
bytes requires neither `actions` nor `executor`. That is D's single largest advantage and
it is not small.

**Cost — state size, and it is worse than it looks.** `nvm.sh` is roughly 100 KB. Two
shells, times every installed version, JSON-escaped. `state.json` is loaded in full by
`StateManager.Load()` on effectively every command — `GetCachedPlan`, `GetToolState`,
`List`, doctor, update checks. Inflating it into a content store taxes every invocation,
not just the rare ones. (`set_env` content is a few hundred bytes and would be free; the
cost is entirely `install_shell_init` and `install_completions`.)

**Cost — secrets.** `source_command` output is arbitrary program output and `set_env`
values are arbitrary strings. Registry recipes are public and reviewed, so this is a thin
risk today, but `state.json` is not written with restrictive permissions the way shell.d
files are (0600 at `shell_init.go:165, 227`; `state.json` permissions were not audited
here).

**Coverage.** Both writers, all five events, uniformly. Content is per-version, so
activate and rollback are covered by construction rather than by re-derivation.

**Gap.** Versions installed before the change carry no `Content`. Their entries would
need a fallback (leave alone? delete? re-render by another mechanism?), so D probably
cannot be the *only* mechanism.

**Testability.** Very good — pure state manipulation, no executor, no subprocess.

**Strongest objection.** It turns the hot-path state file into a blob store to solve a
problem that only manifests on three lifecycle events.

---

### Candidate E — make the on-disk file a stable indirection

**The brief's claim about `tools/current/` is correct.** `Config.CurrentSymlink(name)` is
`filepath.Join(c.CurrentDir, name)` (`internal/config/config.go:416`) where `name` is a
*binary* basename, not a tool name (`createBinarySymlink`,
`internal/install/manager.go:498-519`, uses `filepath.Base(binaryPath)`). Each entry is
either an atomic symlink to `tools/<tool>-<version>/<binaryPath>` (`:506, :514`) or a
generated wrapper script pointing at the same (`createBinaryWrapper`, `:559-576`). There
is no per-tool directory symlink and no `tools/<tool>/current`.

**But the strongest precedent for E already exists elsewhere.** `config.EnvFileContent`
(`internal/config/config.go:446-465`) is exactly this pattern: a managed file whose
content is entirely version-independent, resolving paths at shell-source time through
`${TSUKU_HOME:-$HOME/.tsuku}`. `EnsureEnvFile` (`:477-496`) regenerates it idempotently
from a compile-time constant and even migrates stray user edits into `env.local`
(`migrateEnvExports`, `:502`). This is the shape E is reaching for, already shipped.

**What would have to be built.** A stable per-tool path — e.g. `tools/.active/<tool>` —
maintained atomically alongside the binary symlinks in `createSymlinksForBinaries`,
`Activate`, and `RemoveVersion`'s active-switch branch (`remove.go:192-205`). Then:

- `set_env`: **fully solved, with no re-render at all.** `export NVM_DIR='$TSUKU_HOME/tools/.active/nvm'`
  is version-independent by construction, so every lifecycle event is covered for free.
  Note the branch's `shellQuote` uses single quotes, which would suppress `$TSUKU_HOME`
  expansion — E requires that quoting change.
- `install_shell_init`: **not solved as written.** The action copies the script's *bytes*.
  Indirection means changing it to emit a sourcing stub
  (`. "$TSUKU_HOME/tools/.active/nvm/nvm.sh"`), which is a different action semantics —
  and `source_command` mode has no file to point at, so that mode cannot be indirected at
  all. `install_completions` has the same split.

**Other things it changes.** `ContentHash` starts covering the stub rather than the
script, which is a simplification but changes what doctor's mismatch check means. The
shell cache's brace-group error isolation (`internal/shellenv/cache.go:140-148`) is
unaffected. A symlink layer under `tools/` interacts with
`ValidateSymlinkTarget` (`internal/install/symlink.go:46`) and with
`RebuildShellCache`'s symlink rejection (`cache.go:83-86`) — the latter only inspects
shell.d entries, so it is fine, but the interaction should be checked deliberately.

**Coverage.** `set_env`: all events, perfectly. `install_shell_init`: `source_file` mode
only, and only with an action-semantics change. `source_command`: not covered.

**Strongest objection.** It solves the easy half completely and the hard half not at all,
which makes it a partial mechanism that still needs a second mechanism behind it —
exactly the outcome that makes a design hard to reason about later.

---

### Candidate F — deletion plus lazy regeneration

**Far more of this already exists than the brief suggests.**

`RebuildShellCache` already takes an optional `contentHashes` map and **already excludes
any file whose on-disk content does not match its stored hash**
(`internal/shellenv/cache.go:119-131`), removing the cache entirely if every file is
excluded (`:152-155`). `CheckShellD` already detects the same mismatch
(`internal/shellenv/doctor.go:98-110`). `tsuku doctor` already builds the hash map — from
the **active version's** cleanup actions specifically (`cmd/tsuku/doctor.go:198-215`) —
prints `content hash mismatch`, and exits non-zero (`:243-245`).

So the nvm repro is *already diagnosed today*. What is missing is twofold:

1. **`--fix` does not fix it.** `cmd/tsuku/doctor.go:62-73` rebuilds the cache and
   nothing else. It never re-renders the shell.d file.
2. **The hash map is passed at exactly one call site.** Every other
   `RebuildShellCache` call passes no hashes at all —
   `internal/install/remove.go:400`, `internal/install/update.go:58`,
   `cmd/tsuku/install_deps.go:595`, `cmd/tsuku/plan_install.go:134`. So in the normal
   flow the stale content is concatenated straight into `.init-cache.bash` unverified.

Threading the active version's hashes through those four call sites is mechanical and
would, on its own, convert "the surviving version silently gets the wrong exports" into
"the surviving version gets no exports, and doctor says why." That is a strictly smaller
change than any other candidate here and it degrades safely.

**What would have to be built for the regeneration half.** A trigger. The three named in
the brief are not equal:

- `install`: natural, already writes shell.d.
- `doctor --fix`: natural, already detects the condition.
- **shell start: not available.** `EnvFileContent` sources `.init-cache.{shell}` directly
  as a file (`internal/config/config.go:450-458`); it never invokes the `tsuku` binary.
  Adding a repair hook there means running a process on every shell start, which is the
  precise cost the init cache exists to avoid.

**Coverage.** Content-agnostic, so both writers plus `install_completions`, and all five
events uniformly. That uniformity is its main appeal.

**Strongest objection.** The window. Between `tsuku remove nvm@0.40.6` and the user's next
`install` or `doctor --fix`, `nvm` is simply absent from every new shell — silently, since
the shell cache rebuild is not user-visible. For a tool whose entire purpose is to exist
as a shell function, "temporarily gone" and "broken" are hard to distinguish from the
user's chair.

---

### Precedent survey: does this codebase already regenerate derived state?

Yes, in five places, and the precedents disagree with each other in an instructive way.

**1. `shellenv.RebuildShellCache` (`internal/shellenv/cache.go:29-169`) — regenerate from
a directory scan.** Reads every `*.{shell}` in shell.d, sorts, concatenates with
brace-group error isolation, writes atomically (temp + rename) under a file lock
(`:38-43`), with optional hash-based exclusion. It never consults the plan, the recipe, or
the actions. This is the closest thing tsuku has to a re-render, and its input is *the
filesystem*, not a replayable description.

**2. `config.EnsureEnvFile` (`internal/config/config.go:477-496`) — regenerate from a
constant.** Idempotent, compares before writing, and carries a migration path that
rescues user edits into `env.local` rather than clobbering them.

**3. `install.Manager.Activate` (`internal/install/manager.go:411-470`) — regenerate the
per-binary artifact from state.** This is the precedent that matters most, because it is
the same problem one artifact over. Activate re-creates `tools/current/*` from
`versionState.Binaries` plus the version string (`:448-456`) — from *state*, not from the
plan, not from the recipe, and without importing `actions`. Whatever shell.d ends up
doing, this is the shape the codebase already reaches for.

It also carries the same bug in miniature, which is worth flagging: install creates
**wrapper scripts** when the tool has runtime dependencies and plain symlinks otherwise
(`manager.go:205-213`), but `Activate` always calls `createSymlinksForBinaries`
(`:454`). Activating a wrapper-installed tool silently replaces its wrapper with a bare
symlink and drops the PATH/library prelude. Same class of defect: a derived artifact whose
re-render path does not match its create path.

**4. `index.Rebuild` + `index.CheckStaleness` (`internal/index/rebuild.go`,
`internal/index/stale.go:19-49`) — regenerate a cache from source, with an explicit
staleness predicate (registry mtime vs. stored `built_at`). The precedent here is the
*staleness check as a first-class concept*, which shell.d currently lacks outside doctor.

**5. `install.StaleCleanupActions` / `ExecuteStaleCleanup` (`internal/install/update.go:14-62`)
— the only code that reasons about two versions' cleanup actions at once.** It computes
old-minus-new by `(action, path)` and deletes the difference, then rebuilds affected shell
caches. Called from `cmd/tsuku/update.go:182-183`. Note what it does *not* do: paths
present in **both** old and new are left entirely alone — no re-render, no hash refresh —
which is exactly the overlap case that produces the bug. This function is the natural
place a "the path survived but the content changed" branch would go.

**Counter-precedent worth knowing.** `updates.GarbageCollectVersions`
(`internal/updates/gc.go:15-70`, called at `internal/updates/apply.go:153`) deletes old
version *directories* from disk while touching neither `state.json` nor cleanup actions.
So a GC'd version keeps its `VersionState` entry and keeps its claim on a shell.d path —
which means the `otherPaths` skip in `executeCleanupActions` (`remove.go:333-349`) can be
held open by a version whose directory is already gone. Any mechanism that keys off
"another version still references this path" inherits that hole.

## Implications

**The import cycle is a smaller obstacle than it looks, and knowing that reorders the
candidates.** The blocking edge is one 17-line pure function
(`install.ValidateRequested`, `pin.go:89`). If the exploration converges on a mechanism
that needs `internal/install` to reach `internal/actions`, the prerequisite is a
one-function move to a leaf package, not an architectural rework. Conversely, candidate D
is the only mechanism that needs no such move at all, which is a real and under-credited
advantage.

**The Phase-drop in `ToStoragePlan` is a prerequisite for candidate A and a latent bug
independent of this work.** Because the stored plan doubles as the install-time plan
cache (`install_deps.go:98-106`), a cache-hit reinstall already loses phase routing. Any
plan-replay mechanism has to fix this; the prototype's registry-lookup `StepPhase` is one
way, a `Phase` field on `install.PlanStep` is the other, and they are not mutually
exclusive.

**Candidate F's first half is nearly free and orthogonal to the choice.** Threading the
active version's content hashes into the four unhashed `RebuildShellCache` call sites
converts silent wrongness into loud absence. Whatever mechanism wins, that change is
compatible with it and reduces the blast radius in the interim.

**Candidates split cleanly on what input they trust.** A and C trust a replayable
description (plan, recipe) and pay in executed side effects. B trusts the action's
parameters and pays in a new interface one implementor cannot honor purely. D trusts
stored bytes and pays in state size. E trusts a stable path and pays by covering only
half the writers. F trusts nothing and pays with a gap in coverage over time. The
precedent survey favors the "trust state, regenerate the artifact" family (Activate,
RebuildShellCache) over the "replay the description" family, which has no precedent here
at all.

**Scope is one action wider than the brief states.** `install_completions` shares the
defect. A mechanism that generalizes over "actions that record cleanup actions for
version-independent paths" covers three writers; one hard-coded to shell.d covers two.

## Surprises

1. **The post-install phase is dead code in production.** No recipe declares a phase, so
   `ExecutePhase("post-install")` never has steps. `install_shell_init` runs in the
   install phase against the staging directory. The prototype's `PhaseDeclarer` is
   effectively *introducing* the post-install phase to real installs for the first time,
   not routing within an established one.

2. **`tsuku doctor` already detects the exact repro.** The hash-mismatch check
   (`doctor.go:198-215` building the map from the *active* version, `shellenv/doctor.go:98-110`
   comparing) fires on `00-env-nvm.bash`. It reports and exits non-zero. `--fix` then
   rebuilds only the cache. The diagnosis exists; the repair does not.

3. **`RebuildShellCache` accepts content hashes but almost nobody passes them.** Four of
   five call sites pass none, so the hash-based exclusion that would contain this bug is
   inert everywhere except doctor.

4. **`Activate` has the same defect in the binary artifact.** It rewrites
   `tools/current/*` as plain symlinks regardless of whether install created wrapper
   scripts (`manager.go:454` vs `:205-213`), silently dropping the runtime-dependency PATH
   prelude. Independent of shell.d, and evidence that "re-render on activate" is a gap the
   codebase has more than once.

5. **The stated reason for ruling out candidate D is about the wrong mechanism.** "Only a
   hash is stored" rules out inverting the hash, not storing the content. And because
   `CleanupActions` already live per-version, stored content would solve activate and
   rollback by construction — the two events the brief flags as hardest.

6. **`GarbageCollectVersions` deletes directories without touching state**, so a version
   with no directory can still block a cleanup path via the `otherPaths` check.

## Open Questions

- What are the actual byte sizes at stake for candidate D? `nvm.sh` is ~100 KB and is the
  only real instance today, but the cost model depends on how many versions a user keeps
  (`UpdatesVersionRetention`) and whether `state.json` would gain compression. Worth
  measuring rather than estimating.
- What permissions does `state.json` carry? shell.d files are deliberately 0600
  (`shell_init.go:165, 227`) and the directory 0700 (`:111-117`). If `state.json` is
  laxer, candidate D moves 0600 content into a laxer file.
- Does any code path other than `cmd/tsuku` need to trigger a re-render — specifically,
  does `internal/updates`' auto-update flow reach `RemoveVersion` / `Activate` without
  passing through a `cmd` layer where a driver could be injected? `apply.go:173` calls
  `mgr.Activate` directly from inside `internal/updates`, which suggests a callback seam
  on `install.Manager` is needed regardless of where the driver lives.
- Is `source_command` mode used by any recipe today? Only `nvm.toml` uses
  `install_shell_init` and it uses `source_file`. If `source_command` has zero real users,
  several objections above (replay executes a subprocess; render cannot be pure; E cannot
  indirect it) weaken considerably — but the action still supports it and the validator
  still accepts it.
- Should `install_completions` be in scope? Confirming or excluding it changes whether the
  mechanism needs to generalize.

## Summary

Six candidate mechanisms exist and none is blocked outright: the `internal/install` ->
`internal/actions` import cycle is real (verified: `actions -> version -> install` via a
17-line pure validator at `pin.go:89`) but shallow enough to sever, and `internal/updates`
can already import `internal/executor`. The decisive constraints are elsewhere — the
stored plan silently drops `Phase` (`install.PlanStep`, `plan_conversion.go`), which
blocks plan replay and is already a latent bug in the install-time plan cache; no recipe
declares a phase, so the post-install machinery is dead code today; `tsuku doctor` already
*detects* the exact repro but `--fix` only rebuilds the cache; and the closest precedent
in the codebase (`Activate` regenerating `tools/current/*` from per-version state, without
touching plans or actions) points toward regenerating from stored state rather than
replaying a description. The biggest open question is candidate D's real cost: cleanup
actions already live per-version, so storing rendered content would solve activate and
rollback by construction and needs no import-cycle work at all — the objection is
`state.json` size, and nobody has measured it.
