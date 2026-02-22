# Scrutiny Review: Intent Focus - Issue #1859

**Issue**: feat(dashboard): read structured subcategories with category remap fallback
**Focus**: intent
**Reviewer**: scrutiny-review-decisions (intent)

## Sub-check 1: Design Intent Alignment

### Design Doc Reference

The design doc (DESIGN-structured-error-subcategories.md) describes Phase 4 ("Dashboard Update") as follows:

> Update dashboard deserialization structs (`FailureRecord`, `PackageFailure` in `dashboard.go`) to include `Subcategory`. Update `loadFailureDetailsFromFile()` to read the field from JSONL records. Add a category remap for old records (e.g., `deterministic` -> `generation_failed`, `api_error` -> `network_error`, `validation_failed` -> `install_failed`, `deterministic_insufficient` -> `generation_failed`). Change `loadFailureDetailRecords()` to only call `extractSubcategory()` when the loaded subcategory is empty after loading. Update tests.

The design doc also specifies the data flow for the dashboard:

```
Dashboard: loadFailureDetailsFromFile()
  1. Read category from JSONL (remap old names if needed)
  2. if record.Subcategory != "" -> use it
     else -> extractSubcategory(category, message, exitCode)  [fallback]
```

And a Category Remap Table listing six old-to-new mappings.

### Assessment

**Struct changes**: The implementation adds `Subcategory string` to both `FailureRecord` and `PackageFailure` in `dashboard.go`, with `json:"subcategory,omitempty"` tags. This matches the design doc's intent exactly.

**Category remap**: The `categoryRemap` map in `failures.go` contains all six mappings specified in the design doc's Category Remap Table:
- `api_error` -> `network_error`
- `validation_failed` -> `install_failed`
- `deterministic_insufficient` -> `generation_failed`
- `deterministic` -> `generation_failed`
- `timeout` -> `network_error`
- `network` -> `network_error`

This is a complete match with the design doc.

**Remap application points**: The design doc says remap should be applied in `loadFailureDetailsFromFile()`. The implementation applies `remapCategory()` in both `loadFailureDetailsFromFile()` (for `FailureDetail` construction from both legacy batch and per-recipe formats) and `loadFailures()` (the summary-level loader in `dashboard.go`). The issue AC explicitly requires remap in both functions, and this makes sense -- the summary counts and detail records should use the same canonical names. The design doc's Phase 4 description mentions `loadFailureDetailsFromFile()` specifically but the intent of consistent categories across the board is clearly served.

**Subcategory passthrough**: In `loadFailureDetailsFromFile()`, the implementation reads `f.Subcategory` (legacy batch) and `record.Subcategory` (per-recipe) into `FailureDetail.Subcategory`. This matches the design doc's data flow step 1.

**Conditional extraction**: In `loadFailureDetailRecords()`, the `extractSubcategory()` call is now wrapped in `if allDetails[i].Subcategory == ""`. This exactly matches the design doc's data flow step 2: "if record.Subcategory != '' -> use it, else -> extractSubcategory(...)".

**Placement of remapCategory()**: The function is placed in `failures.go` alongside `extractSubcategory()`. Both functions are category classification utilities, so this placement is sensible and consistent with the design doc's indication that dashboard-specific changes go in `failures.go`.

### Intent Alignment Verdict

The implementation captures the design doc's intent fully. The data flow matches. The remap table is complete. The conditional subcategory logic is correct. The application of remap in `loadFailures()` (not just `loadFailureDetailsFromFile()`) is an expansion beyond what the design doc explicitly mentions for Phase 4 but is correct behavior -- the design doc's intent of "consistent category names" requires it, and the issue AC explicitly calls for it.

**No findings.**

## Sub-check 2: Cross-issue Enablement

The issue description states: "Downstream Dependencies: None. This is a leaf node."

The `context.downstream_issues` is empty. Per instructions, this sub-check is skipped.

## Backward Coherence

**Previous summary**: "Files changed: internal/dashboard/dashboard.go, internal/dashboard/dashboard_test.go, internal/dashboard/failures.go, internal/dashboard/failures_test.go. Key decisions: Placed remapCategory() in failures.go alongside extractSubcategory(). Applied remap in both loadFailures() and loadFailureDetailsFromFile() for consistency. Conditional extractSubcategory() only called when structured subcategory is absent."

This summary describes issue #1859 itself (the previous summary appears to be from the same issue, likely because this is the terminal issue in the sequence). The implementation matches these key decisions exactly:
- `remapCategory()` is in `failures.go` alongside `extractSubcategory()` -- confirmed.
- Remap applies in both `loadFailures()` and `loadFailureDetailsFromFile()` -- confirmed in the diff.
- Conditional `extractSubcategory()` only when subcategory is empty -- confirmed in the diff.

The approach is consistent with the patterns established in prior issues (#1856, #1857, #1858). Specifically:
- Issue #1857 added `Subcategory` to the batch `FailureRecord` in `internal/batch/results.go`. This issue adds the corresponding field to the dashboard's `FailureRecord` and `PackageFailure`, which is the consuming side. No contradiction.
- Issue #1858 aligned CI workflow category names. This issue's remap table handles the old CI workflow names (`deterministic`, `timeout`, `network`) during transition, which is complementary rather than conflicting.

**No findings.**

## Summary of Findings

| # | Severity | AC / Area | Finding |
|---|----------|-----------|---------|
| (none) | - | - | No intent alignment issues found |

**Blocking findings**: 0
**Advisory findings**: 0
