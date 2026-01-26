---
summary:
  constraints:
    - Bulk upload already completed via #1097 (most files in R2)
    - Nightly validation already uses R2 via #1096
    - Must not disrupt existing CI workflows
    - Both git and R2 sources must work during transition
  integration_points:
    - scripts/validate-golden.sh (needs source selection)
    - scripts/validate-all-golden.sh (needs source selection)
    - scripts/r2-download.sh (for R2 access)
    - .github/workflows/nightly-registry-validation.yml (already uses R2)
    - .github/workflows/validate-golden-*.yml (PR validation, still uses git)
  risks:
    - Some recipes may have mismatched content between git and R2 (different versions)
    - R2 may have newer versions than git due to post-merge generation
    - Consistency check may fail for recipes that were regenerated
  approach_notes: |
    Key insight: #1097 already uploaded files to R2, and #1096 integrated R2 into
    nightly validation. This issue (#1098) focuses on:
    1. Adding consistency check script to compare git vs R2
    2. Adding TSUKU_GOLDEN_SOURCE env var for source selection
    3. Updating validation scripts to support both sources
    4. Updating design doc dependency graph

    The bulk upload mentioned in the issue is already done. Focus on the parallel
    operation infrastructure and consistency verification.
---

# Implementation Context: Issue #1098

**Source**: docs/designs/DESIGN-r2-golden-storage.md

## What's Already Done (from prior issues)

- **#1093**: R2 infrastructure setup (bucket, credentials)
- **#1094**: Helper scripts (r2-upload.sh, r2-download.sh, r2-health-check.sh)
- **#1095**: Post-merge generation workflow (publish-golden-to-r2.yml)
- **#1096**: Nightly validation uses R2 with health check and degradation
- **#1097**: Bulk migration of golden files to R2

## What This Issue Needs

From the acceptance criteria:
- [x] All existing golden files uploaded to R2 (done in #1097)
- [ ] Manifest.json populated (not implemented yet - optional)
- [x] Upload verification (done in #1097 workflow)
- [ ] TSUKU_GOLDEN_SOURCE environment variable
- [ ] Consistency check script (git vs R2)
- [ ] Consistency check passes
- [ ] Existing CI workflows continue to pass
- [ ] Documentation updated

## Key Deliverables

1. `scripts/r2-consistency-check.sh` - Compare git vs R2 content
2. Update validate-golden.sh with TSUKU_GOLDEN_SOURCE support
3. Update validate-all-golden.sh with TSUKU_GOLDEN_SOURCE support
4. Update design doc dependency graph (mark #1097 done, #1098 ready)
