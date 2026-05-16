# Lead: What does the API look like after a proposed restructure, and what's the migration path?

## Findings

This is the synthesis lead. The job is concrete: pick the candidates with the
clearest fit to today's call-site shape, sketch one operation's signature for
each, then evaluate whether any of them actually shrinks the install pipeline
or just moves the complexity. The primary input is the production code in
`internal/install/manager.go`, `internal/install/remove.go`, and the call
sites in `cmd/tsuku/install_deps.go`, `cmd/tsuku/remove.go`,
`cmd/tsuku/cmd_rollback.go`, `cmd/tsuku/install_lib.go`,
`internal/updates/apply.go`, and `internal/updates/self.go`.

### Current call-site facts (the baseline any sketch has to beat)

Manager-mutating call sites in production code (excluding tests, helper
internal calls, and `Activate` which is not Source-tagged):

- `cmd/tsuku/install_deps.go:551` — `mgr.InstallWithOptions(toolName, version, exec.WorkDir(), installOpts)`
- `cmd/tsuku/plan_install.go:107` — `mgr.InstallWithOptions(effectiveToolName, plan.Version, exec.WorkDir(), installOpts)`
- `cmd/tsuku/install_lib.go:149` — `mgr.InstallLibrary(libName, version, exec.WorkDir(), opts)` (libraries; out of scope but parallels)
- `cmd/tsuku/cmd_rollback.go:69` — `mgr.Rollback(toolName, ts.PreviousVersion, installevents.SourceManual)`
- `cmd/tsuku/remove.go:84` — `mgr.RemoveVersion(toolName, targetVersion, installevents.SourceManual)`
- `cmd/tsuku/remove.go:94` — `mgr.RemoveAllVersions(toolName, installevents.SourceManual)`
- `cmd/tsuku/remove.go:156` — `mgr.Remove(toolName, installevents.SourceManual)` (orphan cleanup; deprecated wrapper)

Five distinct call sites that pass `Source` today. Wrapper chain on the
install side: `runInstall` / `runInstallWithReporter` / `installWithDependencies`
all carry `src installevents.Source` as a trailing parameter, recursing into
themselves for dependencies and runtime dependencies (see
`install_deps.go:173/183/185/342/366`). The chain isn't deep — it's two
wrappers above `InstallWithOptions` — but it is recursive, and `installLibrary`
forks off it (line 303).

Construction sites: ~30 calls to `install.New(cfg)` across `cmd/tsuku/*` and
`internal/updates/*`. Only three pass `install.WithEventBus`:
`install_deps.go:195`, `cmd_rollback.go:37`, and (indirectly via the bus
parameter) `updates/checker.go` + `updates/apply.go`. The other ~27 call
sites are read-only (list, info, search, plan, doctor, etc.) and don't need
the bus.

The event bus is wired in **one** place: `cmd/tsuku/events_wiring.go`
(`newEventBus`). Three callers: `install_deps.go`, `cmd_rollback.go`,
`cmd_check_updates.go`. Plus the updates package which receives the bus as a
parameter.

`context.Context` is NOT currently threaded through Manager methods. None of
`Install`, `InstallWithOptions`, `Rollback`, `Remove`, `RemoveVersion`,
`RemoveAllVersions`, or `Activate` accept a `ctx`. The plan generator
(`getOrGeneratePlan` in `install_deps.go`) does take `ctx`, but it's
discarded by the time the call reaches `mgr.InstallWithOptions`.

### Candidate A: Status quo (do nothing)

**Signature**:

```go
func (m *Manager) InstallWithOptions(name, version, workDir string, opts InstallOptions) (err error)
```

`Source` lives on `opts.Source` for the options form and as a positional
arg on `Install`. The publish closure (`defer func() { m.publishInstallOutcome(...) }`)
sits inside the method. `state.json` writes happen via
`m.state.UpdateTool` calls embedded in the method body (line 221 et seq.).

**Adding a new cross-cutting concern (e.g., `--dry-run`)**: add
`opts.DryRun bool` to `InstallOptions`, branch inside `InstallWithOptions`
to skip the rename and the state update. Wrapper chain
(`runInstallWithReporter` -> `installWithDependencies`) needs to
propagate the new option (today done via `installOpts := install.DefaultInstallOptions()` and then setting fields), so the trailing-arg
problem from PR #2412 doesn't recur — `InstallOptions` absorbs it. For
operations that DON'T have an options struct (`Rollback`, `RemoveVersion`,
`RemoveAllVersions`, `Remove`), adding a third orthogonal flag triggers
the same churn PR #2412 hit. The wrappers (`runInstall`,
`runInstallWithReporter`, `installWithDependencies`) only thread Source
because Install has it; if Rollback or Remove gained a new orthogonal
flag, no recursive wrapper carries them today, so the blast radius is
much smaller — those call sites are leaf in `cmd/tsuku/`.

**state.json visibility**: hidden inside the method via
`m.state.UpdateTool`, but `mgr.GetState()` is exposed and used heavily in
`install_deps.go` (lines 223, 461, 475, 584, 644, 650). The state manager
is leaking out as a public surface today.

**Honest assessment of A**: The `installWithDependencies` recursion is the
real source of churn. Each recursive call site (lines 342, 366) needs the
new arg. If the recursion were extracted to a single entry-point with
attribution stored once at the top, the recursion-driven churn disappears
without restructuring the Manager.

### Candidate B: `internal/install/installops` layer above Manager

**Shape**: Manager keeps its low-level state and symlink primitives
(`Activate`, `IsVersionInstalled`, `GetState`, `createSymlinksForBinaries`,
the staging/atomic-rename machinery). A new `installops.Service` (or
`installops.Operator`, name TBD) wraps Manager and owns the lifecycle
verbs that publish events, attach attribution, run telemetry hooks.

**Sketch**:

```go
// internal/install/installops/service.go
package installops

type Service struct {
    mgr *install.Manager
    bus *installevents.Bus
}

type InstallRequest struct {
    Tool, Version, WorkDir string
    Binaries               []string
    RequestedVersion       string
    RuntimeDependencies    map[string]string
    Plan                   *install.Plan
    CreateSymlinks         bool
    IsHidden               bool
    Source                 installevents.Source
    // future cross-cutting concerns slot in here:
    DryRun                 bool
    Attribution            map[string]string // arbitrary tags
}

func New(mgr *install.Manager, bus *installevents.Bus) *Service {
    return &Service{mgr: mgr, bus: bus}
}

func (s *Service) Install(ctx context.Context, req InstallRequest) error {
    // pre-state snapshot, publish-after-state defer, then delegate to
    // mgr.InstallWithOptions with a stripped-down options struct that
    // no longer carries Source (Manager becomes attribution-free).
}

func (s *Service) Rollback(ctx context.Context, req RollbackRequest) error { ... }
func (s *Service) RemoveVersion(ctx context.Context, req RemoveRequest) error { ... }
func (s *Service) RemoveAll(ctx context.Context, req RemoveRequest) error { ... }
```

**Call site after migration**:

```go
ops := installops.New(mgr, bus)
err := ops.Install(ctx, installops.InstallRequest{
    Tool: toolName, Version: version, WorkDir: exec.WorkDir(),
    Binaries: binaries, RequestedVersion: versionConstraint,
    Plan: executor.ToStoragePlan(plan),
    RuntimeDependencies: runtimeDeps,
    CreateSymlinks: true,
    Source: src,
})
```

**Adding `--dry-run`**: one field on `InstallRequest`, one branch in the
Service. No wrapper-chain churn because the request struct absorbs the
field. The recursive wrapper chain in `cmd/tsuku/install_deps.go` becomes
a function that takes a single `*installops.Service` and a depth/parent
context — `Source` no longer needs to thread through every recursive
call, it's set once on the top-level request and the recursion carries a
single `req` value forward.

**state.json visibility**: hidden behind Service. `mgr.GetState()` calls
get replaced with `ops.GetTool(name)` / `ops.UpdateAttribution(name, ...)`.
This is the biggest restructure consequence: the ~10-15 `mgr.GetState().UpdateTool(...)`
call sites in `install_deps.go` (lines 223-243, 475-491, 584-612) become
either `ops.MarkExplicit(name, parent)` calls (semantic) or stay as
state-manager calls (which keeps state.json half-exposed).

**Migration path**: incremental. Phase 1: add the package, have it
delegate to existing Manager methods. Phase 2: migrate one caller at a
time (`install_deps.go` first as the highest-traffic). Phase 3: deprecate
Manager.Install / Rollback / Remove*; Manager keeps Activate and the
low-level primitives. Phase 4: collapse the deprecated wrappers. Roughly
3-5 PRs over ~2 weeks if pursued.

**Bus composability**: `bus.Publish` moves to Service. Manager loses the
bus field entirely. Publish-after-state invariant is maintained because
Service's defer reads the named return error from Manager. The invariant
becomes EASIER to maintain because the publish point isn't conflated with
the state-mutation point — Service can call multiple Manager methods in
one logical operation (e.g., InstallLibrary's `InstallWithOptions` + state
side-effects in install_deps.go) and publish ONE event for the composite
operation, which is closer to what `UpdateFailed` actually means.

**Honest cost**: ~600 lines of new package, ~200 lines of Manager logic
moving across the boundary, ~30 construction-site updates. The
`runInstall` / `runInstallWithReporter` / `installWithDependencies` chain
in `install_deps.go` shrinks meaningfully — `src` stops being threaded
because the recursion carries `req`. But the `mgr.GetState().UpdateTool`
calls at lines 223, 475, 584 are the actual structural pain in
`install_deps.go`, and Service only helps if those move behind a new
semantic API (`MarkExplicit`, `RecordDependency`, etc.). Without that,
Service is half a refactor.

### Candidate C: `context.Context`-attribution (lead 4's territory)

**Sketch**:

```go
type srcKey struct{}
func WithSource(ctx context.Context, src installevents.Source) context.Context {
    return context.WithValue(ctx, srcKey{}, src)
}
func sourceFromCtx(ctx context.Context) installevents.Source { ... }

func (m *Manager) InstallWithOptions(ctx context.Context, name, version, workDir string, opts InstallOptions) error {
    src := sourceFromCtx(ctx)
    // ... same body, defer publishes with src extracted from ctx
}
```

Call site:

```go
ctx := installevents.WithSource(globalCtx, src)
err := mgr.InstallWithOptions(ctx, toolName, version, exec.WorkDir(), installOpts)
```

**Adding `--dry-run`**: still requires plumbing — either as a context value
(stretches the "context is not config" guideline) or as a struct field
(same as A). Context-as-attribution helps Source specifically because
Source is set ONCE at the top of a call stack and never changes
mid-recursion. It does NOT help concerns that vary per-operation (DryRun
could vary by tool? probably not, but the abstraction doesn't enforce it).

**Migration path**: incremental. Each Manager method gets a ctx parameter
in a separate PR; callers default to `context.Background()` until they're
migrated. The mechanical work is ~10 method signatures + ~30 call sites.

**Honest assessment**: this is the cheapest restructure but it bakes in
an anti-pattern that the Go docs explicitly call out (context values for
non-request-scoped data). For Source specifically the case is defensible
("Source IS the request scope — it identifies what triggered the install
attempt"). For future concerns it's weaker. Cross-reference lead 4 for
the cost analysis.

### Candidate D: Aggressively extended functional options / OperationOptions

**Sketch**:

```go
type OperationOptions struct {
    Source              installevents.Source
    DryRun              bool
    AuditCorrelationID  string
    // ... every future concern adds a field here
}

func (m *Manager) Install(name, version, workDir string, op OperationOptions, install InstallOptions) error
func (m *Manager) Rollback(name, toVersion string, op OperationOptions) error
func (m *Manager) RemoveVersion(name, version string, op OperationOptions) error
```

This is essentially "promote Source to a shared struct, then add fields."
It's the path of least resistance from today's state. The downside is
that `OperationOptions` becomes a magnet for unrelated concerns and the
boundary between "cross-cutting" and "operation-specific" blurs (see
`InstallOptions` today, which already mixes both — `CreateSymlinks`
versus `Source`).

**Migration path**: instantaneous if done as a single coordinated commit
like PR #2412 Phase 3. Incremental migration is awkward because the
signature changes have to land everywhere at once.

### Candidate E: Middleware chain (Command pattern)

**Sketch**:

```go
type Command interface { Execute(ctx context.Context) error }
type InstallCmd struct { Tool, Version, WorkDir string; ... }
type Middleware func(Command) Command

// Pipeline: attribution -> dry-run gate -> telemetry -> bus.Publish -> Manager
pipeline := Chain(
    WithAttribution(src),
    WithDryRun(dryRun),
    WithEventPublishing(bus),
    Final(mgr.Install),
)
```

**Honest assessment**: this is over-engineered for the current code shape.
tsuku has 5 lifecycle verbs and ~8 production call sites. Middleware
pays off when you have N independent concerns and M operations and the
N*M combinations would otherwise be hand-written. Here N=2-3 and M=5.
The indirection cost dominates the benefit. Skip.

### Migration path for the most promising shape (B + thin caller refactor)

For Candidate B specifically, the incremental migration looks like:

1. **PR 1** — Add `internal/install/installops` package. Service.Install
   delegates to Manager.InstallWithOptions. No callers migrated yet. ~600
   lines new code; passes existing tests.

2. **PR 2** — Migrate `install_deps.go` to use Service. Collapse
   `runInstall` / `runInstallWithReporter` / `installWithDependencies`
   src threading: `src` lives on `InstallRequest`, the recursive call
   builds a new request rather than threading args. Net diff: probably
   neutral on line count, but the recursion contract is cleaner.

3. **PR 3** — Migrate `cmd_rollback.go`, `remove.go`, `install_lib.go`,
   and `updates/apply.go`. Each is a leaf, ~10-line change.

4. **PR 4** — Move `state.json` semantic operations (`MarkExplicit`,
   `RecordDependency`, `UpdateCleanupActions`) from `install_deps.go`
   onto Service. This is where the structural win happens — the
   `mgr.GetState().UpdateTool(name, func(ts *install.ToolState) { ... })`
   pattern in `install_deps.go` lines 223, 475, 584 becomes semantic
   method calls. Saves maybe 30 lines per call site.

5. **PR 5** — Deprecate Manager.Install / Rollback / Remove* (keep as
   thin wrappers calling Service for backward compat in tests). Manager
   becomes the low-level primitive layer.

The migration CAN be incremental. Each PR leaves the codebase in a
working state. The only commit that must be coordinated is PR 5 (the
deprecation), and even that can be a soft deprecation that flags use in
docs without forcing a hard cutover.

### Before/after for `cmd/tsuku/install_deps.go`

The current chain (lines 167-184) is:

```go
func runInstall(toolName, reqVersion, versionConstraint string, isExplicit bool, parent string, client *telemetry.Client, src installevents.Source) error {
    reporter := progress.NewTTYReporter(os.Stderr)
    defer func() { reporter.Stop(); reporter.FlushDeferred() }()
    return installWithDependencies(toolName, reqVersion, versionConstraint, isExplicit, parent, make(map[string]bool), client, reporter, src)
}

func runInstallWithReporter(toolName, reqVersion, versionConstraint string, isExplicit bool, parent string, client *telemetry.Client, reporter progress.Reporter, src installevents.Source) error {
    return installWithDependencies(toolName, reqVersion, versionConstraint, isExplicit, parent, make(map[string]bool), client, reporter, src)
}

func installWithDependencies(toolName, reqVersion, versionConstraint string, isExplicit bool, parent string, visited map[string]bool, telemetryClient *telemetry.Client, reporter progress.Reporter, src installevents.Source) error {
    // ...
    if err := mgr.InstallWithOptions(toolName, version, exec.WorkDir(), installOpts); err != nil { ... }
    // ...
    // recursive:
    if err := installWithDependencies(dep, "", "", false, toolName, visited, telemetryClient, reporter, src); err != nil { ... }
}
```

After Candidate B (sketch — not validated against full file):

```go
type installArgs struct {
    Tool, ReqVersion, VersionConstraint string
    IsExplicit                          bool
    Parent                              string
    Source                              installevents.Source
    Client                              *telemetry.Client
    Reporter                            progress.Reporter
}

func runInstall(args installArgs) error {
    if args.Reporter == nil {
        r := progress.NewTTYReporter(os.Stderr)
        defer func() { r.Stop(); r.FlushDeferred() }()
        args.Reporter = r
    }
    return installWithDependencies(args, make(map[string]bool))
}

func installWithDependencies(args installArgs, visited map[string]bool) error {
    // ... build req from args
    err := ops.Install(ctx, installops.InstallRequest{
        Tool: args.Tool, Version: version, WorkDir: exec.WorkDir(),
        Binaries: binaries, RequestedVersion: args.VersionConstraint,
        Plan: executor.ToStoragePlan(plan),
        RuntimeDependencies: runtimeDeps,
        Source: args.Source,
        IsExplicit: args.IsExplicit, Parent: args.Parent, // moves out of mgr.GetState().UpdateTool
    })
    // recursive: copy args, set Parent and reset IsExplicit
    sub := args; sub.Tool = dep; sub.IsExplicit = false; sub.Parent = args.Tool; sub.ReqVersion = ""; sub.VersionConstraint = ""
    if err := installWithDependencies(sub, visited); err != nil { ... }
}
```

The net win is real but small: trailing-arg threading collapses into a
struct field. The recursive call constructs a sub-args by copying. The
ugly `mgr.GetState().UpdateTool(name, func(ts *install.ToolState) { ... })`
blocks at lines 223, 475, 584 (~75 lines total) become a single
`ops.Install` call that carries IsExplicit/Parent in the request — saves
maybe 50 lines net. NOT a dramatic shrinkage. Mostly the win is
"adding the next cross-cutting concern doesn't churn this file again."

### Bus composability

For Candidate B, `bus.Publish` lives in `installops.Service` exclusively.
Manager loses the bus field and the `publishInstallOutcome` /
`publishRemoveOutcome` helpers. The publish-after-state invariant moves
to Service: a defer at the top of `Service.Install` reads the named
return error from `mgr.InstallWithOptions` and publishes after the call
returns. State writes happen synchronously inside Manager.InstallWithOptions
and complete before the deferred publish fires.

The invariant becomes STRONGER, not weaker, because Service can
distinguish "Manager.InstallWithOptions returned err" from "Service-level
operation failed before/after Manager was called" — today both surface
as InstallFailed.

For Candidate C (context-attribution), bus.Publish stays in Manager.
Only `Source` is extracted from ctx. The invariant doesn't change.

For Candidate D, bus.Publish stays in Manager. No invariant change.

### Status-quo verdict

There is a defensible status-quo case. Evidence:

1. PR #2412's blast radius decomposes (per lead 5) into Source-threading
   (modest) and event-bus-plus-subscribers (most of the file count). The
   Source threading alone wasn't catastrophic.
2. The wrapper recursion in `install_deps.go` is two layers, not five.
   It's annoying but not pathological.
3. The Manager surface for state-mutating ops is 5 methods total. Small
   enough that a future cross-cutting concern hits a tractable number of
   call sites.
4. `state.json` is the only "data store" here. There is no second backend
   to abstract over, so the repository pattern's main motivation (swap
   storage) doesn't apply.

The case AGAINST status quo: every new cross-cutting concern on
non-options-form methods (Rollback, RemoveVersion, RemoveAllVersions)
recurs the PR #2412 pattern. If the team expects 2-3 such concerns in
the next 12 months (lead 2 should estimate), the cumulative cost
exceeds the one-time restructuring cost.

## Implications

**Recommended shape: Candidate B (installops layer), but only if the
team expects to add 2+ new cross-cutting concerns in the next 12 months.**

If the realistic forecast is "just dry-run, maybe context.Context for
cancellation, nothing else," Candidate C (context-attribution) is
cheaper and gets us 80% of the way: ctx threading is needed anyway for
cancellation, Source rides on it for free, future flags go on a struct.

The design phase needs to decide:

1. **Forecast**: how many new cross-cutting concerns in the next 12
   months? Lead 2's inventory of plausible additions is the input;
   the decision is whether to commit to a layer now or wait and see.

2. **`state.json` exposure**: is `mgr.GetState().UpdateTool(name, func(ts *install.ToolState) { ... })` a real smell, or is it the right level
   of abstraction? If it's a smell, Candidate B is justified by the
   structural fix even without forecast pressure. If it's fine, Candidate
   C is enough.

3. **Library install integration**: lead 6's sketches deliberately
   omit `InstallLibrary`. If Service is adopted, libraries should
   live there too — but the issue scope says "out of scope as a
   deliverable." The design needs to decide whether to design for
   libraries-included or libraries-bolt-on-later.

4. **Backwards compatibility**: Manager is in `internal/`, so there's
   no external API contract. The deprecation path is purely internal.
   This is actually a strong argument for restructuring: the cost of
   making the wrong choice is bounded.

## Surprises

1. **The recursion in `installWithDependencies` is the real source of
   Source-threading churn, not Manager's signature.** The Manager method
   gained ONE parameter. The wrappers gained one parameter each, but
   they pass it recursively, so a single recursive call site that
   forgot to forward `src` would silently degrade to the zero value
   (`SourceManual`). The structural fix is collapsing the recursion into
   a request struct that carries attribution as a field — that
   alone removes most of the trailing-arg problem without introducing
   `installops`. This is half a refactor of Candidate B's win, available
   in isolation.

2. **`state.json` is the actual leaky abstraction, not Manager.** The
   `mgr.GetState().UpdateTool(name, func(ts *install.ToolState) { ... })`
   pattern appears in `install_deps.go` at lines 223, 475, 584 (and
   elsewhere). Manager owns state via `m.state.UpdateTool`, but it
   ALSO exposes `m.GetState()` so callers can do their own UpdateTool
   calls. This is the abstraction-leak that lets `IsExplicit`,
   `RequiredBy`, `InstallDependencies`, and cleanup actions get written
   from the CLI layer rather than via semantic Manager methods. Any
   restructure that doesn't fix this leaves the cross-cutting threading
   problem half-solved.

3. **Manager doesn't accept context.Context anywhere.** This is more
   surprising than expected given how much of the rest of the codebase
   threads ctx (the executor, plan generator, version resolver, and
   telemetry client all take ctx). Adding ctx alone — independent of
   any restructure — would be a useful 30-call-site PR that costs ~1
   day and unlocks cancellation. Candidate C (context-attribution)
   piggy-backs on this work.

4. **The bus is wired exactly once, in `events_wiring.go`.** Three
   callers reach in, plus the updates package which carries it as a
   parameter. This is already well-factored — the restructure
   shouldn't fragment this; Service should accept the bus via its
   constructor, the same way Manager does today.

## Open Questions

1. Does the team expect 2+ new cross-cutting concerns in the next 12
   months? (Lead 2's inventory is the input; the decision is
   product/maintenance-strategy, not technical.)

2. Should `state.json` semantic operations
   (`MarkExplicit`/`RecordDependency`/etc.) be on Service, on a new
   `StateOps` type, or stay as anonymous-function closures on
   StateManager.UpdateTool? Option 3 is status quo; option 1 is full
   Candidate B; option 2 is a middle ground that fixes the leak
   without introducing a service layer.

3. Does library install share the abstraction? The issue says
   libraries are explicitly out of scope as a deliverable but in scope
   as "would this apply to libraries too." If Service is adopted,
   libraries should live there, but the design might choose to defer.

4. Is the recursion-into-request-struct refactor (the "half of
   Candidate B" insight) worth doing as a standalone PR even without
   the rest of the restructure? It addresses the trailing-arg threading
   problem at lower cost.

5. Does Candidate C's `context.WithValue(ctx, srcKey{}, src)` actually
   work in Go-idiomatic terms, or does it cross the
   "context-is-not-config" line? Lead 4 should answer; if the answer is
   "fine for Source specifically, not fine for generic cross-cutting,"
   that bounds the candidate's usefulness.

## Summary

Candidate B (a new `internal/install/installops` layer that wraps
Manager, owns lifecycle events, and exposes semantic state operations)
is the most promising restructure, but the recommendation is conditional:
adopt it if the team expects 2+ new cross-cutting concerns in the next
12 months, otherwise prefer Candidate C (context-attribution) plus a
standalone refactor that collapses the `installWithDependencies`
trailing-arg recursion into a request struct. The main implication is
that the real structural smell isn't Manager's signature — it's the
`mgr.GetState().UpdateTool(name, func(ts) { ... })` pattern leaking
state semantics into the CLI layer — and any restructure that doesn't
address that leaves the cross-cutting threading problem half-solved.
The biggest open question is whether the realistic 12-month forecast
of new cross-cutting concerns justifies the layer cost, which is a
product/maintenance decision the design phase needs to resolve before
picking between B and C.
