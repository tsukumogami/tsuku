# Scrutiny Review: Completeness -- Issue #1278

**Issue**: #1278 (re-order queue entries within tiers by blocking impact)
**Scrutiny Focus**: completeness
**Reviewer**: scrutiny-review-decisions agent
**Date**: 2026-02-21

## Methodology

Read the full diff (`git diff HEAD~1`) before examining the requirements mapping. Formed independent assessment of what was implemented, then compared against the mapping claims.

## Files Changed

- `cmd/reorder-queue/main.go` (new, 74 lines) -- CLI entry point
- `internal/reorder/reorder.go` (new, 348 lines) -- core logic
- `internal/reorder/reorder_test.go` (new, 708 lines) -- 16 tests
- `wip/implement-doc-state.json` (modified) -- state update

## AC Extraction from Issue Body

The issue's "Acceptance Criteria" section contains five checkboxes:

| # | AC Text (from issue) | Mapping AC Label |
|---|----------------------|------------------|
| 1 | Define a scoring formula for within-tier ordering (e.g., `sub_score = transitive_block_count`) | "scoring formula" |
| 2 | Go tool (or extension to `cmd/queue-analytics/`) that reads the queue + dashboard blocker data and re-orders entries within each tier | "Go tool" |
| 3 | Reuse the existing transitive blocker computation from `internal/dashboard/dashboard.go` rather than reimplementing | "reuse dashboard computation" |
| 4 | Tier boundaries preserved: tier 1 always before tier 2, tier 2 before tier 3 | "tier boundaries preserved" |
| 5 | Can run as a periodic queue maintenance step or part of the seed workflow | "periodic maintenance step" |

**AC coverage**: All 5 ACs from the issue body have corresponding mapping entries. No missing ACs.

**Phantom ACs**: None. All mapping entries correspond to issue ACs.

## Finding 1: AC "scoring formula" -- claimed "implemented"

**Assessment**: CONFIRMED

The diff introduces `computeScores()` in `internal/reorder/reorder.go` (line 217 of the new file). The scoring formula is: for each queue entry, the score equals the count of packages transitively blocked by that entry's name appearing in `blocked_by` fields across failure records. This is `sub_score = transitive_block_count`, matching the example in the AC. The sort in `Run()` uses this score as the primary sub-tier ordering key (descending), with alphabetical name as tiebreaker.

Tests `TestComputeScores`, `TestReorder_HighBlockingScoreFirst`, and `TestReorder_TransitiveBlockingCounts` verify the formula.

**Severity**: N/A (no finding)

## Finding 2: AC "Go tool" -- claimed "implemented"

**Assessment**: CONFIRMED

`cmd/reorder-queue/main.go` is a new Go CLI tool. It reads the unified priority queue via `batch.LoadUnifiedQueue()` and failure data from a JSONL directory. It delegates to `reorder.Run()` which performs the reordering. The tool supports `--queue`, `--failures-dir`, `--output`, `--dry-run`, and `--json` flags. This is a new standalone tool rather than an extension to `cmd/queue-analytics/`, which the AC explicitly allows ("or extension to `cmd/queue-analytics/`").

The AC also says "reads the queue + dashboard blocker data." The tool reads failure JSONL files directly rather than dashboard blocker data. However, the blocker data is derived from the same failure JSONL files that the dashboard reads, so the data source is equivalent. The AC's mention of "dashboard blocker data" likely refers to the blocker information the dashboard computes, and the tool computes the same information from the same source files.

**Severity**: N/A (no finding)

## Finding 3: AC "reuse dashboard computation" -- claimed "deviated"

**Assessment**: Deviation acknowledged; reason is verifiable.

The AC explicitly says: "Reuse the existing transitive blocker computation from `internal/dashboard/dashboard.go` rather than reimplementing."

The implementation reimplements `computeTransitiveBlockers()` in `internal/reorder/reorder.go`. The new function has the same signature `(dep string, blockers map[string][]string, pkgToBare map[string]string, memo map[string]int) int` and uses the same DFS-with-memo cycle detection algorithm.

**Verification of the stated reason**: "reimplemented same algorithm - dashboard funcs unexported and coupled to dashboard types"

1. **Unexported**: Confirmed. `computeTransitiveBlockers`, `buildBlockerCountsFromQueue`, and `computeTopBlockers` are all lowercase in `internal/dashboard/dashboard.go` (lines 473, 501, 511).

2. **Coupled to dashboard types**: Confirmed. `buildBlockerCountsFromQueue` takes `[]PackageInfo` as input, where `PackageInfo` is a dashboard-specific type with fields like `Category`, `NextRetryAt`, etc. that are irrelevant to the reorder use case. The reorder tool reads failure JSONL files directly, which is a different data ingestion path than the dashboard's `PackageInfo` pipeline.

The deviation is genuine. Exporting the dashboard functions would either require (a) making the reorder package depend on `internal/dashboard` and construct `PackageInfo` objects just to call the function, or (b) refactoring the dashboard to separate the algorithm from the type. Either approach would be more disruptive than reimplementing the ~30-line function. The comment at line 255-256 of `reorder.go` ("This is the same algorithm used in internal/dashboard/dashboard.go") documents the relationship.

**Severity**: advisory -- The deviation is well-justified. The alternative (exporting or refactoring dashboard) would create coupling or churn disproportionate to the benefit. However, the AC was explicit about reuse, so this is worth flagging for the record.

## Finding 4: AC "tier boundaries preserved" -- claimed "implemented"

**Assessment**: CONFIRMED

The sort in `Run()` (lines 182-192 of `reorder.go`) sorts by `Priority` ascending first, then by score descending, then alphabetically. Since `Priority` is the primary sort key, entries within tier 1 always precede tier 2, which always precede tier 3.

Test `TestReorder_TierBoundariesPreserved` explicitly sets up a scenario where a tier 3 entry has the highest blocking score (10) and verifies that tier 1 and tier 2 entries still precede it. Test `TestReorder_MultiTierReordering` verifies independent reordering across three tiers simultaneously.

**Severity**: N/A (no finding)

## Finding 5: AC "periodic maintenance step" -- claimed "implemented"

**Assessment**: CONFIRMED

The tool is a standalone CLI (`cmd/reorder-queue/`) that can be invoked from any workflow, cron job, or CI step. It reads input files, computes the reordering, and writes output. It has `--dry-run` for safe preview and `--json` for machine-readable output suitable for CI integration. The `--output` flag allows writing to a different file than the input, enabling non-destructive use.

The issue AC says "Can run as a periodic queue maintenance step or part of the seed workflow." The tool's design (file-based I/O, no daemon mode, exit-code-based error reporting) makes it suitable for both use cases. No CI workflow integration was added in this commit, but the AC doesn't require it -- it says "can run as," not "is integrated into."

**Severity**: N/A (no finding)

## Summary Table

| AC | Claimed Status | Verified | Severity |
|----|---------------|----------|----------|
| scoring formula | implemented | Yes | -- |
| Go tool | implemented | Yes | -- |
| reuse dashboard computation | deviated | Yes, reason verified | advisory |
| tier boundaries preserved | implemented | Yes | -- |
| periodic maintenance step | implemented | Yes | -- |

## Overall Assessment

All 5 ACs from the issue are accounted for. 4 are implemented with verifiable evidence in the diff. 1 is deviated with a genuine, verifiable reason (unexported functions, type coupling). No missing ACs. No phantom ACs. No blocking findings.

The implementation is solid: 348 lines of production code with 708 lines of tests covering direct blocking, transitive blocking, cycle detection, multi-tier reordering, empty queues, dry-run mode, mixed failure formats, field preservation, and result reporting.
