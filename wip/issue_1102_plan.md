# Issue 1102 Implementation Plan

## Summary

Create a weekly R2 cleanup workflow with three components: version retention enforcement script, main cleanup orchestrator script, and GitHub Actions workflow with scheduled and manual triggers.

## Approach

Build on the existing orphan detection script (r2-orphan-detection.sh) to create a complete cleanup system. The scripts will follow established patterns from r2-health-check.sh and r2-upload.sh for credential handling, output formatting, and error codes. Use a quarantine-based soft delete approach to prevent accidental data loss, with hard delete only after 7-day verification period.

## Files to Create

- `scripts/r2-retention-check.sh` - Script to detect versions exceeding 2-version retention policy per recipe per platform
- `scripts/r2-cleanup.sh` - Main orchestrator that combines orphan detection and retention checking, performs soft/hard deletes
- `.github/workflows/r2-cleanup.yml` - Weekly scheduled workflow with manual dispatch and dry-run support

## Implementation Steps

- [ ] Create `scripts/r2-retention-check.sh`:
  - Accept `--json` output format flag for CI integration
  - Accept `--recipe <name>` to check specific recipe (optional)
  - Accept `--dry-run` for report-only mode
  - List all versions per recipe per platform from R2
  - Parse semantic versions, sort descending
  - Identify versions beyond the latest 2 per platform
  - Output excess version keys in plain text or JSON format
  - Handle pre-release versions (count toward limit)
  - Exit codes: 0 success, 1 error, 2 invalid args

- [ ] Create `scripts/r2-cleanup.sh`:
  - Accept `--dry-run` to report without acting
  - Accept `--max-deletions <N>` to limit operations per run (default 100)
  - Accept `--hard-delete` to remove quarantined files older than 7 days
  - Accept `--skip-retention` to skip retention check (orphans only)
  - Accept `--skip-orphans` to skip orphan check (retention only)
  - Orchestration flow:
    1. Run r2-health-check.sh to verify R2 availability
    2. Run r2-orphan-detection.sh to get orphaned keys
    3. Run r2-retention-check.sh to get excess version keys
    4. Combine results, deduplicate
    5. Soft delete: copy to `quarantine/{date}/` prefix, then delete original
    6. Hard delete: remove items in quarantine/ older than 7 days
    7. Update manifest.json after deletions
  - Output summary: orphans quarantined, versions quarantined, hard deleted
  - Exit codes: 0 success, 1 error, 2 invalid args

- [ ] Create `.github/workflows/r2-cleanup.yml`:
  - Triggers: weekly schedule (Sunday 4 AM UTC), workflow_dispatch
  - Inputs for manual dispatch:
    - `dry_run` (boolean, default true)
    - `hard_delete` (boolean, default false)
    - `max_deletions` (number, default 100)
  - Jobs:
    1. `health-check`: Verify R2 availability before proceeding
    2. `detect`: Run detection scripts, output counts
    3. `cleanup`: Execute soft/hard deletes if not dry-run
    4. `report`: Create GitHub issue summarizing actions taken
  - Use write credentials (R2_ACCESS_KEY_ID_WRITE, R2_SECRET_ACCESS_KEY_WRITE)
  - Environment: `registry-write` for write operations
  - Dry-run mode: log what would happen without acting
  - Create tracking issue with summary of actions taken

- [ ] Test scripts locally with dry-run mode:
  - Verify r2-retention-check.sh identifies excess versions correctly
  - Verify r2-cleanup.sh orchestration flow works
  - Verify quarantine path format: `quarantine/{YYYY-MM-DD}/plans/...`

- [ ] Validate workflow YAML syntax

## Success Criteria

- [ ] Version retention script correctly identifies versions exceeding 2-per-platform policy
- [ ] Orphan detection from existing script integrates with cleanup orchestrator
- [ ] Soft delete moves files to quarantine/ prefix with dated subdirectory
- [ ] Hard delete only removes quarantine files older than 7 days
- [ ] Dry-run mode reports actions without performing them
- [ ] Workflow runs on weekly schedule and supports manual dispatch
- [ ] Cleanup creates GitHub issue summarizing actions taken
- [ ] Maximum deletion limit prevents runaway cleanup
- [ ] All validation tests from issue pass

## Open Questions

None - the design document and existing scripts provide sufficient guidance for implementation.
