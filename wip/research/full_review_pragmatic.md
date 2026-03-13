# Pragmatic Review: PR #2143 (code-coverage-75)

## Summary Verdict

**~60% coverage-only, ~30% high-value, ~10% counterproductive.** Net negative in maintenance burden relative to safety gained.

---

## Structural Issues

### 1. File naming screams "coverage farming" -- Blocking

12 files named `coverage_gap_test.go` through `coverage_gap12_test.go` containing 379 test functions. These files are organized by "what's uncovered" rather than "what behavior needs protecting." This makes them impossible to maintain: when production code changes, there's no logical connection between the gap file and the feature it tests.

**Fix:** Tests should live in files named after the behavior they protect (e.g., `download_preflight_test.go`, `extract_security_test.go`), or be added to the existing `*_test.go` file for the corresponding production file.

### 2. Swallowed errors (`_ = err`) -- Blocking

Five instances across test files where `err` is explicitly discarded:

- `coverage_gap2_test.go:198` -- `TestConfigureMakeAction_Execute_SkipConfigure` calls `Execute()` and discards the error. Comment says "we're just exercising the code path." This test can never fail. It exists solely to make coverage green.
- `coverage_gap12_test.go:359` -- `TestAptInstallAction_Execute_WithPackages` same pattern: `_ = err` with comment "Either way, we're exercising the code path past parameter validation."
- `coverage_gap10_test.go:505` -- `_ = hasRedundantWarning` with comment "May or may not have this warning." A test that doesn't know what the correct behavior is.

**Fix:** Either assert the expected error, or delete the test. A test that can't fail is dead code that falsely inflates confidence.

### 3. Trivial getter/property tests -- Advisory

~11 `IsDeterministic` tests across coverage_gap files that verify a method returns a hardcoded boolean. These methods are typically one-liners like `func (a *FooAction) IsDeterministic() bool { return true }`. They can't meaningfully break independently -- if someone changes the return value, they'd change the test too.

Similarly: `TestExtractAction_Name` (line 291 of coverage_gap_test.go) verifies `Name()` returns `"extract"`. This is testing Go's ability to return a string literal.

**Fix:** Delete IsDeterministic and Name tests from coverage gap files. They're already covered by `TestIsDeterministic` in `decomposable_test.go` which tests all actions in a table-driven loop.

### 4. Duplicate test coverage -- Blocking

Several tests in `coverage_gap10_test.go` duplicate tests already present in `coverage_gap_test.go`:

- `TestGitHubFileAction_Preflight_MissingRepo` (gap_test.go) vs `TestGitHubFileAction_Preflight_NoRepo` (gap10_test.go) -- same test, different names
- `TestDownloadArchiveAction_Preflight_RedundantFormat` (gap_test.go) vs `TestDownloadArchiveAction_Preflight_RedundantArchiveFormat` (gap10_test.go) -- one asserts warnings, the other discards the result with `_ = hasRedundantWarning`
- `TestGitHubArchiveAction_Preflight_MissingRepo` appears in gap10_test.go when already covered in gap_test.go

**Fix:** Deduplicate. Use the version with real assertions.

### 5. Tests that validate stdlib behavior -- Coverage-only

- `TestCpanInstall_Min` (coverage_gap10_test.go:407-418): Tests Go's built-in `min()` function with 3 cases. This is testing the Go runtime, not your code.
- `TestComputeSHA256_NonexistentFile`: Verifies `os.Open` fails on a nonexistent file.
- `TestCopyFile_NonexistentSource`: Same pattern.

**Fix:** Delete tests of stdlib behavior. Keep only if the function wraps stdlib with meaningful logic.

---

## What's Actually High-Value

These tests genuinely protect real behavior and would catch regressions:

1. **Preflight validation tests** (ConfigureMake, ApplyPatch, Download) -- These test input validation rules with table-driven cases. Good regression safety for the recipe validation pipeline.

2. **Security boundary tests** in coverage_gap10_test.go:
   - `TestIsPathWithinDirectory_PartialMatch` -- catches `/tmp/foobar` matching `/tmp/foo` (real security bug)
   - `TestValidateSymlinkTarget_Absolute` / `TestValidateSymlinkTarget_Escape` -- zip slip protection
   - `TestCargoBuildAction_Execute_WithPathTraversalFeature` -- validates feature name sanitization

3. **Download cache round-trip tests** (gap5_test.go): `TestDownloadCache_Invalidate`, `TestDownloadCache_SaveAndCheckNoChecksum` -- tests real cache invalidation behavior with filesystem state.

4. **Binary format detection tests** (coverage_gap7_test.go): Mach-O magic number variants. These protect cross-platform binary detection logic that has real edge cases.

5. **Extract with strip_dirs and file filtering** (coverage_gap2_test.go): Tests actual tar extraction behavior with real archives. Would catch extraction regressions.

6. **Recipe coverage analysis** (recipe/coverage_test.go): Tests the when-clause platform coverage analyzer. Clear behavior, clear assertions, useful for the recipe validation pipeline.

7. **CopyDirectoryExcluding with symlinks** (coverage_gap8_test.go): Tests real filesystem operations including symlink preservation.

8. **Decompose error paths** for composite actions: Missing required params in GitHubArchive, GitHubFile, DownloadArchive. These protect the decomposition pipeline's error handling.

---

## What's Counterproductive

1. **Tests that pass on both success and failure** (5 instances of `_ = err`): These create false coverage. If someone introduces a bug that changes the error type, these tests still pass. Worse, they appear in coverage reports as "tested," discouraging someone from writing a real test later.

2. **Stub action tests** (`TestAptRepoAction_Execute_Stub`, `TestAptPPAAction_Execute_Stub`, `TestDnfRepoAction_Execute_Stub`): These test that stub functions return nil. When the stubs get real implementations, these tests will silently pass or fail unpredictably.

3. **Tests that hit network by design** (`TestDownloadFileAction_Execute_DownloadFailure` and variants): These call `Execute` with `nonexistent.invalid` URLs. They're testing that DNS resolution fails, not that the action handles failures correctly. They'll break or behave differently in environments with DNS interception.

---

## Scorecard

| Category | Count | % |
|----------|-------|---|
| High-value (real behavior, would catch regression) | ~110 | ~29% |
| Coverage-only (exercises code, can't fail meaningfully) | ~230 | ~61% |
| Counterproductive (masks bugs, fragile, or misleading) | ~39 | ~10% |

## Overall Assessment

The PR achieves its stated goal (75% coverage) but approximately two-thirds of the tests are coverage theater. The high-value tests (~110) are genuinely useful and should be kept. The rest add maintenance burden -- they'll break on refactors, require updating for no safety benefit, and make the test suite slower.

**Recommendation:** Keep the high-value tests. Delete the `_ = err` tests, stub tests, IsDeterministic tests (already covered elsewhere), stdlib behavior tests, and duplicates. Reorganize remaining tests out of `coverage_gap*` files into behavior-oriented test files. This would likely land at ~60% coverage with ~100% of the tests being meaningful.
