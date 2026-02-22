# Scrutiny Review: Justification Focus -- Issue #1857

**Issue**: #1857 feat(batch): normalize pipeline categories and add subcategory passthrough
**Focus**: justification
**Reviewer**: coder agent (scrutiny)
**Date**: 2026-02-21

## Overview

The requirements mapping reports all seven ACs as "implemented" with no deviations. Since the justification focus evaluates the quality of deviation explanations, a mapping with zero deviations has limited surface area for this lens. However, the justification focus also checks for avoidance patterns, proportionality, and whether the "all implemented" claim is credible given the diff.

## Mapping Evaluation

### AC 1: "categoryFromExitCode canonical taxonomy" -- claimed: implemented

**Assessment**: Verified. The diff shows `categoryFromExitCode()` in `orchestrator.go` now returns:
- Exit 5: `network_error` (was `api_error`)
- Exit 6: `install_failed` (was `validation_failed`)
- Exit 7: `verify_failed` (was `validation_failed`)
- Exit 9: `generation_failed` (was `deterministic_insufficient`)
- Default: `generation_failed` (was `validation_failed`)
- Exit 3 and 8 unchanged.

Matches the issue AC exactly. No deviation.

### AC 2: "parseInstallJSON uses categoryFromExitCode not CLI category" -- claimed: implemented

**Assessment**: Verified. The diff shows `parseInstallJSON()` now assigns `category = categoryFromExitCode(exitCode)` at the top and never reads `result.Category`. The old code had `cat := result.Category` with a fallback; that's gone entirely. Test case "CLI category ignored when it differs from exit code mapping" explicitly verifies this (exit 6 with CLI category "network_error" returns "install_failed" from exit code).

No deviation.

### AC 3: "subcategory extracted from CLI JSON" -- claimed: implemented

**Assessment**: Verified. `parseInstallJSON()` returns `result.Subcategory` from the parsed JSON. Test cases "subcategory extracted from CLI JSON" (dns_error) and "category always derived from exit code, not CLI JSON" (timeout) verify extraction. Test case "subcategory empty when absent in CLI JSON" verifies the empty case.

No deviation.

### AC 4: "FailureRecord has Subcategory field" -- claimed: implemented

**Assessment**: Verified. `results.go` diff adds `Subcategory string \`json:"subcategory,omitempty"\`` to `FailureRecord`. The `installResult` struct in `orchestrator.go` also gains `Subcategory string \`json:"subcategory"\`` for deserialization.

No deviation.

### AC 5: "generate() fallback uses network_error" -- claimed: implemented

**Assessment**: Verified. The retry-exhaustion fallback in `generate()` (line 373) now uses `"network_error"` instead of `"api_error"`.

No deviation.

### AC 6: "schema updated with canonical enum and subcategory" -- claimed: implemented

**Assessment**: Verified. `failure-record.schema.json` diff replaces old enum values (`no_bottles`, `build_from_source`, `complex_archive`, `api_error`, `validation_failed`) with canonical set (`recipe_not_found`, `network_error`, `install_failed`, `verify_failed`, `missing_dep`, `generation_failed`). `subcategory` added as optional string property. Schema has `additionalProperties: false`, so the new property was necessary to prevent validation failures.

No deviation.

### AC 7: "all tests updated and passing" -- claimed: implemented

**Assessment**: Verified against diff. `TestCategoryFromExitCode` expectations updated. `TestRun_withFakeBinary` and `TestRun_validationFailureGeneric` updated to expect `install_failed`. `TestParseInstallJSON` gains four new subcategory-focused test cases and all existing cases updated to the three-return-value signature. `TestSaveResults_groupsFailuresByEcosystem` fixture categories updated.

No deviation.

## Avoidance Pattern Check

No deviations were claimed, so there are no "too complex" or "out of scope" dismissals to examine.

## Proportionality Check

Seven ACs, all claimed implemented. The diff touches four files (orchestrator.go, orchestrator_test.go, results.go, failure-record.schema.json), which matches the issue's expected file list exactly. The test diff is substantial (new test cases, updated expectations across five test functions). The ratio of ACs to changed code is proportionate.

## Cross-Issue Enablement (Downstream)

### #1858 (CI workflow alignment)
Needs the canonical taxonomy in use by the orchestrator. The implementation provides this: `categoryFromExitCode()` now returns the canonical names that #1858's jq expressions will match. No concerns.

### #1859 (Dashboard update)
Needs:
1. `Subcategory` field on `FailureRecord` -- provided in `results.go`.
2. Canonical category names in JSONL output -- provided via `categoryFromExitCode()`.
3. The dashboard's own structs (`FailureRecord` in `dashboard.go`, `PackageFailure`) are not touched here, which is correct -- that's #1859's job.

The foundation is sufficient for both downstream issues.

## Backward Coherence (Previous Issue #1856)

Previous summary: "Files changed: cmd/tsuku/install.go, cmd/tsuku/install_test.go". #1856 added the `subcategory` field to the CLI's `installError` struct and `classifyInstallError()`. This issue (#1857) consumes that field via `parseInstallJSON()`. The approach is consistent: #1856 produces subcategories at the CLI level, #1857 passes them through at the orchestrator level. No contradictions in conventions, patterns, or naming.

## Summary

All seven mapping entries claim "implemented" status. Each claim is corroborated by the diff. There are no deviations to evaluate for justification quality, which means the justification focus has no findings. The implementation is proportionate to the requirements, touches exactly the expected files, and provides an adequate foundation for downstream issues #1858 and #1859.

**Blocking findings**: 0
**Advisory findings**: 0
