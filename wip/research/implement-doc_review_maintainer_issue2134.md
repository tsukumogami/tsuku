# Maintainer Review: #2134 (test: cover internal/actions to 66%)

## Summary

This issue added ~250 tests across 16 new `coverage_gap*_test.go` files plus `dependencies_shadow_test.go` to bring `internal/actions` from 57.9% to 66.8% coverage.

## Findings

### 1. File naming obscures test intent (Blocking)

**Files:** `coverage_gap_test.go` through `coverage_gap12_test.go`

The 12 `coverage_gap*` files are named after the *reason* the tests exist (filling coverage gaps) rather than *what they test*. A next developer looking for tests of `homebrew.go` logic has to search across `coverage_gap3_test.go`, `coverage_gap8_test.go`, and `coverage_gap10_test.go` to find them, while also checking the pre-existing `homebrew_test.go`. There's no way to know which file contains tests for a given source file without grepping.

The numbered suffix adds no semantic information. The difference between `coverage_gap8_test.go` and `coverage_gap9_test.go` is just "I ran out of space in the previous file." The next developer will look at these and think they test some specific feature called "coverage gap."

**Recommendation:** Merge these tests into the existing `<action>_test.go` files. The tests are already organized by source file internally (each section has a `// -- source_file.go: function_name --` comment), so they slot naturally into existing test files. If some source files don't have a test file yet, create one with a matching name (e.g., tests for `download_file.go` go in `download_file_test.go`).

### 2. Divergent twins: same behavior tested in two `coverage_gap` files (Blocking)

Several test cases appear in two different `coverage_gap` files testing the same error path with slightly different function names:

- `TestDownloadArchiveAction_Execute_DirectoryModeNoVerify` (coverage_gap8_test.go:15) vs `TestDownloadArchiveAction_Execute_DirectoryModeWithoutVerify` (coverage_gap11_test.go:638) -- both test that directory mode without a verify section returns an error, with identical assertions.
- `TestDownloadArchiveAction_Execute_DirectoryWrappedModeNoVerify` (coverage_gap8_test.go:37) vs `TestDownloadArchiveAction_Execute_DirectoryWrappedModeWithoutVerify` (coverage_gap11_test.go:659) -- same pattern.
- `TestGitHubArchiveAction_Execute_DirectoryModeNoVerify` (coverage_gap8_test.go:92) vs `TestGitHubArchiveAction_Execute_DirectoryModeWithoutVerify` (coverage_gap11_test.go:260) -- same.

The next developer seeing both will wonder if there's a subtle behavioral difference that justifies the duplication. There isn't. Remove the duplicates.

Additionally, there's overlap between `coverage_gap2_test.go` and `coverage_gap7_test.go` for `GitHubArchiveAction_Execute_NoRepo/MissingRepo` and `GitHubFileAction_Execute_NoRepo/MissingRepo` -- the gap2 versions use inline `ExecutionContext` construction and the gap7 versions use `newTestExecCtx`. They test the same error path.

### 3. `newTestExecCtx` helper defined but not used consistently (Advisory)

`coverage_gap7_test.go` defines `newTestExecCtx(t)` -- a useful helper that creates a standard `ExecutionContext` for tests. However, `coverage_gap8_test.go` through `coverage_gap12_test.go` sometimes use it and sometimes construct `ExecutionContext` inline with the exact same field values. This inconsistency makes the next developer unsure whether the inline contexts have intentional differences (they mostly don't).

**Recommendation:** Use `newTestExecCtx` consistently, or don't define it. If specific tests need different field values (e.g., `OS: "windows"`), call the helper and override the field.

### 4. Section comments are the only organization (Advisory)

The `// -- source_file.go: function_name --` comment convention is the sole mechanism for navigating within each file. This works while the tests are fresh, but nothing enforces the grouping. As tests get added over time, the comments will drift from reality. This is a natural consequence of the `coverage_gap*.go` structure (finding #1) -- once tests are in per-source-file test files, the section comments become unnecessary.

### 5. Tests that exercise code paths without asserting results (Advisory)

Several tests intentionally trigger errors at network/build stages to exercise code paths, then either ignore the error or use `_ = err`:

- `coverage_gap2_test.go:198`: `_ = err` after `ConfigureMakeAction.Execute` -- the comment explains this is intentional, but `_ = err` reads like forgotten error handling.
- `coverage_gap12_test.go:359`: `_ = err` for `AptInstallAction.Execute` -- same pattern.

These are coverage-motivated tests that don't verify behavior, just execution. This is fine for coverage metrics, but the next developer may think they're real tests and try to add meaningful assertions (or delete them as incomplete). A brief comment like `// Coverage: exercising the skip_configure code path; actual build isn't expected to succeed` would help.

### 6. `TestCpanInstall_Min` tests a builtin (Advisory)

`coverage_gap10_test.go:407` tests the `min` function. In Go 1.21+, `min` is a builtin. If this is a custom `min` function in `cpan_install.go`, the test name should reflect which one it's testing. If it's the builtin, the test is pointless. Worth a quick check.

### 7. Good: `dependencies_shadow_test.go` is well-structured

Unlike the coverage_gap files, `dependencies_shadow_test.go` is named after the feature it tests, has comprehensive coverage of all dependency source combinations, and each test name clearly describes the scenario. This is the pattern the other tests should follow.

## Overall Assessment

The tests themselves are well-written -- good use of `t.Parallel()`, `t.TempDir()`, table-driven tests where appropriate, and meaningful assertions. The test logic is sound and the coverage gains are real.

The blocking issue is organizational: 12 numbered `coverage_gap*` files scatter tests for the same source files across multiple locations, and some tests are duplicated between files. This will slow down every future developer who needs to find or modify tests for a specific action. The fix is mechanical: merge tests into per-source-file test files and remove duplicates.
