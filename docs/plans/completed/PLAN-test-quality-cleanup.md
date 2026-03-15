---
schema: plan/v1
status: Completed
execution_mode: single-pr
issue_count: 10
---

# PLAN: Test Quality Cleanup

## Status

Completed

## Scope Summary

Clean up test files added by the code-coverage-75 effort. Redistribute tests from numbered `coverage_gap*` grab-bag files into canonical `_test.go` files, delete counterproductive tests that can never fail or test stdlib behavior, consolidate repetitive test functions into table-driven tests, and eliminate duplicate tests across the codebase.

## Decomposition Strategy

**Sequential decomposition.** Phase 1 (issues 1-5) handled redistribution, deletion of counterproductive tests, and initial consolidation. Phase 2 (issues 6-10) applies systematic table-driven consolidation and duplicate removal across all packages based on a full codebase scan.

## Issue Outlines

### Issue 1: Redistribute coverage_gap tests into canonical files [DONE]

**Complexity:** testable

**Description:**
Move all test functions from `internal/actions/coverage_gap_test.go` through `coverage_gap12_test.go` (12 files, 379 functions) into the `_test.go` file corresponding to each test's source file. For example, `TestConfigureMakeAction_Preflight` moves from `coverage_gap_test.go` to `configure_make_test.go`. Delete the empty coverage_gap files after redistribution. Also rename `*_coverage_test.go` files in `internal/verify/` and `internal/discover/` to match their source files (e.g., `header_coverage_test.go` content merges into `header_test.go`).

**Acceptance Criteria:**
- No files named `coverage_gap*` remain in the codebase
- No files with `_coverage_` in the name remain in `internal/actions/`
- Every test function lives in the `_test.go` file named after its source file
- All tests pass after redistribution
- No test logic is changed, only file locations

### Issue 2: Delete counterproductive and redundant tests [DONE]

**Complexity:** simple

**Description:**
Remove tests identified by reviewers as counterproductive or redundant:
- Tests with swallowed errors (`_ = err`) that can never fail (~5 instances)
- Stub action tests that verify no-ops return nil (AptRepo, AptPPA, DnfRepo stubs)
- Tests validating stdlib behavior (`TestCpanInstall_Min`, `TestComputeSHA256_NonexistentFile`, `TestCopyFile_NonexistentSource`)
- Duplicate tests covering the same behavior as existing canonical tests
- `IsDeterministic` and `Name()` tests already covered by the table-driven test in `decomposable_test.go`

**Acceptance Criteria:**
- No test functions contain `_ = err` or `_ = result` patterns that discard the value under test
- No duplicate test functions testing the same behavior remain
- All remaining tests pass
- Coverage may drop slightly, which is acceptable

### Issue 3: Consolidate repetitive tests into table-driven tests (actions package) [DONE]

**Complexity:** testable

**Description:**
Within `internal/actions/`, identify groups of test functions that test the same function/method with different inputs and share identical structure. Merge them into table-driven tests with `t.Run` subtests. Focus on the patterns identified in the migration plan: preflight validation groups, decompose error path groups, and Execute error path groups that repeat the same setup-call-assert pattern with different parameters.

**Acceptance Criteria:**
- Groups of 3+ near-identical test functions are consolidated into table-driven tests
- Each table test uses descriptive case names in `t.Run`
- All tests pass after consolidation
- No behavioral changes to what is tested

### Issue 4: Consolidate repetitive tests in non-actions packages [DONE]

**Complexity:** testable

**Description:**
Apply table-driven consolidation to the 11 opportunities identified in the migration plan:
- `gem_exec_test.go`: param validation (3 funcs), version validation (4 funcs), buildEnvironment (3 funcs), findBundler (2 funcs), mock bundler integration (6 funcs), version validation full (2 funcs), relative paths (2 funcs)
- `composites_test.go`: action name tests (3 funcs)
- `repair_loop_test.go`: session generation (2 funcs)
- `dependency_test.go`: resolveRuntimeDeps (3 funcs), mapKeys nil case (1 func into existing table)

Also fix the minor issues: remove `containsStr` helper (use `strings.Contains` directly), differentiate identical error messages in `coverage_gap11_test.go` (now redistributed), and use `t.Setenv()` in `dltest_coverage_test.go`.

**Acceptance Criteria:**
- All 7 mechanical opportunities from the migration plan are consolidated
- At least 2 of 4 judgment-required opportunities are consolidated
- `containsStr` helper removed from all packages
- All tests pass

### Issue 5: Verify test suite health [DONE]

**Complexity:** simple

**Description:**
Run the full test suite, verify all tests pass, check that coverage hasn't dropped below 73% (acceptable regression from deleting low-value tests), and confirm no linter violations. Run `go vet`, the full linter suite, and verify the build succeeds.

**Acceptance Criteria:**
- `go test ./...` passes with zero failures
- `go vet ./...` reports no issues
- Coverage remains above 73%
- No linter violations introduced

### Issue 6: Delete duplicate tests in actions package (~25 functions, ~350 lines) [DONE]

**Complexity:** simple

**Description:**
Delete test functions that duplicate scenarios already covered by existing table-driven tests. The coverage-75 agents created individual test functions without checking for existing coverage. Specific targets:

- `gem_install_test.go`: Delete 8 individual `Execute_*` tests and 5 individual `Decompose_*` tests that duplicate cases in `TestGemInstallAction_Execute_Validation` and `TestGemInstallAction_Decompose_Validation` tables (~195 lines)
- `apply_patch_test.go`: Delete 3 Execute validation duplicates and 4 Decompose validation duplicates already in `Decompose_Errors` table (~70 lines)
- `install_binaries_test.go`: Delete 5 `validateBinaryPath` tests duplicating `TestValidateBinaryPath` table, plus 3 `parseOutputs` tests duplicating `TestParseOutputs` table (~80 lines)
- `npm_install_test.go`: Delete 3 individual Decompose tests duplicating existing table (~45 lines)
- `download_test.go`: Delete `TestDownloadAction_Execute_NoURLParam` (duplicate of `Execute_MissingURL`) and `TestContainsPlaceholder_Direct` (duplicate of table in `preflight_test.go`)
- `preflight_test.go`: Delete `TestRegisteredNames_NotEmpty`, `TestValidateAction_ExistingWithoutPreflight`, `TestPreflightResult_HasWarnings`, `TestPreflightResult_AddWarningf`, `TestPreflightResult_AddErrorf` (all duplicate existing tests)

**Acceptance Criteria:**
- All identified duplicate functions removed
- No test function exists that duplicates a case in an existing table-driven test
- All tests pass after deletion

### Issue 7: Consolidate actions validation tests into tables (~550 lines saved) [DONE]

**Complexity:** testable

**Description:**
Consolidate the repetitive Execute/Decompose validation error tests in `internal/actions/` into table-driven tests. These follow identical patterns: create action + context, call Execute/Decompose with specific params, assert error contains specific substring.

Target files and consolidations:
- `go_build_test.go`: 7 `Execute_Missing*`/`Execute_Invalid*` tests into one table (~100 lines)
- `go_install_test.go`: 6 `Execute_*` validation tests + 5 `Decompose_*` validation tests into two tables (~145 lines)
- `cargo_install_test.go`: 6 `Execute_*` + 4 `Decompose_*` validation tests into two tables (~130 lines)
- `download_file_test.go`: 8 `Execute_*` tests into one or two tables (~100 lines)
- `set_env_test.go`: 4 `ParseVars_*` tests into one table (~45 lines)
- `chmod_test.go`: 4 `Preflight_*` tests into one table (~30 lines)

Table struct pattern for most: `name string, params map[string]any, errContains string`

**Acceptance Criteria:**
- Each group of 3+ identical-structure validation tests is consolidated into a table
- Table cases use descriptive names for `go test -run` filtering
- `t.Parallel()` used in subtests
- All tests pass

### Issue 8: Consolidate actions preflight and misc tests (~145 lines saved) [DONE]

**Complexity:** testable

**Description:**
Consolidate remaining repetitive test patterns in `internal/actions/`:

- `preflight_test.go`: Merge duplicate `TestPreflightResult_ToError` tests, merge `TestRegisteredNames` variants into one test, consolidate download preflight tests (4 funcs) (~125 lines)
- `pipx_install_test.go`: 3 `Execute_*` + 3 `Decompose_*` validation tests into tables (~70 lines)
- `require_system_test.go`: 4 `Preflight_*` tests into one table (~40 lines)
- `run_command_test.go`: 3 `Preflight_*` tests into one table (~25 lines)
- `apply_patch_test.go`: 5 Execute success tests into one table with shared setup (~185 lines saved), merge `NonexistentSubdir` into `PathTraversalSubdir` table

**Acceptance Criteria:**
- All mechanical consolidation opportunities implemented
- All tests pass
- No behavioral changes

### Issue 9: Consolidate tests in verify, discover, builders, and userconfig (~530 lines saved) [DONE]

**Complexity:** testable

**Description:**
Apply table-driven consolidation across non-actions packages:

- `verify/header_test.go`: 5 `ValidateHeader` bad-input cases into table, delete duplicate `ErrorCategory_String_AllTier1` and `ValidationError_Error_AllBranches` tables (~97 lines)
- `verify/external_test.go`: Delete 6 duplicate functions (`ToolType`, `FamilyMismatch`, 4 `getPackagesFromParams` duplicates), merge `isSharedLibraryPath` tables, consolidate nil-result `CheckExternalLibrary` cases (~101 lines)
- `verify/soname_test.go`: 6 `ExtractSoname` error cases into table (~45 lines)
- `verify/integrity_test.go`: Delete duplicate `BrokenSymlink_LstatExists` (~24 lines)
- `verify/rpath_test.go`: 3 `isFatBinaryForRpath` cases into table (~20 lines)
- `verify/index_test.go`: 3 `BuildSonameIndex` zero-size cases into table (~20 lines)
- `discover/registry_test.go`: 4 `ParseRegistry` field validation errors into table (~30 lines)
- `discover/chain_test.go`: 4 `EmitHitEvent` telemetry tests into table (~40 lines)
- `builders/errors_test.go`: 3 `Unwrap` tests + 6 `CheckLLMPrerequisites` cases into tables (~65 lines)
- `userconfig/userconfig_test.go`: 6 default value tests + 4 Set invalid value tests into tables, delete `TestGetLLMBackendDefault` duplicate (~68 lines)

**Acceptance Criteria:**
- All mechanical consolidations implemented
- At least 3 of the judgment-required consolidations implemented
- Duplicate tests deleted
- All tests pass

### Issue 10: Final verification and cleanup [DONE]

**Complexity:** simple

**Description:**
Run the full test suite, verify all tests pass, confirm coverage hasn't dropped below 70%, run `go vet` and linters, clean up `wip/research/` artifacts, and apply `gofmt`/`goimports` to all modified files.

**Acceptance Criteria:**
- `go test ./...` passes with zero failures
- `go vet ./...` reports no issues
- `gofmt -l` reports no unformatted files
- Coverage remains above 70%
- `wip/research/` artifacts removed
- No linter violations introduced
