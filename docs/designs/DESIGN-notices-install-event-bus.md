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

_Filled in during Phase 2/3._

## Decision Outcome

_Filled in during Phase 4._

## Solution Architecture

_Filled in during Phase 4._

## Implementation Approach

_Filled in during Phase 4._

## Security Considerations

_Filled in during Phase 5._

## Consequences

_Filled in during Phase 4._
