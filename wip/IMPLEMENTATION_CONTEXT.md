---
summary:
  constraints:
    - Documentation-only issue (simple tier)
    - Must cover credential rotation, monitoring, troubleshooting, degradation response
    - Runbook location: docs/r2-golden-storage-runbook.md
  integration_points:
    - Workflows: r2-cleanup.yml, r2-health-monitor.yml, r2-cost-monitoring.yml
    - Scripts: r2-health-check.sh, r2-upload.sh, r2-download.sh, r2-cleanup.sh, r2-orphan-detection.sh, r2-retention-check.sh, r2-consistency-check.sh
    - GitHub Environments: registry-write
    - Secrets: R2_ACCESS_KEY_ID_READONLY, R2_SECRET_ACCESS_KEY_READONLY, R2_ACCESS_KEY_ID_WRITE, R2_SECRET_ACCESS_KEY_WRITE, R2_BUCKET_URL
  risks:
    - None (documentation only)
  approach_notes: |
    Create operational runbook documenting R2 golden storage procedures.
    Structure around the three main operational concerns: credentials, monitoring, troubleshooting.
    Include concrete commands and links to workflows.
---

# Implementation Context: Issue #1105

**Source**: docs/designs/DESIGN-r2-golden-storage.md (Phase 6: Monitoring)

## Key Design Requirements

1. **Credential Rotation SOP**:
   - 90-day (quarterly) rotation schedule
   - All four tokens documented
   - Verification before revoking old tokens

2. **Troubleshooting Guide**:
   - Health check failures
   - Upload/download failures
   - Checksum mismatches
   - Manifest inconsistencies

3. **Degradation Response**:
   - Interpreting GitHub issues
   - When to investigate vs wait
   - Manual validation trigger
   - Escalation path

4. **Environment Protection**:
   - `registry-write` environment
   - Reviewer approval workflow
   - Emergency access

## Existing Infrastructure

### Workflows
- `r2-cleanup.yml` - Weekly orphan/retention cleanup
- `r2-health-monitor.yml` - Every 6 hours health check
- `r2-cost-monitoring.yml` - Weekly storage usage check

### Scripts
- `r2-health-check.sh` - Health check (exit 0/1/2)
- `r2-upload.sh` - Upload to R2
- `r2-download.sh` - Download from R2
- `r2-cleanup.sh` - Cleanup orchestrator
- `r2-orphan-detection.sh` - Find orphaned files
- `r2-retention-check.sh` - Find excess versions
- `r2-consistency-check.sh` - Verify manifest consistency

## Files to Create

1. `docs/r2-golden-storage-runbook.md` - Operational runbook
