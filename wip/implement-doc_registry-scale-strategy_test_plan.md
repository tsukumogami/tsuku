# Test Plan: registry-scale-strategy

Generated from: docs/designs/DESIGN-registry-scale-strategy.md
Issues covered: 1
Total scenarios: 8

---

## Scenario 1: Tool builds and runs without errors
**ID**: scenario-1
**Testable after**: #1278
**Commands**:
- `go build ./cmd/queue-analytics/`
- `go vet ./cmd/queue-analytics/`
**Expected**: Build succeeds with exit code 0 and no vet warnings. The tool compiles cleanly whether it is a new binary or an extension of the existing queue-analytics command.
**Status**: pending

---

## Scenario 2: Within-tier ordering changes based on blocking impact
**ID**: scenario-2
**Testable after**: #1278
**Commands**:
- Create a test queue file with three tier-2 entries: A (blocks nothing), B (blocks 1 package), C (blocks 3 packages)
- Create a test failures directory with JSONL records establishing the blocked_by relationships
- Run the reorder tool against the test queue and failures directory
- Read the output queue file and verify entry order within tier 2
**Expected**: The output queue preserves tier boundaries and orders within tier 2 as C, B, A (descending by transitive blocking impact). Entries in other tiers are not interleaved.
**Status**: pending

---

## Scenario 3: Tier boundaries are preserved after reordering
**ID**: scenario-3
**Testable after**: #1278
**Commands**:
- Create a test queue with tier-1 entry X (blocks 0 packages), tier-2 entry Y (blocks 5 packages), tier-3 entry Z (blocks 10 packages)
- Run the reorder tool
- Read the output queue and check ordering
**Expected**: Output order is X, Y, Z. Even though Z has the highest blocking impact, tier-1 entries always appear before tier-2, and tier-2 before tier-3. No cross-tier promotion occurs.
**Status**: pending

---

## Scenario 4: Entries with zero blocking impact retain stable order
**ID**: scenario-4
**Testable after**: #1278
**Commands**:
- Create a test queue with five tier-3 entries, none of which appear in any blocked_by field
- Run the reorder tool
- Compare output order to input order
**Expected**: Entries with equal blocking impact (zero) maintain a stable, deterministic order (e.g., alphabetical by name). The tool does not shuffle entries that have no blocking data.
**Status**: pending

---

## Scenario 5: Transitive blocking is used, not just direct blocking
**ID**: scenario-5
**Testable after**: #1278
**Commands**:
- Create a test queue and failures where: gmp directly blocks coreutils; coreutils directly blocks another-tool; gmp transitively blocks 2 packages while libfoo directly blocks 2 packages
- Run the reorder tool
- Verify gmp and libfoo are ordered by their transitive (total) block count, not just direct
**Expected**: The tool reuses `computeTransitiveBlockers` from `internal/dashboard/dashboard.go`. Both gmp (2 transitive) and libfoo (2 direct, 2 total) should have equal scores, breaking ties deterministically. An entry with 3 transitive blocks should rank above one with 2.
**Status**: pending

---

## Scenario 6: Reorder tool reuses dashboard blocker computation
**ID**: scenario-6
**Testable after**: #1278
**Commands**:
- `go test ./internal/dashboard/ -run TestComputeTopBlockers`
- Inspect the reorder tool source code for imports of `internal/dashboard` package
**Expected**: The reorder tool imports and calls functions from `internal/dashboard` (such as `computeTransitiveBlockers`, `buildBlockerCountsFromQueue`, or `computeTopBlockers`) rather than reimplementing the transitive blocker graph traversal. Unit tests in the dashboard package continue to pass.
**Status**: pending

---

## Scenario 7: Tool handles empty queue and empty failures gracefully
**ID**: scenario-7
**Testable after**: #1278
**Commands**:
- Run the reorder tool with an empty queue file (`{"schema_version":1,"entries":[]}`)
- Run the reorder tool with a queue file but no failures directory
- Run the reorder tool with a queue where all entries are status "success" (no pending/blocked entries to reorder)
**Expected**: All three invocations exit with code 0 and produce a valid output queue. No crashes, no panics. For the empty queue case, the output is also an empty queue. For the no-failures case, the order is unchanged from input.
**Status**: pending

---

## Scenario 8: End-to-end reorder on live data
**ID**: scenario-8
**Testable after**: #1278
**Environment**: manual
**Commands**:
- `go run ./cmd/queue-analytics/ --queue data/queues/priority-queue.json --failures-dir data/failures --output /tmp/reordered-queue.json` (or equivalent flags for the reorder subcommand/tool)
- Compare the before and after queue files using `jq` to extract tier-3 pending entries and verify order changes
**Expected**: When run against the live `data/queues/priority-queue.json` and `data/failures/` directory, the tool produces a reordered queue where entries known to block many packages (visible on the pipeline dashboard as top blockers) appear earlier within their tier than entries that block nothing. The queue validates against the existing schema (schema_version, entries array with valid QueueEntry fields). This is a use-case scenario: it validates the feature delivers value on real pipeline data, not just synthetic test fixtures.
**Status**: pending

---
