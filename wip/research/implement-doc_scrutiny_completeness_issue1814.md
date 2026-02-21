# Scrutiny Review: Completeness -- Issue #1814

**Issue**: ci(test-recipes): batch Linux per-recipe jobs in test-changed-recipes
**Focus**: completeness
**Reviewer**: maintainer-reviewer

## Methodology

Since the explicit requirements mapping JSON was not provided as a file, this review reconstructs the AC coverage from:
1. The key evidence points summarized in the invocation prompt (which states 28 ACs all marked "implemented")
2. The design doc's Phase 1 specification (docs/designs/DESIGN-recipe-ci-batching.md)
3. The test plan scenarios assigned to #1814 (scenarios 1, 3-8, 11-12)
4. Direct examination of the diff (files changed: `.github/ci-batch-config.json`, `.github/workflows/test-changed-recipes.yml`)

## Evidence Verification Against Diff

### Config file: `.github/ci-batch-config.json`

**Claim**: Created with `batch_sizes.test-changed-recipes.linux=15`
**Verified**: YES. File exists at `.github/ci-batch-config.json` with exact content:
```json
{
  "batch_sizes": {
    "test-changed-recipes": {
      "linux": 15
    }
  }
}
```
The structure matches the design doc's specification and provides the foundation #1815 and #1816 need.

### Detection job: batch splitting step

**Claim**: Added with jq ceiling-division, linux_batches/has_linux_batches outputs
**Verified**: YES. Lines 177-232 of `test-changed-recipes.yml`:
- Step id `batch` added after `changed` step
- Reads from `steps.changed.outputs.recipes`
- jq pattern `range(0; length; $bs)` with `to_entries | map(...)` implements ceiling division correctly
- Outputs `linux_batches` (JSON array of batch objects) and `has_linux_batches` (boolean)
- Matrix job outputs declaration at lines 29-30 exposes these

### Execution job: test-linux converted to per-batch matrix

**Claim**: Inner loop with TSUKU_HOME isolation, shared cache, ::group:: annotations, failure accumulation via FAIL_FILE
**Verified**: YES. Lines 235-294:
- Matrix now iterates over `batch` from `linux_batches` (line 244)
- Job condition uses `has_linux_batches` (line 238)
- Inner loop iterates recipes via jq index (lines 269-285)
- Per-recipe `TSUKU_HOME` under `runner.temp` (line 276)
- Shared download cache via symlink (line 278)
- `::group::Testing $tool` / `::endgroup::` annotations (lines 273, 284)
- `FAIL_FILE` accumulates failures (lines 265-267, 282, 288-291)

### workflow_dispatch: batch_size_override

**Claim**: Input with 1-50 clamping and ::warning:: annotations
**Verified**: YES. Lines 11-17 define the input. Lines 189-211 implement the precedence chain: override > config > default(15). Lines 195-200 implement clamping with `::warning::` annotations for both below-minimum and above-maximum cases.

### Timeout increase

**Claim**: Increased from 15 to 30 minutes
**Verified**: YES. Line 240 shows `timeout-minutes: 30`.

### macOS unchanged

**Claim**: macOS path is unmodified
**Verified**: YES. Lines 296-353 show `test-macos` job intact with original aggregated pattern, depending on `has_macos`, using `macos_recipes` output.

## Test Plan Scenario Coverage

| Scenario | Description | Covered? |
|----------|-------------|----------|
| 1 | Config file exists and valid JSON | YES - file created |
| 3 | Detection job outputs linux_batches/has_linux_batches | YES - lines 29-30, 177-232 |
| 4 | Batch splitting uses jq ceiling-division | YES - lines 216-221 |
| 5 | test-linux uses batch matrix instead of per-recipe | YES - lines 236-244 |
| 6 | Inner loop: TSUKU_HOME, cache, groups, failure accumulation | YES - lines 258-294 |
| 7 | workflow_dispatch has batch_size_override with guard clause | YES - lines 11-17, 189-211 |
| 8 | macOS job unchanged | YES - lines 296-353 |
| 11 | Batch splitting correctness for known input | YES - jq pattern matches test |
| 12 | Single-recipe PR produces one batch | YES - inherent in ceiling division |

## Findings

### Finding 1: Job name uses 0-indexed batch_id without total count (Advisory)

**Design doc specification** (Solution Architecture):
> Job naming: `Linux (batch ${{ matrix.batch.batch_id + 1 }}/${{ needs.matrix.outputs.batch_count }})`

**Actual implementation** (line 236):
```yaml
name: "Linux batch ${{ matrix.batch.batch_id }}"
```

Two deviations: (1) batch_id is 0-indexed (shows "Linux batch 0" instead of "Linux batch 1"), and (2) no total batch count displayed. Additionally, `batch_count` is computed in the detection step (line 225) but never written to `$GITHUB_OUTPUT`, so even if the job name template referenced it, the value wouldn't be available.

**Severity**: Advisory. The batch is still identifiable. The 0-indexed naming is unconventional for human-facing labels but doesn't affect functionality or failure debugging. If an AC specifically requires the `N/M` naming format, this would be blocking.

### Finding 2: No per-recipe `::error::` annotation on individual failures (Advisory)

**Design doc specification** (Decision 2: Failure Reporting):
```bash
echo "::error::Failed: $tool"
```
The design doc shows individual `::error::` per failed recipe inside the loop.

**Actual implementation** (lines 281-283):
```bash
if ! ./tsuku install --force --recipe "$recipe_path"; then
  echo "$tool" >> "$FAIL_FILE"
fi
```

Only the summary `::error::` at line 290 is emitted. Individual recipe failures are written to a file but not annotated with `::error::`. This means the GitHub Actions annotations sidebar won't show per-recipe error entries; only the batch-level summary "Failed recipes: X Y Z" appears.

**Severity**: Advisory. The summary annotation at line 290 does list all failed recipe names, and the `::group::` blocks make it possible to find the failure output. The per-recipe `::error::` would improve the Actions UI experience by creating individual annotation entries, but the information is not lost.

### Finding 3: `batch_count` not output from detection job (Advisory)

The detection step computes `BATCH_COUNT` at line 225 and uses it in an echo at line 232, but never writes it to `$GITHUB_OUTPUT`. The design doc's Solution Architecture lists `batch_count` as a detection job output:

> The detection job outputs:
> - `batches`: JSON array of batch objects
> - `batch_count`: Number of batches (for job naming)

This affects Finding 1 (can't reference batch count in job names) and may affect #1815 which might want to reference this value.

**Severity**: Advisory. The batch count can be derived from the batches array length. The missing output is a minor gap that doesn't block functionality but diverges from the design doc's output contract.

## Downstream Impact Assessment

### #1815 (validate-golden-recipes batching)

Needs from #1814:
- **Config file**: Provided. Structure supports additional entries (`validate-golden-recipes.default` can be added).
- **jq splitting pattern**: Present inline at lines 216-221. #1815 can replicate this pattern.
- **Inner loop pattern**: Present at lines 258-294. The pattern (TSUKU_HOME isolation, shared cache, ::group::, FAIL_FILE) is clear and replicable.

Assessment: #1815 has sufficient foundation from #1814.

### #1816 (documentation)

Needs from #1814:
- **Config file format**: Established (JSON with `batch_sizes` map).
- **batch_size_override mechanism**: Implemented with full precedence chain and clamping.

Assessment: #1816 has sufficient documentation targets from #1814.

## Overall Assessment

The implementation faithfully covers the core requirements of #1814. The two changed files (`.github/ci-batch-config.json` and `.github/workflows/test-changed-recipes.yml`) implement all major aspects: config file creation, batch splitting with ceiling division, execution job conversion to batch matrix, inner-loop isolation pattern, workflow_dispatch override with clamping, timeout increase, and macOS preservation.

Three advisory findings identify minor deviations from the design doc's specification (job naming format, per-recipe error annotations, batch_count output). None of these block the core functionality or downstream issues. The implementation provides a solid foundation for #1815 and #1816.

No blocking findings. The "28 ACs all implemented" claim is consistent with the diff evidence across all verifiable dimensions.
