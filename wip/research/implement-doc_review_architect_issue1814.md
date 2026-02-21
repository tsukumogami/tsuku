# Architecture Review (Re-review): Issue #1814

**Issue**: ci(test-recipes): batch Linux per-recipe jobs in test-changed-recipes
**Review focus**: architect
**Files changed**: `.github/ci-batch-config.json`, `.github/workflows/test-changed-recipes.yml`
**Re-review scope**: Fix commit addressed PATH accumulation and batch_count output.

---

## Previous Findings Status

### Advisory 1: `batch_count` not exposed as job output -- RESOLVED

The fix commit adds `batch_count` to `$GITHUB_OUTPUT` (line 228), declares it in the job's outputs (line 31), and updates the job name to use 1-indexed batch numbering with the total count (line 239):

```yaml
name: "Linux (batch ${{ matrix.batch.batch_id + 1 }}/${{ needs.matrix.outputs.batch_count }})"
```

This now matches the design doc's output contract. The `batch_count` output is available for downstream consumers and the naming convention is human-readable. #1815 can follow the same pattern.

### Advisory 2: No per-recipe `::error::` annotation -- UNCHANGED (still advisory)

Still not present (line 291 writes to `FAIL_FILE` without a `::error::` annotation). The macOS path also omits it. Both loops are consistent, and the summary-level `::error::` at line 299 reports all failures. This remains a cosmetic divergence from the design doc's pseudocode, not a structural concern.

---

## Fix Commit Assessment

### PATH convergence between Linux and macOS (previously flagged by maintainer reviewer)

The fix correctly addresses the PATH accumulation issue and, more importantly from an architecture perspective, eliminates the structural divergence between the two inner loops.

**Before the fix**:
- Linux: `export PATH="$TSUKU_HOME/bin:$PATH"` (accumulated each iteration)
- macOS: `echo "$TSUKU_HOME/bin" >> "$GITHUB_PATH"` (different mechanism entirely)

**After the fix** (lines 272-288 Linux, lines 336-352 macOS):
- Both loops save `ORIG_PATH="$PATH"` before the loop
- Both loops set `export PATH="$TSUKU_HOME/bin:$ORIG_PATH"` per iteration
- Both loops carry an identical explanatory comment

The two inner loops are now structurally identical, differing only in how they receive their recipe list (batched matrix variable vs detection output). This is the correct architectural outcome: when #1815 copies this pattern for `validate-golden-recipes.yml`, there's one unambiguous reference implementation, not two competing approaches.

---

## Architectural Assessment (unchanged from first review)

The structural properties identified in the first review remain valid:

1. **Config externalized to `.github/ci-batch-config.json`**: Clean separation of tunable values from workflow logic. Extensible for #1815 and #1816.

2. **Detection/execution separation preserved**: Batching (lines 178-235) layers on top of the existing detection output (`steps.changed.outputs.recipes`) without modifying the detection logic. The separation is clean: detection produces a flat recipe list, batching partitions it, execution iterates within each partition.

3. **No parallel patterns introduced**: The batching config file, inner-loop pattern, and failure accumulation approach are each singular patterns. The fix commit strengthened this by making the macOS loop use the same PATH mechanism as the Linux loop.

4. **Extensibility for downstream issues**: The `batch_count` output being now available completes the output contract that #1815 and #1816 depend on.

---

## Findings

No blocking or advisory findings on this re-review. The previously flagged `batch_count` gap is resolved. The per-recipe `::error::` omission (advisory) is unchanged but consistent between both loops and does not create structural divergence.

| Severity | Count |
|----------|-------|
| Blocking | 0 |
| Advisory | 0 |
