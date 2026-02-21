# Pragmatic Review (Re-review): Issue #1814

**Issue**: ci(test-recipes): batch Linux per-recipe jobs in test-changed-recipes
**Review focus**: pragmatic (simplicity, YAGNI, KISS)
**Files changed**: `.github/ci-batch-config.json`, `.github/workflows/test-changed-recipes.yml`
**Re-review scope**: Fix commit addressing PATH accumulation and batch_count output

---

## Fix 1: PATH accumulation resolved -- Verified

Lines 276 and 288 (Linux), 340 and 352 (macOS): `ORIG_PATH="$PATH"` saved before the loop, `export PATH="$TSUKU_HOME/bin:$ORIG_PATH"` used per iteration. Both loops now share an identical comment explaining why `export PATH` is preferred over `GITHUB_PATH`.

The fix is the simplest correct approach. No over-engineering: save, restore, done.

The macOS loop was updated to match the Linux pattern. This is within scope -- the maintainer review flagged the divergence, and the fix brings both loops into alignment without introducing new abstractions.

---

## Fix 2: batch_count output added -- Verified

Line 228: `echo "batch_count=$BATCH_COUNT" >> $GITHUB_OUTPUT`
Line 31: `batch_count: ${{ steps.batch.outputs.batch_count }}`
Line 239: `name: "Linux (batch ${{ matrix.batch.batch_id + 1 }}/${{ needs.matrix.outputs.batch_count }})"`

Also line 187: `echo "batch_count=0" >> $GITHUB_OUTPUT` in the empty-recipes early exit. Correct -- the output is defined on all code paths.

Minimal fix, matches design doc exactly.

---

## Previous advisory findings -- Status unchanged

1. **ci-batch-config.json**: Still speculative for this issue alone but justified by #1815/#1816. Unchanged. Advisory.
2. **Three-tier batch size fallback**: Still more defensive than necessary. Unchanged. Advisory.

---

## No new findings from the fix commit

The fix is two targeted changes (ORIG_PATH pattern and batch_count output) applied to the right places with no scope creep. No new abstractions, no dead code introduced.

---

## Overall Assessment

Both blocking findings from other reviewers have been addressed with minimal, correct fixes. The ORIG_PATH pattern is the standard shell idiom for this problem. The batch_count output is two lines. No over-engineering in the fix. Nothing blocking, nothing new to flag.
