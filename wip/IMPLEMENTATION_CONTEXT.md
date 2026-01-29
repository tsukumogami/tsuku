---
summary:
  constraints:
    - Job must not fail when batch is disabled (exit 0, not exit 1)
    - File missing treated as enabled (graceful default)
    - Must use jq for JSON parsing
  integration_points:
    - batch-control.json at repo root (reads enabled field)
    - .github/workflows/batch-operations.yml (new workflow file)
    - Downstream #1205 will add circuit breaker checks to this job structure
  risks:
    - Workflow doesn't exist yet; need to create it with placeholder structure
  approach_notes: |
    Create .github/workflows/batch-operations.yml with a pre-flight job that
    reads batch-control.json and outputs can_proceed. Include a placeholder
    processing job gated on the pre-flight output to demonstrate the pattern.
---
