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

## Current Status
**Phase:** 0 - Setup complete (skeleton written, ready for Phase 1)
**Last Updated:** 2026-05-16
