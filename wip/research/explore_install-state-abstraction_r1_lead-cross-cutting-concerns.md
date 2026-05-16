# Lead: What cross-cutting concerns thread through install operations today, and what's likely next?

## Findings

### 1. Today's orthogonal threading

The install pipeline today threads at least **eight** distinct orthogonal
concerns through one or more wrapper layers, two structs, and one set of
package-level globals. The "core" payload is `(toolName, reqVersion,
versionConstraint)` — everything else listed below is non-payload.

For each concern: *where it lives*, *how it's set*, and whether the shape is
consistent. Counts of files touched come from grepping the production tree
(excluding `_test.go`).

| Concern | Carrier | Set via | Threading shape | Files producing | Files consuming |
|---|---|---|---|---|---|
| `installevents.Source` (just added) | Method param on `Install` / `Rollback` / `RemoveVersion` / `RemoveAllVersions` / `Remove`; also `InstallOptions.Source` | Hard-coded literal at every CLI entry point | One-off: appears 5x on Manager methods, 1x on options struct, plus on every wrapper signature | 12 call-site files | `internal/install/manager.go`, `internal/install/remove.go` |
| Progress reporter | Field on `Manager` set by `SetReporter(r)`; also explicitly threaded as a wrapper parameter | Constructed in `runInstall` (TTY) or passed by the update flow (caller-owned) so its lifecycle survives the call | Hybrid: lives as a Manager field but the wrapper chain (`runInstallWithReporter` → `installWithDependencies`) also threads it explicitly because the executor needs it before Manager methods run | `cmd/tsuku/install_deps.go:168` (constructed), `cmd/tsuku/update.go:134` (passed in by caller) | Manager (`getReporter()` warns), Executor (`exec.SetReporter`), each action |
| Event bus | `Option` at Manager construction (`install.WithEventBus(bus)`) | `newEventBus(cfg, telemetryClient)` in every entry path; bus is `nil`-safe | Consistent (option-pattern, single setter), but `telemetryClient` is still passed separately so the bus is constructed twice in some entry paths (`installWithDependencies` and `cmd_apply_updates.go`) | `cmd/tsuku/events_wiring.go`, `internal/updates/self.go`, `cmd_apply_updates.go` | Manager publishes; `internal/notices/subscriber.go` and `internal/telemetry/subscriber.go` consume |
| Telemetry client | Wrapper parameter `client *telemetry.Client` | Constructed at CLI entry, passed down 3 wrapper layers | One-off: predates the bus, technically subsumed by bus now (PR #2412 removed direct `tc.SendUpdateOutcome` calls) but the parameter still threads through `runInstall` → `installWithDependencies` → `installLibrary` for the install-event telemetry that the bus does NOT cover (see `install_deps.go:669`) | `cmd/tsuku/install_deps.go:167-185` | `cmd/tsuku/install_deps.go:669` (direct `Send`), `cmd/tsuku/install_lib.go:184` (direct `Send`) |
| Dependency awareness | `isExplicit bool`, `parent string`, `visited map[string]bool` on `installWithDependencies` | CLI entry sets `isExplicit=true, parent=""`; recursive call sets `isExplicit=false, parent=<currentTool>` | Inconsistent and one-off: three separate parameters that always travel together but aren't grouped. `visited` is created at entry (`runInstall` line 173) which is a half-private invariant — callers must remember to construct the map | `cmd/tsuku/install_deps.go` (8 entry call sites) | `installWithDependencies` itself; affects `IsExplicit`, `RequiredBy` state fields |
| `workDir` (executor handoff) | Method param on `Manager.Install` / `InstallWithOptions` | Comes from `exec.WorkDir()` post-execution | Consistent but couples Manager tightly to the executor's filesystem layout | `cmd/tsuku/install_deps.go:551`, `cmd/tsuku/install_lib.go:149` | `Manager.InstallWithOptions` only |
| CLI flag globals (`installFresh`, `installTargetFamily`, `installRequireEmbedded`, `installSkipSecurity`, `installNoShellInit`, `installDryRun`, `installFrom`, `installEnv`, `installYes`) | Package-level `var` in `cmd/tsuku/install.go:24-39` | Cobra `BoolVar`/`StringVar` at command registration | Inconsistent: NOT threaded through any signature — `installWithDependencies`, `runDryRun`, `runRecipeBasedInstall`, `installLibrary` all read globals directly. Effectively a parallel implicit context that bypasses the wrapper chain entirely | `cmd/tsuku/install.go:358-373` (flag registration) | Read at multiple points in `install_deps.go`, `helpers.go`, `install_lib.go`, `plan_install.go` |
| `recipe.LoaderOptions` (mostly empty) | Method param on `loader.Get` / `loader.GetWithContext` | Hard-coded `recipe.LoaderOptions{}` literal at 10+ call sites | Consistent but unused — it's plumbed everywhere and almost never set to anything; this is the inverse problem (zombie threading) | All recipe load call sites | `internal/recipe/loader.go` |

Additional concerns worth noting:

- **`InstallOptions` itself is part-payload, part-cross-cutting.** Its
  `CreateSymlinks` / `IsHidden` / `Binaries` / `RuntimeDependencies` /
  `RequestedVersion` / `Plan` are arguably core install payload (the
  *what*); but `Source` is purely cross-cutting metadata about
  attribution. Mixing them in one struct is the current compromise. See
  `internal/install/manager.go:67-76`.
- **`autoinstall.Mode` (`ModeConfirm`/`ModeSuggest`/`ModeAuto`)** lives
  entirely in `internal/autoinstall/`; it never reaches `install.Manager`
  because by the time the Manager runs, the consent decision has been
  made and converted into `SourceProjectAuto` on the bus. So consent
  resolution is *not* threaded through the Manager, but it is threaded
  through the autoinstall layer as `Runner.ConsentReader` plus the
  flag/env/config resolution chain in `cmd_run.go:158-204`.

Most pernicious accumulation: `installWithDependencies` now has the
following signature:

```
func installWithDependencies(
    toolName, reqVersion, versionConstraint string,   // payload (3)
    isExplicit bool, parent string,                    // dependency context (2)
    visited map[string]bool,                           // recursion state (1)
    telemetryClient *telemetry.Client,                 // legacy direct telemetry (1)
    reporter progress.Reporter,                        // UX (1)
    src installevents.Source,                          // attribution (1)
) error
```

— 10 positional parameters of which 7 are cross-cutting. PR #2412 added
the 10th; the function had 9 a month ago.

### 2. Patterns of pain — git history of cross-cutting churn

Decomposing the last several PRs that touched `cmd/tsuku/install_deps.go`
and `internal/install/manager.go`:

| PR | Title | Files in install pipeline | Nature of churn |
|---|---|---|---|
| #2412 (PR `880ed188`, May 2026) | introduce lifecycle event bus | 36 files / +3955 / -289 | Added: event bus + 2 subscribers + Notice schema field + `Source` parameter on 5 Manager methods + `Source` on InstallOptions + wrapper signature changes in 6 cmd files. Pure "cross-cutting threading" portion (Source on wrappers + Manager): ~6 cmd files + manager.go + remove.go |
| #2280 (PR `9f6b2fc6`, Apr 2026) | TTY-aware spinner, unified per-package status | 60+ files | Added `Reporter` interface, threaded it through Executor (`exec.SetReporter`), Manager (`SetReporter`), and every action. Each of 43 actions touched. This is a much bigger threading event than #2412 |
| #2201 (PR `bdac6430`, Apr 2026) | tool lifecycle hooks | ~25 files | Added `phase` field on plan steps, new `install_shell_init` action, cleanup-action threading through Manager.RemoveVersion / RemoveAllVersions / ExecuteStaleCleanup. Touched `internal/install/remove.go` and `update.go` heavily |
| #2198 (PR `2174234e`, Mar 2026) | auto-apply with rollback | 13 files | Added `InstallFunc` callback indirection in `internal/updates/apply.go` to avoid import cycle with cmd/tsuku, added `PreviousVersion` state field, added rollback path |
| #2213 (PR `a5a70c22`, Apr 2026) | update outcome telemetry | 13 files | Instrumented MaybeAutoApply (3 branch points), manual update, manual rollback with telemetry calls. #2412 later removed most of these direct calls and replaced them with bus subscribers — i.e. this PR's threading was paid down by the next PR's threading |

**Pattern**: every 4-6 weeks, a feature lands that needs to add a new
piece of metadata or capability across the install/update/rollback/remove
boundary. The shape varies:

- Reporter (#2280) → method on Manager + threaded parameter through executor
- Lifecycle hooks (#2201) → field on plan + new action + new fields on state + threading through remove
- Auto-apply (#2198) → callback type + new state field + new caller package
- Telemetry outcomes (#2213) → direct calls at every branch point
- Source (#2412) → method param on 5 Manager methods + InstallOptions field + threaded through 3 wrapper layers + 12 CLI call sites updated

Each of these was at least 13 files, often more. PR #2412 is not unusual
in size; it's around the median.

### 3. Plausible near-term additions

Six candidates, each grounded in something concrete in the code or open
issues:

1. **`context.Context` for cancellation/timeout on Manager methods.**
   `Manager` currently takes no context. `globalCtx` is set up at line
   195 of `cmd/tsuku/main.go` with SIGINT/SIGTERM cancellation, and is
   threaded into `exec.GeneratePlan` / `exec.ExecutePlan` / `exec.DryRun`
   — but the post-execution `mgr.InstallWithOptions` and
   `mgr.RemoveVersion` calls drop it. A user hitting Ctrl-C during the
   final `copyDir` / `os.Rename` in `Manager.Install` will not interrupt
   cleanly. Adding `ctx` to 5 Manager methods + threading through
   3 wrappers + ~12 call sites = the same shape as #2412.

2. **`--dry-run` for `tsuku install` of a NEW tool.** Today's
   `runDryRun` (`cmd/tsuku/install.go:430`) is a parallel implementation
   that creates a fresh executor and calls `exec.DryRun(globalCtx)` —
   it *completely bypasses* the install pipeline rather than threading
   through it. This is itself evidence of the threading cost: nobody
   wanted to thread a `dryRun bool` through 4 wrappers and the Manager,
   so they wrote a parallel path that doesn't share dependency
   resolution, recipe validation, system-deps display, or the consistent
   wrapper-script generation logic. Issue: this parallel path is already
   drifting (e.g., `runDryRun` calls `exec.DryRun` but doesn't run
   `actions.DetectShadowedDeps` warnings that the real install runs at
   `install_deps.go:277-285`).

3. **Structured audit log of every state mutation.** The lifecycle event
   bus carries five terminal events (`Installed`, `Updated`,
   `RolledBack`, `Removed`, plus failures), but `Manager.Activate`,
   direct `state.UpdateTool` calls in `install_deps.go:222-241,
   475-491, 583-612`, `state.AddLibraryUsedBy`, and the wrapper-script
   regeneration in `createSymlinksForBinaries` are NOT on the bus. A
   compliance-style "every change to disk-state is logged" feature
   cannot use the existing bus; it would need a new subscriber surface
   or new threading. The design doc's Decision 3 explicitly rejected
   the `state.UpdateTool`-shim approach for the bus on semantic grounds,
   but the rejection was specifically about event semantics — audit
   logging has different requirements (no semantic interpretation
   needed, just "this changed").

4. **Per-operation hooks/callbacks ("run this script after install").**
   The codebase already has `install_shell_init` and `install_completions`
   actions (PR #2201) that run in a `post-install` phase. A user-defined
   hook that runs after `tsuku install` succeeds — e.g., a project
   `.tsuku.toml` declares `post_install_hook = "./bin/setup.sh"` — would
   need to thread either through `Manager.InstallWithOptions` (as another
   InstallOptions field) or via a new bus subscriber. Either way it's
   one more orthogonal channel. There's no open commitment to this, but
   project-config-driven hooks are a well-trodden pattern for package
   managers.

5. **Install policies (allowlist / denylist / signature requirement).**
   `cmd/tsuku/install.go:37` already has `installSkipSecurity` to bypass
   cache security checks. The verification action chain
   (`internal/actions/download.go:530+`) already supports PGP signature
   verification but it's per-recipe-author opt-in. A
   user-level policy — "I only install signed recipes" or "I never
   install from distributed sources" — would need to thread a policy
   object through the wrapper chain AND through the recipe loader
   (because policy decisions are sometimes made pre-recipe-load).
   `installRequireEmbedded` is already a tiny version of this, and it
   reaches deep into the loader. There's no open issue for this yet but
   it's a natural follow-on to the distributed recipe support that
   landed in #2160.

6. **Rate-limiting / throttling on auto-apply.** `MaybeAutoApply`
   currently fires every tool through `applyUpdate` sequentially with
   no per-tool, per-window, or per-resource throttling. A user with 50
   installed tools and aggressive update cadences could trigger
   significant background work. A `rateLimiter` would thread either as
   a field on `Manager` (and would need `context.Context` to honor
   timeouts) or as a wrapper concern. No open issue, but the
   `internal/distributed/errors.go:30` `ErrRateLimited` shows that the
   codebase already has the concept of rate-limit handling for
   GitHub-API side, just not for the install pipeline.

For each of these, the cost under the current shape is roughly the same:
add a parameter to 3 wrapper functions in `install_deps.go`, add a
parameter to N Manager methods (or a field on InstallOptions), update
~12 CLI call sites, and decide where the value gets resolved (flag,
env, config, default). That's #2412's exact churn shape.

### 4. The deeper question — recurring class, or one-off?

**Position: recurring class.** Evidence:

- In the last ~3 months (#2198 → #2201 → #2213 → #2280 → #2412), five
  separate PRs each added a new cross-cutting concern that touched the
  install pipeline. Cadence: roughly one every 4-6 weeks.
- The concerns added were not topically related: rollback support, shell
  integration hooks, telemetry granularity, output UX, attribution. They
  came from different motivations, different parts of the product, and
  different driving issues. The common factor is "they all want
  visibility or control across the install/update/rollback/remove
  boundary." That common factor is structural to a package manager,
  not contingent.
- The signature of `installWithDependencies` has grown from 7 to 10
  parameters in this window. Two of the added parameters (`reporter`,
  `src`) are clearly cross-cutting. The third (`client`) was already
  there but PR #2412 left it in place even though the bus subscribers
  *should* have allowed removing it — the cleanup didn't happen because
  there's still one direct `telemetry.Send` call in `install_deps.go:669`
  for the legacy `InstallEvent`. That's a *known unfinished migration*
  caused by the threading cost: removing the telemetry-client parameter
  would have required either threading the bus deeper or moving that
  Send into a subscriber, and neither was in scope.
- The package-level-globals pattern for the install CLI flags
  (`installFresh`, `installTargetFamily`, `installRequireEmbedded`,
  `installSkipSecurity`, `installNoShellInit`) is the same problem
  expressing itself sideways: when threading is too expensive, authors
  reach for implicit globals instead. There are nine such globals in
  `cmd/tsuku/install.go` today. Each one is a piece of cross-cutting
  state that opted out of the threading.
- The `--dry-run` parallel-implementation evidence is the strongest
  signal: when faced with adding a single boolean across the wrapper
  chain, the author wrote a 25-line parallel function instead. That
  function now drifts from the real install path (no shadowed-deps
  warning, different reporter handling).

The "one-off because the event bus is significant" alternative
explanation does not survive contact with these data points. The event
bus itself is one concern; `Source` was one parameter; the threading
work was the bulk of the PR's file count. And the cost would have been
the same if the concern had been `--dry-run`, `ctx`, or anything else
shaped like "tag every state-mutating operation with metadata X."

Frequency estimate: **one new cross-cutting concern every 4-6 weeks**,
under current product velocity. At this rate, the
`installWithDependencies` signature reaches 12 parameters within 12
months unless something changes.

## Implications

The evidence supports the "recurring class of problem" hypothesis. Two
concrete signals reinforce it beyond just counting PRs:

1. **Workarounds are already in the tree.** The package-level CLI flag
   globals, the parallel `runDryRun` implementation, and the legacy
   `telemetry.Client` parameter that survived #2412's bus migration —
   all three are signs that authors are routing around the threading
   cost rather than paying it. The cost is not theoretical; it's
   already producing maintenance debt.
2. **The list of plausible additions is long and shaped identically.**
   `context.Context`, `--dry-run` (as a real first-class concern, not
   the parallel implementation), audit log, post-install hooks,
   policies, throttling — each of these has the same threading shape as
   `Source` and would cost roughly the same. The next 12 months will
   likely see at least 2-3 of them, with several others queued behind.

If the shape stays the same, three predictable outcomes:

- `installWithDependencies` grows to 12-14 parameters
- More CLI flags become package-level globals (the lazy alternative)
- More parallel implementations emerge for concerns that can't be
  threaded cheaply (`runDryRun` is the precedent)

## Surprises

- **`Manager` has no `context.Context`.** The application sets up
  `globalCtx` with signal cancellation and threads it through the
  executor, but the Manager — which does filesystem-mutating work in
  `copyDir`, `os.Rename`, and the wrapper-script writes — never sees
  it. A Ctrl-C during the final atomic-rename window is a silent
  hazard. I expected this to be threaded already; it's not.
- **`installLibrary` takes `src installevents.Source` but never uses
  it.** `cmd/tsuku/install_lib.go:22` accepts the parameter and passes
  it to recursive `installWithDependencies` calls (`line 47`), but the
  library installation itself goes through `Manager.InstallLibrary`
  which doesn't publish any lifecycle events. Libraries are entirely
  outside the bus. So we paid the threading cost for libraries without
  the benefit. (This is a missed-by-design rather than an oversight —
  the design doc deliberately scoped the bus to tools — but it does
  mean the threading cost was over-paid.)
- **`InstallOptions.Source` exists but most callers use the
  `Install(name, version, workDir, src)` method instead.** Both forms
  exist; production code paths in `install_deps.go:550` set
  `installOpts.Source = src` and then call `InstallWithOptions`, but
  `cmd_rollback.go` and `remove.go` use the direct-parameter form.
  Two carrier shapes for the same data is a smell — neither's been
  removed, suggesting nobody's sure which one to commit to.
- **Telemetry has two parallel paths.** The bus subscribers in
  `internal/telemetry/subscriber.go` handle `UpdateOutcome`, but
  `install_deps.go:669` still directly calls `telemetry.NewInstallEvent`
  → `telemetryClient.Send` for the install-event metric. PR #2412
  consciously didn't unify these because the legacy install-event has
  different schema requirements. The `telemetryClient` parameter
  survives in the wrapper chain solely to feed this one Send call.

## Open Questions

- **What's the real call-site shape after each plausible addition?**
  Lead 5 should build the actual file-touch counts for `context.Context`
  threading, a first-class `--dry-run`, and an audit log, comparing
  against PR #2412 as the calibration data point.
- **Is the `InstallOptions` struct a viable anchor for grouping
  cross-cutting state, or is it the wrong shape?** Today it mixes
  payload (`Binaries`, `Plan`) and metadata (`Source`). Lead 3 should
  evaluate whether splitting payload-vs-metadata, or using a `Context`
  struct that lives alongside `InstallOptions`, would be a less
  invasive consolidation than a full repository-pattern restructure.
- **Should the package-level CLI flag globals be in scope?** They're a
  parallel manifestation of the same problem, but they live in
  `cmd/tsuku/` and tightly couple to Cobra. Touching them would change
  the test surface significantly. The scope file is silent on this.
- **Does the library install path want to join the bus?** Right now
  libraries are second-class to the lifecycle events. If a future
  concern (e.g., audit log) requires uniform coverage, this gap
  becomes a real problem. The scope file explicitly defers detailed
  library evaluation — but the shape question affects whether the
  abstraction *can* extend to libraries.

## Summary

The install pipeline currently threads at least eight orthogonal
cross-cutting concerns through a wrapper chain whose innermost function
has grown from 7 to 10 positional parameters in three months, and the
cadence of new such concerns has been roughly one per 4-6 weeks across
the last five install-touching PRs. Three independent in-tree
workarounds — package-level CLI flag globals, the parallel `runDryRun`
function, and the surviving legacy `telemetryClient` parameter — show
authors are already routing around the threading cost rather than
paying it. The biggest open question is whether the next-priority
plausible addition (`context.Context` for cancellation, which the
Manager surprisingly does not accept today) is the right calibration
case for the design phase, since it's the most likely-imminent and has
the cleanest shape match to `Source`.
