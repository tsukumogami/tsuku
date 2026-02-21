# Validation Report: Issue #1278

**Issue**: feat(batch): re-order queue entries within tiers by blocking impact
**Executed by**: tester agent
**Date**: 2026-02-21

## Summary

All 8 scenarios passed. The reorder-queue tool correctly re-orders queue entries within priority tiers based on transitive blocking impact while preserving tier boundaries, handling edge cases, and producing valid output on live data.

---

## Scenario 1: Tool builds and runs without errors

**ID**: scenario-1
**Status**: PASSED

**Commands executed**:
- `go build ./cmd/queue-analytics/` -- exit code 0
- `go build ./cmd/reorder-queue/` -- exit code 0 (the feature was implemented as a separate binary rather than extending queue-analytics)
- `go vet ./cmd/reorder-queue/ ./cmd/queue-analytics/ ./internal/reorder/ ./internal/blocker/ ./internal/dashboard/` -- exit code 0, no warnings

**Notes**: The implementation created `cmd/reorder-queue/` as a new binary and `internal/reorder/` as a new package, rather than extending the existing `cmd/queue-analytics/`. Both build cleanly.

---

## Scenario 2: Within-tier ordering changes based on blocking impact

**ID**: scenario-2
**Status**: PASSED

**Test setup**:
- Queue: 3 tier-2 entries (entry-a, entry-b, entry-c)
- Failures: entry-c blocks 3 packages, entry-b blocks 1 package, entry-a blocks 0

**Output order**: entry-c (score=3), entry-b (score=1), entry-a (score=0)

**Verification**: Order within tier 2 is descending by blocking impact. Entries in the single tier are correctly ordered C, B, A.

---

## Scenario 3: Tier boundaries are preserved after reordering

**ID**: scenario-3
**Status**: PASSED

**Test setup**:
- Queue: X (tier 1, blocks 0), Y (tier 2, blocks 5), Z (tier 3, blocks 10)

**Output order**: X (tier 1), Y (tier 2), Z (tier 3)

**Verification**: Even though Z has the highest blocking impact (10), it remains in tier 3. Tier-1 entries always appear before tier-2, and tier-2 before tier-3. No cross-tier promotion occurs.

---

## Scenario 4: Entries with zero blocking impact retain stable order

**ID**: scenario-4
**Status**: PASSED

**Test setup**:
- Queue: 5 tier-3 entries (delta, echo, bravo, alpha, charlie) -- none appear in blocked_by fields
- Failures: empty directory (no failure data)

**Output order**: alpha, bravo, charlie, delta, echo (alphabetical)

**Verification**: All entries have zero blocking score. The tool sorts them alphabetically, providing a deterministic, stable order. No shuffling occurs.

---

## Scenario 5: Transitive blocking is used, not just direct blocking

**ID**: scenario-5
**Status**: PASSED

**Test setup**:
- gmp directly blocks coreutils; coreutils directly blocks another-tool (gmp transitively blocks 2)
- libfoo directly blocks p1 and p2 (libfoo directly blocks 2, no transitive chain)
- extra-tool directly blocks p3, p4, p5 (score 3)

**Output**:
- extra-tool: score=3 (3 direct)
- gmp: score=2 (1 direct + 1 transitive)
- libfoo: score=2 (2 direct)

**Verification**: gmp's transitive blocking is counted (coreutils + another-tool = 2 total). gmp and libfoo tie at 2, with alphabetical tiebreaker (gmp before libfoo). The tool uses `blocker.ComputeTransitiveBlockers` for the computation.

---

## Scenario 6: Reorder tool reuses dashboard blocker computation

**ID**: scenario-6
**Status**: PASSED

**Source inspection**:
- `internal/reorder/reorder.go` imports `github.com/tsukumogami/tsuku/internal/blocker` (line 16)
- `computeScores()` calls `blocker.BuildPkgToBare(blockers)` (line 132)
- `computeScores()` calls `blocker.ComputeTransitiveBlockers(...)` (line 137)

**Dashboard tests**: `go test ./internal/dashboard/ -run TestComputeTopBlockers` -- all 8 tests pass

**Verification**: The reorder tool uses the shared `blocker` package (extracted from the original `internal/dashboard` code). Both the dashboard and the reorder tool depend on the same `blocker.ComputeTransitiveBlockers` and `blocker.BuildPkgToBare` functions. No reimplementation of graph traversal.

---

## Scenario 7: Tool handles empty queue and empty failures gracefully

**ID**: scenario-7
**Status**: PASSED

**Test 7a -- Empty queue** (`{"schema_version":1,"entries":[]}`):
- Exit code: 0
- Output: TotalEntries=0, Reordered=0

**Test 7b -- Queue with nonexistent failures directory**:
- Exit code: 0
- Output order unchanged (alpha, bravo)
- Failures directory is non-fatal; all scores default to 0

**Test 7c -- Queue where all entries are status "success"**:
- Exit code: 0
- Output: valid queue with unchanged order

**Verification**: All three invocations exit with code 0, produce valid output, no crashes or panics.

---

## Scenario 8: End-to-end reorder on live data

**ID**: scenario-8
**Environment**: manual (requires live pipeline data)
**Status**: PASSED (live data available locally)

**Commands executed**:
```
reorder-queue \
  -queue data/queues/priority-queue.json \
  -failures-dir data/failures \
  -output /tmp/reordered-queue.json \
  -json
```

**Results**:
- Exit code: 0
- Total entries: 5275
- Entries reordered: 4768
- By tier: 18 (tier 1), 27 (tier 2), 5230 (tier 3)
- Top blockers: gmp (score=4), libgit2 (score=2), openssl@3 (score=2)

**Schema validation**: All 5275 entries have valid schema_version, required fields (name, source, priority, status, confidence), and valid priority/status values.

**Tier boundaries**: Preserved -- no entry violates tier ordering.

**Impact on tier-3 pending entries**: 2561 out of 2820 tier-3 pending entries changed position. High-leverage blockers (libgit2, bdw-gc, libidn2, tree-sitter@0.25) moved to the front of tier 3.

**Verification**: The tool delivers on its design goal: entries that unblock the most other packages are processed earlier within their tier. The practical impact is significant given that ~99% of entries are tier 3.

---

## Unit Test Suite

All existing unit tests pass (13 tests in internal/reorder, 9 in internal/blocker, 8 dashboard blocker tests):

- `TestReorder_HighBlockingScoreFirst` -- PASS
- `TestReorder_TierBoundariesPreserved` -- PASS
- `TestReorder_AlphabeticalTiebreaker` -- PASS
- `TestReorder_TransitiveBlockingCounts` -- PASS
- `TestReorder_NoBlockingDataAlphabetical` -- PASS
- `TestReorder_EmptyQueue` -- PASS
- `TestReorder_DryRun` -- PASS
- `TestReorder_MixedFailureFormats` -- PASS
- `TestReorder_EntryFieldsPreserved` -- PASS
- `TestReorder_MultiTierReordering` -- PASS
- `TestReorder_CycleDetection` -- PASS
- `TestComputeScores` -- PASS
- `TestReorder_ResultReportsMovements` -- PASS
