---
summary:
  constraints:
    - Shell + jq only, no Go changes
    - Metrics appended to data/metrics/batch-runs.jsonl
    - Uses BATCH_ID from existing step as primary key
    - Step goes after constraint derivation, before PR creation
  integration_points:
    - .github/workflows/batch-generate.yml (merge job)
    - data/metrics/batch-runs.jsonl (new file)
    - scripts/batch-metrics.sh (new reporting script)
    - validation-results-*.json (read per-platform counts)
  risks:
    - Must include data/metrics/ in git add for PR commit
    - Duration tracking needs a start timestamp captured early
  approach_notes: |
    1. Create data/metrics/.gitkeep
    2. Add metrics collection step using jq to read validation-results files
    3. Add data/metrics/ to git add in PR creation step
    4. Create scripts/batch-metrics.sh for human-readable output
---
