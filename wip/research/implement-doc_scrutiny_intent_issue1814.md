# Scrutiny Review: Intent -- Issue #1814

**Issue**: #1814 ci(test-recipes): batch Linux per-recipe jobs in test-changed-recipes
**Scrutiny focus**: intent
**Files changed**: `.github/ci-batch-config.json`, `.github/workflows/test-changed-recipes.yml`

---

## Sub-check 1: Design Intent Alignment

### Finding 1: Missing `batch_count` output and degraded job naming -- Advisory

**Design doc says** (Solution Architecture, line 249):
> Job naming: `Linux (batch ${{ matrix.batch.batch_id + 1 }}/${{ needs.matrix.outputs.batch_count }})`

**Implementation** (test-changed-recipes.yml, line 236):
```yaml
name: "Linux batch ${{ matrix.batch.batch_id }}"
```

The design explicitly calls for a `batch_count` output from the detection job and a "batch X/Y" naming format. The implementation computes `BATCH_COUNT` (lines 225-232) but never writes it to `$GITHUB_OUTPUT`, and the job name uses only the 0-indexed `batch_id` without the total count.

The naming difference has two aspects:
1. **0-indexed vs 1-indexed**: The design says `batch_id + 1` for human-friendly display (batch 1/5, not batch 0/5). The implementation shows `batch 0`.
2. **Missing total**: Without `/N`, a user seeing "Linux batch 2" in the checks tab can't tell how many total batches exist. The design intended `batch 3/5` for this purpose.

This is **advisory** because: the batching logic itself is correct, the naming is cosmetic, and fixing it later requires only two lines (add `batch_count` output + update the name template). It doesn't affect downstream issues -- #1815 will establish its own job naming. However, the design doc was specific about this, and the omission reduces failure triage speed in the GitHub Actions UI.

### Finding 2: Original `recipes`/`has_changes` outputs preserved -- Aligned

The implementation retains the original `recipes` and `has_changes` outputs (lines 25-26) alongside the new `linux_batches`/`has_linux_batches`. This is correct: the macOS path continues to read `macos_recipes` and `has_macos`, and the design scope explicitly says "No changes to macOS aggregation." Keeping the original outputs ensures backward compatibility for the macOS path. Good call.

### Finding 3: Inner loop pattern mirrors macOS with intentional PATH divergence -- Aligned

The design says to follow "the exact pattern used by the macOS aggregated job." The implementation does this with one deliberate difference: Linux uses `export PATH="$TSUKU_HOME/bin:$PATH"` (line 279) while macOS uses `echo "$TSUKU_HOME/bin" >> "$GITHUB_PATH"` (line 337).

This is a reasonable divergence. Both run inside a single step, but `export PATH` is more predictable for the batched case since `GITHUB_PATH` accumulates across the loop and persists across steps (irrelevant here but potentially confusing). The coder's key decisions note mentions this explicitly.

### Finding 4: Config file structure matches design specification -- Aligned

`.github/ci-batch-config.json` contains:
```json
{
  "batch_sizes": {
    "test-changed-recipes": {
      "linux": 15
    }
  }
}
```

The design doc (Solution Architecture) shows this exact structure with room for additional entries (`validate-golden-recipes`, `validate-golden-execution`). The implementation provides only the entry relevant to this issue, leaving room for #1815 to add its own. The config file lookup logic (line 207) reads using jq with a `// 15` fallback, matching the design.

### Finding 5: Batch splitting jq pattern matches design -- Aligned

The jq expression (lines 216-221) is identical to the design's "Batch Splitting Implementation" section. Ceiling division via `range(0; length; $bs)` with `to_entries` for batch IDs.

### Finding 6: Batch size override and clamping -- Aligned

The `workflow_dispatch` input (lines 12-17) with clamping to 1-50 (lines 194-201) matches the design's specification. The three-tier fallback (override > config file > hardcoded 15) is correct.

### Finding 7: Timeout increased to 30 minutes -- Aligned

The `test-linux` job timeout is 30 minutes (line 240). The design mentions the original 15-minute timeout and notes headroom concerns. Doubling to 30 minutes for batched jobs (up to 15 recipes per batch) is proportionate.

### Finding 8: fail-fast disabled -- Aligned

`fail-fast: false` (line 242) ensures all batches run to completion. Combined with per-recipe failure accumulation within each batch, this means a failure in batch 1 doesn't cancel batch 2, and a failure on recipe 3 within a batch doesn't skip recipe 4. Matches the design's "Failure Reporting" decision.

---

## Sub-check 2: Cross-Issue Enablement

### Downstream Issue #1815: ci(golden-recipes): batch per-recipe jobs in validate-golden-recipes

#1815 needs:

1. **Config file at `.github/ci-batch-config.json`**: Provided. #1815 will add a `validate-golden-recipes` entry to the existing JSON structure. The file is valid JSON and the structure matches what the design shows for multi-workflow configuration. **Adequate.**

2. **jq splitting pattern**: The ceiling-division jq expression is inline in `test-changed-recipes.yml` (lines 216-221) rather than extracted to a shared script. The design doc says "A shared shell function (or inline jq)" and the implementation chose inline. This means #1815 will duplicate the jq expression. This is acceptable since:
   - The expression is 5 lines of jq, not complex enough to warrant a separate file
   - The design doc explicitly offers inline as an option
   - Any future change to the splitting logic would need to update both workflows, but the expression is stable (pure math)
   **Adequate.**

3. **Inner loop pattern as template**: The inner loop (lines 269-294) demonstrates TSUKU_HOME isolation, shared download cache, `::group::` annotations, and failure accumulation. #1815 can adapt this for golden file validation, which has a different inner action (`validate-golden.sh` instead of `tsuku install`). The pattern is clear and self-contained. **Adequate.**

4. **Batch object structure**: `{"batch_id": N, "recipes": [...]}`. #1815's `validate-golden-recipes.yml` currently uses `{"recipe": "name", "category": "type"}` as matrix items. #1815 will need to wrap these into the batch structure. The convention is established. **Adequate.**

### Downstream Issue #1816: docs(ci): document batch size configuration and tuning

#1816 needs:

1. **Config file format**: Established at `.github/ci-batch-config.json` with documented structure. **Adequate.**
2. **Override mechanism**: `batch_size_override` input with 1-50 clamping, 0 = use config default. **Adequate.**
3. **Tuning guidance context**: The implementation provides concrete defaults (15 for Linux, 30-minute timeout) that documentation can reference. **Adequate.**

---

## Backward Coherence

This is the first issue in the sequence, so no backward coherence check is applicable.

---

## Summary

| Severity | Count | Details |
|----------|-------|---------|
| Blocking | 0 | -- |
| Advisory | 1 | Missing `batch_count` output; job naming uses 0-indexed ID without total count (design specified `batch X/Y` format) |

The implementation captures the design's core intent: ceiling-division batching of Linux per-recipe jobs, config-file-driven batch sizes, workflow_dispatch override with clamping, and the macOS-style inner loop pattern with isolation and failure accumulation. The one advisory finding (job naming) is cosmetic and doesn't affect the batching mechanics or downstream issue enablement.

Cross-issue enablement is adequate. The config file, batch object structure, jq splitting pattern, and inner loop template all provide what #1815 and #1816 need to build on.
