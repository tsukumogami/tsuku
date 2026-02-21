# Validation Report: Issue #1814

Generated: 2026-02-21

## Summary

All 9 testable scenarios passed. 7 infrastructure scenarios and 2 use-case scenarios were validated.

---

## Scenario 1: Config file exists and has valid JSON structure
**ID**: scenario-1
**Category**: infrastructure
**Status**: PASSED

**Test execution:**
1. `test -f .github/ci-batch-config.json` -- File exists.
2. `jq empty .github/ci-batch-config.json` -- Valid JSON.
3. `jq -r '.batch_sizes["test-changed-recipes"].linux' .github/ci-batch-config.json` -- Returns `15`.

**Expected**: File exists, is valid JSON, `.batch_sizes["test-changed-recipes"].linux` equals `15`.
**Actual**: All checks passed. Value is `15`.

---

## Scenario 3: Detection job outputs linux_batches and has_linux_batches
**ID**: scenario-3
**Category**: infrastructure
**Status**: PASSED

**Test execution:**
1. `grep 'linux_batches'` -- Found in outputs section (`steps.batch.outputs.linux_batches`), in the batch step's output writes, and in the `test-linux` job's condition/matrix reference.
2. `grep 'has_linux_batches'` -- Found in outputs section, batch step, and `test-linux` job's `if` condition.

**Expected**: Both appear in the workflow's matrix job outputs section.
**Actual**: Both `linux_batches` and `has_linux_batches` are declared as outputs of the `matrix` job and used throughout the workflow.

---

## Scenario 4: Batch splitting uses jq ceiling-division pattern
**ID**: scenario-4
**Category**: infrastructure
**Status**: PASSED

**Test execution:**
1. `grep -c 'range(0; length' .github/workflows/test-changed-recipes.yml` -- Returns `1`.

**Expected**: The jq ceiling-division pattern `range(0; length; $bs)` appears in the workflow.
**Actual**: Found exactly 1 occurrence in the batch splitting step.

---

## Scenario 5: test-linux job uses batch matrix instead of per-recipe matrix
**ID**: scenario-5
**Category**: infrastructure
**Status**: PASSED

**Test execution:**
1. `grep 'matrix.recipe.tool'` -- Returns `NOT_FOUND` (old per-recipe pattern removed).
2. `grep 'matrix.batch'` -- Found in job name template and `toJson(matrix.batch.recipes)`.
3. `grep 'batch_id'` -- Found in jq map expression and job name template.

**Expected**: `matrix.recipe.tool` NOT found, `matrix.batch` and `batch_id` present, job name includes batch index.
**Actual**: Job name is `"Linux (batch ${{ matrix.batch.batch_id + 1 }}/${{ needs.matrix.outputs.batch_count }})"` which includes batch index. All checks pass.

---

## Scenario 6: Inner loop has TSUKU_HOME isolation, shared cache, group annotations, and failure accumulation
**ID**: scenario-6
**Category**: infrastructure
**Status**: PASSED

**Test execution:**
1. `grep 'TSUKU_HOME.*runner.temp'` -- Found: `export TSUKU_HOME="${{ runner.temp }}/tsuku-$tool"` (appears twice: test-linux and test-macos).
2. `grep '::group::'` -- Found: `echo "::group::Testing $tool"` (appears twice).
3. `grep 'tsuku-cache/downloads'` -- Found: `CACHE_DIR="${{ runner.temp }}/tsuku-cache/downloads"` (appears twice).
4. `grep 'failed-recipes'` -- Found: `FAIL_FILE="${{ runner.temp }}/failed-recipes.txt"` (appears twice).

**Expected**: All four patterns present.
**Actual**: All four patterns present in both the test-linux and test-macos jobs.

---

## Scenario 7: workflow_dispatch has batch_size_override input with guard clause
**ID**: scenario-7
**Category**: infrastructure
**Status**: PASSED

**Test execution:**
1. `grep 'batch_size_override'` -- Found in:
   - `workflow_dispatch.inputs.batch_size_override` definition with description 'Override Linux batch size (1-50, 0 = use config default)'
   - Override reading: `OVERRIDE="${{ inputs.batch_size_override }}"`
   - Clamping warnings for below-minimum and exceeds-maximum cases
2. Clamping logic verified: clamps to 1 if < 1, clamps to 50 if > 50.

**Expected**: `batch_size_override` defined as workflow_dispatch input with clamping to 1-50 range.
**Actual**: Input defined with `type: number`, `default: 0`, and explicit clamping logic.

---

## Scenario 8: macOS job is unchanged
**ID**: scenario-8
**Category**: infrastructure
**Status**: PASSED

**Test execution:**
1. `grep 'macos_recipes'` -- Found in:
   - `matrix` job output: `macos_recipes: ${{ steps.changed.outputs.macos_recipes }}`
   - Detection step: writes `macos_recipes=` to GITHUB_OUTPUT
   - `test-macos` job: `RECIPES='${{ needs.matrix.outputs.macos_recipes }}'`
2. `grep 'test-macos'` -- Found: `test-macos:` job definition.

**Expected**: macOS path (test-macos job, macos_recipes output) remains intact.
**Actual**: The test-macos job and macos_recipes output are present and use the same inner-loop pattern (per-recipe isolation, group annotations, failure accumulation) consistent with an aggregated single-job approach for macOS.

---

## Scenario 11: Batch splitting produces correct output for known input
**ID**: scenario-11
**Category**: use-case
**Status**: PASSED

**Test execution:**
```
echo '[{"tool":"a","path":"a.toml"},{"tool":"b","path":"b.toml"},{"tool":"c","path":"c.toml"},{"tool":"d","path":"d.toml"},{"tool":"e","path":"e.toml"}]' | jq -c --argjson bs 2 '. as $all | [range(0; length; $bs)] | to_entries | map({batch_id: .key, recipes: $all[.value:.value + $bs]})'
```

**Expected**: 3 batches: batch 0 with [a,b], batch 1 with [c,d], batch 2 with [e].
**Actual output**:
```json
[{"batch_id":0,"recipes":[{"tool":"a","path":"a.toml"},{"tool":"b","path":"b.toml"}]},{"batch_id":1,"recipes":[{"tool":"c","path":"c.toml"},{"tool":"d","path":"d.toml"}]},{"batch_id":2,"recipes":[{"tool":"e","path":"e.toml"}]}]
```
Matches expected output exactly.

---

## Scenario 12: Single-recipe PR produces one batch with one recipe
**ID**: scenario-12
**Category**: use-case
**Status**: PASSED

**Test execution:**
```
echo '[{"tool":"only","path":"only.toml"}]' | jq -c --argjson bs 15 '. as $all | [range(0; length; $bs)] | to_entries | map({batch_id: .key, recipes: $all[.value:.value + $bs]})'
```

**Expected**: Exactly 1 batch: `[{"batch_id":0,"recipes":[{"tool":"only","path":"only.toml"}]}]`.
**Actual output**:
```json
[{"batch_id":0,"recipes":[{"tool":"only","path":"only.toml"}]}]
```
Matches expected output exactly. Backward compatibility with single-recipe PRs confirmed.
