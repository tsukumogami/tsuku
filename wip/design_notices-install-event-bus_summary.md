# Design Summary: notices-install-event-bus

## Input Context (Phase 0)
**Source:** Freeform topic (user-provided via `/shirabe:design` args)
**Problem:** Notice files drift from `state.json` because multiple call sites write directly without a shared contract. Render-time filtering (attempted in PR #2411) catches one drift class; the proper fix is source-side event emission with `internal/notices` as a listener.
**Constraints (user-confirmed):**
1. In-process pub/sub only; filesystem is the inter-process sync point. Every process wires the full subscriber set.
2. No migration — user will manually clean stale notice files post-release.
3. Notices remain best-effort; a failed subscriber must not block install success.

## Decisions the design must make
1. Event vocabulary (minimal vs. expressive)
2. Bus semantics (synchronous vs. asynchronous delivery)
3. Emitter location (state.json shim vs. explicit publish at each site vs. defer pattern)
4. Subscriber registration (single init in main.go vs. per-package init() functions)
5. `internal/notices` subscriber behavior — which events trigger which file mutations
6. Self-update treatment (same bus + vocabulary as tool updates, or separate)
7. Implementation sequencing (which call sites switch over in what order)

## Out of scope
- Per-user notice preferences / muting
- IPC between processes
- Migration of existing notice files
- Renderer output-format changes (it only sees a now-consistent store)

## Working branch
`design/notices-install-event-bus`

## Operator note
User wants the full workflow driven to Accepted status if possible. Flag any decisions where I want them to weigh in rather than deciding silently.

## Security Review (Phase 5)
**Outcome:** Option 2 — document considerations
**Summary:** Low-severity, security-benign design. Two hardening items folded in: extend `..`/separator validation to `RemoveNotice`, and cap bus re-entrancy (depth 16 + queue size 1024).

## Phase 6 Reviews
**Architecture review:** 12 recommendations applied — defined Logger, named auto-apply subprocess wiring explicitly, merged Phase 4+5 to avoid broken intermediate, specified Source defaults, downgraded options struct to Source param for Activate/Remove*, clarified InstallLibrary out-of-scope.
**Security pass 2:** 6 DESIGN-CHANGE items applied — config-bound logger destination (no caller-passed Logger), queue-size cap, error sanitization (newline normalization + 512-byte truncation), publish-after-state invariant documented, init()-rejection trust-boundary caveat, subscriber-locality contract.

## Revision after first review (2026-05-16)
User pushed back on event granularity. Reasoning: notices and telemetry have different needs, and the previous 3-event vocabulary (Activated/InstallFailed/Removed with Source carrying verb intent) forced both subscribers to infer verb from field shape. New vocabulary is verb-per-event (8 typed events: Installed/Updated/RolledBack/Removed + Failed variants) with Source as an orthogonal tag. Telemetry refactor expanded into scope: `internal/telemetry` becomes a subscriber, replacing direct `tc.SendUpdateOutcome` calls in `apply.go`. `Manager.Install` now owns its own failure-recovery (auto-rollback inside a failed update happens inside Install, so failure events carry post-recovery state in `ActiveAfter`).

## Current Status
**Phase:** 6 - Finalize (revised doc, ready to commit follow-up)
**Last Updated:** 2026-05-16
