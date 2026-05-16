---
status: Accepted
problem: |
  The install pipeline accumulates cross-cutting concerns at a cadence of roughly one new
  orthogonal parameter every 4-6 weeks, and three in-tree workarounds (nine package-level
  CLI flag globals in cmd/tsuku/install.go, a parallel runDryRun function that bypasses
  the install pipeline, and a stranded legacy telemetryClient parameter that PR #2412 could
  not remove) show authors routing around the threading cost rather than paying it.
  installWithDependencies has grown from 7 to 10 positional parameters in three months.
  Manager.GetState().UpdateTool(...) leaks state.json semantics into the CLI layer at three
  sites in install_deps.go, and Manager doesn't accept context.Context anywhere — Ctrl-C
  during the final atomic-rename window is a silent hazard.
decision: |
  Land Candidate C now: thread context.Context through install.Manager's public methods,
  store Source via installevents.WithSource(ctx, src) using a typed key, collapse
  installWithDependencies' trailing-arg recursion into a request struct, add three semantic
  state methods on Manager (MarkExplicit, RecordDependency, RecordCleanup) to replace the
  lambda pattern, and bring InstallLibrary along with new library-specific lifecycle event
  types on the existing bus. Cancellation lands as a free bonus once ctx is threaded.
  Preserve Candidate B (a future installops Service layer above Manager) as an explicit
  charter follow-up to revisit after 1-2 additional cross-cutting concerns land or 3-4
  months elapse — whichever first.
rationale: |
  Candidate C delivers ~80% of Candidate B's threading reduction at ~30-40% of B's cost,
  composes trivially with the lifecycle event bus (bus signature unchanged; only the source
  of Source changes from positional param to ctx extraction at publish callsites), and
  unlocks SIGINT-aware cancellation, which is independently worth shipping. Source behaves
  like a process-level attribute — every call site passes a literal constant — which is the
  textbook case for ctx-based attribution and justifies the override of "context-is-not-config"
  guidance. Pure B's upfront cost is comparable to PR #2412 again with only marginal lifetime
  savings at the conservative 12-month forecast and doesn't deliver cancellation; B+C
  simultaneously exceeds PR #2412's blast radius and contradicts the bounded-blast-radius
  driver. Candidate A is contradicted by Lead 2's recurring-pattern evidence (one new
  cross-cutting concern every 4-6 weeks across five PRs, three in-tree workarounds, parameter
  count growth from 7 to 10 in three months). Doing C first, watching cadence, and then
  deciding on B with one more empirical datapoint is strictly better than committing to
  B blind on a forecast assumption.
upstream: docs/designs/current/DESIGN-notices-install-event-bus.md
---

# DESIGN: Install State Abstraction

## Status

Accepted

## Upstream Design Reference

`docs/designs/current/DESIGN-notices-install-event-bus.md` (status: Current).
That design introduced the lifecycle event bus, the `Source` attribution
parameter, and the verb-per-event vocabulary. The implementation experience
of that design is what motivates this one — specifically, Decision 3
(publisher placement chose method-param threading) and Decision 4 (subscriber
wiring in a helper) are now empirical data points to revisit.

## Context and Problem Statement

The motivating issue (#2413) was filed after PR #2412 landed the install
lifecycle event bus. Threading the new `Source` attribution parameter
through the install pipeline drove the PR's file count higher than the
upstream design predicted ("~10 call sites; manageable; one-time"). The
hypothesis in #2413: state-mutating operations on installed tools live as
a loose collection of methods on `install.Manager`, called from a variety
of entry points (CLI commands, the auto-apply subprocess, the autoinstall
path, the self-update path). Each cross-cutting concern that needs to
thread through every operation — attribution tags like the new `Source`
enum, lifecycle event emission, telemetry hooks, future per-operation
policy like dry-run or rate-limiting — ends up touching every entry point
and every wrapper between the CLI and the Manager.

The exploration (`/explore` Round 1, six leads) produced these refinements:

**The diagnosis was partially right but the proposed shape is wrong.**

- PR #2412's 36 files decompose into 16 one-time-cost files (bus,
  subscribers, schema, renderer, docs), 5 net-negative direct-write-removal
  files, and only **11 files / ~120 LOC** representing the actual
  cross-cutting `Source`-threading cost. The headline "file count was too
  high" framing understates how much of PR #2412 was one-time abstraction
  work that will not recur.
- However: `installWithDependencies` grew from **7 to 10 positional
  parameters in three months**. Five install-touching PRs (#2198, #2201,
  #2213, #2280, #2412) each added a new cross-cutting concern at a
  cadence of one every 4-6 weeks. Three in-tree workarounds — package-
  level CLI flag globals, a parallel `runDryRun` implementation that
  bypasses the install pipeline, and a stranded legacy `telemetryClient`
  parameter PR #2412 could not remove — prove the cumulative cost is
  real and producing maintenance debt.
- The literal Java-style repository pattern as proposed in the issue is
  over-engineered for tsuku's actual shape. The Manager surface for
  state-mutating ops is 5 methods. There is no second storage backend to
  abstract over. Go's struct-with-methods on `*Manager` already IS the
  repository pattern in many readings.

**Two viable architectural shapes emerged:**

- **Candidate C — context.Context-based attribution + standalone recursion-collapse refactor.**
  Thread `ctx` through Manager methods (~15 files; same shape as
  Source-as-param), store `Source` in ctx via a typed key, extract it at
  publish callsites. Separately collapse `installWithDependencies`'s
  trailing-arg recursion into a request struct. Picks up SIGINT-aware
  cancellation (which `Manager` does not support today — a Ctrl-C during
  the final `os.Rename` window is currently a silent hazard) as a free
  bonus. Does not address `state.json` exposure via `mgr.GetState()`.
  Aligns with mainstream Go practice for request-scoped metadata. The
  Go-community guidance against `ctx.WithValue` for non-cancellation
  data is well-known but `Source` qualifies as request-scoped (one value
  per CLI invocation, never changes).

- **Candidate B — `internal/install/installops` Service layer above Manager.**
  A new Service owns the lifecycle verbs (Install, Rollback, Remove*)
  and publishes events. Manager keeps low-level state and symlink
  primitives. Request-struct shape (`InstallRequest`, `RollbackRequest`)
  absorbs future cross-cutting concerns as fields. Critically, hides
  `state.json` semantics behind named methods (`MarkExplicit`,
  `RecordDependency`) — addressing the actual structural smell the
  exploration surfaced: the `mgr.GetState().UpdateTool(name, func(ts){...})`
  pattern leaks state semantics into the CLI layer at three sites in
  `cmd/tsuku/install_deps.go`. Incremental migration possible across
  ~5 PRs.

**The status-quo position (Candidate A) remains defensible** if the
design judges that (a) the cumulative cross-cutting cost is overstated
by the workaround evidence, (b) the `mgr.GetState().UpdateTool` pattern
is fine as a CLI-layer convenience rather than a smell, and (c) the
12-month forecast is closer to 1 new concern than 3+.

## Decision Drivers

In rough priority order:

1. **Cross-cutting recurrence is empirically a recurring class of problem in this codebase.**
   One new orthogonal install-pipeline concern every 4-6 weeks across the
   last five PRs. Whatever shape the design chooses must reduce per-concern
   threading cost at least somewhat, or it does nothing useful.

2. **Cancellation is a real missing capability.** Manager doesn't take
   `ctx` anywhere. SIGINT during the final atomic-rename window cannot
   interrupt cleanly. This is independently worth fixing — and once ctx
   is threaded, `Source`-via-ctx becomes essentially free.

3. **The lifecycle event bus (DESIGN-notices-install-event-bus, Current)
   must compose unchanged.** Any restructure must preserve the
   verb-per-event vocabulary, the synchronous-with-recover delivery, the
   publish-after-state invariant, and the subscriber-locality contract.
   The bus is shipped and load-bearing; the design does not reopen it.

4. **Bounded blast radius.** A migration approaching the size of PR #2412
   itself is acceptable only if it pays for itself across multiple future
   concerns. Incremental migration paths are strongly preferred.

5. **Address the actual smells, not the proposed shape.** The exploration
   surfaced two underlying problems: the `installWithDependencies`
   recursive-trailing-arg pattern, and `state.json` exposure via
   `mgr.GetState()`. A restructure that doesn't touch these leaves the
   threading problem half-solved.

6. **No external API contract.** `install.Manager` is in `internal/`.
   The cost of getting the shape wrong is bounded — there is no
   backward-compatibility obligation. This argues for choosing the
   genuinely better shape rather than the minimum-viable one.

7. **Library install (`InstallLibrary`) scope.** The exploration
   confirmed libraries are second-class to the lifecycle events (no bus
   integration). The design must decide whether libraries join any
   restructure (clean, more work) or stay parallel for now (pragmatic,
   leaves a known gap).

## Decisions Already Made

Captured in `wip/explore_install-state-abstraction_decisions.md` during
the exploration:

- **Java-style literal repository pattern eliminated.** Over-engineered
  for ~5 lifecycle operations with no second storage backend. Go's
  struct-with-methods on `*Manager` is already the repository pattern.
- **Command/middleware/decorator-chain patterns eliminated.** Pays off
  with N independent concerns × M operations where the cross-product
  would otherwise be hand-written. Here N=2-3, M=5; indirection cost
  dominates benefit.
- **Aggressive `OperationOptions` struct (Candidate D from exploration)
  deprioritized.** Essentially "Candidate A plus shared struct"; does
  not address the structural `state.json` exposure smell. May appear
  in the final design as a tactical sub-element of B or C but not as
  the top-level shape.

These are constraints the design treats as settled. Reopening them
requires concrete evidence not surfaced during the exploration.

## Considered Options

The design decomposes into three decisions. The first is the central
architectural choice; the second and third are partly coupled but
independently evaluable.

### Decision 1: Architectural shape for reducing cross-cutting blast radius

**Context.** PR #2412 (lifecycle event bus) shipped successfully but
threading the `Source` attribution parameter exposed structural friction
in `install.Manager`. The exploration's six leads found:

- `installWithDependencies` grew from 7 to 10 positional parameters in
  three months (Lead 2).
- Five install-touching PRs in three months each landed a new
  cross-cutting concern — one every 4-6 weeks (Lead 2).
- Three in-tree workarounds exist that authors built to avoid threading
  cost: 9 package-level CLI flag globals, a parallel `runDryRun` that
  bypasses the install pipeline, and a stranded `telemetryClient`
  parameter PR #2412 could not remove (Lead 2).
- PR #2412's headline "36 files" decomposes into 16 one-time-cost files
  (bus, subscribers, schema, renderer, docs), 5 net-negative
  direct-write-removal files, and only 11 files / ~120 LOC of recurring
  `Source`-threading cost (Lead 5).
- The real structural smell is `mgr.GetState().UpdateTool(name, func(ts){...})`
  leaking state semantics into the CLI layer at three sites in
  `cmd/tsuku/install_deps.go` (Lead 6).
- Manager doesn't take `ctx` anywhere; Ctrl-C during the final
  atomic-rename window is a silent hazard (Lead 4).
- `Source` behaves like a process-level attribute — every call site
  passes a literal constant (Lead 1), making it the textbook case for
  ctx-based attribution.

**Key assumptions:**
- 12-month forecast: approximately 3 new cross-cutting concerns
  (conservative read of the one-per-4-6-weeks base rate). Plausible
  inventory: dry-run, ctx/cancellation, audit/correlation ID.
- `state.json` exposure via `mgr.GetState().UpdateTool(...)` counts as
  a smell to heal — but Decision 2 owns whether to address it now.
- Incremental migration is strongly preferred over single-shot commits
  (Decision Driver 4).
- Cancellation is independently worth shipping (Decision Driver 2).
- The lifecycle event bus stays untouched (Decision Driver 3).
- `Source`-via-`ctx.Value` is defensible Go practice because `Source`
  is request-scoped metadata (set once at the entry-point boundary,
  never changes mid-flight, only informs subscribers — does not
  control branching).

#### Chosen: Composite C-then-B (Candidate C now; Candidate B as charter follow-up)

Land Candidate C as the immediate work. Charter a follow-up evaluation
of Candidate B once 1-2 additional cross-cutting concerns have landed
(or 3-4 months elapse, whichever first).

**What Candidate C entails:**

1. **Thread `context.Context` through `install.Manager` public methods.**
   `Install`, `InstallWithOptions`, `Rollback`, `Remove`, `RemoveVersion`,
   `RemoveAllVersions`, `Activate`, `InstallLibrary` all take
   `ctx context.Context` as the first parameter. Roughly 10 method
   signatures, ~30 call sites, ~15 files touched. Same file-count
   shape as Source-as-param; net negative for `internal/updates/` which
   already has ctx but currently threads Source separately.

2. **Add `installevents.WithSource(ctx, src)` and `installevents.SourceFromContext(ctx)` helpers**
   with a typed `srcKey struct{}`. Manager extracts Source at publish
   callsites (`publishInstallOutcome`, `publishRemoveOutcome`). Source
   as a positional parameter and `InstallOptions.Source` field are
   removed from Manager's public surface. The bus's existing
   empty-Source-drops-with-log behavior catches the "you forgot to set
   ctx value" failure mode.

3. **Collapse `installWithDependencies` trailing-arg recursion into a
   request struct.** A new local `installArgs` struct in
   `cmd/tsuku/install_deps.go` carries `Tool`, `ReqVersion`,
   `VersionConstraint`, `IsExplicit`, `Parent`, `Reporter`, and
   `TelemetryClient`. The recursive call constructs a sub-args by
   copying with overrides instead of threading 10 positional
   parameters.

4. **Cancellation lands as a free bonus.** With ctx threaded, the
   atomic-rename window in `manager.go` can check `ctx.Err()` and
   abort cleanly. SIGINT propagates from `globalCtx` in
   `cmd/tsuku/main.go` to Manager.

**Why not pure A (status quo)?** Lead 2's recurring-pattern evidence
(one new concern every 4-6 weeks, 3 in-tree workarounds,
`installWithDependencies` 7→10 positional params in 3 months) weighs
decisively against. Doing nothing accepts that the next concern
reproduces PR #2412's pattern and the next workaround compounds the
existing three.

**Why not pure B?** B's upfront cost (~5 PRs, ~600 lines new package,
~30 construction sites) is comparable to PR #2412 itself, while
savings only marginally exceed C's at the conservative forecast. B
does not deliver cancellation on its own — a separate ctx-threading
effort would still land, duplicating most of C's blast radius. Doing
C first, watching cadence, then deciding on B with one more datapoint
is strictly better than committing to B blind.

**Why not B+C simultaneously?** Combined blast radius exceeds PR
#2412's, contradicting Decision Driver 4 (bounded blast radius). "Fix
everything at once" is exactly what produced PR #2412's size that
motivated this design.

#### Alternatives Considered

- **Candidate A (status quo).** Continue threading cross-cutting concerns
  as method parameters. Rejected because the recurring-pattern evidence
  shows the cumulative cost is real and compounding. The status-quo
  position would have been defensible if the 12-month forecast were
  closer to 0-1 concerns; at 3+ it doesn't hold.
- **Candidate B (installops Service layer, as primary choice).** New
  `internal/install/installops` package with a `Service` type wrapping
  Manager, owning lifecycle verbs and publishing events. Rejected as
  the primary choice because (a) upfront cost is comparable to PR
  #2412 again; (b) doesn't deliver cancellation on its own;
  (c) committing to B requires the 12-month forecast to firmly support
  2+ concerns, which is a judgment with significant uncertainty. Preserved
  as an explicit follow-up: revisit Candidate B once 1-2 additional
  cross-cutting concerns have landed or 3-4 months have elapsed.
- **Composite B+C (simultaneous).** Land both in one design. Rejected
  because the combined blast radius exceeds PR #2412's, directly
  contradicting Decision Driver 4.
- **Aggressive `OperationOptions` shared struct (Candidate D from exploration).**
  Already eliminated during exploration as "Candidate A plus shared
  struct" that doesn't address the `state.json` exposure smell.
- **Literal Java-style repository pattern.** Already eliminated during
  exploration. Go's struct-with-methods on `*Manager` is already the
  repository pattern in this codebase's shape (5 lifecycle ops, one
  storage backend).
- **Command/middleware/decorator chains.** Already eliminated during
  exploration. N=2-3 concerns × M=5 operations doesn't justify
  indirection cost.

### Decision 2: state.json semantic operations encapsulation

**Context.** `cmd/tsuku/install_deps.go` writes state.json directly at
three sites (lines 223, 475, 584) using the pattern:

```go
mgr.GetState().UpdateTool(name, func(ts *install.ToolState) {
    if isExplicit { ts.IsExplicit = true }
    if parent != "" { /* append to RequiredBy if not present */ }
    ts.InstallDependencies = ...
    // cleanup actions, etc.
})
```

Manager owns `state.json` internally via `m.state.UpdateTool` but
*also* exposes `m.GetState()` so callers do their own writes. Lead 6
called this "the actual leaky abstraction." Two of the three lambda
blocks duplicate the same IsExplicit/RequiredBy logic (~17 lines each).

**Key assumptions:**
- The three call sites are representative of the leak; no hidden
  callers outside `cmd/tsuku/` write state via `GetState().UpdateTool`
  (confirmed by grep).
- Whatever surface exposes the semantic methods must be the *sole*
  write surface — leaving `GetState()` exposed alongside semantic
  methods preserves the leak.
- The semantic operations are stable enough to name; `MarkExplicit`,
  `RecordDependency`, and `RecordCleanup` map directly to the three
  observed write patterns.

#### Chosen (under Decision 1 = C): (b) Add semantic methods on Manager

`Manager.MarkExplicit(name, parent string) error`,
`Manager.RecordDependency(name, dep string) error`, and
`Manager.RecordCleanup(name string, actions []CleanupAction) error`.
The lambda pattern at lines 223, 475, 584 disappears (collapses from
~17 lines each to ~3 lines per concern). `Manager.GetState()` is
either removed from CLI callers or restricted to read-only access
(via narrower `Manager.GetToolState` / `Manager.LoadState` accessors).

Rationale: Decision 1 = C means no Service layer exists yet. Putting
the semantic methods on Manager keeps the surface count minimal and
the migration small (3 call sites → ~6 method calls). If Candidate B
lands later as the charter follow-up, these methods migrate to
`installops.Service` cleanly (rename receiver, update call sites).

This decision is treated as conditional in the cross-validation
record: under Decision 1 = B the answer would be (c) — semantic
methods on `installops.Service`. The cross-validation phase observed
no conflict because Decision 1 chose C.

#### Alternatives Considered

- **(a) Status quo — keep `mgr.GetState()` exposed.** Rejected. Lead 6
  named this pattern as the actual structural smell the design exists
  to address. The IsExplicit/RequiredBy logic is already duplicated
  across three sites; no future state field that follows the same
  pattern will be a one-liner.
- **(c) Move semantic methods to a new Service layer.** Conditional on
  Candidate B. Deferred to the charter follow-up. If/when Candidate B
  lands, the methods migrate from Manager to Service.
- **(d) Standalone `StateOps` type.** Rejected. Introduces a third
  surface (Manager primitives + StateOps semantics + future Service
  lifecycle) that compounds confusion without paying off. Under
  Decision 1 = C, it's strictly more API surface than (b); under
  Decision 1 = B, it's either redundant with Service or has to be
  absorbed into Service later. Strictly worse on the API-surface
  criterion in every Decision 1 outcome.

### Decision 3: Library install scope

**Context.** `internal/install/library.go` defines `Manager.InstallLibrary`
separately from `Manager.Install`. Libraries have their own state
model (`State.Libs`) separate from tools (`State.Tools`). The
lifecycle event bus does not cover libraries — no `Installed{Tool: libname}`
event fires for library installs. PR #2412 added a `src installevents.Source`
parameter to `installLibrary` (the CLI-layer wrapper) but it forwards
without using it; the threading cost was paid, the benefit was not.

**Key assumptions:**
- Issue #2413 explicitly scoped libraries out as a deliverable.
- Libraries have meaningfully different semantics (no symlinks,
  `UsedBy` instead of `RequiredBy`, separate state map, no
  `Activate`, no rollback symmetry).
- Future cross-cutting concerns (audit log, cancellation) will want
  uniform coverage across tools and libraries.

#### Chosen (under Decision 1 = C): (b) bring libraries along + (d) wire library events onto the bus

Under Candidate C, ctx threading is mechanically uniform across
`Install`, `InstallWithOptions`, and `InstallLibrary`. Threading ctx
through `InstallLibrary` is the same pattern, costs essentially the
same per-file as for tools, and is shape-agnostic — it doesn't force
library semantics into a tool-shaped layer because there is no layer.
Cancellation benefit accrues to libraries too. The
`LibraryInstallOptions`-missing-Source inconsistency is healed as a
side effect.

Pair this with **(d)**: while the library install path is being
touched, wire library lifecycle events onto the bus. This is a small
additional cost (per Lead 5's audit-log analysis, a new event
subscriber pattern is ~2 files) but recoups PR #2412's already-paid
`Source` threading cost in `installLibrary` and unblocks future
subscribers that need full-pipeline coverage.

The Cross-cutting effects section of Decision 1's report initially
recommended (a) (libraries stay parallel) under C — calling library
ctx threading a "side effect." Decision 3's dedicated evaluation
chose (b) — calling the same action "extend to libraries explicitly."
Cross-validation observed this as a labeling difference, not a
substantive disagreement: both agree that under C, ctx threads through
`InstallLibrary` and libraries pick up ctx-based attribution. The
design takes the more explicit framing.

#### Alternatives Considered

- **(a) Libraries out of scope, stay parallel.** Rejected under
  Decision 1 = C. Leaves PR #2412's already-paid `Source` threading
  cost in `installLibrary` unrecouped, and leaves bus subscribers
  blind to library installs. A future audit-log subscriber would
  silently miss half the install pipeline.
- **(c) Follow-up design.** Considered. Would respect the original
  scope strictly but defer closing the gap. Rejected because ctx
  threading through `InstallLibrary` is mechanically uniform with the
  tool path under C — there is no separate library design work to
  defer that wouldn't be re-litigating the same shape question.
- **(d) Bus integration only, skip restructure for libraries.**
  Rejected as a standalone option but adopted as a complement to (b).
  Library bus events alone would leave the threading inconsistency
  unaddressed; bundled with (b), the library install path achieves
  uniform first-class status.

## Decision Outcome

The three decisions compose into a single coherent direction:

1. **Architectural shape (Decision 1) — Candidate C now, Candidate B
   as charter follow-up.** Thread `context.Context` through Manager's
   public methods. Store `Source` in ctx via a typed key
   (`installevents.WithSource(ctx, src)` /
   `installevents.SourceFromContext(ctx)`). Collapse
   `installWithDependencies`'s trailing-arg recursion into a request
   struct. Land cancellation as a free bonus.

2. **state.json encapsulation (Decision 2) — semantic methods on
   Manager.** Add `Manager.MarkExplicit`, `Manager.RecordDependency`,
   and `Manager.RecordCleanup`. Remove `mgr.GetState()` write access
   from CLI callers. The three lambda blocks in `install_deps.go`
   collapse to single method calls per concern.

3. **Library scope (Decision 3) — libraries come along.** Thread ctx
   through `InstallLibrary` too. Wire library lifecycle events onto
   the bus so library installs become first-class observables.

**The charter for Candidate B follow-up.** This design explicitly
preserves the option to introduce an `installops.Service` layer later.
Trigger conditions: 1-2 additional cross-cutting concerns have landed
post-Candidate-C, or 3-4 months have elapsed. The follow-up evaluation
would have empirical cadence data instead of a forecast assumption;
the semantic state methods chosen in Decision 2 migrate cleanly from
Manager to Service. A `needs-design` issue will be filed once Candidate
C ships, with the trigger conditions stated.

**Why this composition.** Candidate C delivers ~80% of Candidate B's
threading reduction at ~30-40% of B's cost. It composes trivially with
the lifecycle event bus (bus signature unchanged; only the *source* of
`Source` changes — from positional param to ctx extraction at publish
callsites). It unlocks cancellation, which is independently worth
shipping. The semantic state methods from Decision 2 fix the
duplicated lambda smell without depending on a Service layer. The
library work from Decision 3 recoups PR #2412's already-paid threading
cost. The total blast radius (~15-20 files, ~150-200 LOC) is
meaningfully smaller than PR #2412's actual cross-cutting cost (11
files / ~120 LOC) plus a comparable library/state-method increment.

Three implementation concerns surface from cross-validation, addressed
in Solution Architecture below:

1. **`ctx.Value` for non-cancellation data** is a well-known
   Go-community ambivalence. `Source` qualifies as request-scoped
   metadata, justifying the override; the rationale must appear in
   code comments.
2. **Cancellation must not corrupt state.json.** Manager's `ctx.Err()`
   checks must sit before state writes, not interleave with them.
3. **`Manager.GetState()` removal** is the load-bearing API change.
   Read-only access must remain available via narrower accessors;
   tests and any non-CLI callers need a migration path.

## Solution Architecture

### Overview

`internal/install/manager.go` and `internal/install/remove.go` are
modified to:

1. Accept `context.Context` as the first parameter on every public
   lifecycle method.
2. Extract `installevents.Source` from `ctx` at publish callsites.
3. Expose three new semantic state methods (`MarkExplicit`,
   `RecordDependency`, `RecordCleanup`).
4. Restrict `GetState()` to read-only or remove it from CLI callers
   entirely.

`internal/installevents/events.go` gains two helpers:

```go
// WithSource attaches a Source to ctx via a typed key. Callers pass
// the result as the ctx parameter to install.Manager methods.
func WithSource(ctx context.Context, src Source) context.Context

// SourceFromContext extracts the Source attached by WithSource. Returns
// "" if no Source was set; publishers treat empty Source as a logic bug
// (the bus already drops events with empty Source with a log line).
func SourceFromContext(ctx context.Context) Source
```

`internal/install/library.go` and `cmd/tsuku/install_lib.go` are
updated to:

1. Accept ctx on `InstallLibrary` and friends.
2. Publish lifecycle events for library installs on the same bus
   (using a new vocabulary distinguishing library events from tool
   events — see Library Bus Vocabulary below).

`cmd/tsuku/install_deps.go` is restructured:

1. The recursive `installWithDependencies` becomes a function over an
   `installArgs` struct. The recursive call copies the struct with
   overrides instead of threading 10 positional parameters.
2. The three direct `mgr.GetState().UpdateTool(...)` blocks at lines
   223, 475, 584 are replaced with calls to the new semantic methods.

### Components

```
+---------------------------------------------+
|              cmd/tsuku/ (CLI)               |
|                                             |
|  globalCtx = signal-aware ctx (existing)    |
|  ctx = installevents.WithSource(            |
|          globalCtx, SourceManual)           |
|                                             |
|  installArgs := installArgs{...}            |
|  runInstall(ctx, installArgs)               |
|    -> installWithDependencies(ctx, args,    |
|       visited)                              |
|       -> mgr.InstallWithOptions(ctx, ...)   |
|       -> mgr.MarkExplicit(name, parent)     |
|       -> mgr.RecordDependency(name, dep)    |
|       -> mgr.RecordCleanup(name, actions)   |
+--------------------+------------------------+
                     |
                     v
+---------------------------------------------+
|         internal/install/manager.go         |
|                                             |
|  Manager.Install(ctx, ...) {                |
|    src := installevents.SourceFromContext(  |
|             ctx)                            |
|    // ... existing body ...                 |
|    if err := ctx.Err(); err != nil {        |
|      return err  // before state write      |
|    }                                        |
|    m.state.UpdateTool(...)                  |
|    defer m.publishInstallOutcome(           |
|       ..., src, ...)                        |
|  }                                          |
|                                             |
|  Manager.MarkExplicit(name, parent) error   |
|  Manager.RecordDependency(name, dep) error  |
|  Manager.RecordCleanup(name, actions) error |
|    // each is a m.state.UpdateTool wrapper  |
|    // with one semantic operation           |
|                                             |
|  Manager.InstallLibrary(ctx, ...) {         |
|    // same pattern: extract source from ctx,|
|    // honor cancellation, publish event     |
|  }                                          |
+--------------------+------------------------+
                     |
                     | (existing event bus, unchanged)
                     v
+---------------------------------------------+
|         internal/installevents.Bus          |
|                                             |
|  Publish(event)                             |
|  Subscribe(name, sub)                       |
|  WithSource(ctx, src) context.Context  NEW  |
|  SourceFromContext(ctx) Source         NEW  |
+--------------------+------------------------+
                     |
        +------------+-------------+
        v                          v
+------------------+   +------------------------+
| notices.        |    | telemetry.Subscriber    |
| Subscriber      |    | (existing — unchanged)  |
| (existing)      |    +------------------------+
+------------------+
```

### Key Interfaces

```go
// Package internal/install

type Manager struct {
    /* ... existing fields ... */
    bus *installevents.Bus
}

// All public lifecycle methods gain ctx as the first parameter:
func (m *Manager) Install(ctx context.Context, name, version, workDir string) error
func (m *Manager) InstallWithOptions(ctx context.Context, name, version, workDir string, opts InstallOptions) (err error)
func (m *Manager) Rollback(ctx context.Context, name, toVersion string) error
func (m *Manager) Remove(ctx context.Context, name string) error
func (m *Manager) RemoveVersion(ctx context.Context, name, version string) (err error)
func (m *Manager) RemoveAllVersions(ctx context.Context, name string) (err error)
func (m *Manager) Activate(ctx context.Context, name, version string) error
func (m *Manager) InstallLibrary(ctx context.Context, name, version, workDir string, opts LibraryInstallOptions) error

// InstallOptions loses its Source field; Source flows via ctx.
type InstallOptions struct {
    /* existing fields, minus Source */
}

// Three new semantic state methods replace the lambda pattern:
func (m *Manager) MarkExplicit(name, parent string) error
func (m *Manager) RecordDependency(name, dep string) error
func (m *Manager) RecordCleanup(name string, actions []CleanupAction) error

// GetState() is removed from CLI callers. Two narrower read accessors
// remain (or are added if not present):
func (m *Manager) GetToolState(name string) (*ToolState, error)
func (m *Manager) LoadState() (*State, error)
```

```go
// Package internal/installevents

type srcKey struct{} // unexported, prevents collision

// WithSource attaches src to ctx. CLI callers wrap globalCtx once at
// the entry-point boundary and pass the resulting ctx into Manager.
func WithSource(ctx context.Context, src Source) context.Context {
    return context.WithValue(ctx, srcKey{}, src)
}

// SourceFromContext extracts the Source attached by WithSource.
// Returns "" if not set. The bus's existing empty-Source-drops-with-log
// behavior catches "you forgot to set ctx value" as a logic bug.
func SourceFromContext(ctx context.Context) Source {
    src, _ := ctx.Value(srcKey{}).(Source)
    return src
}
```

The CLI layer (`cmd/tsuku/install_deps.go`):

```go
type installArgs struct {
    Tool, ReqVersion, VersionConstraint string
    IsExplicit                          bool
    Parent                              string
    Reporter                            progress.Reporter
    TelemetryClient                     *telemetry.Client
}

func runInstall(ctx context.Context, args installArgs) error {
    if args.Reporter == nil {
        r := progress.NewTTYReporter(os.Stderr)
        defer func() { r.Stop(); r.FlushDeferred() }()
        args.Reporter = r
    }
    return installWithDependencies(ctx, args, make(map[string]bool))
}

func installWithDependencies(ctx context.Context, args installArgs, visited map[string]bool) error {
    // ... build request from args ...
    if err := mgr.InstallWithOptions(ctx, args.Tool, version, exec.WorkDir(), opts); err != nil {
        return err
    }
    // semantic state methods replace the lambda pattern:
    if args.IsExplicit {
        if err := mgr.MarkExplicit(args.Tool, args.Parent); err != nil { return err }
    }
    if args.Parent != "" {
        if err := mgr.RecordDependency(args.Tool, args.Parent); err != nil { return err }
    }
    // ... etc ...
    // recursive call: copy args with overrides, no positional churn:
    sub := args
    sub.Tool = dep
    sub.IsExplicit = false
    sub.Parent = args.Tool
    sub.ReqVersion = ""
    sub.VersionConstraint = ""
    return installWithDependencies(ctx, sub, visited)
}
```

### Data Flow

#### Successful manual update

1. User runs `tsuku update niwa`.
2. `cmd/tsuku/update.go` constructs
   `ctx := installevents.WithSource(globalCtx, installevents.SourceManual)`.
3. Calls `mgr.Install(ctx, "niwa", "0.11.1", workDir)`.
4. `Manager.Install` extracts `src := installevents.SourceFromContext(ctx)`,
   sees prior `ActiveVersion == "0.11.0"`, performs the update,
   checks `ctx.Err()` once before the atomic-rename window, writes
   state.json, publishes `Updated{Tool: "niwa", FromVersion: "0.11.0",
   ToVersion: "0.11.1", Source: SourceManual}`.
5. Bus invokes notices and telemetry subscribers (existing flow,
   unchanged).

#### Cancellation during install

1. User starts `tsuku install large-tool` and hits Ctrl-C during the
   archive extract.
2. `globalCtx` (set up in `cmd/tsuku/main.go`) is cancelled by the
   SIGINT handler.
3. Manager's next `ctx.Err()` check returns `context.Canceled`.
4. Manager aborts before the atomic rename. Staging directory is
   cleaned up by existing logic.
5. Manager publishes `InstallFailed{Tool: "large-tool", Err: context.Canceled, ...}`.
6. Notices subscriber writes a "cancelled" notice (existing failure
   path; error sanitization applies).
7. Process exits with non-zero status.

The `ctx.Err()` check sits **before** state.json writes (per the
publish-after-state invariant) and **before** the atomic rename (per
the "no half-installed state" property). Mid-action cancellation
(e.g., during a long archive extract) propagates through the
executor's existing ctx threading, which already supports it.

#### Project-auto install

1. `tsuku run` in a directory with `.tsuku.toml` requiring `gh`.
2. `cmd/tsuku/cmd_run.go` constructs
   `ctx := installevents.WithSource(globalCtx, installevents.SourceProjectAuto)`.
3. Calls `mgr.Install(ctx, "gh", "2.47.0", workDir)`.
4. Rest of the flow identical to manual install but with
   `Source: SourceProjectAuto` flowing through ctx.

#### Library install

1. CLI layer constructs ctx with Source as above.
2. Calls `mgr.InstallLibrary(ctx, libName, version, workDir, opts)`.
3. `InstallLibrary` extracts `src` from ctx, writes library state,
   publishes a new library lifecycle event (see Library Bus
   Vocabulary).
4. Subscribers (notices, telemetry) handle the library event
   alongside tool events.

### Library Bus Vocabulary

Libraries have meaningfully different semantics from tools — no
`Activate`, no rollback symmetry, `UsedBy` rather than `RequiredBy`.
Forcing them into the existing `Installed{Tool: libname}` event
shape would lose this signal and confuse subscribers that branch on
the tool/library distinction.

Two options for the vocabulary:

**Option (i): Reuse the existing event types with a `Kind` discriminator.**
Add a `Kind string` field (values: `"tool"`, `"library"`) on every
existing event. Backward-compatible: existing subscribers ignore
unfamiliar field values.

**Option (ii): Add four new library-specific event types.**
`LibraryInstalled`, `LibraryRemoved`, `LibraryInstallFailed`,
`LibraryRemoveFailed`. Subscribers add explicit type-switch arms.

This design picks **Option (ii)**. Rationale: the bus design's
verb-per-event vocabulary (`Updated`, `Installed`, `Removed`, etc.)
intentionally avoids string-typed discriminators. Adding `Kind` would
reintroduce the string-switch pattern the bus design explicitly
rejected. Four new typed events match the existing pattern, give
subscribers compile-time exhaustiveness, and keep tool/library
branching at the type level.

Library events do **not** include `Updated` or `Rollback` variants
because library installs have no version-update semantics today —
each install is a fresh placement. If library updates become a
first-class operation later, the vocabulary extends additively.

### Source threading via ctx

`Source` lives in `ctx` from the moment the CLI entry-point wraps
`globalCtx` with `WithSource`. It flows through every subsequent
function call without appearing in any function signature except
where publish needs to extract it. The bus's existing empty-Source-
drops-with-log behavior catches the "forgot to call `WithSource`"
failure mode.

The `WithSource` call sites are localized:
- `cmd/tsuku/install.go` — `WithSource(globalCtx, SourceManual)` for
  `tsuku install`
- `cmd/tsuku/update.go` — `WithSource(globalCtx, SourceManual)` for
  `tsuku update`
- `cmd/tsuku/remove.go` — `WithSource(globalCtx, SourceManual)` for
  `tsuku remove`
- `cmd/tsuku/cmd_rollback.go` — `WithSource(globalCtx, SourceManual)`
- `cmd/tsuku/cmd_apply_updates.go` —
  `WithSource(globalCtx, SourceAuto)` for the auto-apply subprocess
- `cmd/tsuku/cmd_run.go` — `WithSource(globalCtx, SourceProjectAuto)`
  for the autoinstall path

Six call sites. Every other layer (Manager, executor, plan generator,
recipe loader) just receives `ctx` and forwards it. No code outside
these six call sites sets `Source`.

### Publish-after-state invariant preserved

The lifecycle event bus's existing publish-after-state invariant is
preserved by construction: Manager methods still use `defer` to
publish after their state write completes, and the deferred publish
extracts `Source` from `ctx` (which is captured by the defer's
closure). The bus's `Publish` signature does not change.

### Subscriber-locality contract preserved

The existing subscriber-locality contract (a subscriber may only
mutate the on-disk record for the tool/library named in the event it
is handling) extends naturally to library events. The notices
subscriber's library branch writes/removes `notices/<event.Library>.json`
(or a namespaced equivalent — see Notice File Naming below) and no
other file.

### Notice File Naming for Libraries

Today notice files live at `$TSUKU_HOME/notices/<tool>.json`. A
library installed with the same name as a tool (unlikely but
possible) would collide. The design adds a kind prefix for library
notices: `$TSUKU_HOME/notices/lib--<library>.json`. The `lib--`
prefix is a sentinel; tool names cannot contain `--` in this position
(`notices.WriteNotice` already validates against path-separator and
`..` injection).

### Manager.Activate ctx threading

`Manager.Activate` is internally called by `Manager.Install` for
rollback recovery on update failure. The published `UpdateFailed`
event captures the post-recovery state in `ActiveAfter` per the
event bus design. With ctx threading, `Activate` becomes:

```go
func (m *Manager) Activate(ctx context.Context, name, version string) error {
    if err := ctx.Err(); err != nil { return err }
    // existing body
}
```

`Activate` does NOT publish events directly; the caller (`Install`,
`Rollback`, future Service layer) owns the published event. This is
unchanged from the current bus design.

### Test patterns

Tests construct `ctx` inline:

```go
ctx := installevents.WithSource(context.Background(), installevents.SourceManual)
err := mgr.Install(ctx, "foo", "1.0", workDir)
```

A test helper in `internal/installevents/eventtest/` (or similar)
provides:

```go
// WithSourceManual returns a ctx pre-configured with SourceManual
// for tests that don't care about the Source value.
func WithSourceManual(t *testing.T) context.Context {
    t.Helper()
    return installevents.WithSource(context.Background(), installevents.SourceManual)
}
```

Cancellation tests construct a cancellable ctx:

```go
ctx, cancel := context.WithCancel(installevents.WithSource(context.Background(), installevents.SourceManual))
defer cancel()
go func() {
    // wait for the install to reach a checkpoint
    cancel()
}()
err := mgr.Install(ctx, "large-tool", "1.0", workDir)
require.ErrorIs(t, err, context.Canceled)
```

## Implementation Approach

Six phases, in a single PR end-to-end. Phase boundaries are advisory
for commit organization. Phases 1 and 2 must happen in the same
commit because removing `InstallOptions.Source` without first having
ctx threading would break callers.

### Phase 1: Context threading on Manager (foundation)

Modify `internal/install/manager.go`, `internal/install/remove.go`,
and adjacent files:

- Add `ctx context.Context` as the first parameter on every public
  lifecycle method (`Install`, `InstallWithOptions`, `Rollback`,
  `Remove`, `RemoveVersion`, `RemoveAllVersions`, `Activate`,
  `InstallLibrary`).
- Add an early `ctx.Err()` check at the top of each method that
  performs a state mutation.
- Add a `ctx.Err()` check immediately before the atomic-rename
  window in `Manager.Install`.
- Internal helpers (`createSymlink`, `createBinarySymlink`,
  `createSymlinksForBinaries`, etc.) accept ctx as well so future
  cancellation hooks can be added incrementally.

Update every call site to pass `ctx`. Many can pass
`context.Background()` initially; CLI entry points and the auto-apply
subprocess pass meaningful ctxs.

### Phase 2: Source migration to ctx (must be same commit as Phase 1)

Modify `internal/installevents/events.go`:

- Add `WithSource(ctx, src)` and `SourceFromContext(ctx)` helpers with
  a typed `srcKey struct{}`.
- A code comment on `WithSource` documents the rationale: `Source`
  qualifies as request-scoped metadata (set once at the entry-point
  boundary, never changes mid-flight, only informs subscribers — does
  not control branching). This is the override of the
  "context-is-not-config" guideline.

Modify `internal/install/manager.go` and `remove.go`:

- Remove `Source` from `InstallOptions`.
- Remove the `src installevents.Source` parameter from every public
  Manager lifecycle method.
- At publish callsites, extract `src` from `ctx` via
  `installevents.SourceFromContext(ctx)` instead of from method
  arguments.

Update every CLI entry point to call `installevents.WithSource(globalCtx, ...)`
once and thread the resulting `ctx` through all subsequent calls.
Update `internal/updates/apply.go` and `self.go` to construct ctx
with `SourceAuto`.

### Phase 3: Semantic state methods on Manager (Decision 2)

Add three new methods to `Manager` (in `internal/install/manager.go`
or a new `internal/install/state_ops.go`):

```go
func (m *Manager) MarkExplicit(name, parent string) error
func (m *Manager) RecordDependency(name, dep string) error
func (m *Manager) RecordCleanup(name string, actions []CleanupAction) error
```

Each is a thin wrapper around `m.state.UpdateTool` that encapsulates
one semantic operation. Add unit tests.

Replace the three lambda blocks in `cmd/tsuku/install_deps.go`
(lines 223, 475, 584) with calls to these methods. Remove
`mgr.GetState()` write access from CLI callers (restrict to read-only
via narrower accessors).

### Phase 4: installWithDependencies recursion collapse

In `cmd/tsuku/install_deps.go`:

- Define a local `installArgs` struct.
- Refactor `runInstall`, `runInstallWithReporter`, and
  `installWithDependencies` to take `(ctx, installArgs)` instead of
  10 positional parameters.
- The recursive call constructs a sub-args by copying with overrides.

### Phase 5: Library install joins ctx + bus

Modify `internal/install/library.go`:

- `InstallLibrary` takes `ctx` (lands as part of Phase 1).
- Source is extracted from ctx at publish callsites.

Modify `internal/installevents/events.go`:

- Add four new event types: `LibraryInstalled`, `LibraryRemoved`,
  `LibraryInstallFailed`, `LibraryRemoveFailed`. Implement the
  sealed `Event` interface.

Modify `internal/install/library.go` and `cmd/tsuku/install_lib.go`:

- Publish `LibraryInstalled` / `LibraryInstallFailed` from
  `InstallLibrary`.
- (Future) library removal path publishes corresponding events when
  a remove path is introduced.

Modify `internal/notices/subscriber.go`:

- Add type-switch arms for the four new library events.
- Write to `notices/lib--<library>.json` using the kind-prefixed
  naming convention.

Modify `internal/telemetry/subscriber.go`:

- Add type-switch arms for library events; emit corresponding
  telemetry outcomes.

### Phase 6: Cancellation hooks at safe interruption points

Add `ctx.Err()` checks at points in Manager that can safely abort
without corrupting state:

- Top of every public lifecycle method (already in Phase 1).
- Immediately before the atomic-rename window in `Manager.Install`.
- Between dependency-recording iterations in `installWithDependencies`
  (so cancellation between dep installs aborts cleanly).
- In the executor (already supports ctx; nothing to do).

Cancellation tests in `manager_test.go` verify:
- Cancelling before state write returns `context.Canceled` without
  mutating state.json.
- Cancelling after state write but before publish still publishes
  (the publish defer captures the pre-cancellation state).
- Cancelling during dependency walk stops at the next safe point.

### Validation

- Existing tests in `internal/install/manager_test.go`,
  `internal/install/manager_events_test.go`,
  `internal/install/manager_events_e2e_test.go` adapt to the new
  signatures (pass `installevents.WithSource(context.Background(),
  SourceManual)` instead of the `src` arg).
- New unit tests in `internal/installevents/` cover `WithSource` /
  `SourceFromContext` (round-trip, empty key returns "", typed-key
  collision impossible).
- New unit tests in `internal/install/manager_test.go` cover the
  three semantic state methods (`MarkExplicit`, `RecordDependency`,
  `RecordCleanup`).
- New unit tests for cancellation: cancel-before-state, cancel-after-
  state-before-publish, cancel-during-dependency-walk.
- New end-to-end test for library install on the bus: trigger
  `tsuku install <library>` (or whatever the user-facing entry is),
  assert `LibraryInstalled` event fires and notice file is written.

### Charter for Candidate B follow-up

Once Phase 6 ships:

- File `needs-design` issue: "Evaluate installops Service layer
  follow-up to install-state-abstraction." Acceptance criteria:
  evaluate Candidate B's value given the post-Candidate-C codebase
  and any new cross-cutting concerns since this design.
- Trigger conditions for moving forward: 1-2 additional cross-cutting
  concerns have landed (e.g., `--dry-run`, audit log), or 3-4 months
  elapsed since Candidate C shipped. Whichever comes first.
- If triggered, the follow-up evaluates: should the semantic state
  methods on `Manager` migrate to an `installops.Service`? Does the
  full Candidate B sketch from Lead 6's exploration findings still
  apply? What's the new file-count cost projection given empirical
  cadence data?

## Security Considerations

The Candidate C work introduces no new external inputs, no new network
access, and no new dependencies. The new helpers (`WithSource`,
`SourceFromContext`) are pure functions on a Go value type. Filesystem
effects (notice files for libraries) are limited to the same
`$TSUKU_HOME/notices/` directory the bus already writes to, with
existing validation (`notices.WriteNotice` checks against path-separator
and `..` injection).

**`ctx.Value` key collision.** The `srcKey struct{}` is unexported and
empty. Two unexported empty structs in different packages are
different types; another package cannot accidentally collide with this
key. A malicious caller that imports `installevents` could set its own
value at `srcKey{}`, but that caller is already first-party code
inside the binary; there is no meaningful trust boundary to defend.

**Source value non-PII contract.** Existing constraint from the
upstream bus design: `Source` values are first-party identifiers
(`"manual"`, `"auto"`, `"project-auto"`) chosen by tsuku code. They
must remain non-PII and non-attacker-influenced strings. A code
comment on the `Source` enum already reinforces this; ctx-based
threading does not change the constraint, but adds a new way the
value could be set (via `ctx.WithValue` from an attacker who somehow
controls the ctx — not currently possible because ctx flows top-down
from CLI entry points only).

**Cancellation timing and state corruption.** The publish-after-state
invariant from the upstream bus design must be preserved. `ctx.Err()`
checks sit before state.json writes, not interleaved with them. The
following invariant holds: at any `ctx.Err()` check point, either
(a) the state write hasn't started yet and cancellation is safe, or
(b) the state write has fully completed and the publish closure will
fire with the post-write state. Tests verify both cases.

**No goroutine leaks via ctx.** Manager methods do not spawn
goroutines that outlive the call. The bus's subscriber dispatch is
synchronous (existing design). ctx is only used for cancellation
inside the current goroutine; no `go func() { use(ctx) }()` patterns.

**Library event scope.** The four new library event types
(`LibraryInstalled`, etc.) are first-party-only and subject to the
same subscriber-locality contract as existing events. A subscriber
handling a library event may only mutate the on-disk record for the
library named in the event.

**Notice file path collision.** Tool names and library names share
the `$TSUKU_HOME/notices/` directory. The `lib--` prefix on library
notice filenames is a structural defense against name collisions.
`notices.WriteNotice`'s existing path-traversal validation (must
match `^[a-zA-Z0-9_-]+$`-ish) prevents a malicious library name from
escaping the prefix.

**`Manager.GetState()` removal.** This is the most load-bearing API
change. Production code outside `cmd/tsuku/install_deps.go` does not
currently use the write path of `GetState()`. The removal applies to
CLI callers; internal Manager code continues to use
`m.state.UpdateTool` directly. A read-only narrower API
(`Manager.GetToolState`, `Manager.LoadState`) is preserved for
non-write access. If any third-party (out-of-tree) code depended on
`Manager.GetState()` for writes, this would be a breaking change —
but Manager lives in `internal/` and has no external contract.

## Consequences

### Positive

- **Cancellation now works.** SIGINT during install propagates through
  ctx from `globalCtx` to Manager. Ctrl-C during the atomic-rename
  window is no longer a silent hazard.
- **`Source` threading is essentially free for future read-only
  attribution concerns.** Trace IDs, correlation IDs, request IDs, and
  similar request-scoped metadata can attach to ctx without growing
  any function signature.
- **The `mgr.GetState().UpdateTool(...)` lambda smell is fixed.** Three
  blocks of ~17 lines each collapse to single-line method calls.
  Adding a new state field follows the named-method pattern instead
  of recreating the lambda.
- **Library installs become observable.** Notices and telemetry both
  cover libraries. A future audit-log subscriber automatically covers
  the full install pipeline.
- **`installWithDependencies` recursion is no longer a positional-arg
  growth point.** Future cross-cutting concerns that need to flow
  through the recursion add fields to `installArgs`, not positional
  parameters.
- **The lifecycle event bus design (Current) is preserved unchanged.**
  Bus signature, vocabulary, semantics, invariants all hold. Only the
  *source* of `Source` changes — from positional param to ctx
  extraction at publish callsites.
- **The Candidate B follow-up retains all options.** The semantic
  state methods on Manager migrate cleanly to `installops.Service` if
  B is later adopted; the ctx threading is shape-agnostic and
  composes with any Service-layer shape.

### Negative

- **API surface change touches ~15-20 files.** Comparable to PR
  #2412's actual cross-cutting cost (11 files / 120 LOC) plus
  semantic-method and library work. The PR is non-trivial. It is
  smaller than the alternative (Candidate B's ~5 PRs, ~600 lines new
  package, ~30 construction sites) but not invisible.
- **`ctx.Value` for non-cancellation data is a known Go-community
  ambivalence.** A reviewer unfamiliar with the request-scoped
  rationale could object on principle. Mitigation: explicit code
  comment on `WithSource` documents the rationale; the design doc
  records the override in its assumptions.
- **The publish-after-state invariant is now uncompiled in a second
  way.** A contributor who adds a `ctx.Err()` check between state
  write and publish would re-introduce the drift the bus design
  fixes. Mitigation: code comments at the relevant points; ordering
  tests in the Manager test suite.
- **`Manager.GetState()` removal is the load-bearing API change.**
  Read-only callers must migrate to narrower accessors
  (`GetToolState`, `LoadState`). One-time cost; bounded by
  `internal/` scope.
- **Library event vocabulary is four new types.** Subscribers grow
  type-switch arms. This is the trade-off the bus design accepted
  for compile-time exhaustiveness over string discriminators; this
  design honors it.
- **Candidate B is deferred, not eliminated.** If the 12-month
  forecast is wrong and cross-cutting cadence accelerates, the
  follow-up trigger conditions fire sooner and the team revisits B
  with empirical data — but the design accepts the up-front
  investment in B is delayed.
- **The notice file `lib--` prefix is a structural decision that
  cannot be undone trivially.** Once shipped, library notice files
  use the prefix; changing it later would require a migration of
  on-disk notice files. The naming was chosen to keep tool and
  library notices in the same directory while preventing collision;
  it's a small permanent cost.

### Mitigations

- **The 12-month forecast can be wrong in either direction.** If
  forecast is too high (Candidate B isn't needed), Candidate C is
  still the right work — it delivers cancellation and `Source`
  threading reduction independent of forecast. If forecast is too
  low (cadence accelerates), the follow-up charter fires sooner.
- **Cancellation correctness is testable.** Three explicit unit
  tests cover the cancel-before-write, cancel-after-write-before-
  publish, and cancel-during-dependency-walk scenarios. The
  publish-after-state invariant is verifiable per method.
- **The semantic state methods are obvious extension points.** If a
  future state field follows the same pattern (mutate state, attach
  attribution), it gets a new named method on Manager; the recursion
  in `install_deps.go` doesn't grow.
- **Documentation matches code.** The `WithSource` rationale, the
  publish-after-state invariant, and the subscriber-locality
  contract are documented in code comments at the relevant
  definitions, not only in this design doc. The next contributor
  encountering ctx-based attribution finds the rationale in the
  source.
- **Charter for Candidate B is concrete.** Trigger conditions,
  follow-up question, and acceptance criteria are stated above. The
  next person evaluating B has the empirical baseline (PR #2412's
  numbers, Candidate C's actual cost) and the forecast question
  framed.
