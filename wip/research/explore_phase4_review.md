# Architect Review: DESIGN-pipeline-dashboard-overhaul

## Scope of Review

Evaluated the design document at `docs/designs/DESIGN-pipeline-dashboard-overhaul.md` against five questions: problem statement specificity, missing alternatives, rejection rationale fairness, unstated assumptions, and strawman options. Verified claims against the current codebase at `internal/batch/orchestrator.go`, `internal/dashboard/dashboard.go`, `internal/batch/queue_entry.go`, `scripts/check_breaker.sh`, and related test files.

---

## 1. Problem Statement Specificity

**Verdict: Strong, with one gap.**

The circuit breaker deadlock is described with root-cause precision. The design correctly identifies the two interacting problems in `selectCandidates()` (lines 222-260 of `internal/batch/orchestrator.go`):

1. Backoff filtering applies to half-open probes (line 247: `if entry.NextRetryAt != nil && entry.NextRetryAt.After(now)`)
2. No preference for pending entries over failed entries in half-open state

Both are verifiable in the code. The deadlock scenario is concrete: all entries failed, all in backoff, probe can't select, breaker stays stuck. This is specific enough to evaluate solutions against.

The dashboard usability problem is also well-grounded, referencing specific issues (#1834-#1838) with a quantified bug count (37 across 12 pages). The three-concern overload in Pipeline Health is described concretely.

**Gap**: The problem statement says "Four of six ecosystems (crates.io, npm, pypi, rubygems) are stuck with open circuit breakers" but doesn't say how long they've been stuck or what the actual impact on pipeline throughput is. This matters because it affects how urgently the fix needs to be phased. The design puts it in Phase 1, which is appropriate, but the urgency isn't justified in the problem statement itself. Minor issue -- the fix is clearly needed regardless.

---

## 2. Missing Alternatives

### Decision 1 (Circuit Breaker Fix)

**One missing alternative worth considering**: Separate probe selection from the main `selectCandidates()` function entirely. Instead of modifying the existing function's loop with special-case logic for half-open state, add a dedicated `selectProbes()` that runs before the main selection. This would keep the normal selection path unchanged and make the probe logic independently testable.

This matters architecturally because the current `selectCandidates()` is already doing two things (filtering and limiting), and the proposed change adds a third concern (probe strategy) with a two-pass approach that increases the function's complexity. A separate function would be cleaner. However, the design's chosen approach works and is local to one function, so this is advisory, not blocking.

### Decision 2 (Dashboard Widgets)

No missing alternatives. The two rejected options (tabs, stacked bars) cover the obvious alternatives. The three-widget split is a straightforward decomposition.

### Decision 3 (Readability)

**One missing alternative**: Use UTC with explicit "UTC" label instead of hardcoding ET. The design rejects browser-locale detection, which is reasonable, but doesn't consider the simpler option of just showing UTC (which `dashboard.json` already stores) with a clear label. UTC is timezone-neutral and eliminates DST ambiguity. The design's rationale for ET is that "the operator team is in ET," but this is a single-operator assumption that may not hold as the project grows. Advisory -- not blocking since ET is easy to change later.

### Decision 5 (Tests)

No missing alternatives. The rejected options (Playwright, JSON Schema) are reasonable to exclude, and Go tests + shell checks align with existing test patterns in the codebase.

---

## 3. Rejection Rationale Fairness

### Decision 1: "Reset backoff timers when transitioning to half-open"

**Fair rejection.** The "retry storm" concern is real: if a half-open probe succeeds and the breaker closes, all entries with cleared backoff timers would become immediately eligible. With potentially hundreds of entries per ecosystem, this would create a spike that defeats the purpose of backoff.

### Decision 1: "Add a dedicated probe entry per ecosystem"

**Fair rejection.** This would indeed require schema changes to `QueueEntry` and special handling in the orchestrator. A synthetic entry's success/failure wouldn't reflect real package generation, making it a weak health signal.

### Decision 2: "Keep a single combined panel with tabs"

**Fair rejection.** Tabs do hide information, and the three views answer different questions that operators would want to see simultaneously.

### Decision 2: "Add ecosystem data to existing status bars"

**Fair rejection.** Stacking per-ecosystem data into existing bars would make them unreadable at 6+ ecosystems.

### Decision 3: "Use browser locale for timezone"

**Slightly unfair rejection.** The rationale mentions "server-side rendering, CI environments" as reasons browser locale is unreliable, but the dashboard is explicitly described as a static site with client-side rendering (no SSR). CI environments wouldn't be viewing the dashboard. The real reason to reject browser locale is simpler: operator consistency matters more than per-user customization for an operational dashboard. The stated rationale has a factual inaccuracy, though the conclusion is fine.

### Decision 4: "Address bugs as separate PRs"

**Fair rejection.** File-level conflicts between widget restructuring and bug fixes in the same HTML files would be real. Consolidating is pragmatic.

### Decision 5: "Browser-based end-to-end tests with Playwright"

**Fair rejection.** Introducing a Node.js + Playwright dependency chain for structural validation of a vanilla HTML site is disproportionate.

### Decision 5: "JSON Schema validation"

**Fair rejection.** JSON Schema can't express cross-reference constraints (batch ID format consistency, ecosystem prefix matching), which are the actual bug categories found.

---

## 4. Unstated Assumptions

### Assumption 1: Queue order guarantees pending-first selection

The design's probe selection algorithm says "scan for a StatusPending entry first." This assumes the queue is ordered in a way that makes the first pending entry a good probe candidate. In practice, the queue is sorted by priority then alphabetically (verified in `computeQueueStatus` at `internal/dashboard/dashboard.go:392`). This means the probe for a half-open ecosystem will always be the alphabetically-first pending entry at the highest priority level. This is fine but should be stated -- the design could mention that probe selection is deterministic based on queue order.

### Assumption 2: `by_ecosystem` data is computable from existing queue fields

The design says "computed from the priority queue during dashboard generation." Verified: `QueueEntry.Ecosystem()` at `internal/batch/queue_entry.go:122` extracts the ecosystem from the Source field. The `computeQueueStatus()` function at `internal/dashboard/dashboard.go:348` already iterates all entries and has access to `entry.Ecosystem()`. Adding ecosystem aggregation here is straightforward. **This assumption is correct.**

### Assumption 3: check_breaker.sh transitions are the only path to half-open

The design says probes execute when the breaker is half-open, and `check_breaker.sh` is what transitions from open to half-open. Verified at `scripts/check_breaker.sh:53`: the script uses `jq` to set state to "half-open" when the recovery timeout elapses. But `update_breaker.sh` also exists and could potentially set states. This should be verified -- if there's another path to half-open that doesn't go through `check_breaker.sh`, the probe fix still works (it's based on state, not transition), but the complete picture matters for understanding.

### Assumption 4: The existing `excluded` status doesn't need ecosystem breakdown

The design's `by_ecosystem` schema lists five statuses: pending, failed, success, blocked, requires_manual. The queue also has `excluded` status (defined at `internal/batch/queue_entry.go:53`). This status should either be included in the breakdown or explicitly called out as omitted. If excluded entries exist in the queue but aren't shown in the Ecosystem Pipeline widget, operators won't get the full picture of where entries went.

### Assumption 5: Dashboard HTML changes are CSS/JS only for readability improvements

The design says "Both changes are CSS/JS only -- no backend modifications needed since dashboard.json already contains all the required data" for Decision 3 (readability). This is partially incorrect. The `by_ecosystem` field IS a backend modification needed for the Ecosystem Pipeline widget (acknowledged in Decision 2). The readability changes themselves (timestamp formatting, ecosystem column in failures) are indeed frontend-only, which is correct. This creates a subtle phase dependency: Phase 4 (readability) can proceed independently, but Phase 3 (new widgets) depends on Phase 2 (backend `by_ecosystem` field).

### Assumption 6: Hardcoded ET means Eastern Time without specifying EST vs EDT

The design says "Eastern Time" but doesn't specify whether this accounts for daylight saving time. JavaScript's `toLocaleString` with `America/New_York` handles this automatically, but if the implementation uses a fixed UTC offset, it would be wrong half the year. This is an implementation detail, but the design should specify "America/New_York IANA timezone" rather than "ET" to avoid ambiguity.

---

## 5. Strawman Analysis

**No strawman options detected.** All rejected alternatives are plausible approaches that someone might reasonably propose. Specifically:

- "Reset backoff timers" is a natural first idea when facing the deadlock -- it targets the symptom directly.
- "Dedicated probe entries" is how some circuit breaker implementations work in distributed systems (synthetic health checks).
- "Tabs" is a standard UI pattern for reducing panel count.
- "Browser locale" is the default developer instinct for timezone handling.
- "Playwright" is the standard answer for web testing.
- "JSON Schema" is the standard answer for JSON validation.

Each is rejected on specific, verifiable grounds rather than by framing them uncharitably. The chosen options are not the only viable ones, but the rejections are honest.

---

## Structural Fit Assessment

### State Contract

Adding `ByEcosystem` to `QueueStatus` follows the existing pattern exactly. The struct already has `ByStatus` (map[string]int) and `ByTier` (map[int]map[string]int). Adding `ByEcosystem` (map[string]map[string]int) is a parallel aggregation dimension with the same shape as `ByTier`. **No state contract violation -- the field has a consumer (the Ecosystem Pipeline widget).**

### Dashboard Generator

The `computeQueueStatus()` function at `internal/dashboard/dashboard.go:348` is the natural place to add ecosystem aggregation. It already iterates all entries and calls `entry.Ecosystem()`. Adding a `ByEcosystem` accumulator alongside the existing `ByStatus` and `ByTier` accumulators is structurally consistent. **No parallel pattern introduction.**

### Orchestrator Modification

Modifying `selectCandidates()` in-place is the right location. The function is already responsible for probe limits (line 240-242: `halfOpenCounts`). Extending this logic to prefer pending entries is local to the function. **No action dispatch bypass or structural divergence.**

### Test Pattern

Go tests in `internal/dashboard/` for data contracts match the existing test pattern (26 test functions already exist in `dashboard_test.go`). Shell-based link checking is a new pattern but scoped to CI infrastructure, not core logic. **Acceptable.**

---

## Summary of Findings

| # | Finding | Severity | Category |
|---|---------|----------|----------|
| 1 | Missing alternative: separate `selectProbes()` function vs. modifying `selectCandidates()` | Advisory | Missing alternative |
| 2 | Missing alternative: UTC with label instead of hardcoded ET | Advisory | Missing alternative |
| 3 | Rejection of browser locale cites SSR/CI as reasons, but dashboard is client-rendered | Advisory | Rejection rationale |
| 4 | `excluded` status absent from `by_ecosystem` schema | Advisory | Unstated assumption |
| 5 | "ET" should specify IANA timezone to avoid EST/EDT ambiguity | Advisory | Unstated assumption |
| 6 | Phase dependency: Phase 3 depends on Phase 2 is implicit but not flagged as a constraint | Advisory | Unstated assumption |

No blocking findings. The design aligns with existing architectural patterns. The proposed changes to `QueueStatus`, `selectCandidates()`, and the dashboard HTML follow established conventions in the codebase.
