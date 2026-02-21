# Scrutiny Review: Completeness -- Issue #1815

**Issue**: #1815 (ci(golden-recipes): batch per-recipe jobs in validate-golden-recipes)
**Scrutiny focus**: completeness
**Files changed**: `.github/ci-batch-config.json`, `.github/workflows/validate-golden-recipes.yml`

---

## Independent Assessment of Implementation (Diff-First)

Before evaluating the mapping, I read the full diff.

The diff makes the following changes:

**`.github/ci-batch-config.json`**: Adds a `validate-golden-recipes` entry with `"default": 20`.

**`.github/workflows/validate-golden-recipes.yml`**:
- Adds `workflow_dispatch.inputs.batch_size_override` (type: number, default: 0, description notes 1-50 range)
- Changes `detect-changes` outputs: removes `recipes`, adds `batches` and `batch_count`
- Adds a "Split recipes into batches" step in `detect-changes` that reads batch size from config (falling back to 20), clamps override to 1-50, and splits using the jq ceiling-division pattern
- `validate-golden` job: renames from per-recipe to per-batch matrix, uses `fromJson(needs.detect-changes.outputs.batches)` for matrix, adds job-level R2 env vars, removes per-recipe `actions/cache` step, replaces single-recipe validation step with an inner loop using `::group::`, failure accumulation via `FAIL_FILE`, and exits non-zero after all recipes if any failed
- `validate-exclusions` and `r2-health-check` jobs: unchanged

The implementation is complete and correct. All major structural changes align with the issue's requirements.

---

## AC Extraction from Issue Body

I extracted the full set of acceptance criteria from the issue. The issue has the following AC groups:

### Group 1: Detection job changes
1. `detect-changes` reads `validate-golden-recipes` batch size from `.github/ci-batch-config.json` (default key, falling back to 20 if absent)
2. Detection job splits recipes into batches using the same jq ceiling-division pattern from #1814
3. New outputs: `batches` (JSON array of batch objects) and `batch_count` (integer)
4. Existing `has_changes` output continues to work
5. Batch object shape: `{"batch_id": N, "recipes": [{"recipe": "...", "category": "..."}, ...]}`
6. `workflow_dispatch` input supports `batch_size_override` with a guard clause clamping to 1-50

### Group 2: Execution job changes
7. `validate-golden` job matrix iterates over `batches` instead of individual `recipes`
8. Job naming: `Validate (batch ${{ matrix.batch.batch_id + 1 }}/${{ needs.detect-changes.outputs.batch_count }})`
9. Each batch job checks out the repo, sets up Go, and builds tsuku once
10. Inner loop iterates over `matrix.batch.recipes`, calling `validate-golden.sh` per recipe with correct `RECIPE` and `CATEGORY` env vars
11. Category is derived from the recipe object in the batch (not re-derived from path)
12. R2-related env vars (`R2_BUCKET_URL`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_AVAILABLE`) are set at the job level, not per-recipe
13. `TSUKU_GOLDEN_SOURCE` determination and R2 existence check run per-recipe inside the inner loop
14. Each recipe runs inside `::group::Validate <recipe-name>::endgroup::` for log clarity
15. Failures are accumulated (not fail-fast) and job exits non-zero after all recipes if any failed, with `::error::` annotations listing failed recipes

### Group 3: R2 health check and dependencies
16. `r2-health-check` job is preserved unchanged
17. `validate-golden` job depends on both `detect-changes` and `r2-health-check`
18. `validate-exclusions` job is preserved unchanged

### Group 4: Cache key strategy
19. Per-recipe `actions/cache` key (`golden-downloads-${{ matrix.item.recipe }}`) is replaced with a batch-level approach
20. Since golden validation is lightweight, either use a shared cache key or remove the download cache entirely
21. If a shared cache is used, the key should be `golden-downloads-batch-${{ matrix.batch.batch_id }}` with restore-keys falling back to `golden-downloads-`

### Group 5: Deliverables for downstream
22. Working batched `validate-golden-recipes.yml` workflow (required by #1816)
23. `validate-golden-recipes` entry in `.github/ci-batch-config.json` with `"default": 20` (required by #1816)

---

## Requirements Mapping Evaluation

The provided mapping contains 11 entries. The issue has 23 distinct ACs. The mapping covers a subset of ACs (roughly the most visible structural ones) while omitting 12.

### Mapped ACs -- Verification

**AC: "detect-changes reads batch size from ci-batch-config.json"** -- claimed `implemented`
- Evidence cited: "validate-golden-recipes.yml Split recipes into batches step"
- Diff confirms: Line `BATCH_SIZE=$(jq -r '.batch_sizes["validate-golden-recipes"].default // 20' .github/ci-batch-config.json)` in the "Split recipes into batches" step. Verified.

**AC: "jq ceiling-division pattern for batch splitting"** -- claimed `implemented`
- Diff confirms: The jq expression using `[range(0; length; $bs)] | to_entries | map({batch_id: .key, recipes: $all[.value:.value + $bs]})` matches the design doc pattern exactly. Verified.

**AC: "New outputs: batches and batch_count"** -- claimed `implemented`
- Diff confirms: `outputs:` section adds `batches: ${{ steps.batch.outputs.batches }}` and `batch_count: ${{ steps.batch.outputs.batch_count }}`. Verified.

**AC: "has_changes output preserved"** -- claimed `implemented`
- Diff confirms: `has_changes: ${{ steps.changed.outputs.has_changes }}` remains in `detect-changes` outputs. The `changed` step still writes `has_changes` to `$GITHUB_OUTPUT`. Verified.

**AC: "batch_size_override with 1-50 clamping"** -- claimed `implemented`
- Diff confirms: `batch_size_override` input added; clamping logic uses `[ "$BATCH_SIZE" -lt 1 ]` and `[ "$BATCH_SIZE" -gt 50 ]` guards. Verified.

**AC: "validate-golden job uses batch matrix"** -- claimed `implemented`
- Diff confirms: `matrix: batch: ${{ fromJson(needs.detect-changes.outputs.batches) }}`. Verified.

**AC: "Job naming: Validate (batch N/M)"** -- claimed `implemented`
- Diff confirms: `name: "Validate (batch ${{ matrix.batch.batch_id + 1 }}/${{ needs.detect-changes.outputs.batch_count }})"`. Verified.

**AC: "Inner loop with ::group:: annotations and failure accumulation"** -- claimed `implemented`
- Diff confirms: `echo "::group::Validate $RECIPE"`, `FAIL_FILE` accumulation, and final `exit 1` if `FAIL_FILE` is non-empty. Also `::error::` annotation listing failed recipes. Verified.

**AC: "r2-health-check preserved"** -- claimed `implemented`
- Diff confirms: No lines removed from `r2-health-check` job. Verified.

**AC: "validate-golden depends on detect-changes and r2-health-check"** -- claimed `implemented`
- Diff confirms: `needs: [detect-changes, r2-health-check]` on the `validate-golden` job. Verified.

**AC: "validate-golden-recipes entry in ci-batch-config.json with default: 20"** -- claimed `implemented`
- Diff confirms: `.github/ci-batch-config.json` adds `"validate-golden-recipes": {"default": 20}`. Verified.

All 11 mapped ACs are confirmed by the diff. No phantom ACs (all correspond to real issue requirements).

---

## Missing ACs (Not in Mapping)

The following issue ACs have no entry in the mapping. I evaluate each:

**AC 5: Batch object shape** -- `{"batch_id": N, "recipes": [{"recipe": "...", "category": "..."}, ...]}`
- Diff confirms this is implemented: the jq expression produces `{batch_id: .key, recipes: $all[.value:.value + $bs]}`, and the inner loop reads `jq -r ".[$i].recipe"` and `jq -r ".[$i].category"`. The shape matches.
- Missing from mapping but verifiably implemented. **Advisory** (evidence gap, not implementation gap).

**AC 9: Each batch job checks out the repo, sets up Go, and builds tsuku once**
- Diff confirms: Checkout, `actions/setup-go`, and `go build` steps remain at the job level (before the inner loop). Implemented.
- Missing from mapping but verifiably implemented. **Advisory**.

**AC 10: Inner loop calls validate-golden.sh per recipe with correct RECIPE and CATEGORY env vars**
- Diff confirms: `./scripts/validate-golden.sh "$RECIPE" --category "$CATEGORY"` with `RECIPE` and `CATEGORY` derived from the batch array. Implemented.
- Missing from mapping but verifiably implemented. **Advisory**.

**AC 11: Category derived from recipe object in batch (not re-derived from path)**
- Diff confirms: `CATEGORY=$(echo "$RECIPES" | jq -r ".[$i].category")` reads category from the batch object, not from path manipulation. Implemented.
- Missing from mapping but verifiably implemented. **Advisory**.

**AC 12: R2-related env vars set at job level, not per-recipe**
- Diff confirms: `env:` block at the `validate-golden` job level includes `R2_BUCKET_URL`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_AVAILABLE`. The per-recipe step's `env:` only contains `GITHUB_TOKEN`. Implemented.
- Missing from mapping but verifiably implemented. **Advisory**.

**AC 13: TSUKU_GOLDEN_SOURCE determination and R2 existence check run per-recipe inside the inner loop**
- Diff confirms: The `if [[ "$CATEGORY" == "embedded" ]]` / `elif [[ "$R2_AVAILABLE" == "true" ]]` block and `./scripts/check-golden-exists.sh` call appear inside the `for i in $(seq 0 $((COUNT - 1)))` loop. Implemented.
- Missing from mapping but verifiably implemented. **Advisory**.

**AC 18: validate-exclusions job preserved unchanged**
- The diff shows no changes to lines before the `detect-changes` job. The `validate-exclusions` job is unchanged.
- Missing from mapping but verifiably implemented. **Advisory**.

**AC 19: Per-recipe cache key replaced with batch-level approach**
- Diff confirms: The entire `actions/cache` step was removed. This satisfies the AC since the issue explicitly permits removing the download cache entirely ("either use a shared cache key across batches or remove the download cache entirely").
- Missing from mapping. The coder chose the "remove entirely" path, which is explicitly allowed. **Advisory** (the mapping should have captured the decision, but the implementation is correct).

**AC 20: Either use shared cache key or remove download cache**
- See AC 19. The cache was removed, which is one of the two permitted outcomes. Implemented correctly.
- Missing from mapping. **Advisory**.

**AC 21: If a shared cache is used, key should be `golden-downloads-batch-${{ matrix.batch.batch_id }}`**
- The implementation removes the cache entirely rather than using a shared key. Since the issue treats this as conditional ("If a shared cache is used"), this AC is conditionally inapplicable. No cache is used, so this AC's condition is false. Not a finding.

**AC 22: Working batched validate-golden-recipes.yml workflow (required by #1816)**
- The workflow is structurally correct and complete. This is effectively a delivery confirmation AC rather than a discrete implementation criterion. Verifiable only by running the workflow, but the structural changes are complete.
- Missing from mapping. **Advisory** (it's an integration AC, not a discrete implementation step).

---

## Summary of Findings

### Blocking findings: 0

All 11 mapped ACs are confirmed by the diff. No phantom ACs. No missing ACs that are unimplemented (the 12 unmapped ACs are all verifiably implemented in the diff).

### Advisory findings: 10

The mapping covers only 11 of 23 ACs (roughly 48%). The 12 unmapped ACs fall into two categories:

1. **Implemented but unmapped** (ACs 5, 9, 10, 11, 12, 13, 18): These are real requirements from the issue body that were implemented correctly but not reflected in the mapping. The mapping is incomplete, not the implementation.

2. **Cache strategy ACs** (ACs 19, 20, 21): The coder correctly removed the download cache (a permitted outcome per the issue), but the mapping has no entry explaining this decision. Since the issue gives the coder latitude ("either use a shared cache key or remove"), a mapping entry noting "removed cache entirely -- fits the lightweight golden validation rationale" would have confirmed deliberate choice over accidental omission.

The overall implementation is correct and complete. Every AC from the issue is either confirmed in the diff or conditionally inapplicable. The incomplete mapping is a documentation gap, not an implementation gap.
