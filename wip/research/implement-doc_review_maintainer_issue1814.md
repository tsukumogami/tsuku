# Maintainer Review (Re-review): Issue #1814

**Issue**: ci(test-recipes): batch Linux per-recipe jobs in test-changed-recipes
**Focus**: maintainability (clarity, readability, duplication)
**Files reviewed**: `.github/ci-batch-config.json`, `.github/workflows/test-changed-recipes.yml`
**Review type**: Re-review after addressing blocking feedback from initial maintainability review

---

## Previous Blocking Finding: Resolved

**PATH accumulation in batch loop + unexplained divergence from macOS pattern**

The fix commit saves `ORIG_PATH="$PATH"` before the loop and uses `export PATH="$TSUKU_HOME/bin:$ORIG_PATH"` inside each iteration in both the Linux batch loop (lines 276, 288) and the macOS aggregated loop (lines 340, 352). This eliminates PATH accumulation across iterations.

The two loops are now structurally identical except for how they source their recipe list (line 266 vs 330), which is an inherent difference between the batched and single-job models. The comment block at lines 272-275 / 336-339 explains why `export PATH` is used instead of `GITHUB_PATH` -- because the loop runs within a single step and PATH changes shouldn't leak between iterations. This directly addresses the "next developer won't know why the mechanism differs" concern from the initial review.

**Verdict**: Fully resolved. No remaining misread risk.

---

## Previous Advisory Finding 1: Resolved

**Job name uses 0-indexed batch_id**

Line 239 now reads: `"Linux (batch ${{ matrix.batch.batch_id + 1 }}/${{ needs.matrix.outputs.batch_count }})"`. The `batch_count` output is set at line 228. The naming matches the design doc's intent.

---

## Previous Advisory Finding 2: Still Present (Advisory)

**Missing per-recipe `::error::` annotation**

Lines 290-292 (Linux) and 354-356 (macOS) record failures to a file but don't emit `echo "::error::Failed to install: $tool"` per recipe. Only the batch summary `::error::` at line 299 / 363 appears in the GitHub Actions annotations sidebar. This means a developer looking at the checks tab sees "Failed recipes: X Y Z" but not individual annotations per recipe.

This is a minor usability gap in CI output. It doesn't create misread risk -- the `::group::` blocks clearly delineate per-recipe output, and the summary line names the failures. The macOS loop has the same behavior, so this is consistent.

**Severity**: Advisory (carried forward from initial review, unchanged).

---

## New Finding: None

The fix commit's changes are minimal and well-scoped:
1. Added `ORIG_PATH="$PATH"` and the explanatory comment to both loops
2. Changed `export PATH="$TSUKU_HOME/bin:$PATH"` to `export PATH="$TSUKU_HOME/bin:$ORIG_PATH"` in both loops
3. The macOS loop was updated to use the same mechanism, eliminating the divergence

No new maintainability concerns were introduced.

---

## Overall Assessment

The previous blocking finding is fully resolved. The Linux and macOS inner loops now use the same PATH mechanism with a clear comment explaining the design choice. The code reads well: a developer encountering either loop will see the `ORIG_PATH` save/restore pattern and understand both *what* it does and *why*. The structural parallelism between the two loops means the #1815 implementer can confidently copy the pattern for validate-golden-recipes.

The one remaining advisory finding (per-recipe `::error::` annotations) is a minor CI UX improvement that doesn't affect correctness or create misread risk.

0 blocking findings. 1 advisory finding (carried forward, unchanged).
