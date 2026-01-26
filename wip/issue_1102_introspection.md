# Issue 1102 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-r2-golden-storage.md` (Phase 5: Cleanup Automation)
- Sibling issues reviewed: #1101 (orphan detection - closed)
- Prior patterns identified:
  - R2 helper scripts in `scripts/r2-*.sh`
  - AWS CLI with S3-compatible API for R2 access
  - Environment variable pattern: `R2_BUCKET_URL`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`
  - Health check pattern from `r2-health-check.sh`
  - Scheduled workflow pattern from `nightly-registry-validation.yml` (cron at 2 AM UTC)

## Gap Analysis

### Minor Gaps

1. **Orphan detection integration**: Issue #1101 delivered `scripts/r2-orphan-detection.sh` which outputs orphaned object keys one per line. The issue spec for #1102 already references this correctly in its "Orphan detection identifies..." acceptance criteria, but the spec assumes a script named `r2-orphan-check.sh` in the validation section. The actual script is `r2-orphan-detection.sh`. Update validation script reference.

2. **Script naming convention**: The validation section references `./scripts/r2-cleanup.sh` and `./scripts/r2-retention-check.sh`. These should follow the established `r2-` prefix convention (which they do), but the exact names should be confirmed as the implementation deliverables.

3. **Write credentials**: The issue mentions write credentials for deletion but doesn't specify the environment variable names. From existing patterns, these are `R2_ACCESS_KEY_ID_WRITE` and `R2_SECRET_ACCESS_KEY_WRITE` (referenced in design doc security section). The cleanup workflow will need these.

4. **Environment protection**: Write operations should use the `registry-write` GitHub Environment per the design doc's security section. The workflow implementation should include this protection.

### Moderate Gaps

None identified. The issue spec is comprehensive and aligns with the design document and completed sibling work.

### Major Gaps

None identified. Issue #1101's implementation matches expectations - it delivers a script that outputs orphaned object keys suitable for piping to cleanup commands.

## Recommendation

**Proceed**

The issue spec is complete and aligned with the design document. The only updates needed are minor corrections to script names in the validation section, which can be handled during implementation.

## Proposed Amendments

None required. The minor script name discrepancy in the validation section (`r2-orphan-check.sh` vs `r2-orphan-detection.sh`) is a trivial typo that will naturally be corrected during implementation when integrating with the actual delivered script.

## Implementation Notes from Prior Work

The following patterns from #1101 and existing R2 scripts should be followed:

1. **Script structure**: Use the same header style, argument parsing pattern, and environment variable validation as `r2-orphan-detection.sh`

2. **Error handling**: Exit codes 0 (success), 1 (error), 2 (invalid arguments) - consistent with existing scripts

3. **Output modes**: Support both plain text and `--json` output for integration flexibility

4. **Dry-run default**: The orphan detection script defaults to dry-run mode; the cleanup script should follow the same pattern

5. **AWS CLI pattern**: Use `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_ENDPOINT_URL` environment exports for S3 API calls

6. **Pagination handling**: The orphan detection script handles R2 bucket pagination - cleanup scripts should do the same when listing objects

7. **Temporary files**: Use `mktemp` with trap cleanup, as demonstrated in orphan detection

8. **Workflow conventions**:
   - Pin actions to commit SHAs per design doc security requirements
   - Use `workflow_dispatch` for manual triggers
   - Follow nightly validation pattern for scheduled runs
