# Scrutiny Review: Justification Focus -- Issue #1815

**Issue:** #1815 ci(golden-recipes): batch per-recipe jobs in validate-golden-recipes
**Scrutiny Focus:** justification
**Date:** 2026-02-21

## Methodology

The diff was read first, before examining the requirements mapping, to form an independent understanding of what was implemented. Only after that was the mapping evaluated against the diff.

## Independent Impression of the Diff

The diff changes two files:

1. `.github/ci-batch-config.json` -- Adds `validate-golden-recipes` key with `"default": 20`.
2. `.github/workflows/validate-golden-recipes.yml`:
   - Adds `workflow_dispatch` `batch_size_override` input (type: number, default: 0, range guard 1-50)
   - Modifies `detect-changes` outputs: removes `recipes`, adds `batches` and `batch_count`
   - Adds "Split recipes into batches" step with batch size resolution (override > config file > 20) and jq ceiling-division splitting
   - Converts `validate-golden` job from per-recipe to per-batch matrix
   - Job renamed from `"Validate: ${{ matrix.item.recipe }}"` to `"Validate (batch ${{ matrix.batch.batch_id + 1 }}/${{ needs.detect-changes.outputs.batch_count }})"`
   - Timeout changed from 10 to 15 minutes
   - R2 secrets moved to job-level `env:` block
   - `actions/cache` step removed entirely (no batch-level cache added)
   - Inner loop added using `toJson(matrix.batch.recipes)`, iterating via `seq 0 $((COUNT - 1))`, with `::group::` and `::endgroup::` annotations
   - Failures accumulated in `$FAIL_FILE` with `::error::` annotation at end
   - PATH manipulation is not present in this workflow (only in test-changed-recipes, per previous summary)

Notable: The `validate-exclusions` job and `r2-health-check` job are unchanged in the diff, preserved as-is.

## Mapping Evaluation (Justification Focus)

The submitted requirements mapping contains 12 entries, all marked `"status": "implemented"` with no `reason` or `alternative_considered` fields. There are no reported deviations.

The justification focus evaluates:
1. Reason quality for any deviations
2. Alternative depth
3. Avoidance patterns disguising shortcuts
4. Proportionality

### Deviations Present

All mapping entries report "implemented." However, examining the diff reveals one area where the implementation makes a choice not explicitly flagged as a deviation:

**Cache key strategy (not in the mapping at all)**

The issue body contains a dedicated section on cache key strategy:
> "The per-recipe `actions/cache` key (`golden-downloads-${{ matrix.item.recipe }}`) is replaced with a batch-level approach"
> "Since golden validation is lightweight and primarily CPU-bound (plan generation + hash comparison), either use a shared cache key across batches or remove the download cache entirely for this workflow"
> "If a shared cache is used, the key should be `golden-downloads-batch-${{ matrix.batch.batch_id }}` with restore-keys falling back to `golden-downloads-`"

The implementation removed the cache step entirely, which is one of the two explicitly stated options in the AC ("either use a shared cache key across batches or remove the download cache entirely"). The diff confirms the `actions/cache` step is absent from the modified workflow with no replacement.

Removing the cache is valid per the AC's own text ("or remove the download cache entirely"). This is not a shortcut: the AC specifically anticipated this choice because "golden validation is lightweight and primarily CPU-bound." The implementation chose correctly without reporting a deviation -- though this is a completeness gap (omitted from the mapping), not a justification gap (the choice itself is defensible).

**Conclusion on deviations**: There are no deviations reported, and the one unmapped decision (cache removal) is a valid, AC-sanctioned choice rather than a hidden shortcut.

### Proportionality Check

The mapping covers 12 ACs across the two changed files. The implementation touches exactly those two files (`.github/ci-batch-config.json` and `.github/workflows/validate-golden-recipes.yml`). This is consistent with the issue scope: no new scripts, no Go changes, no infrastructure additions.

With zero deviations on 12 mapped ACs and no evidence of shortcuts in the diff, proportionality is satisfied.

### Avoidance Pattern Check

No deviations means no avoidance patterns to evaluate. The diff shows genuine implementation of the batching pattern, not a superficial change that technically satisfies each AC's text while avoiding the underlying work.

## Findings

### No Blocking Findings

No ACs are mapped with deviation claims that could disguise shortcuts. The one unmapped decision (cache removal) is a valid choice explicitly permitted by the AC text, not an avoidance pattern.

### Advisory: Cache Removal Not Mapped

The cache key strategy section of the issue has multiple sub-ACs and an explicit decision point ("either remove or use shared key"). The implementation chose removal -- a valid option -- but this AC group is absent from the mapping entirely. The justification focus does not penalize absence from the mapping (that is the completeness focus's domain), but notes it for completeness-focus follow-up.

No justification-specific advisory findings beyond acknowledging the omission is absent from the mapping rather than marked as a deviation.

## Overall Assessment

The submitted mapping reports zero deviations. Independent examination of the diff confirms this is accurate: the implementation applies the same pattern established in #1814 to validate-golden-recipes, and every mapped "implemented" claim is supported by code in the diff. The one unmapped decision (complete cache removal) is a legitimate, AC-sanctioned option. There are no avoidance patterns, no disproportionate deviations, and no reason-quality issues to evaluate because there are no deviations to evaluate.
