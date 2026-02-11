---
summary:
  constraints:
    - Forks must NEVER auto-select - always require explicit user confirmation even with --yes flag
    - 10x star threshold for suggesting parent repository instead of fork
    - Graceful degradation when parent metadata fetch fails (rate limits, network issues)
    - Fork check happens after confidence gate but before quality signals OR logic
  integration_points:
    - internal/discover/llm_discovery.go - verifyGitHubRepo() function needs fork detection
    - internal/discover/resolver.go - Metadata struct needs IsFork, ParentRepo, ParentStars fields
    - passesQualityThreshold() function must return false for forks to prevent auto-selection
    - Downstream issues depend on fork fields: #1613 (rate limits), #1614 (ranking), #1615 (UX)
  risks:
    - GitHub API rate limiting on parent fetch (separate request for parent metadata)
    - GitHub API response structure changes for fork/parent fields
    - Edge case where parent repo is also archived or deleted
  approach_notes: |
    Extend the existing verifyGitHubRepo function to:
    1. Check fork: true in API response
    2. Extract parent.full_name for fork repos
    3. Make second API call to fetch parent stars
    4. Add IsFork, ParentRepo, ParentStars to Metadata struct
    5. Update passesQualityThreshold to return false for forks
    6. Display fork warning in confirmation output
---

# Implementation Context: Issue #1612

**Source**: docs/designs/DESIGN-llm-discovery-implementation.md

## Key Design Excerpts

### Fork Detection (from Verification Flow)

The GitHub API returns `fork: true` for repositories that are forks. When detected:

1. Fetch parent repo metadata via `parent.full_name` from the API response
2. Display warning: "This is a fork of {parent}. Consider the original instead."
3. Compare stars: if parent has 10x more stars, suggest parent instead
4. Never auto-select forks - always prompt for confirmation even with `--yes`

### Quality Threshold Logic

1. **Confidence gate (AND)**: If confidence below threshold, reject immediately
2. **Fork check**: Forks never auto-pass - always require explicit confirmation
3. **Quality signals (OR)**: Pass if ANY quality signal meets its threshold

### GitHub API Response Structure

```json
{
  "fork": true,
  "parent": {
    "full_name": "original-owner/repo",
    "stargazers_count": 5000
  }
}
```

## Downstream Dependencies

This issue unblocks:
- #1613 (Rate limit handling) - needs fork fields for graceful degradation
- #1614 (Priority ranking) - needs fork exclusion from auto-selection
- #1615 (Confirmation UX) - needs fork warning display
