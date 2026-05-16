---
status: Proposed
problem: |
  Notice files under $TSUKU_HOME/notices/ are written ad-hoc from several call
  sites (background auto-apply, manual update, self-update, rollback) and read
  blindly by the renderer in internal/updates/notify.go. Nothing keeps the
  store consistent with state.json, so any drift between a notice's
  AttemptedVersion and the tool's installed ActiveVersion surfaces as a
  user-visible lie ("niwa has been updated to 0.10.4" while 0.11.0 is
  installed). PR #2411 filtered at read time; that catches one drift class
  but leaves the store full of stale entries and forces a new filter clause
  for every future drift cause (removed tool, rollback, cancelled install).
decision: |
  Introduce an in-process install lifecycle event bus in a new internal
  package. Every code path that mutates state.json (manual install/update,
  background auto-apply, rollback, remove, self-update) publishes a typed
  event after the mutation succeeds. internal/notices subscribes and
  reconciles the on-disk notice store in response. Each tsuku process wires
  the full subscriber set at startup via a single registration call from
  cmd/tsuku/main.go, so manual and automated paths handle events identically.
  Existing write-sites in apply.go, update.go, and self.go that currently
  call notices.WriteNotice directly are replaced by event publication. The
  renderer becomes a dumb consumer of whatever the store contains.
rationale: |
  A source-side fix collapses every "stale entry" cause into one contract
  (the event vocabulary) instead of growing a tower of read-time filters.
  In-process pub/sub is sufficient because each tsuku process is short-lived
  and the filesystem is the inter-process synchronization point — the
  background auto-apply subprocess publishes events that its own subscribers
  see (including the notices listener that updates files on disk), and the
  next foreground process loads those files. Synchronous, best-effort
  delivery matches the existing notice semantics and avoids introducing
  goroutine lifecycle, panic-recovery, or shutdown questions. The trade-off
  is that every install-path call site needs to learn to publish; that work
  is bounded (≈6 sites) and audit-friendly.
---

# DESIGN: Notices Install Event Bus

## Status

Proposed

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

**Context.** Drives every publisher and every subscriber. The codebase has nine
notice-mutating sites today, fed by state mutations in
`install.Manager.{Install, Activate, RemoveVersion, RemoveAllVersions}` plus
`updates.ApplySelfUpdate`. The vocabulary defines the events those mutation
points publish and how `internal/notices` reacts.

Key assumptions:
- Bus delivery is synchronous-with-recover (Decision 2 outcome).
- Publishers are explicit `Publish()` calls at the small set of state-mutation
  entry points (Decision 3 outcome).
- `Source` is a closed enum extended only via code change.
- `InboxReporter` keeps writing its warning-kind notices directly; those are
  side-channel observations during an install run, not state-change events.

#### Chosen: Three-event vocabulary with publish-on-state-change predicate

Three event types in a new package `internal/installevents`:

```go
type Source string

const (
    SourceInstall      Source = "install"       // tsuku install <tool>
    SourceManualUpdate Source = "manual-update" // tsuku update <tool>
    SourceAutoUpdate   Source = "auto-update"   // background auto-apply
    SourceRollback     Source = "rollback"      // tsuku rollback
    SourceSelf         Source = "self"          // tsuku self-update
)

// Activated fires when state.ActiveVersion changes for a tool.
// Fresh installs (FromVersion == "") and version transitions are both Activated.
// Self-update emits Activated with Tool == "tsuku".
type Activated struct {
    Tool        string
    FromVersion string    // "" if fresh install or self-update from unknown
    ToVersion   string    // always non-empty
    Source      Source
    Timestamp   time.Time
}

// InstallFailed fires when an install attempt did not change state.ActiveVersion
// but the user should be informed. ActiveAfter carries rollback outcome.
type InstallFailed struct {
    Tool             string
    AttemptedVersion string
    ActiveAfter      string // active version after attempt; "" if no prior install
    Err              error
    Source           Source
    Timestamp        time.Time
}

// Removed fires when a tool or version was removed from state.
type Removed struct {
    Tool        string
    Version     string // version removed; "" if all versions removed
    ActiveAfter string // new active version; "" if tool fully gone
    Source      Source
    Timestamp   time.Time
}
```

**Publish contract.**
- `Manager.Install` / `InstallWithOptions`: on success publish `Activated`; on
  failure publish `InstallFailed`. Caller threads the `Source`.
- `Manager.Activate`: publish-on-state-change predicate. If
  `oldActiveVersion != newActiveVersion`, publish `Activated`. Otherwise
  publish nothing.
- `Manager.RemoveVersion` / `RemoveAllVersions`: publish `Removed` after
  state mutation.
- `updates.CheckAndApplySelf`: on success publish `Activated{Tool: "tsuku"}`;
  on failure publish `InstallFailed{Tool: "tsuku"}`. Failure events are new
  behavior — today self-update failures are silent.

**Notices subscriber reaction table.**

| Event | File mutation |
|---|---|
| `Activated` | `WriteNotice{AttemptedVersion: ToVersion, Error: "", Kind: kindFor(Source), Shown: false}` (atomic rename overwrites any prior notice) |
| `InstallFailed` | `WriteNotice{AttemptedVersion, Error: Err.Error(), Kind: kindFor(Source), ConsecutiveFailures: prior+1, Shown: false}` |
| `Removed` | `RemoveNotice(Tool)` |

`kindFor(Source)`: `SourceAutoUpdate` → `KindAutoApplyResult`; all others →
`KindUpdateResult` (the empty-string legacy value).

The motivating bug is eliminated structurally: `Manager.Install`'s failure
path never writes state (`manager.go:168` only runs on success), so the
subsequent rollback `Activate(previousVersion)` finds
`ActiveVersion == previousVersion` already and emits nothing.
`InstallFailed` is the sole notice-write for that flow.

#### Alternatives Considered

- **Single `InstallStateChanged{Tool, From, To, Err, Source}` catch-all.**
  Rejected. Subscriber branching as field patterns has the same logic
  complexity as type discrimination but loses compile-time clarity; the
  Remove case requires a `ToVersion: ""` convention; rollback correctness
  requires the same predicate.
- **Outcome-typed (`InstallSucceeded`, `InstallFailed`, `Removed`,
  `RolledBack`).** Rejected. `RolledBack` doesn't earn its keep — with the
  publish-on-state-change predicate, rollback `Activate` emits nothing and
  `InstallFailed.ActiveAfter` carries the rollback outcome. Dropping
  `RolledBack` collapses to the chosen shape; `InstallSucceeded` reads wrong
  for `tsuku rollback` (where the active version changes without any install
  happening).
- **Lifecycle granularity (6 events including `InstallStarted` and
  `InstallCancelled`).** Rejected as premature. Three of six events would
  have no subscriber today. Publisher pairing (Started + terminal) introduces
  new error surface. Lifecycle events can be added later when a real
  telemetry or UI subscriber needs them.

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

1. **Vocabulary** (Decision 1) — three events: `Activated`, `InstallFailed`,
   `Removed` — define the contract every publisher and subscriber speaks.
2. **Semantics** (Decision 2) — synchronous-with-recover delivery means a
   publisher's `Publish` call returns after all subscribers have observed
   the event (or panicked harmlessly), so the on-disk store reflects the
   most recent state mutation by the time `Publish` returns.
3. **Publisher placement** (Decision 3) — explicit `Publish` calls inside
   the six lifecycle methods in `install.Manager` and `updates.CheckAndApplySelf`
   make every emission audit-friendly. Source is threaded through method
   parameters so the publisher knows who triggered it.
4. **Wiring** (Decision 4) — a single helper in `cmd/tsuku` constructs the
   bus, registers the notices subscriber, and threads the bus into the
   manager and self-update path. Tests construct their own bus per subject.

The motivating bug (drift between notice files and `state.json`) is fixed
structurally rather than by read-time filtering: every state mutation
publishes; the notices subscriber writes or removes files in response;
the renderer becomes a dumb consumer of a now-consistent store. Future
classes of drift (notice for a removed tool, notice after rollback, notice
from a cancelled install) are handled by the same event flow without
adding filter clauses.

Two implementation concerns surface from cross-validation:

1. **`Source` threading.** Decision 1's `Source` enum requires every caller
   of `Manager.Install` / `Manager.Activate` / `Manager.Remove*` to pass a
   `Source` value. This is a method-signature change touching ~10 call
   sites. Manageable but explicit work.
2. **Activate predicate as load-bearing invariant.** Rollback correctness
   depends on `Manager.Activate` publishing only when state actually
   changes. The predicate is a single `if` but contributors must respect
   it. Mitigation: a unit test that asserts no event for a no-op Activate.

## Solution Architecture

### Overview

A new package `internal/installevents` defines the three event types
(`Activated`, `InstallFailed`, `Removed`), the `Source` enum, and a small
`Bus` that delivers events synchronously to registered subscribers. The
existing `internal/notices` package gains a `Subscriber` that translates
events into `WriteNotice` / `RemoveNotice` calls on the file store. The
existing `internal/updates/notify.go` renderer is unchanged structurally —
it continues to read notice files — but now reads a store kept consistent
by event reactions instead of one written directly from N call sites.

The bus is a process-local value, not a global. It's constructed once in
`cmd/tsuku` and threaded into the subsystems that publish (`install.Manager`
and `updates.CheckAndApplySelf`). The same wiring runs in the auto-apply
subprocess (a re-invocation of the same binary), so foreground and
background paths leave the on-disk notice store in identical shape.

### Components

```
+-----------------------------------+
|         cmd/tsuku/main.go         |
|                                   |
|  bus := installevents.NewBus(log) |
|  bus.Subscribe(                   |
|    "notices",                     |
|    notices.NewSubscriber(noticesDir),
|  )                                |
|  mgr := install.NewManager(       |
|    cfg, install.WithEventBus(bus),|
|  )                                |
|  updates.CheckAndApplySelf(       |
|    ..., updates.WithEventBus(bus),|
|  )                                |
+-----------------+-----------------+
                  |
                  | (bus value, not global)
                  v
+-----------------------------------+      +-----------------------------+
|       internal/install/           |      |  internal/updates/          |
|                                   |      |    apply.go, self.go        |
|  Manager.Install/Activate/Remove* |      |                             |
|    -> bus.Publish(Activated|...)  |      |  CheckAndApplySelf          |
|                                   |      |    -> bus.Publish(...)      |
+-----------------+-----------------+      +--------------+--------------+
                  |                                       |
                  +-----------------+---------------------+
                                    |
                                    v
                  +----------------------------------+
                  |   internal/installevents.Bus     |
                  |                                  |
                  |  Publish(event) {                |
                  |    for sub in subs (in order) {  |
                  |      defer recover; sub.Handle() |
                  |    }                             |
                  |    flush queued nested events    |
                  |  }                               |
                  +-----------------+----------------+
                                    |
                                    v
                  +----------------------------------+
                  |  internal/notices.Subscriber     |
                  |                                  |
                  |  Handle(Activated)   -> Write    |
                  |  Handle(InstallFailed) -> Write  |
                  |  Handle(Removed)     -> Remove   |
                  +-----------------+----------------+
                                    |
                                    v
                  +----------------------------------+
                  |  $TSUKU_HOME/notices/<tool>.json |
                  +-----------------+----------------+
                                    |
                                    v
                  +----------------------------------+
                  |  internal/updates/notify.go      |
                  |  (renderer — unchanged in shape) |
                  +----------------------------------+
```

### Key Interfaces

```go
// Package internal/installevents

type Subscriber interface {
    Handle(event Event) // event is one of Activated, InstallFailed, Removed
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

// Source threads through method signatures. See "Source threading" below.
```

The subscriber side (in `internal/notices`):

```go
type Subscriber struct {
    dir string // notices directory
}

func NewSubscriber(dir string) *Subscriber

func (s *Subscriber) Handle(event installevents.Event) {
    switch e := event.(type) {
    case installevents.Activated:
        // WriteNotice's atomic rename overwrites any prior notice file in
        // place; no explicit RemoveNotice needed (avoids a brief no-file
        // window where a concurrent reader would see nothing).
        _ = WriteNotice(s.dir, &Notice{
            Tool:             e.Tool,
            AttemptedVersion: e.ToVersion,
            Kind:             kindFor(e.Source),
            Timestamp:        e.Timestamp,
            Shown:            false,
        })
    case installevents.InstallFailed:
        prior, _ := ReadNotice(s.dir, e.Tool)
        consec := 1
        if prior != nil {
            consec = prior.ConsecutiveFailures + 1
        }
        _ = WriteNotice(s.dir, &Notice{
            Tool:                e.Tool,
            AttemptedVersion:    e.AttemptedVersion,
            Error:               sanitizeError(e.Err), // newline -> " / ", truncated to 512 bytes
            Kind:                kindFor(e.Source),
            ConsecutiveFailures: consec,
            Timestamp:           e.Timestamp,
            Shown:               false,
        })
    case installevents.Removed:
        _ = RemoveNotice(s.dir, e.Tool)
    }
}
```

### Data Flow

#### Successful manual update

1. User runs `tsuku update niwa`.
2. `cmd/tsuku/update.go` calls `mgr.Install("niwa", target, ..., SourceManualUpdate)`.
3. `Manager.Install` writes state.json with new active version, then
   `bus.Publish(installevents.Activated{Tool: "niwa", From: "0.11.0", To: "0.11.1", Source: SourceManualUpdate})`.
4. Bus invokes notices subscriber synchronously.
5. Subscriber removes any prior notice for "niwa" and writes a new success
   notice with `Shown: false`.
6. Update command exits. Next foreground command prints the success banner
   and marks `Shown: true`.

#### Failed auto-apply with rollback

1. Background auto-apply iterates pending entries.
2. For entry "niwa@0.11.1": calls `mgr.Install("niwa", "0.11.1", ..., SourceAutoUpdate)`. Install fails before state.json is touched.
3. `Manager.Install`'s failure path: `bus.Publish(installevents.InstallFailed{Tool: "niwa", AttemptedVersion: "0.11.1", ActiveAfter: "0.11.0", Err, Source: SourceAutoUpdate})`.
4. Subscriber writes a failure notice with `ConsecutiveFailures` incremented.
5. Auto-apply loop performs rollback: `mgr.Activate("niwa", "0.11.0", SourceRollback)`.
6. `Manager.Activate` checks: oldActive == "0.11.0", newActive == "0.11.0".
   No state change → no Publish call.
7. On disk: a single failure notice for "niwa@0.11.1". No stale success
   notice. The motivating bug is structurally impossible.

#### Self-update

1. `updates.CheckAndApplySelf` succeeds in replacing the binary.
2. Publishes `installevents.Activated{Tool: "tsuku", From: "0.11.0", To: "0.11.1", Source: SourceSelf}`.
3. Subscriber writes a notice for "tsuku" the same way as for any tool.
4. Renderer's existing `Tool == SelfToolName` branch already formats this
   distinctly ("tsuku has been updated to 0.11.1"); no renderer change.

#### Remove

1. User runs `tsuku remove niwa`.
2. `Manager.RemoveAllVersions("niwa", SourceInstall)` mutates state.
3. Publishes `installevents.Removed{Tool: "niwa", Version: "", ActiveAfter: "", Source: SourceInstall}`.
4. Subscriber removes any notice file for "niwa". The store can never show
   a banner for a tool the user no longer has.

### Source threading

Three options were considered for how `Source` reaches `Manager.Install`,
`Manager.Activate`, and `Manager.Remove*`:

- **Method parameter** — `Activate(tool, version string, src Source)`.
  Touches every call site. Most explicit. Slightly asymmetric with the
  existing `InstallWithOptions(opts InstallOpts)` shape.
- **Options struct extension** — add `Source` to `InstallOpts`, introduce
  `ActivateOpts` and `RemoveOpts`. Consistent, more boilerplate per call site.
- **`context.Context` value** — read Source from a context value.
  Idiomatic for cross-cutting concerns but invisible at the call site, and
  Source is not a cross-cutting concern; it's a first-class argument.

**Chosen: mixed — extend the existing `InstallOpts` with `Source`, and add a
`Source` parameter to `Manager.Activate` and the two `Remove*` methods.**
`InstallOpts` already carries six fields, so adding a seventh stays
idiomatic. `Activate` and `Remove*` carry no options today, and introducing
a one-field options struct for the sole purpose of attribution would
double the call-site churn for no current benefit. Migrating to options
structs is a non-breaking change later if a second field appears.

**Source defaults and validation.**

- `DefaultInstallOptions()` sets `Source: SourceInstall`, the value for a
  fresh `tsuku install <tool>`. Manual update callers replace it with
  `SourceManualUpdate`; the auto-apply loop replaces it with
  `SourceAutoUpdate`; the rollback path uses `SourceRollback`.
- An empty `Source` at publish time is a bug. The bus validates at the
  publish site: if `event.Source == ""`, it logs a warning naming the
  publisher and discards the event.
- `Source` is not validated against `Tool`. A contributor passing
  `SourceSelf` for a non-`tsuku` tool produces a structurally valid event
  that the notices subscriber handles identically to any other Source.
  This is deliberate: the renderer keys off `Tool`, not `Source`, so
  mislabeling is a logic bug a code review catches, not a security flaw.
  A unit test asserts this property so future refactors that switch
  rendering on `Source` find the dependency.

### Source enum stability contract

`Source` values are first-party identifiers chosen by tsuku code. They
must remain non-PII and non-attacker-influenced strings. They are not
currently rendered in user-facing notice files; a future change that
exposes them in notice output must preserve that property. A code comment
on the enum definition reinforces this.

### `kindFor(Source)` mapping contract

`kindFor` maps `Source` to `Kind` for the notice subscriber's writes.
Today the mapping is `SourceAutoUpdate -> KindAutoApplyResult`, all
others `-> KindUpdateResult`. Both kinds use the same persistent display
behavior in the renderer. **Future extensions of `kindFor` must not map
any `Source` to a single-view kind** (`KindVersionFallback`,
`KindShellInitChange`), because doing so would make the publisher the
deciding party for whether a notice persists. A unit test asserts that
the set of kinds emitted by `kindFor` is a subset of the persistent
kinds.

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
fixing: subscribers would see "Activated to X" while state still says Y,
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

Four sequential phases. The design fits one PR end-to-end; phase
boundaries are advisory for commit organization, not commit gates. The
two rewiring phases (publishers and wiring) collapse into a single
commit so the binary never spends time between "old direct writes
removed" and "bus actually wired" — that intermediate would silently
drop notices.

### Phase 1: Event package and bus

Create `internal/installevents/` with:
- `events.go` — `Source`, `Activated`, `InstallFailed`, `Removed`, sealing
  via the unexported `isInstallEvent()` method. Code comment on `Source`
  reinforcing the non-PII contract.
- `bus.go` — `Bus`, `NewBus(cfg)`, `NewBusForTest(io.Writer)`,
  `Subscribe`, `Publish` with deterministic ordering, defer-recover,
  re-entrancy queue, depth cap (16), and queue-size cap (1024). Empty
  `Source` at publish time logs and drops.
- `bus_test.go` — covers: subscriber receives event in registration order,
  panicking subscriber is recovered and logged, re-entrant publish queues
  then flushes, depth cap drops at 16, queue-size cap drops at 1024,
  nil-safe Publish on a nil bus is a no-op, empty Source is dropped with
  a log line.

Deliverables:
- `internal/installevents/events.go`
- `internal/installevents/bus.go`
- `internal/installevents/bus_test.go`

### Phase 2: Notices subscriber

Create `internal/notices/subscriber.go` and `subscriber_test.go`:
- `NewSubscriber(dir string)` and `Handle(event)` method honoring the
  subscriber-locality contract (only mutates `<dir>/<event.Tool>.json`).
- `kindFor(source) -> Kind` helper, with the persistent-kinds contract
  enforced by a unit test.
- `sanitizeError(err) string` helper: newline normalization and 512-byte
  truncation. Unit test asserts no output contains `\n`.
- Subscriber tests cover all three events against a temp directory,
  including the `ConsecutiveFailures` increment on repeated
  `InstallFailed`.

Also harden `internal/notices`:
- Apply the same tool-name validation in `RemoveNotice` that
  `WriteNotice` already performs (defense in depth).

Deliverables:
- `internal/notices/subscriber.go`
- `internal/notices/subscriber_test.go`
- `internal/notices/notices.go` (RemoveNotice validation)

### Phase 3: Publisher integration in `install.Manager`

Modify `internal/install/`:
- Add `bus *installevents.Bus` field to `Manager`, `WithEventBus(bus)` option.
- Extend `InstallOpts` with `Source`; `DefaultInstallOptions()` sets
  `Source: SourceInstall`.
- Add `Source` parameter to `Manager.Activate`, `Manager.RemoveVersion`,
  `Manager.RemoveAllVersions`.
- In `Manager.Install` / `InstallWithOptions`: success path publishes
  `Activated` **after** the state write; failure path publishes
  `InstallFailed`. A `defer` at function entry with a named-return
  `success bool` is the natural pattern for ensuring exactly one
  terminal event regardless of return path.
- In `Manager.Activate`: the existing early-return at the no-op case
  (`manager.go:257`, `if toolState.ActiveVersion == version { return nil }`)
  doubles as the publish predicate. The bus.Publish call sits **after**
  the state write and runs only when state actually changed. The same
  early-return that prevents redundant symlink work prevents redundant
  events, so the predicate is structurally durable rather than a
  contributor-discipline matter.
- In `Manager.RemoveVersion`, `Manager.RemoveAllVersions`: publish
  `Removed` after state mutation.
- Each `bus.Publish` site has a code comment naming the
  publish-after-state invariant.

Test additions:
- `manager_test.go`: success and failure publish events; no-op Activate
  publishes nothing; remove publishes; nil bus is safe; the in-event
  state read sees the post-write state.

Deliverables:
- `internal/install/state.go`, `manager.go`, `update.go`, `remove.go`
  modifications.
- Updated `manager_test.go`.

### Phase 4: Self-update + call-site rewiring + bus wiring (single commit)

This phase must happen in a single commit. Splitting it leaves the
binary in a state where the old direct writes are gone but the bus
isn't wired yet, silently dropping notices.

Add `cmd/tsuku/events_wiring.go`:
- `newEventBus(cfg) *installevents.Bus` constructs the bus, calls
  `Subscribe("notices", notices.NewSubscriber(notices.NoticesDir(cfg.HomeDir)))`,
  and returns it.
- The Cobra root command (or equivalent setup hook) constructs the bus
  and threads it into `install.NewManager` and `updates.CheckAndApplySelf`.
- **`cmd/tsuku/cmd_apply_updates.go` (the auto-apply subprocess
  subcommand) independently constructs its own bus and threads it.**
  This is explicit; do not rely on `PersistentPreRun` inheritance.

Modify `internal/updates/`:
- Add a `*installevents.Bus` parameter to `CheckAndApplySelf` and
  `ApplySelfUpdate`. Self-update success publishes
  `Activated{Tool: "tsuku", Source: SourceSelf}`; failure publishes
  `InstallFailed{Tool: "tsuku", Source: SourceSelf}`. New behavior:
  self-update failures become visible.
- Remove direct `notices.WriteNotice` / `notices.RemoveNotice` calls in
  `apply.go`, `self.go`, and `cmd/tsuku/update.go`. The
  `install.Manager` publishers now cover those notices.
- The renderer in `notify.go` stays as-is; it reads a now-consistent
  store.

Validation:
- Existing `internal/updates/notify_test.go` tests still pass.
- **Acceptance: an end-to-end integration test seeds `state.json`,
  drives the failure-then-rollback scenario through the auto-apply
  subprocess, and asserts the on-disk store reflects only
  `InstallFailed`.** This test is non-negotiable — without it, a
  forgotten wiring in either entry point is silent.

Deliverables:
- `cmd/tsuku/events_wiring.go`
- `cmd/tsuku/cmd_apply_updates.go` modifications.
- `internal/updates/self.go`, `apply.go` modifications.
- `cmd/tsuku/update.go` modifications.
- End-to-end integration test in a new file under `internal/updates/`
  or `test/functional/`.

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
- Self-update failures become user-visible for the first time. Today they
  log silently to the user-config trace file; with this design they
  produce a notice rendered like any other failure.
- Removing a tool deterministically clears its notices. Today the file is
  orphaned.
- New triggers (`tsuku run` autoinstall, future `tsuku reinstall`) extend
  the Source enum, not the event types.
- Auditing "what happens when a tool is updated" reduces to reading one
  subscriber file plus the bus's `Publish` signature.

### Negative

- Manager API surface widens. Adding `Source` to `InstallOpts` plus a
  `Source` parameter to `Activate` and `Remove*` is real refactor work
  touching ~10 call sites.
- The "publish after state write" invariant is uncompiled. A contributor
  who moves a publish call above its state write reintroduces the drift
  silently. Per-site code comments and per-method ordering tests are
  the only enforcement.
- The notice store has no garbage collection. Every tool that ever
  failed to update leaves a notice file until manually cleared or
  superseded by a later event. Today's behavior is the same, but the
  event bus opens more publishers and so the store grows faster over
  time.
- Adding a subscriber requires updating the wiring helper. For one
  subscriber this is trivial; if the subscriber set grows, the wiring
  helper grows linearly.
- The bus does not fix the existing `MarkShown` race: a foreground
  render reads `Shown: false`, prints, and marks `Shown: true`; a
  concurrent fresh write between read and mark loses the print. This
  race pre-exists; the structural fix is orthogonal.

### Mitigations

- The Source-threading refactor is bounded and one-time. Once done,
  adding a new trigger is a single Source-constant change plus the
  caller wiring; no further API churn.
- The Activate publish predicate piggybacks on the existing no-op
  early-return in `manager.go:257`. The same check that prevents
  redundant symlink work prevents redundant events, making the
  invariant structurally durable rather than discipline-dependent.
- The wiring helper file (`cmd/tsuku/events_wiring.go`) is named to be
  obviously where subscribers live; a reader looking for "what happens
  on Activated" finds it in one grep.
- The acceptance integration test for the failure-then-rollback path
  catches a forgotten wiring in either entry point. The nil-safe
  `Publish` keeps unit-test setup small without silencing the
  acceptance test.
- Notice-store GC is a separate concern tracked as future work; this
  design does not block on it.
- A future per-tool advisory lock (or a CAS-like "write if not shown"
  semantic) can address the `MarkShown` race without changing the
  event bus contract.
