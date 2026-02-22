# Scrutiny Review: Completeness -- Issue #1857

**Issue**: feat(batch): normalize pipeline categories and add subcategory passthrough
**Focus**: completeness
**Reviewer**: coder agent (scrutiny)

## AC-to-Implementation Verification

### AC 1: `categoryFromExitCode()` returns canonical taxonomy values

**Mapping claim**: "categoryFromExitCode canonical taxonomy" -- implemented
**Diff evidence**: `internal/batch/orchestrator.go` lines 491-508. Exit 5 returns `"network_error"` (was `"api_error"`), exit 6 returns `"install_failed"` (was `"validation_failed"`), exit 7 returns `"verify_failed"` (was `"validation_failed"`), exit 9 returns `"generation_failed"` (was `"deterministic_insufficient"`), default returns `"generation_failed"` (was `"validation_failed"`). Exits 3 and 8 unchanged.
**Assessment**: CONFIRMED. Every mapping in the AC matches the diff.

### AC 2: `parseInstallJSON()` always uses `categoryFromExitCode(exitCode)` for the pipeline category

**Mapping claim**: "parseInstallJSON uses categoryFromExitCode not CLI category" -- implemented
**Diff evidence**: `internal/batch/orchestrator.go` lines 444-451. `category = categoryFromExitCode(exitCode)` is assigned unconditionally at the top of the function. The old code had `cat := result.Category` with a fallback to `categoryFromExitCode()` only when empty. The new code ignores `result.Category` entirely.
**Assessment**: CONFIRMED.

### AC 3: `parseInstallJSON()` extracts the `subcategory` field from CLI JSON output and returns it

**Mapping claim**: "subcategory extracted from CLI JSON" -- implemented
**Diff evidence**: `parseInstallJSON` now returns three values `(category string, subcategory string, blockedBy []string)`. On successful JSON parse, returns `result.Subcategory`. On parse failure, returns empty string.
**Assessment**: CONFIRMED.

### AC 4: `installResult` struct includes a `Subcategory string` field for JSON deserialization

**Mapping claim**: Not explicitly listed in mapping.
**Diff evidence**: `internal/batch/orchestrator.go` line 20 adds `Subcategory string \`json:"subcategory"\`` to `installResult`.
**Assessment**: CONFIRMED. The mapping doesn't have a separate entry for this AC, but it's covered implicitly by the "subcategory extracted from CLI JSON" entry since extraction requires the struct field. Not a gap.

### AC 5: `FailureRecord` in `results.go` includes `Subcategory string` with tag `json:"subcategory,omitempty"`

**Mapping claim**: "FailureRecord has Subcategory field" -- implemented
**Diff evidence**: `internal/batch/results.go` line 133 shows `Subcategory string \`json:"subcategory,omitempty"\``.
**Assessment**: CONFIRMED.

### AC 6: Subcategory is populated in the `FailureRecord` returned by `validate()` via `parseInstallJSON()`

**Mapping claim**: Not explicitly listed in mapping.
**Diff evidence**: `orchestrator.go` lines 410-421 (non-network exit in validate) and 425-436 (retry exhaustion in validate) both call `parseInstallJSON()` and pass the returned `subcategory` into `FailureRecord{Subcategory: subcategory}`.
**Assessment**: CONFIRMED. Not explicitly in the mapping but clearly implemented. The mapping's "subcategory extracted from CLI JSON" partially covers this, though the AC is specifically about `validate()` populating `FailureRecord`.

### AC 7: The `generate()` retry-exhaustion fallback uses `"network_error"` instead of `"api_error"`

**Mapping claim**: "generate() fallback uses network_error" -- implemented
**Diff evidence**: `orchestrator.go` line 373 shows `Category: "network_error"` (was `"api_error"`).
**Assessment**: CONFIRMED.

### AC 8: The `generate()` path correctly uses `"missing_dep"` override when `blockedBy` is non-empty and category is `install_failed`

**Mapping claim**: Not explicitly listed in mapping.
**Diff evidence**: `orchestrator.go` line 353 shows `if len(blockedBy) > 0 && category == "install_failed"` (was `"validation_failed"`).
**Assessment**: CONFIRMED. This AC is not in the mapping, but it's clearly implemented in the diff.

### AC 9: `data/schemas/failure-record.schema.json` adds `subcategory` as an optional string property

**Mapping claim**: "schema updated with canonical enum and subcategory" -- implemented
**Diff evidence**: Schema lines 47-49 show `"subcategory": {"type": "string", "description": ...}` added as a property. It is not in the `required` array, making it optional.
**Assessment**: CONFIRMED.

### AC 10: `data/schemas/failure-record.schema.json` updates the `category` enum

**Mapping claim**: "schema updated with canonical enum and subcategory" -- implemented
**Diff evidence**: Schema lines 37-44 show enum `["recipe_not_found", "network_error", "install_failed", "verify_failed", "missing_dep", "generation_failed"]`. The old values (`no_bottles`, `build_from_source`, `complex_archive`, `api_error`, `validation_failed`) are removed.
**Assessment**: CONFIRMED.

### AC 11: Existing tests updated with new category names

**Mapping claim**: "all tests updated and passing" -- implemented
**Diff evidence**:
- `TestCategoryFromExitCode`: exit 5 now expects `"network_error"`, exit 6 expects `"install_failed"`, exit 7 expects `"verify_failed"`, exit 9 expects `"generation_failed"`, default (1) expects `"generation_failed"`. CONFIRMED.
- `TestRun_withFakeBinary`: expects `"install_failed"` (was `"validation_failed"`). CONFIRMED.
- `TestRun_validationFailureGeneric`: expects `"install_failed"` (was `"validation_failed"`). CONFIRMED.
- `TestParseInstallJSON`: test case `"valid JSON no missing recipes"` updated from `wantCategory: "validation_failed"` to `"install_failed"`. The old test case that checked CLI category passthrough now derives from exit code. CONFIRMED.
- `TestSaveResults_groupsFailuresByEcosystem`: fixture updated from `"validation_failed"` to `"install_failed"` and `"api_error"` to `"network_error"`. CONFIRMED.

### AC 12: New test cases verify subcategory extraction through `parseInstallJSON()`

**Mapping claim**: "all tests updated and passing" -- implemented
**Diff evidence**: `TestParseInstallJSON` gains 4 new test cases:
- "category always derived from exit code, not CLI JSON" (with subcategory "timeout")
- "subcategory extracted from CLI JSON" (subcategory "dns_error")
- "subcategory empty when absent in CLI JSON"
- "CLI category ignored when it differs from exit code mapping" (subcategory "tls_error")

All test cases include `wantSubcategory` assertions and the test body calls `parseInstallJSON` with 3 return values.
**Assessment**: CONFIRMED. Four new test cases cover subcategory extraction.

### AC 13: `scripts/requeue-unblocked.sh` and `scripts/reconcile-queue.sh` are unaffected

**Mapping claim**: Not in mapping (verify-only AC).
**Diff evidence**: Neither script appears in `git diff HEAD~1 --name-only`. Grep confirms both scripts only match on `missing_dep`, which is unchanged in the taxonomy.
**Assessment**: CONFIRMED. No changes needed, no changes made.

### AC 14: All existing tests pass: `go test ./internal/batch/...`

**Mapping claim**: "all tests updated and passing" -- implemented
**Evidence**: Test run shows `ok github.com/tsukumogami/tsuku/internal/batch (cached)`.
**Assessment**: CONFIRMED.

## Mapping Quality Check

### Missing ACs in Mapping

The mapping has 7 entries but the issue has 14 ACs. The mapping aggregates several ACs into broader claims:
- ACs 4, 6, 8, 13 have no explicit mapping entry
- ACs 11, 12, 14 are collapsed into "all tests updated and passing"
- ACs 9, 10 are collapsed into "schema updated with canonical enum and subcategory"

However, all ACs are verifiably implemented in the diff. The mapping is coarser-grained than the ACs, but no AC is missing from the implementation.

### Phantom ACs in Mapping

None. All 7 mapping entries correspond to real ACs or groups of ACs from the issue body.

## Summary

All 14 acceptance criteria are implemented and verified against the diff. No blocking findings. No phantom ACs. The mapping is coarser-grained than the issue's AC list (7 entries for 14 ACs), but every AC has corresponding evidence in the changed files.
