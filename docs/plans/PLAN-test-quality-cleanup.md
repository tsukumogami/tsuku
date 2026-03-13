---
schema: plan/v1
status: Done
execution_mode: single-pr
issue_count: 5
---

# PLAN: Test Quality Cleanup

## Status

Done

## Scope Summary

Clean up test files added by the code-coverage-75 effort. Redistribute tests from numbered `coverage_gap*` grab-bag files into canonical `_test.go` files, delete counterproductive tests that can never fail or test stdlib behavior, and consolidate repetitive test functions into table-driven tests.

## Decomposition Strategy

**Sequential decomposition.** Each issue builds on the previous one: redistribution first (moves tests to correct files), deletion second (removes waste), then consolidation (merges repetitive tests). The final two issues handle non-gap-file improvements and verification.

## Issue Outlines

### Issue 1: Redistribute coverage_gap tests into canonical files

**Complexity:** testable

**Description:**
Move all test functions from `internal/actions/coverage_gap_test.go` through `coverage_gap12_test.go` (12 files, 379 functions) into the `_test.go` file corresponding to each test's source file. For example, `TestConfigureMakeAction_Preflight` moves from `coverage_gap_test.go` to `configure_make_test.go`. Delete the empty coverage_gap files after redistribution. Also rename `*_coverage_test.go` files in `internal/verify/` and `internal/discover/` to match their source files (e.g., `header_coverage_test.go` content merges into `header_test.go`).

**Acceptance Criteria:**
- No files named `coverage_gap*` remain in the codebase
- No files with `_coverage_` in the name remain in `internal/actions/`
- Every test function lives in the `_test.go` file named after its source file
- All tests pass after redistribution
- No test logic is changed, only file locations

### Issue 2: Delete counterproductive and redundant tests

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

### Issue 3: Consolidate repetitive tests into table-driven tests (actions package)

**Complexity:** testable

**Description:**
Within `internal/actions/`, identify groups of test functions that test the same function/method with different inputs and share identical structure. Merge them into table-driven tests with `t.Run` subtests. Focus on the patterns identified in the migration plan: preflight validation groups, decompose error path groups, and Execute error path groups that repeat the same setup-call-assert pattern with different parameters.

**Acceptance Criteria:**
- Groups of 3+ near-identical test functions are consolidated into table-driven tests
- Each table test uses descriptive case names in `t.Run`
- All tests pass after consolidation
- No behavioral changes to what is tested

### Issue 4: Consolidate repetitive tests in non-actions packages

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

### Issue 5: Verify test suite health

**Complexity:** simple

**Description:**
Run the full test suite, verify all tests pass, check that coverage hasn't dropped below 73% (acceptable regression from deleting low-value tests), and confirm no linter violations. Run `go vet`, the full linter suite, and verify the build succeeds.

**Acceptance Criteria:**
- `go test ./...` passes with zero failures
- `go vet ./...` reports no issues
- Coverage remains above 73%
- No linter violations introduced
