# Issue 1508 Implementation Plan

## Analysis Summary

This is a validation gate issue for the batch PR coordination system.

### Workflow Bug Status

Both workflow bugs mentioned in the issue have **already been fixed**:

1. **Artifact filename colons** (Fixed)
   - Location: `.github/workflows/batch-generate.yml` lines 751-753, 912-913
   - Fix: `TIMESTAMP_SAFE=$(echo "$TIMESTAMP" | tr ':' '-')`
   - Verified: All failure files use hyphens (e.g., `homebrew-2026-02-07T23-29-03Z.jsonl`)

2. **jq null handling** (Fixed)
   - Location: `scripts/requeue-unblocked.sh` line 62
   - Fix: `grep -v '^\s*null\s*$' "$f" | grep -v '^\s*$' | jq -r '...'`
   - Verified: Script correctly filters null/empty lines

### System Validation

| Check | Status | Evidence |
|-------|--------|----------|
| Batch workflow completes | PASS | 10 consecutive successes |
| No artifact upload failures | PASS | All recent runs successful |
| No jq errors | PASS | Requeue step succeeds |
| Update-dashboard runs | PASS | 5 consecutive successes |
| No conflicting PRs | PASS | No open batch PRs |

### Implementation Approach

Since both bugs are already fixed, this PR will:
1. Document the validation results
2. Update the design doc to reflect completion
3. Close the validation gate

## Files to Modify

- `docs/designs/DESIGN-batch-pr-coordination.md` - Update status to Current, remove Implementation Issues section
- `wip/` artifacts - Clean up after validation

## Validation Checklist

From issue #1508 acceptance criteria:

### Workflow Bugs Fixed
- [x] Fix artifact upload failure: Colons replaced with hyphens in timestamp
- [x] Fix jq processing errors: Null filtering added to grep pipeline

### End-to-End Validation
- [x] Batch workflow completes successfully (verified 10 runs)
- [x] No artifact upload failures
- [x] No jq errors in requeue step
- [x] Update-dashboard workflow triggers and succeeds
- [x] No conflicting batch PRs exist
