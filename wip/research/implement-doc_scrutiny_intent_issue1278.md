# Scrutiny Review: Intent -- Issue #1278

**Issue**: #1278 (re-order queue entries within tiers by blocking impact)
**Focus**: intent
**Reviewer formed independent impression of diff before reading mapping**: Yes

## Sub-check 1: Design Intent Alignment

### What the design doc describes

The design doc (`DESIGN-registry-scale-strategy.md`) places #1278 under M53 (Failure Backend) with the description: "Use existing transitive blocker computation to re-order entries within priority tiers, so high-impact packages are generated first."

The issue body is more explicit:

> The transitive blocker computation already exists in `internal/dashboard/dashboard.go`:
> - `computeTransitiveBlockers()` walks the dependency graph with cycle detection
> - `buildBlockerCountsFromQueue()` inverts `blocked_by` fields into a blocker-to-packages map
> - `computeTopBlockers()` produces ranked results with `DirectCount` and `TotalCount`

AC #3 states: "Reuse the existing transitive blocker computation from `internal/dashboard/dashboard.go` rather than reimplementing"

### What was implemented

The implementation creates a new `internal/reorder` package with its own:
- `computeTransitiveBlockers()` function (lines 249-280 of `reorder.go`)
- `loadBlockerMap()` function (lines 285-303)
- `loadBlockersFromFile()` function (lines 306-346)
- `failureRecord` / `packageFailure` structs (lines 138-153)

Comparing `reorder.computeTransitiveBlockers` with `dashboard.computeTransitiveBlockers`:
- Identical function signature: `(dep string, blockers map[string][]string, pkgToBare map[string]string, memo map[string]int) int`
- Identical algorithm: 0-initialization for cycle detection, deduplication via `seen` map, recursive DFS
- Nearly identical code, line for line

Comparing failure loading:
- `reorder.loadBlockerMap` + `loadBlockersFromFile` parallels `dashboard.loadFailuresFromDir` + `loadFailures`
- Both read JSONL, handle legacy batch format and per-recipe format
- The reorder version extracts only the blocker map; the dashboard version also extracts categories and FailureDetails
- The `failureRecord` and `packageFailure` types in `reorder` duplicate `FailureRecord` and `PackageFailure` from dashboard

The `pkgToBare` reverse index construction in `reorder.computeScores` (lines 228-236) is nearly identical to the one in `dashboard.computeTopBlockers` (lines 514-524).

### Assessment of deviation

The mapping claims the deviation is because "dashboard funcs unexported and coupled to dashboard types." This claim is partially true:

1. **Unexported**: `computeTransitiveBlockers`, `buildBlockerCountsFromQueue`, `loadFailures`, `loadFailuresFromDir` are all unexported (lowercase). This is accurate.

2. **Coupled to dashboard types**: The `loadFailures` function returns `(map[string][]string, map[string]int, map[string]FailureDetails, error)` where `FailureDetails` is a dashboard-specific type. The blocker map itself (`map[string][]string`) is a plain Go type. `computeTransitiveBlockers` takes and returns plain types -- no dashboard-specific types at all.

So the coupling claim is partially valid for the failure loading path but not for the core algorithm. The `computeTransitiveBlockers` function signature uses only primitive/standard types and could have been exported directly from dashboard (or extracted to a shared package) with zero type coupling.

### Design intent verdict

The AC explicitly says "reuse... rather than reimplementing." The implementation reimplements. The issue body specifically names the three functions to reuse. The implementer chose to duplicate rather than refactor.

The practical alternatives were:
1. Export `computeTransitiveBlockers` and `buildBlockerCountsFromQueue` from dashboard (trivial: capitalize first letter)
2. Extract a shared `internal/blockers` package with the algorithm and types
3. Have reorder import dashboard and call through a new exported API

Option 1 would have been minimal effort and preserved the design intent. The algorithm function has no dashboard type dependencies. The failure loading could reasonably be reimplemented (different return needs), but the core algorithm duplication is unnecessary.

**Severity: blocking**. The AC explicitly prohibits reimplementing, and reimplementing was the chosen approach. The deviation's justification (type coupling) does not hold for the core `computeTransitiveBlockers` function, which operates entirely on primitive types. Even the comment in the reimplementation acknowledges this: "This is the same algorithm used in internal/dashboard/dashboard.go" (line 255).

### Practical impact

The duplication creates a maintenance burden: if the transitive blocker algorithm needs to change (e.g., weighted scoring, improved cycle handling), two copies must be updated. This is exactly the scenario the AC was designed to prevent. The issue body's context section explicitly calls out that "the dashboard surfaces this data... but nothing feeds it back into queue ordering" -- the intent is to wire existing computation into queue ordering, not to create a parallel computation.

## Sub-check 2: Cross-Issue Enablement

Skipped. This is a terminal issue with no downstream dependents.

## Backward Coherence

Skipped. No previous issue summary provided (first issue in sequence).

## Findings Summary

| # | AC | Claimed Status | Actual Assessment | Severity |
|---|-----|---------------|-------------------|----------|
| 1 | scoring formula | implemented | Confirmed. `computeScores` uses `transitive_block_count` as the sub-score. Tests verify the formula. | -- |
| 2 | Go tool | implemented | Confirmed. `cmd/reorder-queue/main.go` is a standalone CLI tool with flags for queue path, failures dir, output, dry-run, and JSON output. | -- |
| 3 | reuse dashboard computation | deviated | **Deviation understates the gap.** The core algorithm (`computeTransitiveBlockers`) has zero dashboard type coupling and could have been exported or extracted trivially. The reimplementation duplicates ~100 lines of algorithm code plus ~80 lines of failure loading and type definitions. The AC explicitly says "rather than reimplementing." | **blocking** |
| 4 | tier boundaries preserved | implemented | Confirmed. Sort uses `Priority` as primary key. `TestReorder_TierBoundariesPreserved` verifies tier 1 before tier 2 before tier 3 even when lower tiers have higher scores. | -- |
| 5 | periodic maintenance step | implemented | Confirmed. CLI tool can be invoked standalone or integrated into workflows. Supports `--dry-run` for safe preview. | -- |
