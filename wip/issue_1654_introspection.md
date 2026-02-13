# Issue 1654 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-disambiguation.md`
- Sibling issues reviewed: #1648 (closed), #1650 (closed), #1651 (closed)
- Prior patterns identified:
  - `disambiguate.go` in `internal/discover/` houses core disambiguation logic
  - `isClearWinner()` determines 10x threshold with secondary signals
  - `AmbiguousMatchError` returns when no clear winner exists
  - `SelectionReason` values per design: `single_match`, `10x_popularity_gap`, `priority_fallback`

## Gap Analysis

### Minor Gaps

1. **Orchestrator operates at CLI level, not discover level**: The batch orchestrator invokes `tsuku create` as an external process (see `orchestrator.go:176-224`). It doesn't call the discover package directly. The issue spec says "Orchestrator tracks disambiguation decisions when processing tools with multiple ecosystem matches" but the orchestrator doesn't have access to the `disambiguate()` function internals.

   **Resolution**: The tracking must happen within `tsuku create` (which calls discover) and be exposed via `--json` output or a new mechanism. The orchestrator would parse this output like it parses `installResult` (line 289-299). This is how other failure categories work (see `parseInstallJSON`).

2. **Design doc clarifies batch mode behavior**: Per design (lines 299-319), batch mode should:
   - Always select deterministically using popularity ranking with priority fallback
   - Track selections in batch metrics for later human review
   - Flag `HighRisk` when using `priority_fallback` (no download data available)

   The current `disambiguate()` function returns `AmbiguousMatchError` when there's no clear winner and no callback. In batch/deterministic mode, the CLI passes `--deterministic-only` which would cause disambiguation to fail rather than select. The design expects deterministic selection + tracking.

3. **Location of DisambiguationRecord**: Issue says "added to batch package" but the tracking data originates from discover. Two options:
   - Define in `internal/batch/results.go` alongside `FailureRecord` (matches existing pattern)
   - Record the decision in `tsuku create --json` output, parse in orchestrator

   **Resolution**: Follow existing pattern - define `DisambiguationRecord` in batch package, populate from parsed CLI output.

### Moderate Gaps

None - the design doc and issue spec align. The implementation approach just needs clarification on the CLI-to-orchestrator interface.

### Major Gaps

None.

## Recommendation

**Proceed** - with minor implementation clarification noted below.

## Implementation Notes

The issue's acceptance criteria need to be implemented in two places:

1. **`internal/discover/disambiguate.go`**: Extend to track selection reason and return it. The `disambiguate()` function needs to determine and return:
   - `SelectionReason`: `single_match`, `10x_popularity_gap`, or `priority_fallback`
   - `DownloadsRatio`: The ratio between top two matches
   - Whether `HighRisk` should be set

2. **`internal/batch/results.go`**: Add `DisambiguationRecord` type matching the design:
   ```go
   type DisambiguationRecord struct {
       Tool            string   `json:"tool"`
       Selected        string   `json:"selected"`
       Alternatives    []string `json:"alternatives"`
       SelectionReason string   `json:"selection_reason"`
       DownloadsRatio  float64  `json:"downloads_ratio,omitempty"`
       HighRisk        bool     `json:"high_risk"`
   }
   ```

3. **`internal/batch/results.go`**: Extend `BatchResult` to include disambiguation records:
   ```go
   type BatchResult struct {
       // ... existing fields ...
       Disambiguations []DisambiguationRecord `json:"-"`
   }
   ```

4. **CLI integration**: The `tsuku create --json` output needs to include disambiguation info when selection occurs, which the orchestrator parses.

The issue's note "No dependency on Issue 5" is correct - batch mode doesn't wait for `AmbiguousMatchError` formatting; it intercepts during the selection process itself.
