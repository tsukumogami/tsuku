# Decomposition: notices-install-event-bus

## Strategy
**Horizontal.** This is a refactor with a clear prerequisite chain
(foundation → subscribers → publishers). The design itself lays out
three phases in horizontal order; not enough cross-layer integration
risk to justify a walking-skeleton vertical slice.

## Execution mode
**single-pr.** User explicitly requested via `--single-pr`. The
implementer commits each issue separately within one PR.

## Issues

### <<ISSUE:1>> — feat(installevents): add event types and bus
**Complexity:** testable
**Dependencies:** None
**Rough scope:** New package `internal/installevents/` with 8 event types
(`Installed`, `Updated`, `RolledBack`, `Removed`, plus the four failure
variants), `Source` enum, sealing via `isInstallEvent()`, `Bus` type with
synchronous-with-recover delivery, depth and queue caps, deterministic
ordering, re-entrancy queue. Pure foundation — no callers yet.

### <<ISSUE:2>> — refactor(notices): add Verb field and verb-aware renderer
**Complexity:** testable
**Dependencies:** None (parallel with <<ISSUE:1>>)
**Rough scope:** Add `Verb` field to `Notice` struct (kebab-case verb
name, `omitempty`). Update `internal/updates/notify.go` renderer to
format per-verb messages (install / update / rollback / remove). Empty
`Verb` preserves today's "X has been updated to Y" phrasing for
backward compatibility. Extend tool-name validation in `RemoveNotice`
to match `WriteNotice` (defense-in-depth hardening called out in the
design's Security Considerations).

### <<ISSUE:3>> — feat(subscribers): add notices and telemetry subscribers
**Complexity:** testable
**Dependencies:** <<ISSUE:1>>, <<ISSUE:2>>
**Rough scope:** Two new subscribers, both in their existing packages:
- `internal/notices/subscriber.go` — translates each of the 8 events to
  `WriteNotice` (with `Verb` field set) or `RemoveNotice`. Includes
  `sanitizeError` helper (newline normalization + 512-byte truncation).
  Honors the subscriber-locality contract (only touches the file for
  the event's `Tool`).
- `internal/telemetry/subscriber.go` — translates events to
  `tc.SendInstallOutcome` / `SendUpdateOutcome` / `SendRollbackOutcome`
  / `SendRemoveOutcome`. Detects auto-recovery on `UpdateFailed`
  (`ActiveAfter == FromVersion && FromVersion != ""` → emit both
  failure and rollback events).

Both subscribers can be written and tested in isolation; neither has
runtime effect until subscribed to a live bus in <<ISSUE:4>>.

### <<ISSUE:4>> — refactor(install): publish lifecycle events, rewire telemetry and notices
**Complexity:** testable
**Dependencies:** <<ISSUE:1>>, <<ISSUE:3>> (transitively <<ISSUE:2>>)
**Rough scope:** Single commit per the design. Three sub-pieces that
must land together because removing the direct writes before the bus
is wired silently drops emissions:

1. **Manager refactor.** Add `bus *installevents.Bus` field and
   `WithEventBus` option. Add `Source` parameter to `Install`,
   `RemoveVersion`, `RemoveAllVersions`. New public method
   `Manager.Rollback(tool, toVersion, src)` that publishes
   `RolledBack` / `RollbackFailed`. **Move auto-rollback inside
   `Manager.Install`**: on update failure, internally activate the
   prior version before publishing `UpdateFailed{ActiveAfter: ...}`.
   Per-method publish sites carry the publish-after-state invariant
   comment.
2. **Publisher rewiring.** Remove direct `notices.WriteNotice` and
   `tc.SendUpdateOutcome` calls in `internal/updates/apply.go`,
   `internal/updates/self.go`, `internal/updates/trigger.go`, the
   manual update path in `cmd/tsuku/update.go`, and self-update path.
   Thread the appropriate `Source` value into Manager calls from every
   call site (CLI commands, `internal/autoinstall/`, auto-apply loop).
3. **Bus wiring.** New file `cmd/tsuku/events_wiring.go` with
   `newEventBus(cfg, tc)` that constructs the bus and subscribes both
   subscribers. Wired explicitly in both the foreground root command's
   setup and `cmd/tsuku/cmd_apply_updates.go` (the auto-apply
   subprocess) — do not rely on Cobra `PersistentPreRun` inheritance.

**Acceptance tests** (non-negotiable):
- End-to-end: seed `state.json`, drive a failure-then-rollback through
  the auto-apply subprocess, assert exactly one `UpdateFailed`-derived
  notice on disk AND exactly two telemetry emissions
  (`UpdateOutcomeFailure` + `UpdateOutcomeRollback`).
- End-to-end: trigger a project-auto install (e.g., via `tsuku run`
  with `.tsuku.toml`), assert one notice with `Verb: install` and one
  telemetry emission with `trigger: project-auto`.

## Out of scope (for clarity)

- Started events (`InstallStarted`, `UpdateStarted`, etc.) — design
  notes as future opportunity.
- Notice-store GC — separate concern, tracked as future work in
  Consequences.
- `MarkShown` write-clobber race — pre-existing; orthogonal to this
  refactor.
- Library install events (`InstallLibrary`) — separate state model;
  out of scope per design.
