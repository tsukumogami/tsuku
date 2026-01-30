---
summary:
  constraints:
    - Each section must follow R2 runbook template (Decision Tree, Steps, Resolution, Escalation)
    - All commands must include expected output examples
    - Must reference actual scripts and files (batch-control.json, scripts/rollback-batch.sh, scripts/check_breaker.sh, scripts/update_breaker.sh)
  integration_points:
    - batch-control.json (control file for emergency stop and circuit breaker state)
    - scripts/rollback-batch.sh (batch rollback procedure)
    - scripts/check_breaker.sh and scripts/update_breaker.sh (circuit breaker scripts)
    - .github/workflows/batch-operations.yml (batch workflow)
  risks:
    - Commands must match actual script interfaces
  approach_notes: |
    Create docs/runbooks/batch-operations.md with 5 sections matching the issue acceptance criteria.
    Use the design doc example for "Batch Success Rate Drop" as a template.
---
