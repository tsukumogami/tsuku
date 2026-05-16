# Plan Analysis: notices-install-event-bus

## Source Document
Path: docs/designs/DESIGN-notices-install-event-bus.md
Status: Accepted
Input Type: design

## Scope Summary
Introduce an in-process install lifecycle event bus with verb-per-event vocabulary; make `internal/notices` and `internal/telemetry` both subscribers; refactor `install.Manager` to own auto-rollback so failure events carry post-recovery state.

## Components Identified
- **`internal/installevents` (new package)**: 8 event types, `Source` enum, `Bus` with synchronous-with-recover delivery, depth + queue caps.
- **`internal/notices.Subscriber` (new)**: translates events to `WriteNotice`/`RemoveNotice`. Notice schema gains `Verb` field.
- **`internal/telemetry.Subscriber` (new)**: translates events to `tc.SendUpdateOutcome` etc. Replaces direct telemetry calls in `apply.go`.
- **`internal/updates/notify.go` (modified)**: verb-aware rendering (reads `Notice.Verb`).
- **`install.Manager` (modified)**: gains bus field; `Source` parameter added to public lifecycle methods; new `Manager.Rollback`; auto-rollback moves inside `Manager.Install`.
- **Publisher rewiring in `internal/updates/apply.go`, `internal/updates/self.go`, `cmd/tsuku/*.go`, `internal/autoinstall/`**: remove direct notice/telemetry writes; pass `Source` into manager methods.
- **`cmd/tsuku/events_wiring.go` (new)**: constructs the bus, registers subscribers, threads bus into manager and self-update. Wired in both the foreground root command and the auto-apply subprocess.
- **End-to-end integration tests**: failure-then-rollback flow; project-auto install flow.

## Implementation Phases (from design)
Three phases:
1. **Events package + bus** — foundation; `internal/installevents/` types + bus + tests.
2. **Subscribers + Notice schema update + renderer update** — both subscribers can be built in parallel before publishers land because neither has runtime effect until subscribed. Notice schema gains `Verb`; renderer reads it.
3. **Manager refactor + publisher rewiring + bus wiring** — single commit. Removes direct `notices.WriteNotice` and `tc.SendUpdateOutcome` calls; adds publishers in Manager; wires bus in `cmd/tsuku` (including auto-apply subprocess). Acceptance integration tests are non-negotiable.

## Success Metrics
- Notices store reflects reality by construction; PR #2411 bug class cannot recur.
- Notices and telemetry are consistent for every lifecycle operation (no current mismatches).
- Self-update failures and project-auto installs become observable for the first time.

## External Dependencies
- None. Everything is internal to the tsuku binary.
