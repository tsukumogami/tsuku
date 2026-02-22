# Test Plan: structured-error-subcategories

Generated from: docs/designs/DESIGN-structured-error-subcategories.md
Issues covered: 4
Total scenarios: 14

---

## Scenario 1: classifyInstallError returns subcategory for timeout errors
**ID**: scenario-1
**Testable after**: #1856
**Commands**:
- `go test ./cmd/tsuku/... -run TestClassifyInstallError -v`
**Expected**: Test passes. classifyInstallError returns (ExitNetwork, "timeout") for ErrTypeTimeout errors. The second return value is the subcategory string.
**Status**: pending

---

## Scenario 2: classifyInstallError returns subcategory for DNS, TLS, and connection errors
**ID**: scenario-2
**Testable after**: #1856
**Commands**:
- `go test ./cmd/tsuku/... -run TestClassifyInstallError -v`
**Expected**: Test passes. classifyInstallError returns "dns_error" for ErrTypeDNS, "tls_error" for ErrTypeTLS, and "connection_error" for ErrTypeConnection. ErrTypeNotFound, ErrTypeNetwork (generic), dependency wrappers, and catch-all cases return empty subcategory.
**Status**: pending

---

## Scenario 3: installError JSON includes subcategory when non-empty
**ID**: scenario-3
**Testable after**: #1856
**Commands**:
- `go test ./cmd/tsuku/... -run TestInstallErrorJSON -v`
**Expected**: Test passes. When Subcategory is set (e.g., "timeout"), the marshaled JSON contains "subcategory":"timeout". When Subcategory is empty, the key is omitted entirely from JSON output (omitempty behavior).
**Status**: pending

---

## Scenario 4: handleInstallError populates subcategory in JSON output
**ID**: scenario-4
**Testable after**: #1856
**Commands**:
- `go build ./cmd/tsuku`
- `go vet ./cmd/tsuku/...`
**Expected**: Build succeeds with no errors. go vet passes. The installError struct literal in handleInstallError uses both return values from classifyInstallError (code and subcategory) and sets the Subcategory field.
**Status**: pending

---

## Scenario 5: orchestrator categoryFromExitCode returns canonical taxonomy
**ID**: scenario-5
**Testable after**: #1857
**Commands**:
- `go test ./internal/batch/... -run TestCategoryFromExitCode -v`
**Expected**: Test passes. Exit code 5 returns "network_error" (was "api_error"). Exit code 6 returns "install_failed" (was "validation_failed"). Exit code 7 returns "verify_failed" (was "validation_failed"). Exit code 9 returns "generation_failed" (was "deterministic_insufficient"). Default returns "generation_failed" (was "validation_failed"). Exit codes 3 and 8 are unchanged.
**Status**: pending

---

## Scenario 6: parseInstallJSON derives category from exit code, not CLI string
**ID**: scenario-6
**Testable after**: #1857
**Commands**:
- `go test ./internal/batch/... -run TestParseInstallJSON -v`
**Expected**: Test passes. When CLI JSON contains {"category":"network_error", "subcategory":"timeout"} with exit code 5, the returned category is "network_error" from categoryFromExitCode(5), not from the CLI's category string. The CLI category field is ignored for pipeline classification. The subcategory "timeout" is extracted and returned.
**Status**: pending

---

## Scenario 7: parseInstallJSON returns empty subcategory when absent from CLI JSON
**ID**: scenario-7
**Testable after**: #1857
**Commands**:
- `go test ./internal/batch/... -run TestParseInstallJSON -v`
**Expected**: Test passes. When CLI JSON has no subcategory field, parseInstallJSON returns an empty subcategory string alongside the category and blocked-by list.
**Status**: pending

---

## Scenario 8: FailureRecord includes subcategory in JSONL output
**ID**: scenario-8
**Testable after**: #1857
**Commands**:
- `go test ./internal/batch/... -run TestSaveResults -v`
**Expected**: Test passes. FailureRecord struct has a Subcategory field with json:"subcategory,omitempty" tag. When subcategory is non-empty, it appears in JSONL output. When empty, it is omitted.
**Status**: pending

---

## Scenario 9: failure-record schema accepts canonical categories and subcategory
**ID**: scenario-9
**Testable after**: #1857
**Commands**:
- `python3 -c "import json; schema=json.load(open('data/schemas/failure-record.schema.json')); cats=schema['properties']['failures']['items']['properties']['category']['enum']; print(cats); assert 'network_error' in cats; assert 'install_failed' in cats; assert 'verify_failed' in cats; assert 'generation_failed' in cats; assert 'api_error' not in cats; assert 'validation_failed' not in cats; print('subcategory' in schema['properties']['failures']['items']['properties']); print('PASS')"`
**Expected**: The schema's category enum contains the six canonical values (recipe_not_found, network_error, install_failed, verify_failed, missing_dep, generation_failed). Old values (api_error, validation_failed, deterministic_insufficient) are removed. A subcategory property exists as an optional string. Output ends with "PASS".
**Status**: pending

---

## Scenario 10: CI workflow jq uses canonical category names
**ID**: scenario-10
**Testable after**: #1858
**Commands**:
- `grep -c '"network_error"' .github/workflows/batch-generate.yml`
- `grep -c '"generation_failed"' .github/workflows/batch-generate.yml`
- `grep -c '"network"' .github/workflows/batch-generate.yml`
- `grep -c '"timeout"' .github/workflows/batch-generate.yml`
- `grep -c '"deterministic"' .github/workflows/batch-generate.yml`
**Expected**: "network_error" and "generation_failed" each appear at least once. The old standalone category names "network" (as a category value, not as part of "network_error"), "timeout" (as a category value), and "deterministic" (as a standalone category value) no longer appear in category-mapping jq expressions. Exit codes 124 and 137 map to "network_error" instead of "timeout".
**Status**: pending

---

## Scenario 11: dashboard remapCategory translates old category strings
**ID**: scenario-11
**Testable after**: #1859
**Commands**:
- `go test ./internal/dashboard/... -run TestRemapCategory -v`
**Expected**: Test passes. remapCategory("api_error") returns "network_error". remapCategory("validation_failed") returns "install_failed". remapCategory("deterministic_insufficient") returns "generation_failed". remapCategory("deterministic") returns "generation_failed". remapCategory("timeout") returns "network_error". remapCategory("network") returns "network_error". Canonical names pass through unchanged: remapCategory("missing_dep") returns "missing_dep".
**Status**: pending

---

## Scenario 12: dashboard prefers structured subcategory over heuristic extraction
**ID**: scenario-12
**Testable after**: #1859
**Commands**:
- `go test ./internal/dashboard/... -run TestLoadFailureDetailRecords -v`
**Expected**: Test passes. Records with a non-empty "subcategory" field in JSONL retain that value and do not have it overwritten by extractSubcategory(). Records without a subcategory field still get heuristic extraction (backward compatibility). Both per-recipe format and legacy batch format records support subcategory passthrough.
**Status**: pending

---

## Scenario 13: end-to-end pipeline produces consistent categories across paths
**ID**: scenario-13
**Testable after**: #1857, #1858, #1859
**Environment**: manual
**Commands**:
- Build tsuku: `go build -o tsuku ./cmd/tsuku`
- Trigger a validate-path failure (e.g., `./tsuku install nonexistent-tool-xyz --json`) and capture JSON output
- Verify JSON output contains "category" and optionally "subcategory" fields
- Run dashboard generation against data/failures/ with mixed old/new JSONL records
- Inspect dashboard output for category consistency
**Expected**: The validate-path JSON output uses CLI category names with subcategory when applicable. When the orchestrator processes this output via parseInstallJSON, it derives the pipeline category from the exit code. The dashboard output contains only canonical category keys (recipe_not_found, network_error, install_failed, verify_failed, missing_dep, generation_failed) with no legacy names (api_error, validation_failed, deterministic_insufficient, deterministic, timeout, network).
**Status**: pending

---

## Scenario 14: full test suite passes with all changes
**ID**: scenario-14
**Testable after**: #1856, #1857, #1858, #1859
**Commands**:
- `go test ./cmd/tsuku/... -v`
- `go test ./internal/batch/... -v`
- `go test ./internal/dashboard/... -v`
- `go vet ./...`
**Expected**: All tests pass. No regressions in existing functionality. go vet reports no issues. The three test packages cover: CLI subcategory output (cmd/tsuku), orchestrator category normalization and subcategory passthrough (internal/batch), and dashboard category remap with conditional subcategory extraction (internal/dashboard).
**Status**: pending
