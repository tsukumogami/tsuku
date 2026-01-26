# Issue 1099 Implementation Plan

## Overview

Update PR validation workflows to use R2 golden files for registry recipes while maintaining git-based validation for embedded recipes.

## Key Insight

The TSUKU_GOLDEN_SOURCE environment variable was added in #1098. The validate-golden.sh script already handles:
- `git` mode: Use testdata/golden/plans/
- `r2` mode: Download from R2 and validate

The workflows just need to:
1. Run R2 health check
2. Set appropriate TSUKU_GOLDEN_SOURCE based on category and R2 health
3. Provide R2 credentials when using r2 mode

## Deliverables

### 1. Update validate-golden-recipes.yml

**Changes:**
- Add R2 health check step (reusable from nightly workflow pattern)
- Add R2 credentials to validation step (read-only)
- Set TSUKU_GOLDEN_SOURCE based on category:
  - embedded → git (always)
  - registry → r2 (if healthy) or skip with warning (if unhealthy)

**New workflow structure:**
```
jobs:
  validate-exclusions: (unchanged)
  detect-changes: (unchanged)
  r2-health-check:
    - Check R2 availability
    - Output: r2_healthy (true/false)
  validate-golden:
    - Add R2 credentials from secrets
    - Set TSUKU_GOLDEN_SOURCE based on category and r2_healthy
    - Skip registry recipes with warning if R2 unhealthy
```

### 2. Update validate-golden-execution.yml

**Consideration:** This workflow triggers on `testdata/golden/plans/**/*.json` changes. Registry golden files are now in R2, not git.

**Approach:** Maintain current behavior for embedded recipes (git-based), add recipe change trigger for registry execution validation with R2.

**Changes:**
- Add path trigger for `recipes/**/*.toml`
- Add R2 health check step
- For registry recipes: download golden files from R2, then execute
- For embedded recipes: use existing git-based execution

### 3. Graceful Degradation

When R2 is unavailable:
- Registry recipe validation: Skip with `::warning::` (not failure)
- Embedded recipe validation: Continue normally (uses git)
- PR can still pass if embedded validation succeeds

## Implementation Steps

### Step 1: Update validate-golden-recipes.yml

1. Add `r2-health-check` job (copy pattern from nightly-registry-validation.yml)
2. Add `needs: r2-health-check` to validate-golden job
3. Pass R2 credentials to validation step
4. Add logic to set TSUKU_GOLDEN_SOURCE and skip registry if R2 unhealthy

### Step 2: Update validate-golden-execution.yml

1. Add path trigger for `recipes/**/*.toml`
2. Add `r2-health-check` job
3. Modify detect-changes to handle:
   - Git golden file changes (existing behavior for embedded)
   - Recipe changes (new behavior for registry with R2 download)
4. Add R2 download step for registry recipes
5. Add graceful degradation for R2 unavailability

### Step 3: Test locally

Verify bash syntax for modified workflows:
```bash
bash -n .github/workflows/validate-golden-recipes.yml
bash -n .github/workflows/validate-golden-execution.yml
```

## Files to Modify

1. `.github/workflows/validate-golden-recipes.yml`
2. `.github/workflows/validate-golden-execution.yml`

## Testing

1. Verify existing CI workflows still pass
2. Test graceful degradation by reviewing workflow logic
3. Manual verification after merge (requires actual R2 access)
