# Review: Maintainer Focus -- Issue #1859

**Issue**: feat(dashboard): read structured subcategories with category remap fallback
**Focus**: maintainer (clarity, readability, duplication)
**Files reviewed**: `internal/dashboard/failures.go`, `internal/dashboard/dashboard.go`, `internal/dashboard/failures_test.go`, `internal/dashboard/dashboard_test.go`

## Finding 1: `knownSubcategories` and `categoryRemap` share keys with different semantics

**File**: `internal/dashboard/failures.go`, lines 41-61
**Severity**: Advisory

The `knownSubcategories` map (line 41) contains `"api_error"` and `"timeout"` as valid bracket-extracted subcategory tags. The `categoryRemap` map (line 54) contains `"api_error"` and `"timeout"` as old category strings that get remapped.

These are different concepts operating at different levels: `knownSubcategories` validates bracket tags inside error messages (subcategory extraction), while `categoryRemap` translates top-level category strings. The two maps are used in completely different code paths -- `knownSubcategories` in `extractSubcategory()` and `categoryRemap` in `remapCategory()` -- so there's no runtime collision.

However, the next developer reading `failures.go` will see `"api_error"` and `"timeout"` in both maps and wonder whether they're related or duplicated. A brief comment on `knownSubcategories` noting that these strings serve a different role (bracket tag validation, not category names) would prevent that moment of confusion.

**Suggestion**: Add a clarifying comment on `knownSubcategories`:

```go
// knownSubcategories validates bracket-extracted tags against an allowlist.
// These are subcategory values extracted from error messages (e.g., "[api_error]"),
// not to be confused with categoryRemap keys which operate on top-level categories.
```

This is advisory because the code paths are separate and the risk of actual misuse is low.

## Finding 2: Clear separation of structured vs. heuristic subcategory extraction

**File**: `internal/dashboard/failures.go`, lines 154-164
**Severity**: Not a finding -- positive observation

The conditional guard at line 157 (`if allDetails[i].Subcategory == ""`) with the comment on lines 154-155 makes the two-path behavior immediately clear. A developer encountering this will understand the design intent: structured subcategories from JSONL take precedence, heuristic extraction is the fallback. This matches the design doc's data flow and is well-commented.

## Finding 3: `remapCategory()` placement and naming are clear

**File**: `internal/dashboard/failures.go`, lines 63-70
**Severity**: Not a finding -- positive observation

The function name `remapCategory` accurately describes what it does: it translates old category strings. The placement next to `categoryRemap` (the data it operates on) and `extractSubcategory` (the other classification function) is logical. The godoc comment on lines 63-64 explains the passthrough behavior for canonical names. The function is used in four places (two in `loadFailureDetailsFromFile`, two in `loadFailures`) and the behavior is identical at each call site, so there's no divergent twin risk.

## Finding 4: Test names accurately describe what they test

**File**: `internal/dashboard/failures_test.go`
**Severity**: Not a finding -- positive observation

The new tests follow a clear naming pattern:
- `TestLoadFailureDetailRecords_structuredSubcategoryPerRecipe` -- tests structured subcategory passthrough for per-recipe format
- `TestLoadFailureDetailRecords_structuredSubcategoryLegacy` -- tests structured subcategory passthrough for legacy batch format
- `TestLoadFailureDetailRecords_noSubcategoryFallsBackToHeuristic` -- tests heuristic fallback when no structured subcategory exists
- `TestRemapCategory` -- table-driven test covering all remap entries plus passthrough cases
- `TestLoadFailureDetailRecords_mixedOldNewCategories` -- end-to-end test mixing old and new formats

Test assertions match their names. No test name lies detected.

## Finding 5: Test for `remapCategory` also covers canonical passthrough and unknown values

**File**: `internal/dashboard/failures_test.go`, lines 764-794
**Severity**: Not a finding -- positive observation

The `TestRemapCategory` function tests all six remap entries, all six canonical names (passthrough), and one unknown value. This is complete coverage of the `categoryRemap` map and the passthrough behavior. The next developer can confidently add or modify remap entries by updating the table test.

## Finding 6: Existing `loadFailures` tests updated to expect remapped categories

**File**: `internal/dashboard/dashboard_test.go`, lines 144-182
**Severity**: Not a finding -- positive observation

`TestLoadFailures_legacyFormat` was updated so that assertions check for canonical names (`install_failed` instead of `validation_failed`, `network_error` instead of `api_error`). The test comments note the remapping explicitly (e.g., "api_error->network_error"). This prevents the next developer from being confused about which category names to expect.

## Summary

No blocking findings. One advisory finding about the shared keys between `knownSubcategories` and `categoryRemap` that could momentarily confuse a reader, but the code paths are separate so no misread would lead to a bug.

The implementation is clear and well-structured. The remap function, conditional subcategory extraction, and struct field additions are all straightforward and consistent with existing patterns in the codebase. Test coverage is thorough and test names accurately describe their behavior. The code reads well from a next-developer perspective.
