# Maintainer Review: Issue #1816

**Issue**: #1816 docs(ci): document batch size configuration and tuning
**Review focus**: maintainer (clarity, readability, duplication)
**Date**: 2026-02-21

---

## Files Reviewed

- `docs/workflow-validation-guide.md` (lines 149-219, new "CI Batch Configuration" section)
- `CONTRIBUTING.md` (line 614, new paragraph after CI workflow table)

---

## Findings

### Finding 1: "is tracked as follow-up work" with no tracking artifact

**File**: `docs/workflow-validation-guide.md:219`
**Severity**: Advisory

The sentence "Batching these jobs is tracked as follow-up work" implies an active tracking artifact (a GitHub issue, a TODO, something). There is no such artifact. The next person reading this will think "tracked where?" and go looking for an issue that doesn't exist. If it's not tracked anywhere beyond this sentence, say "should be batched in a follow-up" rather than "is tracked as follow-up work."

This is advisory rather than blocking because the follow-up section itself is well-structured -- it names the three specific jobs and explains why they were deferred. The misleading verb is a minor irritant, not a misread risk.

### Finding 2: CONTRIBUTING.md paragraph placement is appropriate

**File**: `CONTRIBUTING.md:614`
**Severity**: None

The batch jobs paragraph sits immediately after the CI Validation Workflows table, which is exactly where a contributor would look. The paragraph is concise, explains the "why" (batched jobs), the "what to do when it fails" (expand the job log, find the `::group::` section), and links to the full guide for tuning details. Clear and well-placed.

### Finding 3: Documentation accurately mirrors implementation

**Files**: `docs/workflow-validation-guide.md:149-219` cross-referenced with `.github/workflows/test-changed-recipes.yml` and `.github/workflows/validate-golden-recipes.yml`
**Severity**: None

Verified the following claims in the documentation against the actual workflow YAML:

- Both workflows accept `batch_size_override` as a `workflow_dispatch` input (confirmed: lines 13-14 in both)
- The 1-50 clamping behavior with `::warning::` annotations (confirmed: both workflows have the clamp logic)
- Default fallback values (15 for test-changed-recipes Linux, 20 for validate-golden-recipes default) (confirmed: jq `// 15` and `// 20` fallbacks)
- Job naming convention `"Linux (batch X/Y)"` and `"Validate (batch X/Y)"` (confirmed)
- `::group::Testing $tool` pattern for per-recipe log grouping (confirmed: line 282 in test-changed-recipes.yml)

The documentation doesn't invent behavior or lag behind reality. This is clean.

### Finding 4: Section structure is well-organized

**File**: `docs/workflow-validation-guide.md:149-219`
**Severity**: None

The section follows a logical progression: what batching is -> how it's configured -> how to override manually -> when to tune -> how to find results -> what's not batched yet. Each subsection answers one question without digressing. A contributor who needs just the `workflow_dispatch` override can jump directly to that subsection via the heading.

### Finding 5: The 10-minute / 3-minute thresholds lack context on where they come from

**File**: `docs/workflow-validation-guide.md:196-201`
**Severity**: Advisory

The tuning guidelines say to decrease batch size when jobs exceed 10 minutes and increase when they finish under 3 minutes. These thresholds match the design doc's analysis, but the guide doesn't explain why those numbers specifically. A contributor might wonder: is 10 minutes an arbitrary round number, or is it tied to the 15-minute workflow timeout? Is 3 minutes meaningful or just a gut feel?

A one-sentence note connecting the 10-minute threshold to the workflow timeout would help. Something like "The 15-minute workflow timeout leaves only 5 minutes of headroom for a 10-minute batch." This is advisory because the thresholds themselves are reasonable and the tuning direction is clear even without the explanation.

---

## Summary

**Blocking findings**: 0
**Advisory findings**: 2

The documentation is well-structured, accurate relative to the implementation, and placed where contributors will find it. No name-behavior mismatches, no divergent twins, no magic values without explanation (the config file values are explained in the "Why the sizes differ" paragraph).

Two minor items: (1) the follow-up section says "is tracked" when nothing is actually tracking it beyond this paragraph -- rewording to "should be batched in a follow-up" would be more honest; (2) the tuning thresholds of 10 and 3 minutes would benefit from a brief note connecting them to the workflow timeout, so the next person tuning understands the constraint they're working within.
