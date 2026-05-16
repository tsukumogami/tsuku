---
status: Accepted
problem: |
  Notice files under $TSUKU_HOME/notices/ drift from state.json because every
  install-state mutation writes them directly (background auto-apply, manual
  update, self-update, rollback). PR #2411's render-time filter catches one
  drift class but leaves the store stuffed with stale entries. Separately,
  telemetry emission in internal/updates/apply.go is wired by direct calls to
  tc.SendUpdateOutcome alongside the notice writes, so any future code path
  that mutates install state has to remember both — a duplication that has
  already caused inconsistencies (self-update success has a notice but no
  telemetry; project-auto installs emit telemetry but no notice).
decision: |
  Introduce an in-process install lifecycle event bus (internal/installevents)
  with a typed verb-per-event vocabulary: Installed, Updated, RolledBack,
  Removed for the four lifecycle verbs the user invokes, plus InstallFailed,
  UpdateFailed, RollbackFailed, RemoveFailed for their failure counterparts.
  A Source enum (SourceManual, SourceAuto, SourceProjectAuto) is orthogonal
  to the verb and tags every event. install.Manager owns its own
  failure-recovery (the auto-rollback inside a failed update happens silently
  inside Manager.Install), so failure events carry an ActiveAfter field that
  describes the post-recovery state and one operation always produces exactly
  one event. internal/notices and internal/telemetry both become subscribers
  in this PR; direct WriteNotice and tc.SendUpdateOutcome calls in apply.go
  and update.go are removed. The renderer becomes a dumb consumer.
rationale: |
  Verb-per-event matches both subscribers' needs at compile time: the notices
  subscriber writes per-verb user-facing messages, and the telemetry
  subscriber maps each event type to a distinct outcome bucket without
  inferring intent from field shape. Source-as-orthogonal-tag keeps the trigger
  axis independent of the verb axis. Folding auto-rollback into UpdateFailed's
  ActiveAfter rather than emitting a separate RolledBack event preserves
  "one operation = one event" — explicit tsuku rollback stays RolledBack;
  automatic recovery within an update flow stays internal. In-process pub/sub
  remains sufficient because each tsuku process is short-lived and the
  filesystem is the inter-process synchronization point. Synchronous,
  best-effort delivery matches the existing notice semantics and avoids
  introducing goroutine lifecycle or shutdown questions.
---

# DESIGN: Notices Install Event Bus

## Status

Accepted

## Context and Problem Statement

`internal/notices/` persists a JSON file per tool at
`$TSUKU_HOME/notices/<tool>.json` describing the outcome of an update attempt
(`AttemptedVersion`, `Error`, `Shown`, `Kind`, …). The store has two writers
and one reader:

| Caller | File | Purpose |
|--------|------|---------|
| Background auto-apply | `internal/updates/apply.go:148-158` | Writes a `Shown=false` success notice on update success; failure notice on failure |
| Manual update | `cmd/tsuku/update.go:200-209, 384-394` | Removes any prior notice, writes a success notice |
| Self-update | `internal/updates/self.go` | Writes a notice for tsuku's own version |
| Renderer | `internal/updates/notify.go:71-111` | Reads `Shown=false` notices, prints, marks `Shown=true` |

Because nothing keeps the store synchronized with `state.json`, drift produces
user-visible lies. The reported case: `tsuku update niwa` printed "niwa has
been updated to 0.10.4" because a prior background auto-apply had written that
notice; the user (or a subsequent run) had since moved past 0.10.4 to 0.11.0,
and the current run was about to install 0.11.1.

The renderer runs in `PersistentPreRun` (`cmd/tsuku/main.go:78`), so the stale
banner prints before the current command body could overwrite the notice.
A previous attempt (PR #2411) added a staleness check at the render site that
compared each notice's `AttemptedVersion` against the installed `ActiveVersion`
and suppressed mismatches. That fix kept the store full of stale entries (the
filter is reactive) and only handled one drift cause. Every additional class
of drift — a notice for a tool the user just removed, a notice that contradicts
a rollback, a notice from a cancelled install — would need its own filter
clause at the read site.

The right fix is to keep the store consistent with reality at the point
reality changes. That requires every mutation path to signal what happened,
and `internal/notices` to react.

## Decision Drivers

- **Single contract for "what is true"**: one event vocabulary captures every
  way the install state can change, instead of N×M write-sites × notice
  shapes.
- **Uniform behavior across triggers**: a tool installed manually via
  `tsuku update foo` and a tool installed by background auto-apply must
  leave the notice store in identical shape. Subscribers cannot depend on
  who published.
- **In-process only**: every tsuku invocation is a fresh process. The bus
  exists inside a single process; the filesystem is the only cross-process
  channel. No daemon, no shared memory, no socket.
- **Best-effort, synchronous delivery**: a subscriber error must not block
  install success and must not panic the host process. Delivery is
  synchronous so the foreground command's exit reflects a settled store.
- **Auditable subscriber wiring**: a reader should be able to point to one
  file and answer "what runs when an install succeeds?"
- **No migration burden**: users (currently one) will manually clean stale
  notice files after release. The design does not include a startup
  reconciliation pass.
- **Bounded blast radius**: existing call sites that today write notices
  directly should switch over in a small, ordered set of changes; the
  renderer should not change behavior except inasmuch as it now reads a
  consistent store.

## Considered Options

The design decomposes into four independent decisions, each evaluated separately
and then cross-checked for compatibility.

### Decision 1: Event vocabulary, payload shape, self-update treatment

**Context.** Drives every publisher and every subscriber. Two subscribers are
in scope for this PR: `internal/notices` (which writes user-facing notice
files) and `internal/telemetry` (which emits outcome events to the
telemetry endpoint). They have different needs:

- **Notices** writes one notice file per tool with a verb-specific message
  ("X has been installed", "X has been updated to Y", "X was rolled back to
  Z"). The verb matters for the user-facing message.
- **Telemetry** today emits distinct outcome events per verb
  (`UpdateOutcomeSuccess`, `UpdateOutcomeFailure`, `UpdateOutcomeRollback`)
  plus install and remove buckets. Each event type maps to a distinct
  funnel.

A vocabulary built on "state changed" alone (e.g., a single `Activated`
event that fires for any `ActiveVersion` transition) forces both
subscribers to infer the verb from field shape, which is awkward for
notices and brittle for telemetry. The right shape is **verb-per-event**:
each lifecycle verb the user invokes gets its own type, with `Source` as
an orthogonal tag.

Key assumptions:
- Bus delivery is synchronous-with-recover (Decision 2 outcome).
- Publishers are explicit `Publish()` calls at lifecycle methods
  (Decision 3 outcome).
- `Source` is a closed enum extended only via code change.
- `InboxReporter` keeps writing its warning-kind notices directly; those
  are side-channel observations during an install run, not lifecycle
  events.
- `install.Manager.Install` owns its own failure recovery: on failure of
  an update, it activates the prior version internally before publishing.
  This makes failure events carry post-recovery state.

#### Chosen: Verb-per-event vocabulary (8 typed events) with Source as orthogonal tag

```go
type Source string

const (
    SourceManual      Source = "manual"        // user-invoked CLI command
    SourceAuto        Source = "auto"          // background auto-apply
    SourceProjectAuto Source = "project-auto"  // .tsuku.toml auto-approval
)

// Success events — one per verb.

type Installed struct {
    Tool      string
    Version   string
    Source    Source
    Timestamp time.Time
}

type Updated struct {
    Tool        string
    FromVersion string
    ToVersion   string
    Source      Source
    Timestamp   time.Time
}

type RolledBack struct {
    Tool        string
    FromVersion string // version we rolled away from
    ToVersion   string // version we rolled to
    Source      Source
    Timestamp   time.Time
}

type Removed struct {
    Tool        string
    Version     string // specific version removed; "" if all versions removed
    ActiveAfter string // new active version; "" if tool fully gone
    Source      Source
    Timestamp   time.Time
}

// Failure events — one per verb. ActiveAfter on UpdateFailed describes the
// state after any automatic recovery; on others it's omitted because the
// state didn't change.

type InstallFailed struct {
    Tool             string
    AttemptedVersion string
    Err              error
    Source           Source
    Timestamp        time.Time
}

type UpdateFailed struct {
    Tool             string
    AttemptedVersion string
    FromVersion      string // version active before the attempt
    ActiveAfter      string // active version after attempt + any auto-recovery;
                            //   == FromVersion if rollback succeeded
                            //   == "" if no prior version existed
                            //   == AttemptedVersion if rollback also failed
    Err              error
    Source           Source
    Timestamp        time.Time
}

type RollbackFailed struct {
    Tool             string
    AttemptedVersion string // version we tried to roll to
    FromVersion      string // version that was active before the attempt
    Err              error
    Source           Source
    Timestamp        time.Time
}

type RemoveFailed struct {
    Tool             string
    AttemptedVersion string
    Err              error
    Source           Source
    Timestamp        time.Time
}
```

**Self-update.** `tsuku self-update` (manual) and the background
self-update check (auto) emit `Updated{Tool: "tsuku", Source: SourceManual|SourceAuto}`
or `UpdateFailed{Tool: "tsuku", ...}`. There is no separate `SelfUpdated`
event; the special Tool value identifies self-update, and the renderer
already keys off `Tool == SelfToolName` for self-update phrasing. Failure
visibility is new behavior — today self-update failures are silent.

**Publish contract.**

- `Manager.Install(tool, version, source)`:
  - If no prior `ActiveVersion`: success emits `Installed`; failure emits
    `InstallFailed`.
  - If prior `ActiveVersion` exists: this is an update under the hood.
    Success emits `Updated{FromVersion: prior, ToVersion: new}`; failure
    triggers an internal `Manager.activate(prior)` recovery, then emits
    `UpdateFailed{FromVersion: prior, ActiveAfter: prior_or_attempted_if_recovery_also_failed}`.
- `Manager.Rollback(tool, toVersion, source)`: success emits `RolledBack`;
  failure emits `RollbackFailed`.
- `Manager.RemoveVersion(tool, version, source)` /
  `Manager.RemoveAllVersions(tool, source)`: success emits `Removed`;
  failure emits `RemoveFailed`.
- `updates.CheckAndApplySelf(bus, source)`: success emits
  `Updated{Tool: "tsuku"}`; failure emits `UpdateFailed{Tool: "tsuku"}`.

**Internal `Manager.activate(tool, version)`** is the unexported helper
used by `Install` for failure recovery and by Rollback for the actual
symlink mutation. It never publishes; the public method that called it
decides whether and which event to emit.

**Notices subscriber reaction table** (the Notice schema gains a `Verb`
field — `install` | `update` | `rollback` | `remove` — so the renderer
formats verb-specific messages):

| Event | Notice mutation |
|---|---|
| `Installed` | `WriteNotice{Verb: install, AttemptedVersion: Version, Error: "", Shown: false}` |
| `Updated` | `WriteNotice{Verb: update, AttemptedVersion: ToVersion, Error: "", Shown: false}` |
| `RolledBack` | `WriteNotice{Verb: rollback, AttemptedVersion: ToVersion, Error: "", Shown: false}` |
| `Removed` | `RemoveNotice(Tool)` |
| `InstallFailed` | `WriteNotice{Verb: install, AttemptedVersion, Error: sanitize(Err), ConsecutiveFailures: prior+1, Shown: false}` |
| `UpdateFailed` | `WriteNotice{Verb: update, AttemptedVersion, Error: sanitize(Err), ConsecutiveFailures: prior+1, Shown: false}` |
| `RollbackFailed` | `WriteNotice{Verb: rollback, AttemptedVersion, Error: sanitize(Err), ConsecutiveFailures: prior+1, Shown: false}` |
| `RemoveFailed` | `WriteNotice{Verb: remove, AttemptedVersion, Error: sanitize(Err), ConsecutiveFailures: prior+1, Shown: false}` |

The previous `kindFor(Source)` mapping is gone. `Kind` continues to
distinguish `KindAutoApplyResult` (still set when Source == SourceAuto)
from the default for backward compatibility with the existing renderer
branch, but the verb-specific message is driven by the new `Verb` field.

**Telemetry subscriber reaction table.** Today `internal/updates/apply.go`
calls `tc.SendUpdateOutcome(...)` three times (success/failure/rollback);
with the bus, those calls move to `internal/telemetry/subscriber.go`:

| Event | Telemetry emission |
|---|---|
| `Installed` | `NewInstallOutcomeSuccessEvent` |
| `Updated` | `NewUpdateOutcomeSuccessEvent` |
| `RolledBack` | `NewRollbackOutcomeSuccessEvent` |
| `Removed` | `NewRemoveOutcomeSuccessEvent` (when fully removed) |
| `InstallFailed` | `NewInstallOutcomeFailureEvent` |
| `UpdateFailed` (`ActiveAfter == FromVersion`) | `NewUpdateOutcomeFailureEvent` + `NewUpdateOutcomeRollbackEvent` |
| `UpdateFailed` (`ActiveAfter != FromVersion`) | `NewUpdateOutcomeFailureEvent` only |
| `RollbackFailed` | `NewRollbackOutcomeFailureEvent` |
| `RemoveFailed` | `NewRemoveOutcomeFailureEvent` |

Every emitted telemetry event tags `trigger = Source` (manual, auto,
project-auto) so funnels pivot by trigger without inferring from other
fields.

**Future opportunity: lifecycle Started events.** This vocabulary
intentionally omits `InstallStarted` / `UpdateStarted` / etc. They would
be useful for "where does install actually fail?" funnel analysis but
require publishing twice per operation, and no current subscriber needs
them. A follow-up PR can introduce them additively (existing subscribers
ignore unknown events) when there's a real consumer.

#### Alternatives Considered

- **Three-event vocabulary (Activated / InstallFailed / Removed) with
  Source carrying verb intent.** Initially chosen, then rejected after
  this PR's review. Forces both subscribers to infer verb from field
  shape (`FromVersion == ""` means install, else update; rollback hidden
  in Source). Workable for notices but the wrong shape for telemetry,
  where install-success and update-success are different funnels that
  must be cleanly separable at the type level.
- **Single `LifecycleEvent{Verb, Outcome, Tool, ...}` catch-all.**
  Rejected. Subscribers switch on string-typed Verb / Outcome fields;
  loses compile-time exhaustiveness. The verb-per-event shape gives
  exhaustive type switches at every subscriber.
- **Two-event vocabulary (`Succeeded`, `Failed`) with Verb as a field.**
  Rejected for the same reason — the verb is type-shaped, not
  value-shaped, so events should be typed by verb.
- **Lifecycle granularity (Started + Completed + Failed per verb,
  16 events).** Rejected as premature. Started events have no subscriber
  today; adding them is a non-breaking future extension. Documented as a
  future opportunity above.

### Decision 2: Bus delivery semantics

**Context.** The bus runs inside a short-lived CLI process. Subscriber
failure must not block install success or panic the host. Foreground exit
should reflect a settled store.

#### Chosen: Synchronous with per-subscriber recover

`Publish` runs subscribers inline, in deterministic registration order, each
wrapped in a `defer recover()`. Errors and recovered panics are logged with
the subscriber's registered name; nothing propagates to the publisher.
Re-entrant publishes from within a subscriber handler are queued and
flushed after the current event finishes (preserving causal order).

#### Alternatives Considered

- **Pure synchronous (no recover).** Rejected. A single panicking subscriber
  would crash the install — unacceptable for best-effort.
- **Asynchronous fire-and-forget.** Rejected. Breaks the synchronous-feeling
  exit contract, forces a flush at every CLI exit path, and makes tests
  rely on timing.
- **Asynchronous with explicit `Flush`.** Rejected. Adds a runtime loop and
  exit-path flush surface for no measurable benefit; subscriber work is
  cheap (small JSON writes).
- **Synchronous with errors propagated.** Rejected. Forces every `Publish`
  caller to handle errors it cannot act on; inverts the best-effort
  guarantee.
- **Disallow re-entrancy (panic on nested `Publish`).** Rejected. Accidental
  re-entrancy will happen; panicking in best-effort code is worse than
  queue-and-flush, which preserves natural ordering.

### Decision 3: Publisher site placement

**Context.** Same outcome (an event is emitted) can be reached three ways
with very different maintainability profiles. Surveyed ~15 state-mutation
call sites: only 6 are lifecycle-meaningful publishers; the rest are
bookkeeping (`AddRequiredBy`, `RemoveRequiredBy`, hidden-flag flips).

#### Chosen: Explicit `Publish()` at each lifecycle site

Publishers are explicit `bus.Publish(...)` calls inside the ~6 lifecycle
methods: `Manager.Install` (and `InstallWithOptions`), `Manager.Activate`,
`Manager.RemoveVersion`, `Manager.RemoveAllVersions`, `Manager.Remove`, and
`updates.CheckAndApplySelf`. The auto-apply loop in `internal/updates/apply.go`
calls into these methods rather than publishing directly.

A new constructor option threads the bus into `install.Manager`. A wiring
helper in `cmd/tsuku` constructs the bus, registers subscribers, and passes
the bus to the manager. Source values are threaded through method
parameters (or an options struct extension) at the call site.

#### Alternatives Considered

- **State.json shim (`StateManager.UpdateTool` wrapper auto-publishes).**
  Rejected. `state.UpdateTool` is called for at least four semantically
  distinct reasons (install, activate, version-remove, dependency
  bookkeeping). The rollback path's `Activate(prior)` would emit an event
  indistinguishable from a normal user activation. The shim also cannot
  emit non-state events (e.g., a future `Cancelled`).
- **Defer-based instrumentation (`defer bus.Publish(buildEvent(err))`).**
  Rejected. Rollback in `apply.go:164-182` happens outside the function that
  would carry the defer, destroying locality. Named-return subtleties and
  partial-payload construction at function entry add fragility for no real
  benefit.

### Decision 4: Subscriber registration mechanism

**Context.** Multiple entry points must wire the same subscriber set so
manual and automated installs leave the store in identical shape. The
codebase has exactly one `func main` for installs; the auto-apply
"subprocess" is the same binary re-invoked with the hidden `apply-updates`
subcommand. The "every entry point wires the same set" constraint is
therefore satisfied trivially by wiring once in `cmd/tsuku`.

#### Chosen: Single explicit wiring helper in `cmd/tsuku`

A `bus.New()` constructor plus per-subscriber `Subscribe(name, sub)` calls
gathered in one file under `cmd/tsuku/` (for example
`cmd/tsuku/events_wiring.go`). The CLI root command constructs the bus
once and threads it into `install.Manager` and `updates.CheckAndApplySelf`
via constructor options.

The helper form (rather than a variadic constructor) gives one named
function listing the subscriber set, natural conditional wiring (e.g., a
future `--quiet` flag could skip a subscriber), and consistency with the
codebase's existing pattern for config-dependent collaborators
(`recipe.NewLoader`, `registry.New`).

#### Alternatives Considered

- **Per-package `init()` self-registration.** Rejected. Requires a global
  default bus, contradicting Decision 2's commitment to a bus value. Config
  isn't resolved at `init()` time, so subscribers can't be fully constructed
  there. Test isolation would require global-state resets the codebase has
  been moving away from.
- **Constructor-injected variadic subscriber list.** Functionally
  equivalent to the chosen helper. Rejected on three minor points: no named
  single-purpose function for "the subscriber set", slice-building required
  for conditional registration, and the call site mixes with other main
  construction.

## Decision Outcome

The four decisions compose into a single coherent design:

1. **Vocabulary** (Decision 1) — eight typed events along a verb-per-event
   axis (`Installed`, `Updated`, `RolledBack`, `Removed` + their failure
   counterparts) with `Source` (`SourceManual`, `SourceAuto`,
   `SourceProjectAuto`) as an orthogonal tag. Two subscribers — notices
   and telemetry — both consume the same events; verb-per-event makes
   each subscriber's reaction a clean type switch rather than a field
   inference.
2. **Semantics** (Decision 2) — synchronous-with-recover delivery means
   a publisher's `Publish` call returns after all subscribers have
   observed the event (or panicked harmlessly), so the on-disk notice
   store and the telemetry pipeline both reflect the most recent state
   mutation by the time `Publish` returns.
3. **Publisher placement** (Decision 3) — explicit `Publish` calls
   inside the lifecycle methods on `install.Manager`
   (`Install` / `Rollback` / `RemoveVersion` / `RemoveAllVersions`) plus
   `updates.CheckAndApplySelf`. `Manager.Install` owns its own
   failure-recovery (the auto-rollback inside a failed update), so it
   emits exactly one event per operation with the recovery state baked
   into `UpdateFailed.ActiveAfter`. Source is threaded as a method
   parameter.
4. **Wiring** (Decision 4) — a single helper in `cmd/tsuku` constructs
   the bus, registers the notices and telemetry subscribers, and threads
   the bus into the manager and self-update path. The auto-apply
   subprocess wires its bus explicitly. Tests construct their own bus
   per subject.

The motivating bug (drift between notice files and `state.json`) is
fixed structurally rather than by read-time filtering: every state
mutation publishes; subscribers reconcile their own stores in response;
the renderer becomes a dumb consumer.

The vocabulary also fixes a separate inconsistency: today telemetry
events are emitted from `apply.go` next to notice writes, but the two
sets aren't aligned (self-update success has a notice but no telemetry;
project-auto installs emit telemetry but no notice). With both as
subscribers, every lifecycle operation produces a notice and a
telemetry event by construction; mismatches become impossible.

Three implementation concerns surface from cross-validation:

1. **`Source` threading.** Adding `Source` as a parameter on the public
   Manager methods touches ~10 call sites. Manageable; one-time.
2. **`Manager.Install` takes on failure recovery.** Today
   `internal/updates/apply.go` orchestrates the rollback after a failed
   update by calling `mgr.Activate(previousVersion)`. With the new
   model, this recovery moves inside `Manager.Install` so the bus event
   carries the post-recovery state. This is a small refactor but
   changes the responsibility split between `apply.go` and
   `install.Manager`.
3. **Notice schema change.** The `Notice` struct gains a `Verb` field
   so the renderer can format per-verb messages. Backward-compatible
   (old files default to empty Verb → renderer falls back to today's
   "X has been updated to Y" phrasing).

## Solution Architecture

### Overview

A new package `internal/installevents` defines eight typed event types
(verb × outcome: Installed, Updated, RolledBack, Removed, plus their
failure counterparts), the `Source` enum, and a small `Bus` that
delivers events synchronously to registered subscribers.

Two subscribers register in production wiring:

- **`internal/notices.Subscriber`** translates events into
  `WriteNotice` / `RemoveNotice` calls. The existing
  `internal/updates/notify.go` renderer is structurally unchanged —
  it continues to read notice files — but now reads a store kept
  consistent by event reactions instead of one written directly from
  N call sites.
- **`internal/telemetry.Subscriber`** translates events into
  `tc.SendUpdateOutcome` / `SendInstallOutcome` / etc. calls. Today
  these calls happen inline in `internal/updates/apply.go` next to
  the direct notice writes; the subscriber consolidates them and
  ensures every lifecycle event produces exactly one telemetry
  emission.

The bus is a process-local value, not a global. It's constructed once
per process and threaded into the subsystems that publish
(`install.Manager` and `updates.CheckAndApplySelf`). Both entry points
of the tsuku binary — the foreground CLI and the auto-apply
subprocess (`cmd/tsuku/cmd_apply_updates.go`) — wire the same
subscriber set, so manual and automated lifecycle operations leave
both subscriber stores in identical shape.

### Components

```
+-------------------------------------------+
|           cmd/tsuku/main.go               |
|     and cmd/tsuku/cmd_apply_updates.go    |
|                                           |
|  bus := installevents.NewBus(cfg)         |
|  bus.Subscribe("notices",                 |
|    notices.NewSubscriber(noticesDir))     |
|  bus.Subscribe("telemetry",               |
|    telemetry.NewSubscriber(client))       |
|  mgr := install.NewManager(               |
|    cfg, install.WithEventBus(bus))        |
|  updates.CheckAndApplySelf(               |
|    ..., bus, source)                      |
+----------------+--------------------------+
                 |
                 | (bus value, not global)
                 v
+-------------------------------------------+   +-----------------------------+
|       internal/install/                   |   |  internal/updates/          |
|                                           |   |    apply.go, self.go        |
|  Manager.Install (owns failure recovery)  |   |                             |
|    -> bus.Publish(Installed|Updated|      |   |  CheckAndApplySelf          |
|                  InstallFailed|UpdateFailed)  |    -> bus.Publish(...)      |
|  Manager.Rollback                         |   |                             |
|    -> bus.Publish(RolledBack|RollbackFailed)  +--------------+--------------+
|  Manager.RemoveVersion/RemoveAllVersions  |                  |
|    -> bus.Publish(Removed|RemoveFailed)   |                  |
+----------------+--------------------------+                  |
                 |                                             |
                 +---------------------+-----------------------+
                                       |
                                       v
                  +------------------------------------+
                  |   internal/installevents.Bus       |
                  |                                    |
                  |  Publish(event) {                  |
                  |    for sub in subs (in order) {    |
                  |      defer recover; sub.Handle()   |
                  |    }                               |
                  |    flush queued nested events      |
                  |  }                                 |
                  +-----------------+------------------+
                                    |
                  +-----------------+------------------+
                  v                                    v
+------------------------------+    +-------------------------------+
| internal/notices.Subscriber  |    | internal/telemetry.Subscriber |
|                              |    |                               |
| switch e := event.(type) {   |    | switch e := event.(type) {    |
|   case Installed: Write      |    |   case Installed: Send        |
|   case Updated:   Write      |    |   case Updated:   Send        |
|   case RolledBack: Write     |    |   case RolledBack: Send       |
|   case Removed:   Remove     |    |   case Removed:   Send        |
|   case *Failed:   Write      |    |   case UpdateFailed:          |
| }                            |    |     Send (+rollback if        |
|                              |    |     ActiveAfter==FromVersion) |
+--------------+---------------+    |   case *Failed: Send          |
               |                    | }                             |
               v                    +---------------+---------------+
+--------------------------------+                  |
| $TSUKU_HOME/notices/<tool>.json|                  v
| (Notice schema gains Verb field)|   +------------------------------+
+--------------+-----------------+    | telemetry endpoint           |
               |                      | (existing client + worker)   |
               v                      +------------------------------+
+--------------------------------+
| internal/updates/notify.go     |
| (renderer — reads Verb to      |
| format per-verb messages)      |
+--------------------------------+
```

### Key Interfaces

```go
// Package internal/installevents

type Subscriber interface {
    Handle(event Event) // event is one of the eight types defined in events.go
}

type Event interface {
    isInstallEvent() // sealed: implemented only by event types in this package
}

type Bus struct { /* unexported */ }

// NewBus constructs an empty bus configured for production wiring. The bus
// reads cfg.HomeDir to derive the trace-file path; subscriber errors and
// recovered panics are written there, never to stderr. Tests use NewBusForTest
// (below) to supply a captured logger.
func NewBus(cfg *config.Config) *Bus

// NewBusForTest constructs a bus that writes diagnostics to the provided
// io.Writer. Used only from _test.go files.
func NewBusForTest(diag io.Writer) *Bus

// Subscribe registers sub under name. Names appear in error logs. Calling
// Subscribe after Publish is a logic bug (panic in debug builds, log + ignore
// in release).
func (b *Bus) Subscribe(name string, sub Subscriber)

// Publish runs every subscriber's Handle in registration order. Each Handle
// is wrapped in defer/recover; a panicking subscriber is logged and skipped.
// Re-entrant Publish calls during a Handle are queued and flushed after the
// current event finishes (preserving causal order). The bus enforces two
// safety caps: a re-entrancy depth limit of 16 and a pending-queue size
// limit of 1024. Events that would exceed either cap are dropped with a
// log line.
func (b *Bus) Publish(event Event)
```

The publisher side:

```go
// install.Manager gains a bus field set via constructor option.
type Manager struct {
    /* ... existing fields ... */
    bus *installevents.Bus // may be nil; nil-safe Publish is a no-op
}

func WithEventBus(bus *installevents.Bus) Option { /* ... */ }

// Public lifecycle methods take Source as a parameter:
//   Install(tool, version string, src Source) error
//   Rollback(tool, toVersion string, src Source) error
//   RemoveVersion(tool, version string, src Source) error
//   RemoveAllVersions(tool string, src Source) error
//
// Install owns its own failure recovery: on failure of an update, it
// calls the unexported activate(prior) helper before publishing
// UpdateFailed with ActiveAfter reflecting the post-recovery state.
// internal/updates/apply.go no longer orchestrates rollback itself.
```

The notices subscriber (in `internal/notices`):

```go
type Subscriber struct {
    dir string // notices directory
}

func NewSubscriber(dir string) *Subscriber

func (s *Subscriber) Handle(event installevents.Event) {
    switch e := event.(type) {
    case installevents.Installed:
        _ = WriteNotice(s.dir, successNotice(e.Tool, "install", e.Version, e.Source, e.Timestamp))
    case installevents.Updated:
        _ = WriteNotice(s.dir, successNotice(e.Tool, "update", e.ToVersion, e.Source, e.Timestamp))
    case installevents.RolledBack:
        _ = WriteNotice(s.dir, successNotice(e.Tool, "rollback", e.ToVersion, e.Source, e.Timestamp))
    case installevents.Removed:
        _ = RemoveNotice(s.dir, e.Tool)
    case installevents.InstallFailed:
        _ = WriteNotice(s.dir, failureNotice(s.dir, e.Tool, "install", e.AttemptedVersion, e.Err, e.Source, e.Timestamp))
    case installevents.UpdateFailed:
        _ = WriteNotice(s.dir, failureNotice(s.dir, e.Tool, "update", e.AttemptedVersion, e.Err, e.Source, e.Timestamp))
    case installevents.RollbackFailed:
        _ = WriteNotice(s.dir, failureNotice(s.dir, e.Tool, "rollback", e.AttemptedVersion, e.Err, e.Source, e.Timestamp))
    case installevents.RemoveFailed:
        _ = WriteNotice(s.dir, failureNotice(s.dir, e.Tool, "remove", e.AttemptedVersion, e.Err, e.Source, e.Timestamp))
    }
}

// successNotice and failureNotice build Notice values. failureNotice
// reads the prior notice file to compute ConsecutiveFailures and
// sanitizes Err (newline normalization + 512-byte truncation).
```

The telemetry subscriber (in `internal/telemetry`):

```go
type Subscriber struct {
    client *Client // existing telemetry client
}

func NewSubscriber(c *Client) *Subscriber

func (s *Subscriber) Handle(event installevents.Event) {
    if s.client == nil {
        return // telemetry disabled
    }
    trigger := string(event.GetSource()) // "manual", "auto", "project-auto"
    switch e := event.(type) {
    case installevents.Installed:
        s.client.SendInstallOutcome(NewInstallOutcomeSuccessEvent(e.Tool, e.Version, trigger))
    case installevents.Updated:
        s.client.SendUpdateOutcome(NewUpdateOutcomeSuccessEvent(e.Tool, e.FromVersion, e.ToVersion, trigger))
    case installevents.RolledBack:
        s.client.SendRollbackOutcome(NewRollbackOutcomeSuccessEvent(e.Tool, e.FromVersion, e.ToVersion, trigger))
    case installevents.Removed:
        if e.ActiveAfter == "" { // full removal only; per-version removes are noise
            s.client.SendRemoveOutcome(NewRemoveOutcomeEvent(e.Tool, e.Version, trigger))
        }
    case installevents.InstallFailed:
        s.client.SendInstallOutcome(NewInstallOutcomeFailureEvent(e.Tool, e.AttemptedVersion, ClassifyError(e.Err), trigger))
    case installevents.UpdateFailed:
        s.client.SendUpdateOutcome(NewUpdateOutcomeFailureEvent(e.Tool, e.AttemptedVersion, ClassifyError(e.Err), trigger))
        if e.ActiveAfter == e.FromVersion && e.FromVersion != "" {
            // Automatic recovery succeeded — emit the rollback event too.
            s.client.SendUpdateOutcome(NewUpdateOutcomeRollbackEvent(e.Tool, e.FromVersion, e.AttemptedVersion, trigger))
        }
    case installevents.RollbackFailed:
        s.client.SendRollbackOutcome(NewRollbackOutcomeFailureEvent(e.Tool, e.AttemptedVersion, ClassifyError(e.Err), trigger))
    case installevents.RemoveFailed:
        // No telemetry today; reserved for future.
    }
}
```

### Data Flow

#### Successful manual update

1. User runs `tsuku update niwa`.
2. `cmd/tsuku/update.go` calls `mgr.Install("niwa", "0.11.1", SourceManual)`.
3. `Manager.Install` sees a prior `ActiveVersion == "0.11.0"`, performs
   the update, writes state.json, then publishes
   `Updated{Tool: "niwa", FromVersion: "0.11.0", ToVersion: "0.11.1", Source: SourceManual}`.
4. Bus invokes notices subscriber synchronously: writes a success notice
   with `Verb: update`, `AttemptedVersion: "0.11.1"`, `Shown: false`.
5. Bus invokes telemetry subscriber synchronously: emits
   `UpdateOutcomeSuccess{tool: "niwa", from: "0.11.0", to: "0.11.1", trigger: "manual"}`.
6. Update command exits. Next foreground command prints "niwa has been
   updated to 0.11.1" and marks `Shown: true`.

#### Failed auto-apply with automatic rollback

1. Background auto-apply iterates pending entries.
2. For entry "niwa@0.11.1": calls `mgr.Install("niwa", "0.11.1", SourceAuto)`.
3. `Manager.Install` attempts the update, fails. State.json was not
   written. `Manager.Install` internally calls `activate("0.11.0")` to
   restore the prior version (silent, no event).
4. `Manager.Install` publishes
   `UpdateFailed{Tool: "niwa", AttemptedVersion: "0.11.1", FromVersion: "0.11.0", ActiveAfter: "0.11.0", Err, Source: SourceAuto}`.
5. Notices subscriber writes a failure notice with `Verb: update`,
   `ConsecutiveFailures` incremented.
6. Telemetry subscriber emits
   `UpdateOutcomeFailure{tool: "niwa", attempted: "0.11.1", trigger: "auto"}`
   AND `UpdateOutcomeRollback{tool: "niwa", from: "0.11.0", attempted: "0.11.1", trigger: "auto"}`
   because `ActiveAfter == FromVersion` indicates recovery succeeded.
7. On disk: a single failure notice for "niwa@0.11.1". No stale success
   notice. The motivating bug is structurally impossible.

#### Self-update

1. `updates.CheckAndApplySelf(bus, SourceAuto)` succeeds in replacing
   the binary, current version "0.11.0", new "0.11.1".
2. Publishes
   `Updated{Tool: "tsuku", FromVersion: "0.11.0", ToVersion: "0.11.1", Source: SourceAuto}`.
3. Notices subscriber writes a notice for "tsuku" the same way as for
   any tool, with `Verb: update`.
4. Renderer's existing `Tool == SelfToolName` branch formats this as
   "tsuku has been updated to 0.11.1".
5. Telemetry subscriber emits self-update outcome the same way as any
   tool update.

#### Explicit rollback

1. User runs `tsuku rollback niwa`.
2. `cmd/tsuku/cmd_rollback.go` calls `mgr.Rollback("niwa", "0.11.0", SourceManual)`.
3. `Manager.Rollback` activates the prior version, writes state.json,
   publishes `RolledBack{Tool: "niwa", FromVersion: "0.11.1", ToVersion: "0.11.0", Source: SourceManual}`.
4. Notices subscriber writes a success notice with `Verb: rollback`.
5. Telemetry subscriber emits `RollbackOutcomeSuccess`.

#### Project-auto install

1. `tsuku run` in a directory with `.tsuku.toml` requiring `gh`.
2. `internal/autoinstall/` calls `mgr.Install("gh", "2.47.0", SourceProjectAuto)`.
3. No prior version → publishes
   `Installed{Tool: "gh", Version: "2.47.0", Source: SourceProjectAuto}`.
4. Notices subscriber writes a notice with `Verb: install`.
5. Telemetry subscriber emits `InstallOutcomeSuccess{trigger: "project-auto"}`,
   making project-auto adoption observable for the first time.

#### Remove

1. User runs `tsuku remove niwa`.
2. `Manager.RemoveAllVersions("niwa", SourceManual)` mutates state.
3. Publishes `Removed{Tool: "niwa", Version: "", ActiveAfter: "", Source: SourceManual}`.
4. Notices subscriber removes any notice file for "niwa". The store can
   never show a banner for a tool the user no longer has.
5. Telemetry subscriber emits a remove-outcome event.

### Source threading

`Source` is a method parameter on every public `Manager` lifecycle method
(`Install`, `Rollback`, `RemoveVersion`, `RemoveAllVersions`) and on
`updates.CheckAndApplySelf`. Method parameter beats an options struct
because each method carries exactly one attribution field today; the
extra struct boilerplate would more than double the call-site noise.
Migration to options structs remains non-breaking if a second
per-method attribution field ever appears.

`InstallOpts` already exists and carries multiple fields; the design
adds Source to it via a `Source` field plus a `Source` parameter on the
`Install` shortcut. The shortcut sets `opts.Source = src` and delegates
to `InstallWithOptions`.

**Source defaults and validation.**

- The bus validates at the publish site: if `event.Source == ""`, it
  logs a warning naming the publisher and discards the event. Every
  call site must pass a value; there is no default.
- `Source` is not validated against `Tool`. A contributor passing
  `SourceProjectAuto` for a tool installed manually produces a
  structurally valid event that subscribers handle identically. The
  renderer keys off `Tool` and `Verb`, not `Source`, so mislabeling is a
  logic bug a code review catches, not a security flaw. A unit test
  asserts this property so future refactors that switch rendering on
  `Source` find the dependency.

### Source enum stability contract

`Source` values are first-party identifiers chosen by tsuku code. They
must remain non-PII and non-attacker-influenced strings. They surface
in telemetry as the `trigger` tag and may also be rendered in
user-facing output in the future (e.g., "(auto-updated)"). A code
comment on the enum definition reinforces this.

### Notice schema change

The `Notice` struct gains one new field:

```go
type Notice struct {
    /* ... existing fields ... */
    Verb string `json:"verb,omitempty"` // "install" | "update" | "rollback" | "remove"
}
```

The renderer in `internal/updates/notify.go` selects per-verb phrasing:

| Verb | Success message | Failure message |
|---|---|---|
| `install` | `<Tool> has been installed (<AttemptedVersion>)` | `Install failed: <Tool> -> <AttemptedVersion>: <Error>` |
| `update` | `<Tool> has been updated to <AttemptedVersion>` | `Update failed: <Tool> -> <AttemptedVersion>: <Error>` |
| `rollback` | `<Tool> has been rolled back to <AttemptedVersion>` | `Rollback failed: <Tool> -> <AttemptedVersion>: <Error>` |
| `remove` | (not written; Removed event removes the file) | `Remove failed: <Tool> <AttemptedVersion>: <Error>` |

Notice files written before this change have `Verb == ""`; the renderer
falls back to the current "X has been updated to Y" phrasing for
backward compatibility.

### Wiring contract

The bus and the notices subscriber must be wired in **every entry point
that publishes events**. The tsuku binary has two such entry points:

1. **Foreground commands.** The Cobra root command's `PersistentPreRun`
   (or equivalent setup hook in `cmd/tsuku/main.go`) calls `newEventBus`
   and threads the bus into `install.NewManager(...)` and
   `updates.CheckAndApplySelf`.
2. **Auto-apply subprocess.** `cmd/tsuku/cmd_apply_updates.go` (the
   hidden subcommand the foreground re-invokes for background updates)
   must independently call `newEventBus` and thread the bus into its own
   `install.Manager`. Cobra's `PersistentPreRun` inheritance is not
   sufficient on its own because the subcommand may override it.

Acceptance: an end-to-end integration test seeds `state.json`, drives a
failure-then-rollback scenario through the auto-apply subprocess, and
asserts the on-disk notice store reflects only `InstallFailed`. Without
this test, a forgotten wiring in either entry point is silent (the
nil-safe `Publish` makes both publish and discard look identical).

### Publish-after-state invariant

Every publish site emits the event **after** the state.json write that
the event reports. A comment at each `bus.Publish(...)` call states this
explicitly. Reversing the order reintroduces the drift the design is
fixing: subscribers would see "Updated to X" while state still says Y,
and the notice file would carry the wrong version. Per-method unit tests
verify the ordering by reading state inside a synchronous subscriber and
asserting it matches the event payload.

### Subscriber locality contract

A subscriber may only mutate the on-disk record for the **tool named in
the event it is handling**. The notices subscriber writes/removes
`notices/<event.Tool>.json` and no other file. This contract preserves
per-tool atomicity for future subscribers; a "summary" subscriber that
rewrote multiple notice files at once would open a lost-update window
between foreground rendering and background re-writes. The contract is
stated in the `Subscriber` interface's godoc.

### Error sanitization

Before persisting `InstallFailed.Err.Error()` to a notice file, the
subscriber:

1. Replaces `\n` and `\r` with ` / ` so the renderer prints a single line.
2. Truncates to 512 bytes with a `…` suffix.

This prevents HTTP response bodies, multi-line stack traces, and noisy
upstream error text from contaminating the notice store. A unit test
asserts no `Notice.Error` contains a newline.

### InboxReporter ordering

`InboxReporter.Stop()` (in `internal/progress/`) writes a richer
notice file when warnings accumulate during an install run. Today it
runs **after** `Manager.Install` returns. With the bus, the subscriber's
write happens synchronously inside `bus.Publish` called from `Manager.Install`,
which still happens before `Stop()` runs. The InboxReporter's
richer-on-warnings notice continues to overwrite the subscriber's plain
success notice. Do not reorder.

### Scope: tool tracking, not library tracking

`InstallLibrary` (`internal/install/library.go`) manages a separate
state model for libraries (`State.Libs`). Libraries are not surfaced in
the notice store today and stay out of scope for this design. If
library install failures should eventually surface to the user, a
follow-up design extends the vocabulary; this design does not.

### Manager.Remove (deprecated wrapper)

`Manager.Remove(name)` is a deprecated wrapper around
`Manager.RemoveAllVersions(name, ...)`. The wrapper does not need its
own publish call; `RemoveAllVersions` publishes, and the wrapper
inherits via delegation. The deprecated wrapper may be removed as part
of this work or kept for now.

## Implementation Approach

Three phases. The design fits one PR end-to-end; phase boundaries are
advisory for commit organization, not commit gates. The big phase
(Phase 3) must be one commit: it removes the existing direct
`WriteNotice` and `tc.SendUpdateOutcome` calls and replaces them with
publishes, all while the bus + subscribers come online. Splitting it
would leave the binary in a state where the old direct emissions are
gone but the bus isn't fully wired.

### Phase 1: Event package and bus (foundation)

Create `internal/installevents/` with:
- `events.go` — `Source` enum, the eight event types (`Installed`,
  `Updated`, `RolledBack`, `Removed`, `InstallFailed`, `UpdateFailed`,
  `RollbackFailed`, `RemoveFailed`), sealing via an unexported
  `isInstallEvent()` method, a `GetSource()` accessor for subscribers
  that need uniform Source access. Code comment on `Source` reinforcing
  the non-PII contract.
- `bus.go` — `Bus`, `NewBus(cfg)`, `NewBusForTest(io.Writer)`,
  `Subscribe`, `Publish` with deterministic ordering, defer-recover,
  re-entrancy queue, depth cap (16), queue-size cap (1024). Empty
  `Source` at publish time logs and drops.
- `bus_test.go` — covers: subscriber receives event in registration
  order, panicking subscriber is recovered and logged, re-entrant
  publish queues then flushes, depth cap drops at 16, queue-size cap
  drops at 1024, nil-safe Publish on a nil bus is a no-op, empty
  Source is dropped with a log line.

Deliverables:
- `internal/installevents/events.go`
- `internal/installevents/bus.go`
- `internal/installevents/bus_test.go`

### Phase 2: Subscribers + Notice schema update (foundation)

Two subscribers; both can be built in parallel before the publishers
land because neither has runtime effect until subscribed to a live bus.

**Notices subscriber** (`internal/notices/subscriber.go`):
- `NewSubscriber(dir string)` and `Handle(event)` method honoring the
  subscriber-locality contract (only mutates `<dir>/<event.Tool>.json`).
- `sanitizeError(err) string` helper: newline normalization and
  512-byte truncation. Unit test asserts no output contains `\n`.
- Subscriber tests cover all eight events against a temp directory,
  including `ConsecutiveFailures` increment on repeated `*Failed`
  events.

**Notice schema update** (`internal/notices/notices.go`):
- Add `Verb` field to `Notice` struct (string, kebab-case verb name).
- Apply the same tool-name validation in `RemoveNotice` that
  `WriteNotice` already performs (defense in depth).

**Renderer update** (`internal/updates/notify.go`):
- Read `Verb` to format per-verb messages per the table in Solution
  Architecture. Empty `Verb` keeps today's "X has been updated to Y"
  phrasing for backward compatibility with notice files written before
  this change.
- The render flow remains unchanged structurally — it still iterates
  unshown notices and marks them shown.

**Telemetry subscriber** (`internal/telemetry/subscriber.go`):
- `NewSubscriber(client *Client)` and `Handle(event)` method per the
  reaction table in Decision 1.
- `ClassifyError(err) string` reuse from the existing telemetry
  package.
- Tests assert the rollback-detection logic for `UpdateFailed`
  (`ActiveAfter == FromVersion && FromVersion != ""` → emits both
  failure and rollback events).

Deliverables:
- `internal/installevents/events.go` (touched again to add `GetSource`)
- `internal/notices/subscriber.go`
- `internal/notices/subscriber_test.go`
- `internal/notices/notices.go` (Verb field + RemoveNotice validation)
- `internal/updates/notify.go` (verb-aware rendering)
- `internal/updates/notify_test.go` (per-verb cases)
- `internal/telemetry/subscriber.go`
- `internal/telemetry/subscriber_test.go`

### Phase 3: Manager refactor + publisher wiring + call-site rewiring (single commit)

This phase must happen in a single commit.

**Manager refactor** (`internal/install/manager.go`, `remove.go`, etc.):
- Add `bus *installevents.Bus` field to `Manager`, `WithEventBus(bus)`
  option.
- Add `Source` parameter to `Manager.Install`,
  `Manager.RemoveVersion`, `Manager.RemoveAllVersions`. Extend
  `InstallOpts` with `Source` for the `InstallWithOptions` form.
- **Add `Manager.Rollback(tool, toVersion string, src Source) error`**
  as a distinct public method. It activates the prior version, writes
  state.json, and publishes `RolledBack` (or `RollbackFailed`).
- **Move auto-rollback into `Manager.Install`.** On failure of an
  update (prior `ActiveVersion` existed), `Install` internally calls
  the unexported `activate(tool, prior)` helper to restore the prior
  version, then publishes `UpdateFailed` with `ActiveAfter` set to the
  post-recovery state. `internal/updates/apply.go` no longer
  orchestrates rollback itself — it just calls `mgr.Install`.
- Per-method publish sites have code comments naming the
  publish-after-state invariant.
- New unit tests in `manager_test.go`: each method publishes the right
  event on success and failure; the in-event state read sees
  post-write state; nil bus is safe; rollback recovery inside Install
  populates `ActiveAfter` correctly.

**Publisher rewiring** (`internal/updates/`, `cmd/tsuku/`):
- `internal/updates/apply.go`: remove direct `notices.WriteNotice`
  and `tc.SendUpdateOutcome` calls. The loop just calls `mgr.Install`
  with the entry's `Source: SourceAuto` and lets the publishers in
  `Manager.Install` fire all events.
- `internal/updates/self.go`, `internal/updates/trigger.go`: add a
  `*installevents.Bus` parameter where needed. Self-update success
  publishes `Updated{Tool: "tsuku", Source: <Manual or Auto>}`;
  failure publishes `UpdateFailed{Tool: "tsuku"}`. New behavior:
  self-update failures become user-visible.
- `cmd/tsuku/install.go`, `update.go`, `remove.go`, `rollback.go`,
  `self_update.go`, etc.: pass the appropriate Source value into the
  Manager calls. Remove any direct `notices.WriteNotice` /
  `tc.SendUpdateOutcome` calls.
- `internal/autoinstall/` calls `mgr.Install` with
  `Source: SourceProjectAuto`, surfacing project-auto installs to both
  subscribers (new behavior; today these emit telemetry but no notice).

**Bus wiring** (`cmd/tsuku/events_wiring.go`):
- `newEventBus(cfg, tc) *installevents.Bus` constructs the bus,
  subscribes `notices.NewSubscriber(...)` and (if telemetry is enabled)
  `telemetry.NewSubscriber(tc)`, and returns it.
- The Cobra root command's setup hook constructs the bus and threads
  it into `install.NewManager` and `updates.CheckAndApplySelf`.
- **`cmd/tsuku/cmd_apply_updates.go` (the auto-apply subprocess
  subcommand) independently constructs its own bus and threads it.**
  Do not rely on `PersistentPreRun` inheritance.

Validation:
- Existing tests in `internal/updates/notify_test.go`,
  `internal/install/manager_test.go`,
  `internal/telemetry/event_test.go` still pass (possibly with minor
  adjustments to match the new event flow).
- **Acceptance: an end-to-end integration test seeds `state.json`,
  drives the failure-then-rollback scenario through the auto-apply
  subprocess, and asserts both the on-disk notice store reflects
  exactly one `UpdateFailed`-derived notice AND telemetry emitted
  exactly the expected pair (`UpdateOutcomeFailure` +
  `UpdateOutcomeRollback`).** This test is non-negotiable — without
  it, a forgotten wiring in either entry point is silent.
- A second integration test for project-auto: trigger `tsuku run` in
  a directory with `.tsuku.toml`, assert the resulting `Installed`
  event produces both a notice with `Verb: install` and a telemetry
  emission with `trigger: project-auto`.

Deliverables:
- `internal/install/manager.go`, `state.go`, `state_tool.go`,
  `remove.go`, `update.go` modifications.
- `internal/install/rollback.go` (new file housing `Manager.Rollback`)
  or extend existing rollback handling.
- `internal/updates/apply.go`, `self.go`, `trigger.go` modifications.
- `cmd/tsuku/events_wiring.go`, `cmd_apply_updates.go`,
  `install.go`, `update.go`, `remove.go`, `rollback.go`,
  `self_update.go` modifications.
- `internal/autoinstall/` modifications.
- End-to-end integration tests under `test/functional/`.

## Security Considerations

The event bus is an in-process Go value with no new external inputs, no
new network access, and no new dependencies. Publishers and the single
notices subscriber are first-party packages compiled into the same
binary; there is no meaningful trust boundary between them. Filesystem
effects are limited to `$TSUKU_HOME/notices/<tool>.json` writes (atomic
via `os.Rename`) and removes — the same paths `notices.WriteNotice` and
`notices.RemoveNotice` already touch today.

**Path-traversal defense.** Event `Tool` fields originate from
`state.json` keys or hard-coded literals (`"tsuku"` for self-update),
not from user-controlled string input. `notices.WriteNotice` validates
against path-separator and `..` injection. As a defense-in-depth
hardening, the implementation should extend the same validation to
`notices.RemoveNotice` — today only `WriteNotice` checks. This is a
small, isolated change in `internal/notices/notices.go` and is not
exploitable in practice given current callers, but it removes an easy
footgun for future call sites.

**Re-entrancy bounds.** The bus queues nested `Publish` calls and
flushes them after the current event. The notices subscriber does not
call back into the bus, but a future subscriber that does — buggy or
malicious — could grow the queue along two axes: depth (nested
handlers) and breadth (a single handler queuing many siblings). The
implementation caps re-entrancy depth at 16 and pending-queue size at
1024. Events that would exceed either cap are dropped with a log line,
matching the bus's "log subscriber error, do not propagate" stance.

**Panic containment and logging destination.** Per-subscriber
`defer recover()` catches panics from `Handle`. The recovered value
and subscriber name are logged. Subscriber implementations must not
panic with values that embed secrets; if the notice file isn't the
right channel for some future signal, that signal should be added as a
typed return or new event, not a panic message.

The bus's logger destination is locked in by the constructor. Production
wiring uses `NewBus(cfg *config.Config)`, which routes diagnostics to
the trace file under `$TSUKU_HOME/log/` — not to stderr. Tests use
`NewBusForTest(io.Writer)`. There is no caller-passed `Logger`
parameter that an alternate wiring could supply with `os.Stderr` by
accident.

**Information disclosure and error sanitization.**
`InstallFailed.Err.Error()` lands in the notice file via the
subscriber's `sanitizeError`: newlines and carriage returns are
replaced with ` / ` and the result is truncated to 512 bytes. This
prevents HTTP response bodies, multi-line stack traces, and noisy
upstream error text from contaminating the store. Self-update failures
become user-visible for the first time (previously silent); the
truncation cap prevents a hijacked update endpoint from amplifying its
error text into log spam.

**Publish-after-state invariant.** Every publish site emits its event
after the state.json write that the event reports. Reversing the order
silently reintroduces the drift the design fixes: subscribers would
write notice files describing a state the disk doesn't reflect.
Per-site code comments and ordering tests enforce this; there is no
compile-time guarantee.

**Subscriber locality.** A subscriber may only mutate the on-disk
record for the tool named in the event it is handling. This contract
preserves per-tool atomicity for any future subscriber and is stated
in the `Subscriber` interface's godoc.

**Trust boundary inside the process.** The bus, publishers, and the
notices subscriber are all first-party code in the same binary; there
is no meaningful trust boundary between them. This property depends on
Decision 4's rejection of `init()`-time registration. If that decision
is reversed (e.g., to allow plugin subscribers loaded from outside the
binary), this analysis must be revisited.

**Cross-process atomicity.** The foreground command and the background
auto-apply subprocess each construct their own process-local bus. They
synchronize through atomic file operations (`os.Rename`, `os.Remove`),
identical to today. No new TOCTOU window is introduced.

## Consequences

### Positive

- The notices store reflects reality by construction. The PR #2411 bug
  class (read-time filters catching write-time drift) cannot recur.
- Notices and telemetry are guaranteed-consistent: today each is
  emitted from a different code path, leading to mismatches
  (self-update success has a notice but no telemetry; project-auto
  installs emit telemetry but no notice). With both as subscribers on
  the same bus, every lifecycle operation produces a paired emission
  by construction.
- Self-update failures become user-visible for the first time. Today
  they log silently to the trace file.
- Project-auto installs (`.tsuku.toml` auto-approval) become
  observable in both notices and telemetry for the first time.
- Removing a tool deterministically clears its notices. Today the
  file is orphaned.
- Verb-per-event vocabulary means subscribers branch on event type
  rather than infer from field shape. Adding a sub-feature like
  "count rollbacks per week" is a one-line subscriber addition.
- New triggers extend the `Source` enum without changing event types.
- Auditing "what happens when a tool is updated" reduces to reading
  two subscriber files plus the bus's `Publish` signature.

### Negative

- Eight event types is more types than the minimum viable. Adding a
  ninth verb (e.g., a future `Pinned` event) is one more type plus
  per-subscriber handling. Compared to a single
  `LifecycleEvent{Verb, Outcome, …}`, the type explosion is the
  trade-off for compile-time exhaustiveness at subscribers.
- Manager API surface widens. Adding `Source` parameters to four
  public methods, plus a new `Manager.Rollback` method, plus moving
  auto-rollback into `Manager.Install`, is meaningful refactor work
  touching ~15 call sites and shifting responsibility between
  `apply.go` and the manager.
- The "publish after state write" invariant is uncompiled. A
  contributor who moves a publish call above its state write
  reintroduces the drift silently. Per-site code comments and
  per-method ordering tests are the only enforcement.
- The notice store has no garbage collection. Every tool that ever
  failed to update leaves a notice file until manually cleared or
  superseded by a later event. Today's behavior is the same, but the
  event bus opens more publishers (project-auto) and so the store
  grows faster over time.
- Adding a subscriber requires updating the wiring helper. For two
  subscribers this is trivial; if the subscriber set grows, the
  wiring helper grows linearly.
- The bus does not fix the existing `MarkShown` race. This race
  pre-exists; the structural fix is orthogonal.
- Telemetry subscriber is a new dependency path: a bug in the
  subscriber could mean telemetry events stop firing, where today
  the calls are inline and failures are local to the call site.
  Mitigation: subscriber is heavily unit-tested with table-driven
  cases covering the rollback-detection logic.

### Mitigations

- The Source-threading and Manager refactor are bounded and one-time.
  Once done, adding a new trigger is a single Source-constant change
  plus the caller wiring; no further API churn.
- Moving auto-rollback into `Manager.Install` is the natural shape:
  `Install` is the only caller that has the prior version handy and
  the only place that decides whether to recover. `apply.go` becomes
  a simpler loop that delegates.
- The wiring helper file (`cmd/tsuku/events_wiring.go`) is named to
  be obviously where subscribers live; a reader looking for "what
  happens on Updated" finds it in one grep.
- Both acceptance integration tests (failure-then-rollback,
  project-auto install) catch a forgotten wiring or a missed
  subscriber emission in either entry point. The nil-safe `Publish`
  keeps unit-test setup small without silencing the acceptance
  tests.
- Notice-store GC is a separate concern tracked as future work; this
  design does not block on it.
- A future per-tool advisory lock (or a CAS-like "write if not shown"
  semantic) can address the `MarkShown` race without changing the
  event bus contract.
