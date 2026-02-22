# Scrutiny Review: Justification Focus - Issue #1859

**Issue**: #1859 (feat(dashboard): read structured subcategories with category remap fallback)
**Scrutiny Focus**: justification
**Reviewer Assessment Date**: 2026-02-21

## Overview

The requirements mapping contains 7 entries, all with status "implemented". There are zero deviations reported. Since the justification focus evaluates the quality of deviation explanations, the primary question is whether all "implemented" claims hold and whether any deviations are being hidden behind "implemented" status.

## Mapping Entries Evaluated

### AC 1: "remapCategory translates old category strings"
**Claimed status**: implemented
**Assessment**: Confirmed. The diff shows `remapCategory()` defined in `failures.go` with a `categoryRemap` map covering all 6 old-to-new translations specified in the issue AC: `api_error` -> `network_error`, `validation_failed` -> `install_failed`, `deterministic_insufficient` -> `generation_failed`, `deterministic` -> `generation_failed`, `timeout` -> `network_error`, `network` -> `network_error`. `TestRemapCategory` covers all mappings plus passthrough for canonical names and unknown categories.
**Severity**: No finding.

### AC 2: "Subcategory field added to FailureRecord and PackageFailure"
**Claimed status**: implemented
**Assessment**: Confirmed. The diff in `dashboard.go` adds `Subcategory string \`json:"subcategory,omitempty"\`` to both `FailureRecord` (line ~156) and `PackageFailure` (line ~167).
**Severity**: No finding.

### AC 3: "loadFailureDetailsFromFile reads subcategory from both formats"
**Claimed status**: implemented
**Assessment**: Confirmed. The diff in `failures.go` shows `loadFailureDetailsFromFile()` now includes `Subcategory: f.Subcategory` for legacy batch format entries and `Subcategory: record.Subcategory` for per-recipe format records. Both paths include the subcategory in the `FailureDetail` struct construction.
**Severity**: No finding.

### AC 4: "remap applies in both loadFailures and loadFailureDetailsFromFile"
**Claimed status**: implemented
**Assessment**: Confirmed. `loadFailures()` in `dashboard.go` calls `remapCategory()` in both the legacy batch path (`cat := remapCategory(f.Category)`) and the per-recipe path (`cat := remapCategory(record.Category)`). `loadFailureDetailsFromFile()` in `failures.go` calls `remapCategory()` in both the legacy batch construction (`Category: remapCategory(f.Category)`) and the per-recipe construction (`Category: remapCategory(record.Category)`). Tests in `dashboard_test.go` verify the remap behavior in `TestLoadFailures_legacyFormat`, `TestLoadFailures_perRecipeFormat`, `TestLoadFailures_perRecipeWithBlockedBy`, and `TestLoadFailures_malformedLines`.
**Severity**: No finding.

### AC 5: "extractSubcategory conditional - only called when subcategory empty"
**Claimed status**: implemented
**Assessment**: Confirmed. The diff in `failures.go:loadFailureDetailRecords()` wraps the `extractSubcategory()` call in `if allDetails[i].Subcategory == ""`. Records with a non-empty subcategory skip the heuristic.
**Severity**: No finding.

### AC 6: "structured subcategory preserved without heuristic override"
**Claimed status**: implemented
**Assessment**: Confirmed. This is the positive consequence of AC 5. `TestLoadFailureDetailRecords_structuredSubcategoryPerRecipe` verifies that a per-recipe record with `"subcategory":"timeout"` retains that value. `TestLoadFailureDetailRecords_structuredSubcategoryLegacy` verifies the same for legacy batch format with `"subcategory":"dns_error"`. The end-to-end test `TestLoadFailureDetailRecords_mixedOldNewCategories` verifies `pkg5` with structured subcategory `"tls_error"` passes through while `pkg4` without subcategory gets heuristic extraction.
**Severity**: No finding.

### AC 7: "backward compat - records without subcategory get heuristic extraction"
**Claimed status**: implemented
**Assessment**: Confirmed. `TestLoadFailureDetailRecords_noSubcategoryFallsBackToHeuristic` verifies that a record without a subcategory field gets `[no_bottles]` extracted via the heuristic. The mixed test also confirms `pkg4` (no subcategory, exit code 9) gets `deterministic_failed` via heuristic fallback.
**Severity**: No finding.

## Deviation Analysis

No deviations reported. After examining the diff against all acceptance criteria from the issue body, no hidden deviations were found. Every AC in the issue has a corresponding implementation and test coverage.

## Avoidance Pattern Check

No avoidance patterns detected. All ACs are implemented, not deferred. No "out of scope" or "can be added later" claims.

## Proportionality Check

7 ACs, 7 "implemented" -- proportionate. The diff touches exactly the files the issue specifies (dashboard.go, failures.go, and their tests). The implementation is focused with no extraneous changes.

## Summary

No blocking or advisory findings. All 7 mapping entries claim "implemented" status and the diff confirms each claim. The justification focus has nothing to evaluate since there are no deviations to justify. The implementation is a clean, complete match to the issue requirements.
