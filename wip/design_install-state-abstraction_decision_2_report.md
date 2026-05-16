<!-- decision:start id="install-state-abstraction-decision-2" status="assumed" -->
### Decision: Hide state.json semantic operations behind named methods

## Question

Should state.json semantic operations (`IsExplicit`, `RequiredBy`,
`InstallDependencies`, cleanup actions) be hidden behind named operations
rather than exposed via `mgr.GetState().UpdateTool(...)`?

## Context

Today, `cmd/tsuku/install_deps.go` writes state.json directly at three
sites (lines 223, 475, 584) using the pattern:

```go
mgr.GetState().UpdateTool(name, func(ts *install.ToolState) {
    if isExplicit { ts.IsExplicit = true }
    if parent != "" {
        // append to RequiredBy if not present
        ...
    }
    ts.InstallDependencies = ...
    // cleanup actions, etc.
})
```

`Manager` owns `state.json` internally via `m.state.UpdateTool` but
*also* exposes `m.GetState()` so callers do their own writes. Lead 6
(`wip/research/explore_install-state-abstraction_r1_lead-restructure-sketch.md`)
called this "the actual leaky abstraction" — `IsExplicit`, `RequiredBy`,
`InstallDependencies`, and cleanup actions get written from the CLI
layer rather than via semantic Manager methods. Two of the three lambda
blocks duplicate the same IsExplicit/RequiredBy logic (~17 lines each);
the third extends it with dependency-recording and cleanup-action
storage.

This decision is structurally coupled to Decision 1 (which picks between
Candidate A status-quo, Candidate B installops Service layer, or
Candidate C ctx-attribution). The right surface for the semantic methods
depends on whether a Service layer exists.

## Assumptions

- Decision 1 is being decided in parallel; this report must remain valid
  under any of its three outcomes.
- The three lambda call sites in `install_deps.go` are representative
  of the leak; no hidden callers outside `cmd/tsuku/` write state via
  `GetState().UpdateTool`. (Confirmed by grep: only `install_deps.go`
  uses the write pattern in production code.)
- Whatever surface exposes the semantic methods must be the *sole*
  write surface — leaving `GetState()` exposed alongside semantic
  methods preserves the leak.
- The semantic operations are stable enough to name. `MarkExplicit`,
  `RecordDependency`, and `RecordCleanup` map directly to the three
  observed write patterns; future state fields will follow the same
  shape (one method per semantic concept) rather than expanding an
  options struct.

## Options Considered

### (a) Status quo: keep `mgr.GetState()` exposed

Lambda pattern persists at the three call sites.

- Pro: zero migration cost.
- Pro: state writes are visible at the call site.
- Con: doesn't address the smell Lead 6 explicitly named. The threading
  problem Decision 1 is solving recurs *for state writes*: any new
  cross-cutting concern that wants to attach attribution or telemetry
  to a state write has to be added manually at each lambda site.
- Con: the IsExplicit/RequiredBy logic is already duplicated across
  three blocks. Adding a fourth state-write site repeats the pattern.

### (b) Add semantic methods on Manager

`Manager.MarkExplicit(name, parent string) error`,
`Manager.RecordDependency(name, dep string) error`,
`Manager.RecordCleanup(name string, actions []CleanupAction) error`.
`Manager.GetState()` becomes read-only or is removed entirely; the
lambda pattern disappears.

- Pro: hides state semantics without depending on Decision 1.
- Pro: smallest migration footprint (3 call sites → ~6 method calls).
- Pro: Manager remains the single owner of state.json.
- Con: Manager's API surface grows. Manager already mixes lifecycle
  verbs (`Install`, `Rollback`, `Remove*`) with state-fragment writes;
  this option doubles down on that mixing.
- Con: if Decision 1 picks B (Service layer), these methods would
  belong on Service, not Manager — the work has to move later.

### (c) Move semantic methods to the new Service layer

`installops.Service.MarkExplicit(...)`, etc.; `Manager.GetState()`
removed from CLI callers; Manager becomes a primitive layer with only
the low-level state mutations Service composes.

- Pro: clean separation. Manager = primitives, Service = semantics.
- Pro: state writes are auditable at the Service boundary
  (attribution, telemetry, future cross-cutting concerns attach
  uniformly).
- Pro: composes naturally with the publish-after-state invariant the
  bus depends on — semantic methods and event publishing share the
  same Service surface.
- Con: only viable if Decision 1 picks Candidate B.
- Con: bigger upfront cost (new package, migrate three sites *and*
  delegate to Manager primitives).

### (d) Standalone `StateOps` type

A new `install.StateOps` (or similar) wraps `*StateManager`, exposes
the semantic methods, and lives independent of any operation-level
restructure. Adoptable whether Decision 1 picks A, B, or C.

- Pro: addresses the smell without committing to the larger
  restructure.
- Pro: works whether Decision 1 picks A, B, or C.
- Con: introduces a third surface ("what's the difference between
  Manager's lifecycle ops, StateOps's semantic state methods, and
  StateManager's primitives?").
- Con: if Decision 1 lands on B later, `StateOps` either gets absorbed
  into Service (rework) or persists as redundant indirection. Either
  way, (d) is a transitional shape, not a destination.

## Decision Criteria

| Criterion | (a) Status quo | (b) Manager methods | (c) Service methods | (d) StateOps |
|-----------|---------------|---------------------|---------------------|--------------|
| Encapsulates state.json semantics | No (leak preserved) | Yes | Yes | Yes |
| Migration cost (call sites) | Zero | 3 sites, ~6 calls | 3 sites + new package | 3 sites + new type |
| API surface impact | None | Grows Manager (~3 methods) | Grows Service | New type, parallel to Manager |
| Works under Decision 1 = A | Yes | Yes | No | Yes |
| Works under Decision 1 = B | Yes (suboptimal) | Wrong location | Natural fit | Redundant with Service |
| Works under Decision 1 = C | Yes | Yes | No | Yes |
| Honest about the smell Lead 6 named | No | Yes | Yes | Yes |

## Chosen: Compound answer

**If Decision 1 picks Candidate B (installops Service layer): adopt (c).**
The semantic methods belong on `installops.Service` alongside the
lifecycle verbs. Manager becomes the primitive layer; `Service.MarkExplicit`,
`Service.RecordDependency`, and `Service.RecordCleanup` compose Manager's
existing `state.UpdateTool` primitive. `mgr.GetState()` is removed from
CLI callers entirely.

**If Decision 1 picks Candidate C (ctx-attribution) or Candidate A
(status quo): adopt (b).** Add `Manager.MarkExplicit`,
`Manager.RecordDependency`, and `Manager.RecordCleanup` as methods on
Manager itself. The lambda pattern at lines 223, 475, 584 disappears.
`Manager.GetState()` is restricted to read-only access (or removed
from CLI callers, exposed via a narrower `Manager.GetToolState` /
`Manager.LoadState` read API).

Either way, the answer is **not (a)**. Lead 6 named this as a real
structural smell. The IsExplicit/RequiredBy lambda is already
duplicated three times; treating that as fine requires positive
justification this design has not surfaced.

The answer is also **not (d)**. A standalone `StateOps` type is a
transitional shape — it works under any Decision 1 branch, but in
both the (b) world and the (c) world it would be the wrong destination.
Under Decision 1=A or C, (b) is simpler (one fewer type). Under
Decision 1=B, (c) is cleaner (one less indirection). Choosing (d)
hedges by accepting strictly more long-term cost than the
conditionally-best answer.

## Rationale

The decision criteria converge on a clear shape: the right level of
abstraction for these operations is semantic ("mark this tool as
explicit," "record that parent requires this tool"), not field-level
("update the IsExplicit boolean and append to the RequiredBy slice").
The current lambda pattern duplicates 17 lines of control flow across
two sites and extends it at a third — that's the symptom Lead 6
correctly diagnosed. Options (b), (c), and (d) all fix the symptom;
the question is which surface owns the methods.

The choice between (b) and (c) is structurally the same operation
located on different objects — the cost of moving the methods later
is mechanical (rename receiver, update call sites) but real. Choosing
based on Decision 1's outcome minimizes that cost: if a Service layer
exists, semantic state methods belong on it because Service is the
already-correct home for "operations that attach attribution and
publish events"; if no Service layer exists, putting the methods on
Manager keeps the surface count minimal.

Treating this as a compound decision is correct because:

1. The smell is real (settled by exploration; Lead 6's diagnosis is
   uncontested).
2. The fix is clearly named (the three lambda sites map directly to
   three named methods).
3. The *location* of the fix is the only contested axis, and that
   axis is fully determined by Decision 1.

Option (d) tries to avoid the conditionality by introducing a third
surface. That's worse on the API-surface criterion in every Decision 1
outcome, and it's worse on the migration-cost criterion in the long
run if Decision 1 lands on B.

## Coupling to Decision 1

This decision is **conditionally decided on Decision 1**. The
conditionality is explicit and resolves to a concrete answer under
each branch:

| Decision 1 outcome | This decision |
|--------------------|---------------|
| A (status quo) | (b) — methods on Manager |
| B (installops Service) | (c) — methods on Service |
| C (ctx-attribution) | (b) — methods on Manager |

The cross-validation phase should treat this as a soft pre-commitment.
If Decision 1 picks B, this report's recommendation collapses to (c)
without re-litigation. If Decision 1 picks A or C, it collapses to (b).

The compound answer is *not* "do both" or "wait and see." In every
Decision 1 outcome, there is a single correct surface; the conditionality
just selects which surface.

## Alternatives Rejected

- **(a) Status quo**: Rejected. Lead 6 explicitly named the
  `mgr.GetState().UpdateTool` pattern as the actual structural smell
  the design exists to address. The IsExplicit/RequiredBy logic is
  already duplicated across three sites; no future state field that
  follows the same pattern will be a one-liner. Treating this as a
  CLI-layer convenience requires positive justification the
  exploration did not surface.

- **(d) Standalone StateOps type**: Rejected. Introduces a third
  surface (Manager primitives + StateOps semantics + future Service
  lifecycle) that compounds confusion without paying off. Under
  Decision 1=A or C, it's strictly more API surface than (b). Under
  Decision 1=B, it's either redundant with Service or has to be
  absorbed into Service later. The "decouple from Decision 1" framing
  is real but not load-bearing — the compound answer in this report
  also decouples, by being explicit about the branch.

## Consequences

Under (b) (Decision 1 = A or C):

- Manager gains 3 named methods (`MarkExplicit`, `RecordDependency`,
  `RecordCleanup`). Manager's surface is already 5 lifecycle methods +
  several read methods; adding 3 brings it to ~8 mutating methods.
  This is within Go-community norms for a domain type.
- The three lambda blocks in `install_deps.go` collapse from ~17 lines
  each to ~3 lines (a single method call per concern).
- `Manager.GetState()` is removed from CLI callers; reads happen via
  narrower accessors (`Manager.GetToolState`, `Manager.LoadState`)
  that don't expose the write path.
- The semantic methods are obvious extension points for future
  cross-cutting concerns: attribution can be attached uniformly
  at the method boundary.

Under (c) (Decision 1 = B):

- `installops.Service` gains the same 3 named methods plus the
  lifecycle verbs. Manager is reduced to a primitive layer that owns
  `state.UpdateTool`, atomic-rename, symlink creation, and binary
  staging. `Manager.GetState()` is removed entirely from CLI callers.
- Service becomes the single auditable boundary for all state writes,
  not just lifecycle events — attribution, telemetry, dry-run, and
  future concerns attach at one place.
- The publish-after-state invariant the event bus depends on becomes
  easier to enforce: state semantics and event publication live on
  the same surface.

Under either branch:

- The IsExplicit/RequiredBy duplication across the three lambda blocks
  becomes a single implementation. Any future change to how parents
  are recorded (e.g., deduplicating elsewhere, attaching timestamps)
  happens in one place.
- Future state fields (the design forecast suggests one new orthogonal
  concern every 4-6 weeks) get added as named methods rather than as
  lambda mutations, keeping the smell from recurring.

<!-- decision:end -->
