---
summary:
  constraints:
    - DisambiguationRecord is for batch orchestrator, not interactive CLI
    - Selection reasons must be: single_match, 10x_popularity_gap, priority_fallback
    - HighRisk flag only set for priority_fallback (no download data available)
    - Must integrate with existing BatchResult metrics structure
  integration_points:
    - internal/batch/orchestrator.go - Track disambiguation decisions during batch processing
    - internal/discover/disambiguate.go - Reuse ranking logic and selection criteria
    - BatchResult struct - Include disambiguation records in metrics
  risks:
    - Need to understand how batch orchestrator intercepts ecosystem probe results
    - Ensure compatibility with existing batch metric collection
    - Must not break existing batch workflow
  approach_notes: |
    Per DESIGN-disambiguation.md Phase 5 (Batch Integration):
    - Batch mode always selects deterministically using popularity ranking with priority fallback
    - Track selections in batch metrics for later human review
    - Record: tool name, selected source, alternatives, selection reason, downloads ratio
    - HighRisk flag when using priority_fallback (no popularity data)

    The DisambiguationRecord should capture enough info for dashboard display (issue 1655).
    Selection reasons map to disambiguation logic:
    - single_match: Only one ecosystem match found
    - 10x_popularity_gap: Clear winner with >10x downloads
    - priority_fallback: Close matches, selected by ecosystem priority (risky)
---

# Implementation Context: Issue #1654

**Source**: docs/designs/DESIGN-disambiguation.md (Phase 5: Batch Integration)

## Key Design Excerpt

From DESIGN-disambiguation.md, Component 4: Batch Tracking:

```go
type DisambiguationRecord struct {
    Tool            string   `json:"tool"`
    Selected        string   `json:"selected"`
    Alternatives    []string `json:"alternatives"`
    SelectionReason string   `json:"selection_reason"` // "single_match", "10x_popularity_gap", "priority_fallback"
    DownloadsRatio  float64  `json:"downloads_ratio,omitempty"`
}
```

From issue acceptance criteria:
- HighRisk field (bool) set to true when selection used priority_fallback

## Integration Point

The batch orchestrator processes tools without human interaction. It needs deterministic
disambiguation without prompting, but must track which tools had ambiguous matches for
later human review via the dashboard (issue 1655).
