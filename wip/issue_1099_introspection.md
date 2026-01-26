# Issue 1099 Introspection

## Context Reviewed
- Design doc: docs/designs/DESIGN-r2-golden-storage.md
- Sibling issues reviewed: #1093, #1094, #1095, #1096, #1097, #1098 (all closed)
- Prior patterns identified: R2 helper scripts, health check, TSUKU_GOLDEN_SOURCE

## Gap Analysis

### Minor Gaps

1. **TSUKU_GOLDEN_SOURCE already implemented**: #1098 added environment variable support to validate-golden.sh. The workflows just need to use it.

2. **Health check pattern established**: The nightly-registry-validation.yml already implements the R2 health check pattern from #1096.

3. **Execution workflow path trigger**: The validate-golden-execution.yml currently triggers on `testdata/golden/plans/**/*.json` changes. For registry recipes, the golden files are now in R2, not git. Need to reconsider the trigger.

### Moderate Gaps

1. **Execution workflow fundamental change**: The validate-golden-execution.yml is designed around git-based golden file paths. With R2 migration:
   - Registry golden files no longer live in git
   - The workflow trigger `testdata/golden/plans/**/*.json` won't catch registry golden file changes
   - Need to trigger on recipe changes instead and download golden files from R2

   **Proposed approach**: For registry recipes, trigger execution validation on recipe file changes, download golden files from R2, then execute.

### Major Gaps
- None

## Recommendation
Proceed with clear approach - the infrastructure is in place, just need to wire it into the PR validation workflows.

## Key Patterns from Prior Issues

1. **R2 health check** (from #1094, #1096):
   - `scripts/r2-health-check.sh` - 5s timeout, 2000ms latency threshold
   - Returns exit code 0 for healthy, non-zero for degraded/failure

2. **TSUKU_GOLDEN_SOURCE** (from #1098):
   - Already implemented in validate-golden.sh
   - Set to `r2` for registry recipes when R2 is healthy
   - Downloads files automatically via `download_r2_golden_files()`

3. **Graceful degradation** (design doc):
   - R2 unavailable → skip registry validation with warning (not failure)
   - Embedded recipes always use git validation

4. **R2 credentials** (from #1094):
   - R2_BUCKET_URL, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY
   - Read-only tokens sufficient for validation

## Implementation Strategy

### validate-golden-recipes.yml

1. Add R2 health check step before validation
2. For registry recipes:
   - If R2 healthy: Set TSUKU_GOLDEN_SOURCE=r2
   - If R2 unhealthy: Skip with warning
3. Embedded recipes always use TSUKU_GOLDEN_SOURCE=git

### validate-golden-execution.yml

Key insight: This workflow is about executing plans to verify they work. Currently triggers on golden file changes.

**For registry recipes:**
- Can't trigger on golden file changes (files are in R2)
- Could trigger on recipe changes and download golden files from R2
- But this duplicates validate-golden-recipes.yml's change detection

**Simpler approach:**
- Keep git-path trigger for embedded recipes
- Add separate R2-based execution validation to nightly workflow (already done in #1096)
- PR execution validation focuses on what changed in git (embedded recipes)

**Or:**
- Add recipe change trigger to validate-golden-execution.yml
- When registry recipe changes, download from R2 and execute

For now, let the existing execution validation focus on embedded recipes (git-based), and the nightly workflow handles R2-based registry execution. This minimizes PR validation complexity while maintaining coverage.
