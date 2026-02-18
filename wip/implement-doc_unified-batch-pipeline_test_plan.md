# Test Plan: Unified Batch Pipeline

Generated from: docs/designs/DESIGN-unified-batch-pipeline.md
Issues covered: 3
Total scenarios: 14

---

## Scenario 1: Mixed-ecosystem entries are selected without filtering
**ID**: scenario-1
**Testable after**: #1741
**Commands**:
- `go test ./internal/batch/... -run TestSelectCandidates -v -count=1`
- Verify that an orchestrator with no FilterEcosystem set selects pending/failed entries from homebrew, cargo, github, and npm ecosystems in a single call to selectCandidates()
**Expected**: All eligible entries across all ecosystems are returned as candidates. Entries with status "success", "blocked", or "excluded" are still skipped. The old ecosystem prefix filter (HasPrefix) is no longer present in selectCandidates().
**Status**: passed
**Validated**: 2026-02-18 -- Tests TestSelectCandidates_selectsAllEcosystems, TestSelectCandidates_filtersCorrectly, TestSelectCandidates_includesFailedEntries all pass. No HasPrefix in orchestrator.go.

---

## Scenario 2: Circuit breaker open skips entries from that ecosystem
**ID**: scenario-2
**Testable after**: #1741
**Commands**:
- `go test ./internal/batch/... -run TestSelectCandidates_breakerOpen -v -count=1`
- Create a queue with entries from homebrew, cargo, and github. Set BreakerState to {"homebrew": "open", "cargo": "closed"}. Call selectCandidates().
**Expected**: Homebrew entries are skipped. Cargo and github entries are selected. The orchestrator reads breaker state per-entry from Config.BreakerState and skips entries whose ecosystem has state "open".
**Status**: passed
**Validated**: 2026-02-18 -- TestSelectCandidates_breakerOpen passes. Homebrew entries with breaker "open" correctly skipped; github (no state) and cargo ("closed") selected.

---

## Scenario 3: Circuit breaker half-open limits to one probe entry per ecosystem
**ID**: scenario-3
**Testable after**: #1741
**Commands**:
- `go test ./internal/batch/... -run TestSelectCandidates_breakerHalfOpen -v -count=1`
- Create a queue with 3 cargo entries (all pending). Set BreakerState to {"cargo": "half-open"}. Call selectCandidates().
**Expected**: Exactly 1 cargo entry is selected (the first eligible one). The remaining cargo entries are skipped because the half-open limit of 1 probe per ecosystem is reached.
**Status**: passed
**Validated**: 2026-02-18 -- TestSelectCandidates_breakerHalfOpen passes. With homebrew and cargo both "half-open", exactly 1 probe entry per ecosystem selected (2 total from 5 entries).

---

## Scenario 4: FilterEcosystem restricts to a single ecosystem for manual dispatch
**ID**: scenario-4
**Testable after**: #1741
**Commands**:
- `go test ./internal/batch/... -run TestSelectCandidates -v -count=1`
- Create a queue with entries from homebrew, cargo, and github. Set Config.FilterEcosystem to "cargo". Call selectCandidates().
**Expected**: Only cargo entries are returned. Homebrew and github entries are skipped despite being eligible. This preserves the manual dispatch debugging use case.
**Status**: passed
**Validated**: 2026-02-18 -- TestSelectCandidates_filterEcosystem passes all 4 subtests (github, cargo, homebrew, npm). Each ecosystem filter correctly restricts selection; npm filter returns 0 candidates.

---

## Scenario 5: Per-entry rate limiting uses ecosystem-specific delays
**ID**: scenario-5
**Testable after**: #1741
**Commands**:
- `go test ./internal/batch/... -run TestRun_rateLimiting -v -count=1`
- Create a queue with entries from different ecosystems (e.g., cargo and rubygems). Run the orchestrator and verify that rate limit delays are looked up per-entry using ecosystemRateLimits[entry.Ecosystem()].
**Expected**: The rubygems entry uses a 6-second rate limit while cargo uses 1 second. Unknown ecosystems fall back to the 1-second defaultRateLimit. The github ecosystem uses 2 seconds.
**Status**: passed
**Validated**: 2026-02-18 -- TestEcosystemRateLimits and TestRun_rateLimiting both pass. Rate limit map verified for all 9 ecosystems. Timing test confirms per-entry delays applied correctly (3 cargo entries at 100ms test rate took >=180ms).

---

## Scenario 6: BatchResult tracks per-ecosystem breakdown
**ID**: scenario-6
**Testable after**: #1741
**Commands**:
- `go test ./internal/batch/... -run TestRun_perEcosystemResults -v -count=1`
- Run a mixed batch with 2 homebrew entries (1 success, 1 fail) and 1 cargo entry (success). Inspect the BatchResult.
**Expected**: BatchResult.Ecosystems is {"homebrew": 2, "cargo": 1}. BatchResult.PerEcosystem["homebrew"] has Total=2, Succeeded=1, Failed=1. BatchResult.PerEcosystem["cargo"] has Total=1, Succeeded=1, Failed=0. The old Ecosystem string field no longer exists on the struct.
**Status**: passed
**Validated**: 2026-02-18 -- TestRun_perEcosystemResults passes. Ecosystems map and PerEcosystem breakdown correctly populated for 4 entries across homebrew/cargo/github. Structural check confirms no "Ecosystem string" field on BatchResult.

---

## Scenario 7: Batch ID uses date-only format
**ID**: scenario-7
**Testable after**: #1741
**Commands**:
- `go test ./internal/batch/... -run TestRun -v -count=1`
- Run a batch and check the BatchResult.BatchID format.
**Expected**: BatchID is in the format "2026-02-17" (date only, no ecosystem suffix). The old format "2026-02-17-homebrew" is no longer generated.
**Status**: passed
**Validated**: 2026-02-18 -- TestGenerateBatchID passes. generateBatchID() returns "2026-02-17" (date-only format) with mocked time.

---

## Scenario 8: Failure files are grouped by ecosystem
**ID**: scenario-8
**Testable after**: #1741
**Commands**:
- `go test ./internal/batch/... -run TestSaveResults_groupsFailuresByEcosystem -v -count=1`
- Run a batch where homebrew and cargo entries both fail. Call SaveResults(). Check the failures directory.
**Expected**: Two separate failure files are written: one for homebrew failures and one for cargo failures. Each file contains only failures from its respective ecosystem. The ecosystem is derived from the FailureRecord.PackageID prefix.
**Status**: passed
**Validated**: 2026-02-18 -- TestSaveResults_groupsFailuresByEcosystem passes. SaveResults creates separate failure files prefixed "homebrew-" and "cargo-" in the failures directory. batch-results.json written alongside the queue file.

---

## Scenario 9: CLI accepts no -ecosystem flag and optional -filter-ecosystem
**ID**: scenario-9
**Testable after**: #1741
**Commands**:
- `go build -o /tmp/batch-generate-test ./cmd/batch-generate`
- `/tmp/batch-generate-test -ecosystem homebrew` (should fail with unknown flag)
- `/tmp/batch-generate-test --help` (should show -filter-ecosystem as optional)
**Expected**: The `-ecosystem` flag is rejected as unknown. The `-filter-ecosystem` flag appears in help output and is optional (the binary does not exit with error when it is omitted). The binary reads batch-control.json for breaker state.
**Status**: passed
**Validated**: 2026-02-18 -- Binary builds. "-ecosystem homebrew" exits 2 with "flag provided but not defined: -ecosystem". "--help" exits 0 and shows "-filter-ecosystem string  optional: only process entries from this ecosystem (for debugging)". Binary reads batch-control.json via -control-file flag.

---

## Scenario 10: QueueEntry.Validate rejects path traversal in ecosystem prefix
**ID**: scenario-10
**Testable after**: #1741
**Commands**:
- `go test ./internal/batch/... -run TestQueueEntry_Validate_pathTraversal -v -count=1`
- Create QueueEntry with Source "../../etc:exploit" and call Validate().
**Expected**: Validate() returns an error indicating the ecosystem prefix is invalid due to path separator or traversal characters. Sources like "cargo:valid" pass validation.
**Status**: passed
**Validated**: 2026-02-18 -- TestQueueEntry_Validate_pathTraversalEcosystem passes all 4 subtests: forward slash, backslash, dot-dot in prefix, slash in prefix. All return errors mentioning "path traversal". Valid sources like "cargo:ripgrep" pass validation.

---

## Scenario 11: Workflow removes ECOSYSTEM default and breaker preflight
**ID**: scenario-11
**Testable after**: #1741, #1742
**Commands**:
- Inspect `.github/workflows/batch-generate.yml` for absence of hardcoded ECOSYSTEM default
- Verify the "Preflight circuit breaker check" step is removed
- Verify concurrency group is "queue-operations" (no ecosystem suffix)
- Verify PR branch name, title, and labels do not reference ECOSYSTEM
- Verify queue file reference is "priority-queue.json" (not per-ecosystem)
**Expected**: The workflow no longer defaults ECOSYSTEM to "homebrew" for cron. The concurrency group is a single "queue-operations". The circuit breaker preflight is gone. PR creation uses date-only branch names without ecosystem. The legacy per-ecosystem queue file reference on line 1118 is fixed to "priority-queue.json".
**Status**: pending

---

## Scenario 12: Workflow reads batch-results.json for per-ecosystem breaker updates
**ID**: scenario-12
**Testable after**: #1741, #1742
**Commands**:
- Inspect the "Update circuit breaker" step in `.github/workflows/batch-generate.yml`
- Verify it reads `data/batch-results.json` and iterates over ecosystem keys
- Verify it calls `update_breaker.sh` per ecosystem with success/failure outcome
**Expected**: The workflow loops over each ecosystem in batch-results.json and invokes update_breaker.sh once per ecosystem. If batch-results.json is missing or empty, the step completes without error.
**Status**: pending

---

## Scenario 13: Dashboard shows ecosystem breakdown on run summaries
**ID**: scenario-13
**Testable after**: #1741, #1743
**Commands**:
- `go test ./internal/dashboard/... -v -count=1`
- Verify RunSummary struct has Ecosystems map[string]int instead of Ecosystem string
- Verify parseMetrics handles both old format (ecosystem string) and new format (ecosystems object)
- Verify dashboard.json output contains ecosystems object on runs
**Expected**: RunSummary.Ecosystems is populated from metrics records. Old records with "ecosystem": "homebrew" produce {"homebrew": N}. New records with "ecosystems": {"homebrew": 3, "cargo": 5} parse correctly. The generated dashboard.json uses the ecosystems object format.
**Status**: pending

---

## Scenario 14: Dashboard pages render multi-ecosystem format
**ID**: scenario-14
**Testable after**: #1741, #1743
**Commands**:
- Inspect `website/pipeline/runs.html` for ecosystems object handling
- Inspect `website/pipeline/run.html` for per-ecosystem breakdown display
- Inspect `website/pipeline/index.html` for compact multi-ecosystem rendering
- Verify runs.html ecosystem filter uses "contains" matching instead of exact match
- Verify hardcoded "homebrew:" in loadFailures (dashboard.go line 431) is replaced
- Verify hardcoded "homebrew" default in failures.go line 229 is replaced with "unknown" or derived value
**Expected**: runs.html renders ecosystem breakdown (e.g., "homebrew: 3, cargo: 5") instead of a single string. The ecosystem filter dropdown matches batches that contain the selected ecosystem. run.html shows per-ecosystem results in the detail view. index.html shows compact ecosystem info in the recent runs sidebar. The hardcoded homebrew assumptions in the Go data layer are removed.
**Status**: pending
