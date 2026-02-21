# Pragmatic Review: Issue #1815

**Issue**: #1815 ci(golden-recipes): batch per-recipe jobs in validate-golden-recipes
**Review focus**: pragmatic (simplicity, YAGNI, KISS)
**Files changed**: `.github/ci-batch-config.json`, `.github/workflows/validate-golden-recipes.yml`

---

## Approach Assessment

The implementation applies the batching pattern from #1814 to a second workflow. Two files changed: a one-line addition to a config file, and a workflow conversion from per-recipe matrix to per-batch matrix with an inner loop. No new scripts, no new abstractions, no Go code changes. This is straightforward pattern reuse.

---

## Findings

### No blocking findings.

The implementation is the simplest correct approach for this issue.

---

### Advisory 1: FAIL_FILE vs FAILED string -- minor pattern inconsistency

**File**: `.github/workflows/validate-golden-recipes.yml:235-291`
**Severity**: Advisory

`test-changed-recipes.yml` (#1814) accumulates failures in a `FAILED` shell string variable. This workflow uses a temp file (`FAIL_FILE`). Both work correctly. The temp file approach is marginally more defensive (handles hypothetical special characters in recipe names), but recipe names are constrained to alphanumeric + hyphens, so neither approach has a correctness advantage over the other.

Not blocking because: the inconsistency is minor, both approaches are correct and bounded, and forcing consistency here would mean changing #1814's approach too (out of scope).

---

## Summary

| Severity | Count |
|----------|-------|
| Blocking | 0 |
| Advisory | 1 |

The change is minimal and correct. It reuses the batching pattern from #1814 without introducing new abstractions, configuration surfaces, or code paths. The batch size config, `workflow_dispatch` override, and clamping logic are all required by the design doc and are already established patterns from the previous issue. The inner loop follows the proven `::group::`/failure-accumulation pattern.

The one advisory finding (FAIL_FILE vs string variable) is a minor inconsistency between the two workflows that doesn't affect correctness. Both approaches work and are easy to understand.
