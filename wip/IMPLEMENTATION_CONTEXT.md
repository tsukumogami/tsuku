---
summary:
  constraints:
    - Fail-open policy: PR always created, auto-merge is an optimization
    - Uses PAT_BATCH_GENERATE secret for gh pr merge
    - Only enable auto-merge when EXCLUDED_COUNT=0
  integration_points:
    - .github/workflows/batch-generate.yml (add step after PR creation)
    - EXCLUDED_COUNT env var (already computed in aggregation step)
  risks:
    - EXCLUDED_COUNT may not be exported to env properly
    - PR URL needs to be captured from gh pr create output
  approach_notes: |
    Add single step after PR creation. Check EXCLUDED_COUNT, either
    gh pr merge --auto --squash or gh pr comment. Also add --subject/--body
    for squash commit message (Part 1 pattern from earlier experiment).
---
