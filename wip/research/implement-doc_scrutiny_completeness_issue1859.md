# Scrutiny Review: Completeness -- Issue #1859

**Issue**: feat(dashboard): read structured subcategories with category remap fallback
**Focus**: completeness
**Reviewer**: scrutiny agent

## Issue Acceptance Criteria (extracted from issue body)

### Struct changes
1. `FailureRecord` in `internal/dashboard/dashboard.go` includes `Subcategory string \`json:"subcategory,omitempty"\``
2. `PackageFailure` in `internal/dashboard/dashboard.go` includes `Subcategory string \`json:"subcategory,omitempty"\``

### Subcategory passthrough
3. `loadFailureDetailsFromFile()` reads `record.Subcategory` into `FailureDetail.Subcategory` for per-recipe format records
4. `loadFailureDetailsFromFile()` reads `f.Subcategory` from `PackageFailure` entries for legacy batch format records

### Category remap
5. A `remapCategory()` function translates old category strings:
   - `api_error` -> `network_error`
   - `validation_failed` -> `install_failed`
   - `deterministic_insufficient` -> `generation_failed`
   - `deterministic` -> `generation_failed`
   - `timeout` -> `network_error`
   - `network` -> `network_error`
6. Categories that already match the canonical taxonomy pass through unchanged
7. The remap applies in both `loadFailureDetailsFromFile()` and `loadFailures()` so that Failures counts and FailureDetails use consistent names

### Conditional subcategory extraction
8. `loadFailureDetailRecords()` only calls `extractSubcategory()` for records where `Subcategory` is empty after loading from JSONL
9. Records with a non-empty `Subcategory` from JSONL retain that value without heuristic override

### Tests
10. Test case: per-recipe record with `"subcategory": "timeout"` passes through without calling `extractSubcategory()`
11. Test case: legacy batch record with `"subcategory": "dns_error"` passes through without calling `extractSubcategory()`
12. Test case: record without `subcategory` field still gets heuristic extraction (backward compatibility)
13. Test case: `remapCategory("api_error")` returns `"network_error"`
14. Test case: `remapCategory("deterministic")` returns `"generation_failed"`
15. Test case: `remapCategory("validation_failed")` returns `"install_failed"`
16. Test case: `remapCategory("timeout")` returns `"network_error"`
17. Test case: `remapCategory("network")` returns `"network_error"`
18. Test case: `remapCategory("deterministic_insufficient")` returns `"generation_failed"`
19. Test case: `remapCategory("missing_dep")` returns `"missing_dep"` (no change)
20. Test case: end-to-end `loadFailureDetailRecords()` with mixed old/new records produces canonical categories throughout

## Requirements Mapping Evaluation

### Mapping Entry 1: "remapCategory translates old category strings"
- **Claimed status**: implemented
- **Issue ACs covered**: #5, #6 (remap function + passthrough for canonical names)
- **Evidence in diff**: `failures.go` adds `categoryRemap` map (lines 54-61) and `remapCategory()` function (lines 65-70). The map contains all six required translations. The function returns the input unchanged if not in the map, satisfying the passthrough requirement.
- **Assessment**: VERIFIED. The code matches the AC exactly.

### Mapping Entry 2: "Subcategory field added to FailureRecord and PackageFailure"
- **Claimed status**: implemented
- **Issue ACs covered**: #1, #2
- **Evidence in diff**: `dashboard.go` diff shows `Subcategory string \`json:"subcategory,omitempty"\`` added to both `FailureRecord` (line 154) and `PackageFailure` (line 163).
- **Assessment**: VERIFIED. Both struct additions are present with correct JSON tags.

### Mapping Entry 3: "loadFailureDetailsFromFile reads subcategory from both formats"
- **Claimed status**: implemented
- **Issue ACs covered**: #3, #4
- **Evidence in diff**: `failures.go` diff shows `loadFailureDetailsFromFile()` now includes `Subcategory: f.Subcategory` in the legacy batch format (line 237) and `Subcategory: record.Subcategory` in the per-recipe format (line 265).
- **Assessment**: VERIFIED. Both paths populate the Subcategory field.

### Mapping Entry 4: "remap applies in both loadFailures and loadFailureDetailsFromFile"
- **Claimed status**: implemented
- **Issue ACs covered**: #7
- **Evidence in diff**: In `dashboard.go`, `loadFailures()` calls `remapCategory()` in both the legacy batch loop (`cat := remapCategory(f.Category)` at line 435) and the per-recipe branch (`cat := remapCategory(record.Category)` at line 449). In `failures.go`, `loadFailureDetailsFromFile()` calls `remapCategory()` for both legacy batch (`Category: remapCategory(f.Category)` at line 234) and per-recipe (`Category: remapCategory(record.Category)` at line 261).
- **Assessment**: VERIFIED. All four call sites are present in the diff.

### Mapping Entry 5: "extractSubcategory conditional - only called when subcategory empty"
- **Claimed status**: implemented
- **Issue ACs covered**: #8
- **Evidence in diff**: `failures.go` diff shows the loop in `loadFailureDetailRecords()` now has `if allDetails[i].Subcategory == ""` guarding the `extractSubcategory()` call (lines 154-160).
- **Assessment**: VERIFIED.

### Mapping Entry 6: "structured subcategory preserved without heuristic override"
- **Claimed status**: implemented
- **Issue ACs covered**: #9
- **Evidence in diff**: Same conditional as entry 5 -- when Subcategory is non-empty, `extractSubcategory()` is skipped.
- **Assessment**: VERIFIED. The conditional guard prevents heuristic override.

### Mapping Entry 7: "backward compat - records without subcategory get heuristic extraction"
- **Claimed status**: implemented
- **Issue ACs covered**: #12 (and functionally tested in #10-11 inverse)
- **Evidence in diff**: The else-branch of the conditional calls `extractSubcategory()` with the existing parameters, identical to the old unconditional path.
- **Assessment**: VERIFIED.

## Test AC Coverage

| Test AC | Test Function | Verified in Diff |
|---------|--------------|-----------------|
| #10 (per-recipe subcategory passthrough) | `TestLoadFailureDetailRecords_structuredSubcategoryPerRecipe` | YES - asserts `Subcategory == "timeout"` for per-recipe record with structured subcategory |
| #11 (legacy batch subcategory passthrough) | `TestLoadFailureDetailRecords_structuredSubcategoryLegacy` | YES - asserts `Subcategory == "dns_error"` for legacy batch record with structured subcategory |
| #12 (no subcategory -> heuristic) | `TestLoadFailureDetailRecords_noSubcategoryFallsBackToHeuristic` | YES - asserts `Subcategory == "no_bottles"` from heuristic extraction of bracketed tag |
| #13 (remapCategory api_error) | `TestRemapCategory/api_error` | YES |
| #14 (remapCategory deterministic) | `TestRemapCategory/deterministic` | YES |
| #15 (remapCategory validation_failed) | `TestRemapCategory/validation_failed` | YES |
| #16 (remapCategory timeout) | `TestRemapCategory/timeout` | YES |
| #17 (remapCategory network) | `TestRemapCategory/network` | YES |
| #18 (remapCategory deterministic_insufficient) | `TestRemapCategory/deterministic_insufficient` | YES |
| #19 (remapCategory missing_dep passthrough) | `TestRemapCategory/missing_dep` | YES |
| #20 (end-to-end mixed records) | `TestLoadFailureDetailRecords_mixedOldNewCategories` | YES - 6 records mixing legacy and new formats, verifies remapped categories and structured subcategory preservation |

All 11 test ACs are covered by corresponding test functions in the diff.

## Phantom AC Check

The 7 mapping entries correspond to the following issue ACs:
- Entry 1 -> ACs #5, #6
- Entry 2 -> ACs #1, #2
- Entry 3 -> ACs #3, #4
- Entry 4 -> AC #7
- Entry 5 -> AC #8
- Entry 6 -> AC #9
- Entry 7 -> backward compat (captures the spirit of ACs #8, #9, #12)

No phantom ACs detected. All mapping entries correspond to actual issue requirements.

## Missing AC Check

All 20 ACs from the issue body are accounted for in either the mapping entries or the test coverage verification above. No missing ACs.

## Summary

No blocking or advisory findings. All 20 acceptance criteria from the issue body are covered by the implementation and verified in the diff.
