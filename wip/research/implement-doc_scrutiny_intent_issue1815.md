# Scrutiny Review: Intent -- Issue #1815

**Issue**: #1815 ci(golden-recipes): batch per-recipe jobs in validate-golden-recipes
**Scrutiny focus**: intent
**Files changed**: `.github/ci-batch-config.json`, `.github/workflows/validate-golden-recipes.yml`

---

## Sub-check 1: Design Intent Alignment

The design doc (DESIGN-recipe-ci-batching.md) describes `validate-golden-recipes.yml` as the simpler application of the batching pattern established in #1814: "no platform detection, no macOS handling, and golden validation is faster than recipe installation." The Solution Architecture section calls this Phase 2.

### Finding 1: Advisory detected from #1814 now corrected -- Aligned (positive)

The scrutiny review of #1814 flagged an advisory finding: `batch_count` was computed but never written to `$GITHUB_OUTPUT`, and job naming used `batch_id` (0-indexed, no total) rather than the design's specified `batch X/Y` format.

Issue #1815's implementation gets this right:

- `batch_count` is written to `$GITHUB_OUTPUT` (line 198 of the current workflow file)
- Job name: `"Validate (batch ${{ matrix.batch.batch_id + 1 }}/${{ needs.detect-changes.outputs.batch_count }})"` (line 203)

This matches the design doc's specified naming pattern exactly (Solution Architecture, validate-golden-recipes.yml section). The implementation also corrects the pattern relative to what #1814 left behind, which is appropriate: #1815 was building fresh from the design, not copying #1814's output verbatim.

### Finding 2: Cache key strategy -- design intent partially addressed

The issue body (Cache key strategy AC) requires either a batch-level shared cache or removal of the per-recipe download cache. The design notes that "golden validation is lightweight and primarily CPU-bound (plan generation + hash comparison)."

The diff shows the entire `Cache downloads` step was removed from `validate-golden` (no replacement added). This aligns with the issue's stated intent that golden validation is CPU-bound, making a download cache unnecessary for this workflow. The design doc does not mandate a replacement cache, it offers two options ("use a shared cache key across batches or remove the download cache entirely"), and the implementation chose removal.

This is a reasonable interpretation and is consistent with the design doc's characterization of golden validation as CPU-bound. **Aligned.**

### Finding 3: R2 env vars moved to job level -- design intent satisfied

The AC states: "The R2-related env vars (`R2_BUCKET_URL`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_AVAILABLE`) are set at the job level, not per-recipe."

The implementation places all four R2 env vars in the `validate-golden` job-level `env:` block (lines 212-216 of the workflow file). The `Validate batch recipes` step's `env:` block contains only `GITHUB_TOKEN`. This matches the design intent: R2 credentials are uniform across all recipes in a batch, while per-recipe logic (golden source selection) runs inside the inner loop. **Aligned.**

### Finding 4: Batch object shape uses {recipe, category} -- design intent satisfied

The design doc specifies: "The batch objects here should wrap `{recipe, category}` items, keeping the existing shape inside `batch.recipes[]`." The detection step's jq expression (lines 188-193) builds batches from the existing `{recipe, category}` JSON, and the inner loop reads `.[$i].recipe` and `.[$i].category`. **Aligned.**

### Finding 5: Inner loop uses `toJson()` for batch recipes injection -- deviation from design, acceptable

The design doc shows an inner loop similar to:

```bash
for recipe in $(echo "$RECIPES" | jq -r '.[].path'); do
```

The implementation uses `toJson()` to pass the batch recipes array:

```yaml
RECIPES='${{ toJson(matrix.batch.recipes) }}'
```

This differs from the `test-changed-recipes.yml` approach where the recipes variable is loaded via single-quoted shell substitution: `RECIPES='${{ steps.changed.outputs.recipes }}'`. In `validate-golden-recipes.yml`, the implementation uses `toJson(matrix.batch.recipes)` instead of simply referencing `matrix.batch.recipes`.

Both approaches produce valid JSON for consumption by jq. The `toJson()` approach is functionally equivalent here. The design doc does not specify which GitHub Actions expression to use for injecting the batch array into the shell step. **Advisory** (approach works; the `toJson()` vs. direct reference distinction is a minor consistency gap with the pattern established in #1814, but it's not wrong).

### Finding 6: Path for TSUKU_HOME isolation -- not applicable to this workflow

The design describes TSUKU_HOME isolation in the inner loop. In `test-changed-recipes.yml`, each recipe gets its own `TSUKU_HOME` because recipe *installation* could interfere across recipes sharing a home dir. Golden file *validation* does not install recipes; it generates and compares plans. The implementation does not use per-recipe TSUKU_HOME isolation in the inner loop, which is correct for this workflow.

The design doc notes this in the scope: "The inner loop calls `validate-golden.sh` per recipe with appropriate env vars." No isolation requirement is stated for validation-only workflows. **Aligned.**

### Finding 7: `detect-changes` job name unchanged -- minor consistency gap, advisory

The job name in `validate-golden-recipes.yml` remains "Detect Changes" (line 66 of the workflow file). This is unchanged from before the implementation. The design does not specify job naming for the detection job. No finding.

### Finding 8: Timeout increased to 15 minutes -- design intent satisfied

The design's Implementation Notes state: "The existing per-recipe timeout is 10 minutes. With a default batch size of 20, golden validation is fast enough (mostly CPU: plan generation + hash comparison) that 15 minutes should be sufficient for a full batch." The diff shows the timeout changed from 10 to 15 minutes. **Aligned.**

### Finding 9: `fail-fast: false` present -- aligned

The implementation has `fail-fast: false` (line 209 of the workflow file). Matches the design's failure accumulation intent. **Aligned.**

### Finding 10: Failure accumulation uses FAIL_FILE not FAILED string -- minor variance, aligned

The `test-changed-recipes.yml` inner loop (from #1814) accumulated failures in a `FAILED` string variable. This implementation uses a temp file (`FAIL_FILE="${{ runner.temp }}/failed-recipes.txt"`).

Both approaches satisfy the design's intent: accumulate failures, then report at the end with `::error::` and exit 1. The temp file approach avoids shell quoting issues with recipe names containing spaces or special characters (theoretical concern, recipe names are alphanumeric + hyphens, but defensive). This is a minor deviation from the established pattern, but it's a safe one. The design doc does not prescribe the implementation mechanism. **Aligned.**

---

## Sub-check 2: Cross-Issue Enablement

Downstream issue: **#1816** -- docs(ci): document batch size configuration and tuning

The downstream issue needs "both workflows batched before it can document the complete picture. Specifically, it needs the `validate-golden-recipes` key present in `.github/ci-batch-config.json` and the workflow working end-to-end."

Checking what #1816 requires from this implementation:

1. **`validate-golden-recipes` entry in `.github/ci-batch-config.json`**: Present.
   ```json
   "validate-golden-recipes": {
     "default": 20
   }
   ```
   #1816 can document this key alongside `test-changed-recipes.linux`. **Adequate.**

2. **Working batched `validate-golden-recipes.yml`**: The workflow is modified. The batch splitting, matrix structure, inner loop, and failure accumulation are all in place. #1816 can reference this workflow's behavior to document tuning. **Adequate.**

3. **`workflow_dispatch` override for documentation**: The `batch_size_override` input with 0 = use config default, clamping to 1-50, is present. This is what #1816 needs to document the override mechanism for this workflow. **Adequate.**

4. **Config file structure extensibility**: The JSON structure already contains both `test-changed-recipes` and `validate-golden-recipes` entries. #1816 can show the complete config file with both entries, enabling full documentation of the format. **Adequate.**

Cross-issue enablement is fully satisfied.

---

## Backward Coherence

The previous issue summary states:
- "Mirrored macOS aggregated pattern for Linux inner loop"
- "Used toJson() to pass batch recipes into shell"
- "Used PATH manipulation directly instead of GITHUB_PATH"
- "Kept original recipes/has_changes outputs for macOS path"

Checking #1815's implementation against these established conventions:

- **Inner loop pattern**: Followed. Per-recipe `::group::`/`::endgroup::`, failure accumulation, final exit-nonzero. **Consistent.**
- **toJson() usage**: The previous summary says #1814 used `toJson()`. The current implementation also uses `toJson(matrix.batch.recipes)` for injecting batch recipes. **Consistent.**
- **PATH manipulation**: Not applicable; golden validation doesn't need to add tsuku to PATH (it calls scripts, not installed tsuku tools).
- **batch_count output**: The previous issue's intent review flagged that #1814 failed to output `batch_count`. #1815 correctly outputs `batch_count`. There is no convention collision here -- #1815 is correcting a gap, not redefining something established.
- **No changes to unrelated jobs**: `r2-health-check` and `validate-exclusions` are preserved unchanged. **Consistent.**

No backward coherence conflicts.

---

## Summary

| Severity | Count | Details |
|----------|-------|---------|
| Blocking | 0 | -- |
| Advisory | 1 | `toJson(matrix.batch.recipes)` for injecting the batch array differs from the direct shell substitution pattern in `test-changed-recipes.yml`, creating minor inconsistency between the two workflows. Both work correctly. |

The implementation captures the design's intent for Phase 2 batching. The core mechanics -- ceiling-division batching, config-file-driven batch size, `workflow_dispatch` override with 1-50 clamping, job-level R2 env vars, per-recipe inner loop with `::group::` annotations, failure accumulation via FAIL_FILE, and corrected `batch X/Y` job naming -- all align with the design document's specifications. The advisory finding about `toJson()` vs. direct reference is a minor stylistic inconsistency that doesn't affect correctness or downstream enablement.

Cross-issue enablement for #1816 is complete. The config file has the `validate-golden-recipes` entry, the workflow is batched end-to-end, and the override mechanism is in place for documentation.

Backward coherence with #1814 is clean. The one gap from #1814 (missing `batch_count` output and degraded job naming) is corrected in this issue rather than replicated.
