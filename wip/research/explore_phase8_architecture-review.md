# Architecture Review: Pipeline Dashboard Overhaul

**Design:** `docs/designs/DESIGN-pipeline-dashboard-overhaul.md`
**Reviewer:** architect-reviewer
**Date:** 2026-02-22

---

## 1. Is the architecture clear enough to implement?

**Verdict: Yes, with two gaps to close.**

The design specifies four layers (orchestrator, dashboard generator, JSON schema, HTML/JS/CSS) and maps each decision to concrete code locations. The pseudocode for `selectCandidates()` in the Solution Architecture section (lines 206-236) is detailed enough to implement directly. The `QueueStatus` struct extension and JSON schema additions are shown with types and examples.

### Gap 1: Ecosystem extraction is already available but the design doesn't mention it

The design says `ByEcosystem` should be "computed from the priority queue during dashboard loading." The `QueueEntry.Ecosystem()` method already exists at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/batch/queue_entry.go:122` and extracts the ecosystem prefix from `Source`. The design should reference this directly rather than leaving implementers to rediscover it. Not blocking -- just a documentation gap.

### Gap 2: The `excluded` status is missing from the JSON schema example

The design's JSON schema example at line 259-272 shows `pending`, `failed`, `success`, `blocked`, `requires_manual`, and `total` per ecosystem. But the existing `QueueEntry` status constants (defined at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/batch/queue_entry.go:47-54`) include `StatusExcluded = "excluded"`. The schema example should include `excluded` or the design should state it's intentionally omitted. If excluded entries aren't counted, the per-ecosystem totals won't match the global totals, which could confuse dashboard consumers.

**Advisory.** The schema example is illustrative, and the actual implementation will iterate over `queue.Entries` and count whatever statuses exist. But it should be called out.

---

## 2. Are there missing components or interfaces?

### 2a. `computeQueueStatus` needs modification, and the design doesn't specify how

The design says to add `ByEcosystem` to `QueueStatus` and "compute ecosystem breakdown during queue loading." The existing `computeQueueStatus()` function at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/dashboard/dashboard.go:348-401` already iterates over all queue entries to build `ByStatus`, `ByTier`, and `Packages`. Adding `ByEcosystem` is a natural extension of the same loop -- one additional map operation per entry. This fits the existing pattern perfectly.

**No structural concern.** The implementation slot is clear.

### 2b. No `total` field in the per-ecosystem breakdown

The existing `QueueStatus` has a `Total` field (line 103). The design's JSON schema shows a `total` per ecosystem (line 269), but the Go struct proposal at line 248 types `ByEcosystem` as `map[string]map[string]int`. If `total` is a derived field (sum of all statuses), it could be computed in JS. If it's a Go-computed field, the inner map needs to include it. This is a minor implementation detail but worth calling out for consistency with the dashboard JS code that will consume it.

**Advisory.** Decide where `total` per ecosystem is computed (Go generator or JS consumer) and document it.

### 2c. Ecosystem Health widget data -- no new backend fields needed

The design correctly identifies at line 308 that the Ecosystem Health widget uses existing `health.ecosystems` data. I verified this: `EcosystemHealth` at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/dashboard/dashboard.go:74-79` already has `BreakerState`, `Failures`, `LastFailure`, and `OpensAt`. The "time until recovery" mentioned in the design (line 93) would be computed in JS from `OpensAt`, which is already an ISO timestamp. No backend change needed.

**No concern.**

### 2d. Missing: how `selectCandidates` interacts with `FilterEcosystem`

The current `selectCandidates()` has a `FilterEcosystem` config option (line 232-234) that restricts selection to one ecosystem. The design's pseudocode doesn't mention this. When `FilterEcosystem` is set and the filtered ecosystem is half-open, the two-pass logic should still apply. The implementation needs to handle this interaction, but it's straightforward -- the filter applies before the half-open logic.

**Advisory.** Add a note about `FilterEcosystem` interaction in the design, or leave it as an implementation detail with a test case.

---

## 3. Are the implementation phases correctly sequenced?

**Verdict: Yes, with one sequencing concern.**

### Phase ordering is correct

- Phase 1 (circuit breaker fix) has no dependencies. Correct.
- Phase 2 (dashboard backend) is independent of Phase 1. Correct -- they modify different files (`orchestrator.go` vs `dashboard.go`).
- Phase 3 (new widgets) depends on Phase 2 for `by_ecosystem` data. Correct.
- Phases 4 and 5 (readability, bug fixes) are frontend-only. Correct.
- Phase 6 (tests) validates everything. Correct final phase.

### Concern: Phase 5 (bug fixes) may conflict with Phase 3/4

The design acknowledges overlap between bug fixes and widget changes (line 145). Phases 3, 4, and 5 all modify `website/pipeline/index.html` and other overlapping HTML files. If implemented as separate PRs, they will conflict. The design suggests a single PR (line 446) which resolves this, but the phase numbering implies separate implementation steps.

**Advisory.** If phases are implemented as separate commits within a single PR, the sequencing works. If they become separate PRs, Phases 3-5 should be merged into a single "frontend overhaul" phase to avoid conflicts.

---

## 4. Are there simpler alternatives we overlooked?

### 4a. The two-pass approach in `selectCandidates` could be single-pass

The design proposes a two-pass approach: first pass collects pending entries per half-open ecosystem, second pass fills in fallbacks. Looking at the current implementation at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/batch/orchestrator.go:222-260`, it's a single-pass loop. The two-pass approach introduces a `fallback` map and a second iteration.

A simpler alternative: single-pass with deferred selection. During the loop, when a half-open ecosystem is encountered:
- If entry is pending and no probe selected for that ecosystem: select it immediately, mark as probed.
- If entry is pending and a probe is already selected: skip.
- If entry is failed: save as fallback only if no probe yet selected for that ecosystem.
- After the loop: for ecosystems with a fallback but no probe, add the fallback.

This is functionally identical to the design's approach but the "first pass" and "second pass" description makes it sound like two full iterations of the queue. In practice, the design's pseudocode (lines 207-235) already shows a single loop with a post-loop fallback fill-in, which is the same as what I describe. The naming is slightly misleading -- it's a "single pass with deferred fallback," not a true two-pass.

**No concern.** The pseudocode is already the simple approach despite the "two-pass" label.

### 4b. Backoff bypass for probes is the right call

The design correctly identifies that applying entry-level backoff to half-open probes is the root cause of the deadlock. The alternative ("reset backoff timers when transitioning to half-open") was rightly rejected -- it would cause retry storms.

There is one subtle point the design doesn't address: what happens to the probe entry's backoff state after it runs? If the probe (a pending entry) fails, `recordFailure()` at line 264-269 will set `NextRetryAt` on it. That's correct behavior -- the entry should enter normal backoff after being used as a probe. But if the probe is a failed entry (the fallback path), its `FailureCount` will increment again and its `NextRetryAt` will be pushed further out. This is also correct -- the entry failed again, so backoff should increase. No design change needed, just worth noting for test coverage.

### 4c. `ByEcosystem` vs extending `ByTier`

An alternative: instead of a flat `ByEcosystem` map, nest ecosystem breakdown within the existing `ByTier` structure, producing `by_tier -> tier -> ecosystem -> status -> count`. This would answer "which ecosystems are stalled at which priority level?" in a single data structure.

Rejected as over-engineering. The Ecosystem Pipeline widget in the design answers "where is work concentrated?" by ecosystem, not by tier-ecosystem combinations. The flat `ByEcosystem` is simpler and matches the widget's needs. Tier-level ecosystem breakdown can be derived from the `Packages` map if ever needed.

---

## 5. Structural fit assessment

### Fits the existing architecture

1. **Dashboard generator pattern**: Adding `ByEcosystem` to `QueueStatus` follows the same aggregation pattern as `ByStatus` and `ByTier` in `computeQueueStatus()`. Same loop, same data source, one more map. No new abstractions needed.

2. **Circuit breaker integration**: The `selectCandidates()` change stays within the orchestrator's existing responsibility. It reads `BreakerState` from config (already populated from `batch-control.json`) and modifies candidate selection logic. No new package imports. No action dispatch bypass.

3. **JSON schema evolution**: Adding `by_ecosystem` to `dashboard.json` is purely additive. Existing frontend code won't break because it doesn't reference the new field. The design explicitly notes this at line 49.

4. **Frontend changes**: The HTML/JS/CSS changes are within `website/pipeline/`, the established location for dashboard pages. No new pages are added (the design correctly notes 12 existing pages cover all views, line 40).

### Potential structural concerns

1. **State contract**: The new `ByEcosystem` field in `QueueStatus` will be serialized to `dashboard.json`. The design specifies that the Ecosystem Pipeline widget will consume it (line 95). As long as the widget is implemented in the same PR as the backend change, no contract drift occurs. If the backend ships without the widget, the field would be a dead field in the schema.

   **Blocking if phases ship independently.** Phase 2 (backend) must ship with or before Phase 3 (widgets). The design's phase ordering already ensures this.

2. **No parallel pattern introduction**: The design doesn't introduce any parallel dispatching, configuration, or data flow patterns. All changes extend existing mechanisms.

3. **Dependency direction**: `internal/dashboard/` imports `internal/batch/` (for `UnifiedQueue`, `QueueEntry`). This is the existing dependency direction and won't change. No inverse dependency is introduced.

---

## 6. Test coverage assessment

The existing test files are solid:

- `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/batch/orchestrator_test.go`: 20+ tests covering `selectCandidates` with backoff, breaker states, ecosystem filtering, batch size limits. The half-open test at line 252-289 tests the current (buggy) behavior. New tests needed:
  - Half-open with all entries in backoff (current deadlock scenario)
  - Half-open with mix of pending and failed entries (pending-first preference)
  - Half-open fallback to failed entry when no pending entries exist
  - Half-open with `FilterEcosystem` set

- `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/dashboard/dashboard_test.go`: 40+ tests covering `computeQueueStatus`, `loadHealth`, `computeTopBlockers`, integration. New tests needed:
  - `ByEcosystem` aggregation correctness
  - `ByEcosystem` with mixed ecosystems
  - `ByEcosystem` empty queue edge case

The design's Phase 6 (validation tests) correctly targets Go tests for data contracts and shell-based link checking. This matches the project's existing test patterns (Go unit tests, no browser testing).

---

## Summary of findings

| Finding | Severity | Location |
|---------|----------|----------|
| `excluded` status missing from JSON schema example | Advisory | Design doc line 259-272 |
| `FilterEcosystem` interaction with half-open probes not discussed | Advisory | Design doc pseudocode lines 207-235 |
| Per-ecosystem `total` field -- compute in Go or JS? | Advisory | Design doc line 269 vs Go struct line 248 |
| Phases 3-5 may conflict if implemented as separate PRs | Advisory | Design doc lines 345-393 |
| ByEcosystem field ships without consumer if phases deploy independently | Blocking if phases ship independently | Phase 2 vs Phase 3 coupling |

**Overall: The design fits the existing architecture.** All proposed changes extend existing patterns (orchestrator candidate selection, dashboard aggregation, JSON schema, HTML widgets). No parallel patterns, no dependency inversions, no action dispatch bypasses. The implementation is feasible with the current codebase structure.
