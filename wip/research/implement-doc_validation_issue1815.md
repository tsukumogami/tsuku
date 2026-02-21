# Validation Report: Issue #1815

Issue: ci(golden-recipes): batch per-recipe jobs in validate-golden-recipes
Scenarios tested: scenario-2, scenario-9, scenario-10, scenario-13
Date: 2026-02-21

---

## Scenario 2: Config file includes validate-golden-recipes entry after second issue

**ID**: scenario-2
**Category**: infrastructure
**Environment**: automatable
**Status**: PASSED

**Command executed**:
```
jq -r '.batch_sizes["validate-golden-recipes"].default' .github/ci-batch-config.json
```

**Expected**: The value is `20`
**Actual**: `20`

The config file `.github/ci-batch-config.json` contains a `validate-golden-recipes` entry with `"default": 20` as specified by the design.

---

## Scenario 9: validate-golden-recipes.yml uses batch matrix with inner loop

**ID**: scenario-9
**Category**: infrastructure
**Environment**: automatable
**Status**: PASSED

**Commands executed and results**:

1. `grep 'matrix.item.recipe' .github/workflows/validate-golden-recipes.yml || echo 'NOT_FOUND'`
   - Result: `NOT_FOUND` (old per-recipe pattern removed -- as expected)

2. `grep 'matrix.batch' .github/workflows/validate-golden-recipes.yml`
   - Result: Found in two locations:
     - Job name: `"Validate (batch ${{ matrix.batch.batch_id + 1 }}/${{ needs.detect-changes.outputs.batch_count }})"`
     - Inner loop: `RECIPES='${{ toJson(matrix.batch.recipes) }}'`

3. `grep '::group::' .github/workflows/validate-golden-recipes.yml`
   - Result: Found: `echo "::group::Validate $RECIPE"`

4. `grep 'ci-batch-config' .github/workflows/validate-golden-recipes.yml`
   - Result: Found in detection job: reads from `.github/ci-batch-config.json` with `validate-golden-recipes` key

**Additional validations performed**:
- `fail-fast: false` is set on the strategy (confirmed)
- R2 env vars (`R2_BUCKET_URL`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_AVAILABLE`) are set at job level (confirmed)
- Failure accumulation via `FAIL_FILE` / `failed-recipes.txt` is present (confirmed)
- `::endgroup::` matching `::group::` markers are present (confirmed)
- `::error::Failed recipes` annotation at end of batch (confirmed)
- `timeout-minutes: 15` for the batch job (confirmed)
- `batch_size_override` input in `workflow_dispatch` with clamping to 1-50 range (confirmed)
- `detect-changes` outputs `batches`, `batch_count`, and `has_changes` (confirmed)
- jq ceiling-division pattern `range(0; length; $bs)` in detect-changes step (confirmed)

All checks passed.

---

## Scenario 10: validate-golden-recipes.yml preserves R2 health check dependency

**ID**: scenario-10
**Category**: infrastructure
**Environment**: automatable
**Status**: PASSED

**Command executed**:
```python
import yaml
wf = yaml.safe_load(open('.github/workflows/validate-golden-recipes.yml'))
needs = wf['jobs']['validate-golden']['needs']
assert 'r2-health-check' in needs and 'detect-changes' in needs
```

**Expected**: The `validate-golden` job depends on both `r2-health-check` and `detect-changes`
**Actual**: `needs: ['detect-changes', 'r2-health-check']`

Both dependencies are present. The `r2-health-check` job itself is preserved unchanged with its original structure (health check logic, `r2_available` output). The `validate-exclusions` job is also preserved unchanged.

---

## Scenario 13: End-to-end batch workflow runs on a multi-recipe PR

**ID**: scenario-13
**Category**: use-case
**Environment**: manual
**Status**: SKIPPED

**Reason**: `environment: GitHub Actions CI runner not available`

This scenario requires creating an actual PR with 5+ modified recipe TOML files and observing the GitHub Actions checks tab. This cannot be automated in a local testing environment. The scenario requires:
1. A branch with 5+ modified recipe files
2. A PR opened against `main`
3. Observation of GitHub Actions workflow execution
4. Verification of batched job naming (e.g., "Validate (batch 1/N)")
5. Verification of `::group::` sections in job logs
6. Verification that `::error::` annotations list failed recipe names

This requires human verification in a real GitHub Actions environment.

---

## Summary

| Scenario | Category | Status |
|----------|----------|--------|
| scenario-2 | infrastructure | PASSED |
| scenario-9 | infrastructure | PASSED |
| scenario-10 | infrastructure | PASSED |
| scenario-13 | use-case | SKIPPED (environment) |

Automatable scenarios: 3/3 passed
Manual scenarios: 1/1 skipped (requires GitHub Actions environment)
