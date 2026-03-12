# Pragmatic Review: Issue #2134 (test: cover internal/actions to 66%)

## Summary

~250 tests across 16 new files. Test-only changes, no production code modified. Tests are straightforward and match the issue scope.

## Findings

### 1. BLOCKING: Race condition on TSUKU_HOME in util_resolve_test.go

**File:** `internal/actions/util_resolve_test.go`
**Lines:** All 15 Resolve* tests (lines ~126-680)

Every test in this file calls `t.Parallel()` and then mutates the process-wide `TSUKU_HOME` environment variable via `os.Setenv`. Environment variables are shared across all goroutines in a process. When these tests run concurrently, each test's `os.Setenv("TSUKU_HOME", tmpDir)` clobbers the value another parallel test just set, and the deferred restore can restore a stale value.

This will produce flaky test results. Under `-race`, this will be flagged as a data race.

**Fix:** Either (a) remove `t.Parallel()` from all tests in this file, or (b) use `t.Setenv("TSUKU_HOME", tmpDir)` which is available since Go 1.17 and automatically prevents `t.Parallel()` misuse (it panics if called on a parallel test, forcing the correct design).

The cleanest approach: drop `t.Parallel()` from these tests and use `t.Setenv` for automatic cleanup. If parallelism within this file matters for speed, restructure the Resolve functions to accept a base path parameter rather than reading from the environment.

### 2. ADVISORY: Swallowed errors in coverage_gap2_test.go and coverage_gap12_test.go

**Files:**
- `internal/actions/coverage_gap2_test.go:198` -- `_ = err` after `ConfigureMakeAction.Execute`
- `internal/actions/coverage_gap12_test.go:359` -- `_ = err` after `AptInstallAction.Execute`

These tests exercise code paths but discard the error entirely without any assertion. The comment says "we're just exercising the code path" but this means the test can't detect regressions. If the function starts panicking or returning a different class of error, the test won't catch it.

**Fix:** At minimum assert `err != nil` (since both are expected to fail) to verify the function returns an error rather than panicking.

### 3. ADVISORY: Dead import verification in extract_formats_test.go

**File:** `internal/actions/extract_formats_test.go`
**Lines:** 129, 215, 754

Lines like `_ = bzip2.NewReader(bytes.NewReader([]byte{}))` and `_ = &lzip.Reader{}` and `var _ io.Reader = (*lzip.Reader)(nil)` exist solely to keep imports alive. The comment in the bz2 test acknowledges Go lacks a bzip2 writer, so the test can only test error paths. The import-verification lines add no coverage value.

**Fix:** These are harmless but noisy. Consider removing the dead lines since the imports are already used by the error-path tests above them. If the import is truly only needed for the type assertion, the test function that exercises the error path already uses it.

### 4. ADVISORY: File naming convention (coverage_gap{N}_test.go)

16 files but only 4 have descriptive names (`dependencies_shadow_test.go`, `extract_formats_test.go`, `system_config_validate_test.go`, `util_resolve_test.go`). The remaining 12 are `coverage_gap{1-12}_test.go`. The numbered names make it hard to find tests for a specific function -- you have to grep rather than navigate by filename.

This is a naming concern, which is out of scope for pragmatic review. Noting only because the existing test files in the package use descriptive names per source file (e.g., `chmod_test.go`, `download_test.go`).
