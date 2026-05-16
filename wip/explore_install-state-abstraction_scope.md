# Explore Scope: install-state-abstraction

## Visibility

Public

## Core Question

Would consolidating install state operations behind a single abstraction (a
repository-style layer, or whatever idiom fits Go best) simplify the install
pipeline and reduce the blast radius of cross-cutting changes? Issue #2413
arose from PR #2412, where threading the new `Source` attribution through the
install pipeline drove the file count higher than expected. The investigation
should determine whether the cost is structural (and worth restructuring) or
local (and not worth restructuring), and what shape any restructure would
take if pursued.

## Context

The motivating PR (#2412) introduced an in-process install lifecycle event bus
(`internal/installevents`) with `internal/notices` and `internal/telemetry`
as subscribers. The accompanying design document (now at
`docs/designs/current/DESIGN-notices-install-event-bus.md`, status Current)
made four cross-validated decisions, including:

- Verb-per-event vocabulary with `Source` (manual / auto / project-auto) as
  an orthogonal tag.
- Publisher placement: explicit `bus.Publish(...)` calls inside
  `Manager.Install`, `Manager.Rollback`, `Manager.RemoveVersion`,
  `Manager.RemoveAllVersions`, and `updates.CheckAndApplySelf`.
- `Source` threading: method parameter on the public Manager lifecycle
  methods plus `InstallOptions.Source` for the options form.

The design's Decision 3 explicitly rejected two alternatives:

1. A state.json shim that auto-publishes from `state.UpdateTool` — rejected
   because `UpdateTool` is called for at least four semantically distinct
   reasons (install, activate, version-remove, dependency bookkeeping) and
   the rollback path's `Activate(prior)` would emit an event indistinguishable
   from a normal user activation.
2. Defer-based instrumentation (`defer bus.Publish(buildEvent(err))`) —
   rejected because rollback in `apply.go` happened outside the function
   that would carry the defer.

The design predicted "Adding Source as a parameter on the public Manager
methods touches ~10 call sites. Manageable; one-time." Implementation
appears to have exceeded that prediction: the issue body notes wrappers
between the CLI and the Manager (`runInstall`, `runInstallWithReporter`,
`installWithDependencies`, `installLibrary`) each needed to learn the
new argument.

Today's state-mutation reality (snapshotted before exploration):

- `state.UpdateTool`, `state.UpdateToolWithoutLock`, `state.RemoveTool`,
  `state.Save` in `internal/install/state*.go` are the only paths to
  `state.json`.
- `install.Manager`'s public lifecycle methods now take `Source`:
  `Install`, `InstallWithOptions`, `Rollback`, `RemoveVersion`,
  `RemoveAllVersions`, `Remove` (deprecated wrapper).
- Manager call sites in non-test production code: ~9 files across
  `cmd/tsuku/`, `internal/autoinstall/`, `internal/index/`.

The issue's hypothesis: a layer that owns the full lifecycle of an installed
tool (state, symlinks, events, telemetry) might make cross-cutting
threading local. The hypothesis is not pre-committed; the investigator
is free to conclude this isn't worth doing or to propose a different shape.

## In Scope

- Mapping the current install state operation surface: every public method
  that mutates installed-tool state, plus every caller in production code.
- Inventory of cross-cutting concerns that thread through these operations
  today (`Source`, lifecycle events, telemetry hooks) and ones plausible
  in the next 12 months (dry-run, audit/observability, rate-limiting,
  per-operation hooks).
- Go-idiomatic patterns for the proposed shape: repository pattern,
  service layer with embedded options, command pattern, middleware /
  interceptor chain, context-based attribution threading. Evidence-based
  evaluation against the actual call-site shape, not a generic survey.
- A "what does adding a new orthogonal concern look like" cost model,
  both for the status quo and for any proposed shape. Use a concrete
  hypothetical (e.g., `--dry-run`) to ground the comparison.
- The trade-offs the design phase will need to weigh: indirection cost,
  testability, migration cost, runtime cost (none expected), and the
  consequences for the just-shipped event bus design.
- Library install state (`State.Libs`) — explicitly only as the question
  "would this abstraction apply to libraries too, or are they fundamentally
  different?" The detailed evaluation of library refactoring stays out of
  scope per the issue.

## Out of Scope

- The lifecycle event bus itself. PR #2412's design is shipped and Current;
  any abstraction must compose with it, not replace it.
- Library-install refactoring as a deliverable.
- Strategic-level questions about whether tsuku should refactor at this
  point in its lifecycle (tactical scope).
- Detailed migration sequencing — that's a `/plan` concern after `/design`
  accepts the shape.
- Reworking the `state.json` schema or the on-disk format.

## Research Leads

1. **What is the actual current surface of install state operations, and which call sites does it touch?**
   Map every public method on `install.Manager` that mutates installed-tool
   state, including indirect mutations via `Activate`, version pinning, and
   `state.UpdateTool` direct callers. List every wrapper between the CLI
   layer and `Manager` for the install/update/rollback/remove pipelines
   (e.g., `runInstall`, `runInstallWithReporter`, `installWithDependencies`,
   `installLibrary`). Establish the baseline: how many entry points,
   how deep is the wrapper chain, what state does each layer add.

2. **What cross-cutting concerns thread through these operations today, and what's likely in the next 12 months?**
   The issue specifically calls out `Source`. Survey other orthogonal
   concerns: progress reporters (already plumbed via `SetReporter`),
   `context.Context` (currently not threaded through Manager), workDir,
   dependency awareness, telemetry. For each, capture how it's plumbed
   today. Then identify plausible near-term additions: a `--dry-run`
   flag, audit logging beyond telemetry, per-operation timeouts,
   rate-limiting on auto-apply, structured cancellation. For each
   plausible concern, estimate the call-site cost under the current shape.

3. **Which Go-idiomatic patterns plausibly fit the current call-site shape, and how do they compare?**
   The issue mentions "DAO/Repository from Java." Survey Go patterns that
   address the same root problem (operations spread thin, orthogonal
   threading expensive): repository (interface + concrete), service layer
   with options-struct-per-call, command pattern with handler middleware,
   functional-options builder, context-attribution + functional facade,
   decorator chain (closer to middleware). Reference Go projects that
   solve similar shape problems in their codebases (e.g., k8s controller
   pattern, cobra command middleware, popular open-source CLI tools).
   For each pattern, evaluate fit to tsuku's current `Manager` shape,
   note the indirection cost, and the cost of adding a future concern.

4. **Would `context.Context`-based attribution threading actually work, and what's its real cost?**
   The DESIGN-notices-install-event-bus document explicitly rejected
   defer-based instrumentation but did not deeply evaluate
   `context.Context.WithValue` for attribution. Investigate: (a) does the
   Manager currently accept a `context.Context` on its public methods?
   (b) what's the call-site cost of threading one through if it does not?
   (c) what's the cost of pulling a `Source` from the context inside
   publish callsites? (d) what are Go-community norms about
   `context.WithValue` for non-cancellation attribution data — is this an
   anti-pattern, an accepted compromise, or actively recommended? Be
   honest about the trade-off (typed key safety, context propagation
   discipline, easier mocking vs. value type-erasure and the
   "context-is-not-config" guidance from the Go docs).

5. **What's the maintenance cost of the current shape under realistic future scenarios?**
   Build the cost model. For three plausible scenarios — adding a
   `--dry-run` flag, adding per-operation audit logging beyond the bus,
   adding `context.Context` for cancellation/timeout — estimate: how many
   files touched, how much call-site churn, how much risk of a wrapper
   forgetting the concern. Compare against the status quo's known cost
   (PR #2412 file count). Don't extrapolate from one data point if the
   data point isn't representative; PR #2412 included unrelated work
   (event bus + two new subscribers + Notice schema change). Decompose
   PR #2412's churn into "Source-threading" vs "event-bus" vs "other"
   to get a clean baseline.

6. **What does the API look like after a proposed restructure, and what concretely is the migration path?**
   For the most promising shape(s) from leads 3-4, sketch the post-
   restructure API surface: what methods, what arguments, where
   `Source` lives, how subscribers attach, how `Manager` interacts
   with the new layer (delegate, replace, sit-behind). Then sketch
   the migration: can the change be incremental (add the new layer,
   migrate callers one by one) or does it require a single coordinated
   commit? What does each call site look like before vs. after?
   This lead is the most speculative; the goal is producing concrete
   enough artifacts that the design phase has something to react to.
   If no shape emerges that's clearly better than the status quo,
   that's a valid finding — flag it.
