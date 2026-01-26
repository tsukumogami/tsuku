---
summary:
  constraints:
    - R2 object key convention: plans/{category}/{recipe}/v{version}/{platform}.json
    - Retention policy: keep latest 2 versions per recipe per platform
    - Quarantine workflow: soft delete to quarantine/{date}/... before hard delete
    - Maximum deletion limit per run to prevent runaway cleanup
    - Write credentials required (R2_ACCESS_KEY_ID_WRITE, R2_SECRET_ACCESS_KEY_WRITE)
  integration_points:
    - scripts/r2-orphan-detection.sh - existing orphan detection (from #1101)
    - .github/workflows/ - new r2-cleanup.yml workflow
    - AWS CLI - for R2 object operations (list, copy, delete)
  risks:
    - Accidental deletion of valid golden files (mitigated by quarantine period)
    - Version comparison edge cases (pre-release versions, non-semver)
    - Large bucket pagination may take time
  approach_notes: |
    Build on the existing orphan detection script. Create three new scripts:
    1. r2-retention-check.sh - identifies versions exceeding retention policy
    2. r2-cleanup.sh - orchestrates cleanup with soft delete and hard delete
    3. .github/workflows/r2-cleanup.yml - weekly scheduled workflow

    Use quarantine prefix for soft delete: quarantine/{date}/plans/...
    Hard delete removes quarantine items older than 7 days.
---

# Implementation Context: Issue #1102

**Source**: docs/designs/DESIGN-r2-golden-storage.md (Phase 5: Cleanup Automation)

## Key Design Requirements

1. **Version retention**: Keep latest 2 versions per recipe per platform
2. **Orphan detection**: Already implemented in #1101, reuse for cleanup
3. **Soft delete**: Move to quarantine/ prefix rather than immediate deletion
4. **Hard delete**: Remove quarantined files older than 7 days
5. **Safety features**:
   - Dry-run mode (no actual deletions)
   - Maximum deletion limit per run
   - GitHub issue summarizing actions taken

## Quarantine Workflow

```
Detection → Grace period → Soft delete → Hard delete

plans/a/ack/v3.8.0/linux-amd64.json
→ quarantine/2026-01-24/plans/a/ack/v3.8.0/linux-amd64.json
```

## Files to Create

1. `scripts/r2-retention-check.sh` - Version retention detection
2. `scripts/r2-cleanup.sh` - Main cleanup orchestrator
3. `.github/workflows/r2-cleanup.yml` - Weekly scheduled workflow

## Existing Integration Points

- `scripts/r2-orphan-detection.sh` - Already detects deleted recipes
- `scripts/r2-health-check.sh` - Can verify R2 availability before cleanup
- `scripts/r2-download.sh`, `scripts/r2-upload.sh` - Helper patterns to follow
