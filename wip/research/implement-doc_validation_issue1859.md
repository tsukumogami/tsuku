# Validation Report: Issue #1859 (Dashboard Category Remap + Subcategory Passthrough)

## Scenario 11: dashboard remapCategory translates old category strings
**ID**: scenario-11
**Status**: PASSED
**Command**: `go test ./internal/dashboard/... -run TestRemapCategory -v`
**Result**: All 13 sub-tests passed. Verified:
- `remapCategory("api_error")` returns `"network_error"`
- `remapCategory("validation_failed")` returns `"install_failed"`
- `remapCategory("deterministic_insufficient")` returns `"generation_failed"`
- `remapCategory("deterministic")` returns `"generation_failed"`
- `remapCategory("timeout")` returns `"network_error"`
- `remapCategory("network")` returns `"network_error"`
- Canonical names pass through unchanged: `missing_dep`, `network_error`, `install_failed`, `generation_failed`, `recipe_not_found`, `verify_failed`
- Unknown categories pass through unchanged: `"something_else"` returns `"something_else"`

The `categoryRemap` map at `/home/dangazineu/dev/workspace/tsuku/tsuku-4/public/tsuku/internal/dashboard/failures.go` line 54-61 contains the expected mappings, and `remapCategory()` at line 65-70 correctly does a map lookup with pass-through for unmapped keys.

---

## Scenario 12: dashboard prefers structured subcategory over heuristic extraction
**ID**: scenario-12
**Status**: PASSED
**Commands**: `go test ./internal/dashboard/... -run "TestLoadFailureDetailRecords_structuredSubcategory|TestLoadFailureDetailRecords_noSubcategoryFallsBackToHeuristic|TestLoadFailureDetailRecords_mixedOldNewCategories" -v`
**Result**: All 4 tests passed. Verified:

### Per-recipe format with structured subcategory (TestLoadFailureDetailRecords_structuredSubcategoryPerRecipe):
- JSONL record `{"subcategory":"timeout","category":"network_error","exit_code":5}` retains `Subcategory="timeout"` without calling `extractSubcategory()`
- Category `"network_error"` passes through unchanged

### Legacy batch format with structured subcategory (TestLoadFailureDetailRecords_structuredSubcategoryLegacy):
- JSONL record with `{"subcategory":"dns_error","category":"network_error","message":"DNS resolution failed..."}` retains `Subcategory="dns_error"` even though the message contains no bracket tags

### Heuristic fallback for records without subcategory (TestLoadFailureDetailRecords_noSubcategoryFallsBackToHeuristic):
- JSONL record without subcategory field but with message `"[no_bottles] no prebuilt bottles"` correctly gets `Subcategory="no_bottles"` via bracket tag extraction

### Mixed old/new categories with subcategory passthrough (TestLoadFailureDetailRecords_mixedOldNewCategories):
- Old categories remapped: `api_error`->`network_error`, `validation_failed`->`install_failed`, `deterministic`->`generation_failed`, `timeout`->`network_error`
- Structured subcategory `"tls_error"` preserved for pkg5
- Exit code fallback produces `"deterministic_failed"` for pkg4 (exit code 9, no message)
- Heuristic extraction produces `"timeout"` for pkg1 ("API timeout" message)

The key code path is in `loadFailureDetailRecords()` at `/home/dangazineu/dev/workspace/tsuku/tsuku-4/public/tsuku/internal/dashboard/failures.go` lines 155-163:
```go
for i := range allDetails {
    if allDetails[i].Subcategory == "" {
        allDetails[i].Subcategory = extractSubcategory(...)
    }
}
```
This correctly checks for empty subcategory before calling the heuristic.

---

## Scenario 13: end-to-end pipeline produces consistent categories across paths
**ID**: scenario-13
**Status**: PASSED (partially manual)
**Environment**: manual
**Result**: Validated through existing integration tests rather than end-to-end CLI invocation.

### What was tested:
1. **CLI JSON output path**: `handleInstallError()` uses `classifyInstallError()` to get both exit code and subcategory, then `categoryFromExitCode()` to produce the CLI-facing category string. The `installError` struct includes `Subcategory` field with `omitempty`. Code verified at `install.go` lines 367-383.

2. **Orchestrator parsing path**: `TestParseInstallJSON` (in `internal/batch`) confirms that `parseInstallJSON()` derives pipeline category from exit code via its own `categoryFromExitCode()`, not from the CLI's category string. Subcategory is extracted from CLI JSON and returned.

3. **Dashboard remapping path**: `TestLoadFailureDetailRecords_mixedOldNewCategories` confirms that `remapCategory()` translates old category names to canonical names in both legacy batch and per-recipe JSONL formats. `TestGenerate_integration` confirms the full `Generate()` path produces valid dashboard JSON.

4. **Category consistency**: The dashboard output uses only canonical category keys. The `categoryRemap` map covers all old names: `api_error`, `validation_failed`, `deterministic_insufficient`, `deterministic`, `timeout`, `network`.

### What was NOT tested:
- Actual CLI invocation with `--json` capturing live JSON output (the `--json` flag requires a tool install to fail at the network/install/verify stage; `recipe_not_found` exits before `handleInstallError` for some code paths)
- Real batch pipeline run with dashboard generation against production data

---

## Scenario 14: full test suite passes with all changes
**ID**: scenario-14
**Status**: PASSED
**Commands and results**:

### `go test ./cmd/tsuku/... -v`
- **Result**: PASS
- All tests passed including `TestClassifyInstallError`, `TestInstallErrorJSON`, install flag tests, and all other CLI tests
- Exit: `ok github.com/tsukumogami/tsuku/cmd/tsuku 0.026s`

### `go test ./internal/batch/... -v`
- **Result**: PASS
- All tests passed including `TestCategoryFromExitCode`, `TestParseInstallJSON` (with subcategory extraction tests), `TestSaveResults_groupsFailuresByEcosystem`, and all other batch tests
- Exit: `ok github.com/tsukumogami/tsuku/internal/batch (cached)`

### `go test ./internal/dashboard/... -v`
- **Result**: PASS
- All tests passed including `TestRemapCategory` (13 sub-tests), `TestLoadFailureDetailRecords_structuredSubcategoryPerRecipe`, `TestLoadFailureDetailRecords_structuredSubcategoryLegacy`, `TestLoadFailureDetailRecords_noSubcategoryFallsBackToHeuristic`, `TestLoadFailureDetailRecords_mixedOldNewCategories`, `TestGenerate_integration`, and all other dashboard tests
- Exit: `ok github.com/tsukumogami/tsuku/internal/dashboard 0.016s`

### `go vet ./...`
- **Result**: PASS (no output, exit code 0)
- No issues reported across entire codebase
