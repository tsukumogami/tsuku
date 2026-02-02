---
summary:
  constraints:
    - Retain workflow_dispatch triggers alongside schedule
    - Homebrew only for now (single ecosystem)
    - Circuit breaker must gate scheduled runs
  integration_points:
    - .github/workflows/seed-queue.yml (add schedule trigger)
    - .github/workflows/batch-generate.yml (add schedule trigger with default inputs)
  risks:
    - Scheduled batch-generate needs default input values since cron can't pass inputs
    - Empty queue runs must exit gracefully (no PR, no error)
  approach_notes: |
    Add schedule cron to both workflows. For batch-generate, scheduled runs
    need hardcoded defaults (ecosystem=homebrew, batch_size=10) since cron
    triggers don't support inputs. seed-queue similarly needs defaults.
    Verify empty queue handling in the existing "Check for recipes to merge" step.
---

# Implementation Context: Issue #1412

**Source**: docs/designs/DESIGN-registry-scale-strategy.md

Two workflow files need schedule triggers added. The seed-queue already uses
`queue.Merge()` which is idempotent. The batch-generate workflow already handles
empty results via the `steps.check.outputs.changes` gate.
