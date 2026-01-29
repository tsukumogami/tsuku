---
summary:
  constraints:
    - Upload must use telemetry worker API endpoint, not direct D1 access
    - Upload step must run even when validation fails (if: always())
    - Failed upload must not block workflow completion (non-fatal with warning)
  integration_points:
    - telemetry/src/index.ts - new batch metrics API endpoint
    - .github/workflows/batch-operations.yml - post-batch upload step
    - D1 batch_runs and recipe_results tables (schema from #1208)
  risks:
    - Batch validation workflow may not produce structured output suitable for upload
    - Telemetry worker needs auth for write endpoint or public write like /event
  approach_notes: |
    Two components: (1) new POST /batch-metrics endpoint in telemetry worker that
    inserts into batch_runs + recipe_results tables, (2) workflow step that collects
    validation results and POSTs them to the endpoint. Design specifies per-ecosystem
    SLIs (Homebrew >= 85%, others >= 98%) powered by this data.
---
