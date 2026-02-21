# Architecture Review: #1278 (re-order queue entries within tiers by blocking impact)

## Review Focus: architect
## Issue: #1278

---

## Summary

The implementation introduces a new `internal/reorder/` package and a `cmd/reorder-queue/` CLI tool that reorders priority queue entries within each tier by transitive blocking impact. The core transitive blocker computation correctly reuses the shared `internal/blocker/` package, and the dependency direction (reorder -> batch, reorder -> blocker) is clean.

---

## Findings

### 1. ADVISORY: Duplicate failure JSONL parsing logic

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/reorder/reorder.go` lines 50-67, 144-208

The reorder package defines its own `failureRecord` and `packageFailure` structs and implements its own JSONL parsing in `loadBlockerMap()` / `loadBlockersFromFile()`. This logic closely mirrors `internal/dashboard/dashboard.go`'s `loadFailures()` / `loadFailuresFromDir()`, including the same two-format handling (legacy batch format with `failures[]` array, and per-recipe format with `recipe` + `blocked_by` fields). The same fallback logic for empty ecosystem (`eco = "homebrew"`) appears in both places.

There are now three places that define structs for reading failure JSONL records:
- `internal/batch/results.go` (`FailureRecord`, `FailureFile`) -- the write-side types
- `internal/dashboard/dashboard.go` (`FailureRecord`, `PackageFailure`) -- the dashboard's read-side types
- `internal/reorder/reorder.go` (`failureRecord`, `packageFailure`) -- the reorder's read-side types

The reorder package only needs the `blocked_by` extraction (the `map[string][]string` return value). The dashboard's `loadFailuresFromDir()` already returns this exact value as its first return.

**Impact**: If the JSONL format changes (e.g., a new failure format variant), three locations need updating instead of two. The `internal/blocker/` package was correctly extracted as a shared package for the computation logic; the JSONL parsing could have been shared similarly.

**Severity**: Advisory. The duplication doesn't compound structurally -- the reorder package's read-only subset is narrower than the dashboard's, and the unexported types keep it contained. No other package will copy this pattern. However, the test plan (scenario 6) explicitly specified reusing dashboard code, so this divergence from the plan is worth noting.

**Suggestion**: Extract a shared `loadBlockerMapFromDir(dir string) (map[string][]string, error)` function, possibly in the `blocker` package (which already describes itself as "used by both the pipeline dashboard and the queue reorder tool"). The dashboard's `loadFailuresFromDir()` returns blockers as its first return value, but its signature includes dashboard-specific types. A dedicated function returning just the blocker map would serve both consumers.

### 2. No findings on blocker computation reuse (positive)

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/reorder/reorder.go` line 132-141

The implementation correctly imports and uses `internal/blocker.ComputeTransitiveBlockers()` and `internal/blocker.BuildPkgToBare()`. The `blocker` package header comment was already updated to document both consumers. This follows the established pattern -- the `blocker` package was factored out specifically to be shared.

### 3. No findings on dependency direction (positive)

**Files**: `internal/reorder/` imports only `internal/batch` and `internal/blocker`. Both are lower-level data/computation packages with no upward dependencies. `cmd/reorder-queue/` imports only `internal/reorder`. The dependency graph is clean:

```
cmd/reorder-queue -> internal/reorder -> internal/batch (data types)
                                      -> internal/blocker (computation)
```

No circular dependencies, no cross-level imports.

### 4. No findings on CLI surface (positive)

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/cmd/reorder-queue/main.go`

The implementation follows the existing pattern of purpose-built CLI tools in `cmd/` (consistent with `cmd/batch-generate/`, `cmd/seed-queue/`, `cmd/queue-analytics/`, `cmd/bootstrap-queue/`). The test plan's scenario 1 mentioned the possibility of extending `queue-analytics` vs creating a new tool. Creating a dedicated tool is consistent with the project's established approach (the design doc's "Built Beyond Original Scope" section explicitly documents that "Purpose-built CLI tools" is the pattern). No overlap with existing subcommands.

### 5. No findings on queue contract preservation (positive)

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/reorder/reorder.go` lines 72-126

The implementation reads via `batch.LoadUnifiedQueue()` and writes via `batch.SaveUnifiedQueue()`, using the same contract as the orchestrator. No new fields added to `QueueEntry` or `UnifiedQueue`. The sort operates on the existing `Priority` field as a tier boundary and uses entry `Name` for score lookup. All existing entry fields (status, confidence, failure_count, next_retry_at, disambiguated_at) are preserved through the sort -- the test `TestReorder_EntryFieldsPreserved` verifies this explicitly.

---

## Overall Assessment

The implementation fits the codebase architecture well. The critical design decision -- reusing `internal/blocker/` for transitive computation rather than reimplementing it -- was made correctly. The dependency graph is clean. The CLI follows the established purpose-built-tool pattern.

The one structural concern is the duplicated failure JSONL parsing, which introduces a parallel pattern for reading failure data. This is advisory rather than blocking because: (a) the reorder package's copy is a narrow subset of what the dashboard reads, (b) the types are unexported and won't be imported by other packages, and (c) the `internal/blocker/` package comment already documents the intended sharing boundary. If the failure JSONL format evolves, this will need attention, but it doesn't create a compounding problem today.
