---
summary:
  constraints:
    - PR validation must NOT block development when R2 is unavailable (graceful degradation)
    - Registry recipes use R2, embedded recipes remain in git
    - Health check required before R2 download (5s timeout, 2000ms latency threshold)
    - Two-tier degradation: R2 available → validate, R2 unavailable → skip with warning
  integration_points:
    - .github/workflows/validate-golden-recipes.yml (add R2 download, health check)
    - .github/workflows/validate-golden-execution.yml (add R2 download, health check)
    - scripts/validate-golden.sh (already has TSUKU_GOLDEN_SOURCE support from #1098)
    - scripts/r2-health-check.sh (existing health check script)
    - scripts/r2-download.sh (existing download helper)
  risks:
    - Workflows may need different error handling for R2 vs git failures
    - Category detection must be accurate to route to correct storage backend
    - Platform matrix detection needs to work with R2-downloaded files
    - Changed recipe detection (git diff) must continue to work
  approach_notes: |
    The key insight from the design: PR validation uses temporary artifacts, not committed
    golden files. After this change, "stored golden files" means R2 for registry recipes
    and git for embedded recipes.

    The TSUKU_GOLDEN_SOURCE environment variable support was added in #1098. The workflows
    need to:
    1. Run health check before attempting R2 download
    2. If healthy, set TSUKU_GOLDEN_SOURCE=r2 for registry recipes
    3. If unhealthy, skip registry validation with warning (not failure)
    4. Embedded recipes always use TSUKU_GOLDEN_SOURCE=git

    Graceful degradation table:
    | Scenario | Registry Recipes | Embedded Recipes | PR Status |
    |----------|------------------|------------------|-----------|
    | R2 available | Validate | Validate | Pass/Fail based on results |
    | R2 unavailable | Skip with warning | Validate | Warning if skipped |
---

# Implementation Context: Issue #1099

**Source**: docs/designs/DESIGN-r2-golden-storage.md (Phase 4: Migration - PR validation update)

## What This Issue Does

Update PR validation workflows to use R2 as the source of golden files for registry recipes, while embedded recipes continue to use git-based golden files.

## Prior Work (#1098)

The TSUKU_GOLDEN_SOURCE environment variable support was added to validate-golden.sh and validate-all-golden.sh. This allows selecting between:
- `git` (default): Use git-based golden files
- `r2`: Download from R2 and validate against those
- `both`: Validate against git, then compare with R2

The R2 download logic is already implemented in validate-golden.sh via the `download_r2_golden_files()` function.

## Key Files to Modify

1. `.github/workflows/validate-golden-recipes.yml` - Add R2 health check and conditional R2 download
2. `.github/workflows/validate-golden-execution.yml` - Add R2 health check and conditional R2 download

## Degradation Behavior

PR validation should NOT block development when R2 is unavailable:
- R2 available: Validate registry recipes against R2 golden files
- R2 unavailable: Skip registry validation with warning (not failure)
- Embedded recipes: Always validated against git (unaffected by R2 status)
