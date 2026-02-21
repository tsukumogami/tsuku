# Review: Maintainer -- Issue #1278

**Issue**: #1278 (re-order queue entries within tiers by blocking impact)
**Focus**: maintainability (clarity, readability, duplication)
**Files reviewed**: `internal/reorder/reorder.go`, `internal/reorder/reorder_test.go`, `cmd/reorder-queue/main.go`, `internal/blocker/blocker.go`, `internal/blocker/blocker_test.go`

## Finding 1: Duplicated failure record types (Divergent twins)

**Severity**: Advisory

**Location**: `internal/reorder/reorder.go:52-67` vs `internal/dashboard/dashboard.go:143-165`

The `failureRecord` and `packageFailure` types in `internal/reorder` are trimmed-down copies of `FailureRecord` and `PackageFailure` in `internal/dashboard`. Both describe the same JSONL format with the same core fields:

reorder:
```go
type failureRecord struct {
    SchemaVersion int              `json:"schema_version"`
    Ecosystem     string           `json:"ecosystem,omitempty"`
    Failures      []packageFailure `json:"failures,omitempty"`
    Recipe        string           `json:"recipe,omitempty"`
    Category      string           `json:"category,omitempty"`
    BlockedBy     []string         `json:"blocked_by,omitempty"`
}
```

dashboard:
```go
type FailureRecord struct {
    SchemaVersion int              `json:"schema_version"`
    Ecosystem     string           `json:"ecosystem,omitempty"`
    Environment   string           `json:"environment,omitempty"`
    UpdatedAt     string           `json:"updated_at,omitempty"`
    Timestamp     string           `json:"timestamp,omitempty"`
    Failures      []PackageFailure `json:"failures,omitempty"`
    Recipe        string           `json:"recipe,omitempty"`
    Platform      string           `json:"platform,omitempty"`
    Category      string           `json:"category,omitempty"`
    ExitCode      int              `json:"exit_code,omitempty"`
    BlockedBy     []string         `json:"blocked_by,omitempty"`
}
```

The core algorithm was correctly extracted into `internal/blocker`. But the failure record types and the JSONL loading logic (`loadBlockersFromFile` vs `loadFailures`) remain duplicated. If the JSONL format changes (e.g., a new `blocked_by` variant or schema version bump), both places need updating.

This is advisory, not blocking, because: (a) the reorder version only extracts blocker data (simpler return type) while dashboard also extracts categories and details, so a single function would need to return a superset; (b) the JSONL format is stable and schema-versioned; (c) the trimmed types in reorder are a valid design choice to avoid importing dashboard types. A shared `internal/failures` package for the record types and a `LoadBlockerMap(dir)` function could consolidate this, but the current separation is defensible.

## Finding 2: `Priority` field used for tier sorting with no named constants

**Severity**: Advisory

**Location**: `internal/reorder/reorder.go:97-98`, `cmd/reorder-queue/main.go:56`

The sort comparator uses `queue.Entries[i].Priority` as the tier, and the CLI output says "Tier %d" when printing per-tier counts. But `Priority` is an `int` with no named constants in the codebase. The `QueueEntry.Priority` doc comment says "1=critical, 2=popular, 3=standard", but there are no `const` values.

The next developer reading the sort comparator will see `Priority < Priority` and think "priority 1 sorts before priority 2 -- is lower priority first or higher priority first?" The doc comment on the struct field clarifies this (lower number = higher priority), but nothing at the sort site hints at the meaning. The code is correct, but a one-line comment at the sort site like `// Lower priority number = higher tier, processed first` would save the next reader a trip to the struct definition.

This is advisory because the `QueueEntry.Priority` doc comment is clear enough and the field naming is an existing convention this issue didn't introduce.

## Finding 3: `loadBlockerMap` error is silently swallowed

**Severity**: Advisory

**Location**: `internal/reorder/reorder.go:82-87`

```go
blockers, err := loadBlockerMap(opts.FailuresDir)
if err != nil {
    // Non-fatal: if no failure data exists, all scores are 0 and
    // the queue retains its alphabetical ordering within tiers.
    blockers = make(map[string][]string)
}
```

The comment explains the intent well. But `loadBlockerMap` returns errors for two different reasons: (1) no `.jsonl` files found, and (2) `filepath.Glob` itself failing. Case 1 is a legitimate "no data" situation. Case 2 is a filesystem error that probably means something is wrong with the path. Both are treated the same way.

The next developer debugging a "reorder did nothing" issue will have no log output indicating whether failure data was found or not. Adding a brief log line or including the error in the Result struct would make debugging faster. This is minor because the `--dry-run` and `--json` outputs show TopScores being empty, which is a reasonable signal.

## Finding 4: Test names accurately describe behavior

**Severity**: N/A (positive observation)

Test names are clear and scenario-oriented: `TestReorder_HighBlockingScoreFirst`, `TestReorder_TierBoundariesPreserved`, `TestReorder_CycleDetection`, etc. Each test name describes the invariant it verifies, not the implementation detail it exercises. The test helpers (`writeQueue`, `writeFailures`, `readQueue`, `entryNames`) reduce ceremony without hiding important setup. Well done.

## Finding 5: CLI human-readable output uses map iteration order for tiers

**Severity**: Advisory

**Location**: `cmd/reorder-queue/main.go:54-56`

```go
for tier, count := range result.ByTier {
    fmt.Fprintf(os.Stderr, "  Tier %d: %d entries\n", tier, count)
}
```

Go map iteration order is random. When there are tiers 1, 2, and 3, the output might print them as "Tier 3, Tier 1, Tier 2" on one run and "Tier 1, Tier 2, Tier 3" on another. This only affects the human-readable stderr output (JSON output is fine since it serializes the map). The output will look messy but won't cause incorrect behavior. Sorting the tier keys before printing would make the output deterministic.

## Finding 6: Blocker package extraction is clean

**Severity**: N/A (positive observation)

The `internal/blocker` package cleanly extracts the core transitive blocker algorithm shared between `internal/dashboard` and `internal/reorder`. The package-level comment explains its purpose and its two consumers. The function signatures use plain types (`map[string][]string`, `map[string]int`) with no coupling to either consumer's domain types. The memo pattern and cycle detection via 0-initialization are well-documented in the function's godoc. This directly addresses the design doc's AC3 ("reuse the existing transitive blocker computation") after the initial deviation was fixed.

## Summary

No blocking findings. The implementation is well-structured: the core algorithm lives in a shared `internal/blocker` package, the `internal/reorder` package has a clean API with `Options`/`Result` types, and the CLI is minimal. Test coverage exercises the key invariants (tier preservation, transitive scoring, cycle safety, dry-run, mixed formats).

The failure record type duplication between `internal/reorder` and `internal/dashboard` (Finding 1) is the most notable maintenance concern, but it's defensible given the different return needs of the two consumers. The other advisories are minor readability improvements: sorted tier output in the CLI (Finding 5), a disambiguating comment at the sort site (Finding 2), and logging when failure data is missing (Finding 3).
