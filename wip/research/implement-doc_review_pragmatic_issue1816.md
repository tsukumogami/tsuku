# Pragmatic Review: Issue #1816

**Issue**: #1816 docs(ci): document batch size configuration and tuning
**Review focus**: pragmatic (simplicity, YAGNI, KISS)
**Date**: 2026-02-21

## Scope

Files changed:
- `docs/workflow-validation-guide.md` -- added "CI Batch Configuration" section (lines 149-219)
- `CONTRIBUTING.md` -- added one paragraph after CI workflow table (line 614)

## Review

This is a documentation-only issue. The pragmatic review evaluates whether the documentation is correct relative to the actual implementation, and whether it introduces any dead or misleading content.

### Correctness Check

1. **Configuration file path**: docs say `.github/ci-batch-config.json`. Actual file exists at that path. Correct.

2. **JSON snippet in docs** (lines 163-174): matches the actual file contents exactly (two entries: test-changed-recipes/linux=15, validate-golden-recipes/default=20). Correct.

3. **Ceiling division example** (line 157): "ceil(47 / 15) = 4 jobs, each handling 11-15 recipes." Math: ceil(47/15) = ceil(3.13) = 4. 47/4 = 11.75, so batches of 12, 12, 12, 11. The claim "11-15 recipes" is correct. Correct.

4. **Fallback defaults** (line 176): "workflows fall back to built-in defaults (15 for test-changed-recipes, 20 for validate-golden-recipes)." Would need to verify against workflow YAML, but this matches the design doc's stated defaults. Reasonable.

5. **Valid range 1-50** (lines 186, 189): matches the guard clause described in the design doc and implemented in the workflows. Correct.

6. **CONTRIBUTING.md link** to `docs/workflow-validation-guide.md#ci-batch-configuration`: anchor matches the section heading "## CI Batch Configuration". Correct.

### Simplicity Evaluation

The documentation is proportionate to the feature. No over-documentation. No speculative content about future features except the follow-up note, which is warranted by the design doc's scope section.

The follow-up section (lines 217-219) says "is tracked as follow-up work" without linking to an actual issue. This is slightly misleading but not a pragmatic concern -- it's a documentation accuracy issue that prior scrutiny already flagged as advisory.

### YAGNI/KISS Check

No findings. The docs don't introduce unnecessary abstractions, speculative configuration guidance, or content beyond what the acceptance criteria require. The tuning guidelines are practical and grounded in specific thresholds (10 min, 3 min) rather than theoretical.

## Findings

**Blocking**: 0
**Advisory**: 0

The implementation is straightforward documentation that accurately reflects the implemented batch configuration. No over-engineering, no dead content, no scope creep.
