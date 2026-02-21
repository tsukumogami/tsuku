# Architecture Review: Issue #1815

**Issue**: #1815 ci(golden-recipes): batch per-recipe jobs in validate-golden-recipes
**Review focus**: architect
**Files changed**: `.github/ci-batch-config.json`, `.github/workflows/validate-golden-recipes.yml`

---

## Design Alignment

The implementation correctly applies the Phase 2 batching pattern from DESIGN-recipe-ci-batching.md to `validate-golden-recipes.yml`. The design doc explicitly calls this out as "simpler: no platform detection, no macOS handling, and golden validation is faster than recipe installation."

The structural layers match the design intent:

1. **Detection/batching separation**: The existing `changed` step produces a flat recipe list. The new `batch` step partitions it. The execution job iterates over batch objects. This is the same layering established in #1814 for `test-changed-recipes.yml`.

2. **Config-driven batch sizes**: The `validate-golden-recipes` entry in `.github/ci-batch-config.json` with `"default": 20` follows the config file pattern from #1814. The key structure (`batch_sizes.<workflow>.<platform-or-default>`) is consistent.

3. **Inner loop with grouping and failure accumulation**: `::group::Validate $RECIPE`, accumulation via `FAIL_FILE`, final `::error::` summary, and `exit 1` if any failed. Matches the established pattern.

---

## Pattern Consistency

### Consistent patterns (no findings):

**Batch splitting jq expression**: Identical to #1814's:
```
. as $all | [range(0; length; $bs)] | to_entries | map({batch_id: .key, recipes: $all[.value:.value + $bs]})
```

**Batch size resolution cascade**: `workflow_dispatch` override > config file > hardcoded default. Identical logic structure, with the same 1-50 clamping guards. The fallback defaults differ appropriately (15 for `test-changed-recipes` linux, 20 for `validate-golden-recipes` default).

**Matrix consumption**: `fromJson(needs.detect-changes.outputs.batches)` is the same pattern as `fromJson(needs.matrix.outputs.linux_batches)`.

**Recipe injection**: Both workflows use `RECIPES='${{ toJson(matrix.batch.recipes) }}'` and iterate with `jq -r ".[$i].<field>"`. The field names differ (`tool`/`path` vs `recipe`/`category`) because the underlying recipe objects differ, which is correct.

**Job naming**: `"Validate (batch ${{ matrix.batch.batch_id + 1 }}/${{ needs.detect-changes.outputs.batch_count }})"` matches the `"Linux (batch ...)"` pattern from #1814, with appropriate prefix substitution. Uses 1-indexed display with `+ 1`, consistent.

**Failure accumulation**: Both workflows use `FAIL_FILE` with a temp file. The macOS job in `test-changed-recipes.yml` also uses `FAIL_FILE`. All three inner loops (test-linux, test-macos, validate-golden) converge on this mechanism.

### Appropriate deviations (not findings):

**No TSUKU_HOME isolation**: Golden validation runs scripts (`validate-golden.sh`) rather than installing tools, so per-recipe TSUKU_HOME isolation is unnecessary. The omission is correct for this workflow's semantics.

**No PATH manipulation**: Same reasoning. Golden validation doesn't install binaries that need to be on PATH.

**No download cache**: The per-recipe `actions/cache` step was removed entirely rather than converted to batch-level caching. The design doc explicitly permits this ("either use a shared cache key or remove the download cache entirely"), and golden validation is CPU-bound (plan generation + hash comparison), not download-bound. Correct decision.

**Per-recipe `::error::` annotations**: The inner loop emits `::error::` per recipe on failure (line 273-278), which is more granular than the `test-changed-recipes.yml` pattern where the per-recipe `::error::` is absent. This is not a divergence problem -- the validate-golden inner loop already had these annotations in the pre-batch code, and preserving them provides better diagnostics. No structural concern.

---

## Interface Design

**Output contract**: The `detect-changes` job exposes `has_changes`, `batches`, and `batch_count`. This matches the contract established by #1814's detection job (which exposes `has_linux_batches`, `linux_batches`, `batch_count`). The naming differs because `validate-golden-recipes.yml` has a single platform, so platform-prefixed names would be misleading. Using `batches` instead of `linux_batches` is the right choice.

**R2 env vars at job level**: Moving R2 credentials from per-step env to job-level env is a clean structural improvement. The credentials are uniform across all recipes in a batch; per-recipe env vars (`TSUKU_GOLDEN_SOURCE`) that vary are set in the inner loop. Correct separation.

---

## Extensibility

The design doc's "Out of scope" section notes that `validate-golden-execution.yml` per-recipe jobs should be batched in a follow-up. The patterns established by #1814 and #1815 provide a clear template for that work:

- Config file already has room for `validate-golden-execution` entry (the design doc's Solution Architecture shows this)
- The jq batch splitting expression is copy-paste-able
- The inner loop pattern is consistent across both existing workflows

No structural barriers to the planned extension.

---

## Dependency Direction

No Go code changes. The workflow files reference config files, scripts, and GitHub Actions -- all at the same level. No dependency direction concerns.

---

## Findings

| Severity | Count |
|----------|-------|
| Blocking | 0 |
| Advisory | 0 |

The implementation follows the established batching pattern from #1814 without introducing parallel patterns or structural divergence. Where it deviates (no TSUKU_HOME isolation, no download cache, no PATH manipulation), the deviations are justified by the different semantics of golden validation vs. recipe installation. The config file, batch splitting, matrix consumption, inner loop, and failure accumulation all use the same mechanisms as the reference implementation.
