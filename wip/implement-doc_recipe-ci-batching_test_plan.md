# Test Plan: Recipe CI Batching

Generated from: docs/designs/DESIGN-recipe-ci-batching.md
Issues covered: 3
Total scenarios: 14

---

## Scenario 1: Config file exists and has valid JSON structure
**ID**: scenario-1
**Testable after**: #1814
**Category**: infrastructure
**Environment**: automatable
**Commands**:
- `test -f .github/ci-batch-config.json`
- `jq empty .github/ci-batch-config.json`
- `jq -r '.batch_sizes["test-changed-recipes"].linux' .github/ci-batch-config.json`
**Expected**: File exists, is valid JSON, and `.batch_sizes["test-changed-recipes"].linux` equals `15`
**Status**: passed (validated #1814)

---

## Scenario 2: Config file includes validate-golden-recipes entry after second issue
**ID**: scenario-2
**Testable after**: #1815
**Category**: infrastructure
**Environment**: automatable
**Commands**:
- `jq -r '.batch_sizes["validate-golden-recipes"].default' .github/ci-batch-config.json`
**Expected**: The value is `20`
**Status**: passed (validated #1815)

---

## Scenario 3: Detection job outputs linux_batches and has_linux_batches
**ID**: scenario-3
**Testable after**: #1814
**Category**: infrastructure
**Environment**: automatable
**Commands**:
- `grep 'linux_batches' .github/workflows/test-changed-recipes.yml`
- `grep 'has_linux_batches' .github/workflows/test-changed-recipes.yml`
**Expected**: Both `linux_batches` and `has_linux_batches` appear in the workflow's matrix job outputs section
**Status**: passed (validated #1814)

---

## Scenario 4: Batch splitting uses jq ceiling-division pattern
**ID**: scenario-4
**Testable after**: #1814
**Category**: infrastructure
**Environment**: automatable
**Commands**:
- `grep -c 'range(0; length' .github/workflows/test-changed-recipes.yml`
**Expected**: The jq ceiling-division pattern `range(0; length; $bs)` appears in the workflow
**Status**: passed (validated #1814)

---

## Scenario 5: test-linux job uses batch matrix instead of per-recipe matrix
**ID**: scenario-5
**Testable after**: #1814
**Category**: infrastructure
**Environment**: automatable
**Commands**:
- `grep 'matrix.recipe.tool' .github/workflows/test-changed-recipes.yml || echo 'NOT_FOUND'`
- `grep 'matrix.batch' .github/workflows/test-changed-recipes.yml`
- `grep 'batch_id' .github/workflows/test-changed-recipes.yml`
**Expected**: `matrix.recipe.tool` is NOT found (old per-recipe pattern removed). `matrix.batch` and `batch_id` are present. Job name includes batch index.
**Status**: passed (validated #1814)

---

## Scenario 6: Inner loop has TSUKU_HOME isolation, shared cache, group annotations, and failure accumulation
**ID**: scenario-6
**Testable after**: #1814
**Category**: infrastructure
**Environment**: automatable
**Commands**:
- `grep 'TSUKU_HOME.*runner.temp' .github/workflows/test-changed-recipes.yml`
- `grep '::group::' .github/workflows/test-changed-recipes.yml`
- `grep 'tsuku-cache/downloads' .github/workflows/test-changed-recipes.yml`
- `grep 'failed-recipes' .github/workflows/test-changed-recipes.yml`
**Expected**: All four patterns are present: per-recipe TSUKU_HOME under runner.temp, `::group::` annotations, shared download cache directory, and failure accumulation file
**Status**: passed (validated #1814)

---

## Scenario 7: workflow_dispatch has batch_size_override input with guard clause
**ID**: scenario-7
**Testable after**: #1814
**Category**: infrastructure
**Environment**: automatable
**Commands**:
- `grep 'batch_size_override' .github/workflows/test-changed-recipes.yml`
**Expected**: `batch_size_override` is defined as a workflow_dispatch input. The workflow contains logic to clamp values to the 1-50 range.
**Status**: passed (validated #1814)

---

## Scenario 8: macOS job is unchanged
**ID**: scenario-8
**Testable after**: #1814
**Category**: infrastructure
**Environment**: automatable
**Commands**:
- `grep 'macos_recipes' .github/workflows/test-changed-recipes.yml`
- `grep 'test-macos' .github/workflows/test-changed-recipes.yml`
**Expected**: The macOS path (`test-macos` job, `macos_recipes` output) remains intact and unmodified from the current workflow
**Status**: passed (validated #1814)

---

## Scenario 9: validate-golden-recipes.yml uses batch matrix with inner loop
**ID**: scenario-9
**Testable after**: #1815
**Category**: infrastructure
**Environment**: automatable
**Commands**:
- `grep 'matrix.item.recipe' .github/workflows/validate-golden-recipes.yml || echo 'NOT_FOUND'`
- `grep 'matrix.batch' .github/workflows/validate-golden-recipes.yml`
- `grep '::group::' .github/workflows/validate-golden-recipes.yml`
- `grep 'ci-batch-config' .github/workflows/validate-golden-recipes.yml`
**Expected**: Old per-recipe `matrix.item.recipe` is NOT found. `matrix.batch` is used instead. Inner loop has `::group::` annotations. Detection job reads from `ci-batch-config.json`.
**Status**: passed (validated #1815)

---

## Scenario 10: validate-golden-recipes.yml preserves R2 health check dependency
**ID**: scenario-10
**Testable after**: #1815
**Category**: infrastructure
**Environment**: automatable
**Commands**:
- `python3 -c "import yaml; wf=yaml.safe_load(open('.github/workflows/validate-golden-recipes.yml')); needs=wf['jobs']['validate-golden']['needs']; assert 'r2-health-check' in needs and 'detect-changes' in needs, f'needs={needs}'"`
**Expected**: The `validate-golden` job depends on both `r2-health-check` and `detect-changes`
**Status**: passed (validated #1815)

---

## Scenario 11: Batch splitting produces correct output for known input
**ID**: scenario-11
**Testable after**: #1814
**Category**: use-case
**Environment**: automatable
**Commands**:
- `echo '[{"tool":"a","path":"a.toml"},{"tool":"b","path":"b.toml"},{"tool":"c","path":"c.toml"},{"tool":"d","path":"d.toml"},{"tool":"e","path":"e.toml"}]' | jq -c --argjson bs 2 '. as $all | [range(0; length; $bs)] | to_entries | map({batch_id: .key, recipes: $all[.value:.value + $bs]})'`
**Expected**: Produces 3 batches: batch 0 with [a,b], batch 1 with [c,d], batch 2 with [e]. This validates the jq ceiling-division logic that the workflow uses.
**Status**: passed (validated #1814)

---

## Scenario 12: Single-recipe PR produces one batch with one recipe
**ID**: scenario-12
**Testable after**: #1814
**Category**: use-case
**Environment**: automatable
**Commands**:
- `echo '[{"tool":"only","path":"only.toml"}]' | jq -c --argjson bs 15 '. as $all | [range(0; length; $bs)] | to_entries | map({batch_id: .key, recipes: $all[.value:.value + $bs]})'`
**Expected**: Produces exactly 1 batch: `[{"batch_id":0,"recipes":[{"tool":"only","path":"only.toml"}]}]`. Backward compatibility with single-recipe PRs is preserved.
**Status**: passed (validated #1814)

---

## Scenario 13: End-to-end batch workflow runs on a multi-recipe PR
**ID**: scenario-13
**Testable after**: #1814, #1815
**Category**: use-case
**Environment**: manual
**Commands**:
- Create a branch that modifies 5+ recipe TOML files (e.g., add a trailing comment to each)
- Open a PR targeting `main`
- Observe the GitHub Actions checks tab
**Expected**: The `test-changed-recipes` workflow produces batched jobs named like "Linux batch 0" (or similar with batch index) instead of individual jobs per recipe. The `validate-golden-recipes` workflow similarly produces batched jobs named like "Validate (batch 1/N)". Each batched job's log shows `::group::` sections for individual recipes. If any recipe fails, the `::error::` annotation at the end of the job lists the failed recipe names. macOS jobs remain unchanged (one aggregated job). Total job count is significantly less than 2 * N (where N is the number of changed recipes).
**Status**: skipped (validated #1815 -- environment: GitHub Actions CI runner not available)

---

## Scenario 14: Documentation covers batch config, override, and tuning
**ID**: scenario-14
**Testable after**: #1816
**Category**: use-case
**Environment**: automatable
**Commands**:
- `grep -l 'batch_size_override' docs/workflow-validation-guide.md CONTRIBUTING.md`
- `grep -l 'ci-batch-config' docs/workflow-validation-guide.md CONTRIBUTING.md`
- `grep -l 'batch' CONTRIBUTING.md`
**Expected**: At least one of `docs/workflow-validation-guide.md` or `CONTRIBUTING.md` documents: (1) what batch sizes control and where they're configured, (2) how to use the `batch_size_override` input, (3) the valid range 1-50, and (4) guidelines for when to increase or decrease batch sizes. `CONTRIBUTING.md` mentions that recipe CI uses batched jobs.
**Status**: pending

---
