# Scrutiny Review: Justification -- Issue #1826

## Summary

One deviation found. Zero blocking findings. One advisory.

## Deviation Analysis

### AC: "Cross-recipe canonical name collision CI validation"

**Claimed status:** deviated
**Reason given:** "Deferred to #1829 - requires full recipe set"

**Assessment: Advisory -- valid deferral with a minor nuance.**

The deviation explanation is genuine. The design doc's Phase 1 says: "Add validation for the satisfies field: ...no entries that collide with existing recipe canonical names." But the validation function `validateSatisfies()` in `validate.go` is a structural, per-recipe check invoked from `parseBytes()`. It has no access to the loader, embedded registry, or any other recipe set -- only the recipe being parsed. Checking whether a satisfies entry like `"sqlite3"` collides with the canonical name of another recipe (`sqlite`) requires cross-recipe knowledge.

The design doc's #1829 description explicitly includes: "Adds cross-recipe duplicate detection at CI time so conflicting satisfies claims are caught before merge." This is the natural home for the check.

This is not an avoidance pattern. The deferral is architecturally motivated (validation scope doesn't have the needed context) and the downstream issue (#1829) explicitly picks it up.

**Minor nuance:** The design doc's Uncertainties section distinguishes two types of collision checks:
1. Cross-recipe `satisfies` duplicates (two recipes claiming `openssl@3`) -- clearly #1829
2. Canonical name collision (recipe `foo` satisfies `sqlite` where `sqlite` is a recipe name) -- listed as a validation concern in Phase 1

Type 2 could theoretically be done at the single-recipe level if the validator had access to the embedded registry's name list, but wiring that into structural validation would couple two layers that are currently independent. Deferring both to CI-time validation (#1829) is pragmatic.

**Severity:** Advisory. The runtime behavior is safe either way -- exact name match always takes priority over satisfies fallback, so a canonical name collision can't cause incorrect resolution. The check is about catching data quality issues early.

## Proportionality Check

Six of seven ACs are "implemented" with one deviation. The deviation is on a cross-cutting validation concern, not on a core AC. The core functionality (schema, index, fallback, per-recipe validation, tests) is all present. No signs of selective effort.
