# Validation Report: Issue #1857

**Date**: 2026-02-22
**Scenarios tested**: scenario-5, scenario-6, scenario-7, scenario-8, scenario-9

## Scenario 5: orchestrator categoryFromExitCode returns canonical taxonomy

**ID**: scenario-5
**Command**: `go test ./internal/batch/... -run TestCategoryFromExitCode -v`
**Status**: PASSED

**Details**:
The `TestCategoryFromExitCode` test in `orchestrator_test.go` (line 313) covers all exit code mappings:
- Exit code 3 -> "recipe_not_found" (unchanged)
- Exit code 5 -> "network_error" (was "api_error")
- Exit code 6 -> "install_failed" (was "validation_failed")
- Exit code 7 -> "verify_failed" (was "validation_failed")
- Exit code 8 -> "missing_dep" (unchanged)
- Exit code 9 -> "generation_failed" (was "deterministic_insufficient")
- Default (exit 1) -> "generation_failed" (was "validation_failed")

All seven cases pass. The function implementation at `orchestrator.go` line 491 uses a clean switch statement with a docstring documenting the full canonical pipeline taxonomy.

---

## Scenario 6: parseInstallJSON derives category from exit code, not CLI string

**ID**: scenario-6
**Command**: `go test ./internal/batch/... -run TestParseInstallJSON -v`
**Status**: PASSED

**Details**:
The `TestParseInstallJSON` test at `orchestrator_test.go` (line 619) includes 9 subtests. Relevant to this scenario:

1. "category always derived from exit code, not CLI JSON": CLI JSON has `category:"network_error"` and `subcategory:"timeout"` with exit code 5. The returned category is "network_error" (from `categoryFromExitCode(5)`), and subcategory "timeout" is extracted from the CLI JSON. The CLI category field is not used for pipeline classification.

2. "CLI category ignored when it differs from exit code mapping": CLI JSON has `category:"network_error"` with exit code 6. The returned category is "install_failed" (from `categoryFromExitCode(6)`), proving the CLI's category string is ignored. Subcategory "tls_error" is still extracted.

3. "subcategory extracted from CLI JSON": Confirms subcategory "dns_error" is extracted when present.

The `parseInstallJSON` function at `orchestrator.go` line 444 always calls `categoryFromExitCode(exitCode)` first, then unmarshals the JSON only to extract `Subcategory` and `MissingRecipes`. The CLI's `Category` field in the JSON is parsed but never used for the return value.

---

## Scenario 7: parseInstallJSON returns empty subcategory when absent from CLI JSON

**ID**: scenario-7
**Command**: `go test ./internal/batch/... -run TestParseInstallJSON -v`
**Status**: PASSED

**Details**:
The "subcategory empty when absent in CLI JSON" subtest verifies that when CLI JSON has no `subcategory` field (`{"status":"error","category":"install_failed","message":"bad tarball","missing_recipes":[],"exit_code":6}`), `parseInstallJSON` returns an empty subcategory string alongside the category "install_failed" and an empty blocked-by list.

Additional coverage from:
- "invalid JSON falls back to exit code": Returns empty subcategory when JSON parsing fails entirely.
- "empty stdout falls back to exit code": Returns empty subcategory with empty input.
- "valid JSON with missing recipes": Returns empty subcategory when the field is absent in normal JSON.

---

## Scenario 8: FailureRecord includes subcategory in JSONL output

**ID**: scenario-8
**Command**: `go test ./internal/batch/... -run TestSaveResults -v` + manual JSON serialization verification
**Status**: PASSED

**Details**:
The `FailureRecord` struct at `results.go` line 130 has:
```go
Subcategory string `json:"subcategory,omitempty"`
```

Verified behavior via manual serialization test:
1. When `Subcategory` is non-empty (e.g., "timeout"), the JSON output includes `"subcategory":"timeout"`.
2. When `Subcategory` is empty string, the `"subcategory"` key is omitted entirely from JSON output due to `omitempty`.

The `TestSaveResults_groupsFailuresByEcosystem` test confirms that `SaveResults` / `WriteFailures` correctly produces JSONL output. While it doesn't have a test case with non-empty subcategory specifically, the struct tag behavior is deterministic and verified separately.

The `validate()` method at `orchestrator.go` line 384 correctly populates `Subcategory` from `parseInstallJSON` into the `FailureRecord`.

---

## Scenario 9: failure-record schema accepts canonical categories and subcategory

**ID**: scenario-9
**Command**: Python schema validation script
**Status**: PASSED

**Details**:
The schema at `data/schemas/failure-record.schema.json` was validated:

1. **Category enum** contains exactly the six canonical values:
   - `recipe_not_found`, `network_error`, `install_failed`, `verify_failed`, `missing_dep`, `generation_failed`

2. **Old values removed**: `api_error`, `validation_failed`, and `deterministic_insufficient` are not in the enum.

3. **Subcategory property exists** as an optional string:
   - Type: `string`
   - Not in the `required` array
   - Description: "Optional detail within the category (e.g., timeout, dns_error, tls_error)"

4. **Required fields**: `package_id`, `category`, `message`, `timestamp` (subcategory is correctly optional).

---

## Summary

| Scenario | Status | Method |
|----------|--------|--------|
| scenario-5 | PASSED | Unit test: TestCategoryFromExitCode |
| scenario-6 | PASSED | Unit test: TestParseInstallJSON (9 subtests) |
| scenario-7 | PASSED | Unit test: TestParseInstallJSON (subcategory absent subtest) |
| scenario-8 | PASSED | Unit test: TestSaveResults + manual JSON verification |
| scenario-9 | PASSED | Python schema validation script |

All 5 scenarios passed. No issues found.
