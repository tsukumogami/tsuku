# Pragmatic Review: Issue #1278

**Issue**: #1278 (re-order queue entries within tiers by blocking impact)
**Focus**: pragmatic (simplicity, YAGNI, KISS)
**Files reviewed**:
- `internal/blocker/blocker.go` (new, 61 lines)
- `internal/blocker/blocker_test.go` (new, 133 lines)
- `internal/reorder/reorder.go` (new, 297 lines)
- `internal/reorder/reorder_test.go` (new, 653 lines)
- `cmd/reorder-queue/main.go` (new, 75 lines)
- `internal/dashboard/dashboard.go` (modified -- refactored to use `internal/blocker`)

## Findings

### Finding 1: `buildResult` / `Move` / `EntriesMoved` complexity for reporting (advisory)

**File**: `internal/reorder/reorder.go:219-296`

`buildResult` is ~75 lines of code that computes per-tier position diffs and a top-10 scored entry list. The `Move` type, `EntriesMoved` map, and the position diffing logic exist to report which entries moved and by how much.

Consumers:
- The CLI `--json` output marshals the full `Result` (including `EntriesMoved`) -- useful for machine consumers.
- The human-readable CLI output only uses `result.Reordered` (count) and `result.TopScores` -- never iterates `EntriesMoved`.
- One test (`TestReorder_ResultReportsMovements`) exercises `EntriesMoved`.

The `EntriesMoved` data is useful if a CI workflow or operator wants to see exactly what moved, which is plausible for a periodic maintenance tool. The `TopScores` list helps operators verify the reorder is doing something sensible. Both serve the `--json` flag's purpose.

**Verdict**: The reporting code is more detailed than strictly necessary, but it's bounded (no abstraction layers, no interfaces) and serves a concrete output path (`--json`). Not blocking.

### Finding 2: `scoredEntry` local struct in `buildResult` duplicates `ScoredEntry` (advisory)

**File**: `internal/reorder/reorder.go:256-261`

`buildResult` defines a local `scoredEntry` struct (unexported, with lowercase fields) that's immediately converted to the exported `ScoredEntry` type. The local struct could be eliminated by building `[]ScoredEntry` directly.

```go
// Current: local struct then conversion
type scoredEntry struct {
    name  string
    score int
    tier  int
}
// ... builds []scoredEntry, sorts, then converts to []ScoredEntry

// Simpler: build ScoredEntry directly
var allScored []ScoredEntry
for _, e := range entries {
    if s := scores[e.Name]; s > 0 {
        allScored = append(allScored, ScoredEntry{Name: e.Name, Score: s, Tier: e.Priority})
    }
}
```

Minor. The conversion is small and inert.

### Finding 3: No blocking findings on correctness or edge cases

The implementation handles the important edge cases correctly:
- Empty queue returns early (line 77-79)
- Missing/empty failures dir is non-fatal (line 83-87, falls through to zero scores)
- Cycle detection via 0-initialization in memo map (blocker.go:25)
- Deduplication of blocked packages (blocker.go:28-34)
- Both legacy batch format and per-recipe format JSONL are handled (reorder.go:187-204)
- Malformed JSONL lines are skipped, not fatal (reorder.go:183-185)
- `sort.SliceStable` ensures deterministic output for equal-score entries (alphabetical tiebreaker at reorder.go:105)

The shared `internal/blocker` package cleanly extracts the algorithm that was previously unexported in `internal/dashboard`. The dashboard was refactored to use the shared package. This satisfies the AC's intent to reuse rather than reimplement (the scrutiny reviews were based on an earlier commit that duplicated the code -- this has since been addressed).

## Summary

No blocking findings. The implementation is straightforward: load queue, load failure data, compute scores via shared blocker algorithm, sort within tiers, write output. The `internal/blocker` extraction is the right call -- it's the simplest way to share the algorithm between dashboard and reorder without coupling either to the other's types. The CLI is minimal (75 lines, no framework). Two advisory notes: the `buildResult` reporting logic is more detailed than the human-readable output consumes (justified by `--json`), and a local struct could be eliminated by building the exported type directly.
