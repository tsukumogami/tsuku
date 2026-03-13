# Architect Review: coverage_gap Test Files

## Summary Verdict

**Blocking on organization. The tests bypass the codebase's established test-file-per-source-file pattern, creating a parallel structure that will diverge.** The test logic itself is mostly sound, but the file organization violates the project's existing convention and introduces a naming pattern that other contributors will either copy (compounding the problem) or ignore (creating inconsistency).

---

## Structural Findings

### 1. Parallel test organization pattern -- Blocking

The existing codebase follows a strict convention: each source file `foo.go` has a corresponding `foo_test.go`. The `internal/actions/` directory already contains 30+ properly-named test files: `download_test.go`, `extract_test.go`, `chmod_test.go`, `composites_test.go`, `cargo_build_test.go`, etc.

The 12 `coverage_gap{N}_test.go` files introduce a second organizational pattern: tests grouped by "what coverage tool said was missing" rather than by production module. This is visible in the files themselves -- each uses inline comments like `// -- configure_make.go: Preflight --` and `// -- download_cache.go: cacheKey and cachePaths --` to label which source file the tests belong to.

A single `coverage_gap_test.go` contains tests for `configure_make.go`, `decomposable.go`, `download_cache.go`, `preflight.go`, `composites.go`, `download.go`, `apply_patch.go`, `extract.go`, `gem_common.go`, and `fossil_archive.go`. That is 10 production files tested from one grab-bag file.

**Why this compounds**: When someone modifies `configure_make.go`, they will check `configure_make_test.go` (which exists). They won't know that `coverage_gap_test.go`, `coverage_gap2_test.go`, `coverage_gap5_test.go`, `coverage_gap6_test.go`, and `coverage_gap9_test.go` also contain ConfigureMake tests. Those tests will break silently in CI with no obvious connection to the change. Worse, if this pattern is accepted, future coverage work will add `coverage_gap13_test.go` and beyond.

The same pattern appears in `internal/verify/` (5 coverage-named files) and `internal/discover/` (1 coverage-named file), though with slightly better naming (e.g., `header_coverage_test.go` at least hints at the module).

**Quantified fragmentation**:

| Source file | Test files containing its tests |
|---|---|
| `configure_make.go` | `configure_make_test.go`, `coverage_gap_test.go`, `coverage_gap2_test.go`, `coverage_gap4_test.go`, `coverage_gap9_test.go` |
| `utils.go` | `utils_test.go`, `coverage_gap4_test.go`, `coverage_gap5_test.go`, `coverage_gap8_test.go` |
| `install_binaries.go` | `install_binaries_test.go`, `coverage_gap6_test.go`, `coverage_gap7_test.go`, `coverage_gap8_test.go`, `coverage_gap11_test.go` |
| `composites.go` | `composites_test.go`, `coverage_gap_test.go`, `coverage_gap2_test.go`, `coverage_gap5_test.go`, `coverage_gap7_test.go`, `coverage_gap8_test.go`, `coverage_gap9_test.go`, `coverage_gap10_test.go`, `coverage_gap11_test.go` |
| `download.go` | `download_test.go`, `coverage_gap2_test.go`, `coverage_gap4_test.go`, `coverage_gap5_test.go`, `coverage_gap7_test.go`, `coverage_gap8_test.go`, `coverage_gap10_test.go`, `coverage_gap12_test.go` |

**Fix**: Redistribute every test function into the `_test.go` file for its corresponding source file. The inline comments already document the mapping.

### 2. Test duplication across coverage_gap files -- Blocking

Several tests appear in multiple coverage_gap files testing the same behavior:

- `TestGitHubFileAction_Preflight_MissingRepo` (coverage_gap_test.go:294) vs `TestGitHubFileAction_Preflight_NoRepo` (coverage_gap10_test.go:448) -- same assertion, different name
- `TestGitHubArchiveAction_Preflight_MissingRepo` appears in coverage_gap5_test.go:376 and coverage_gap10_test.go:434
- `TestDownloadArchiveAction_Preflight_RedundantFormat` (coverage_gap_test.go:408) vs `TestDownloadArchiveAction_Preflight_RedundantArchiveFormat` (coverage_gap10_test.go:486) -- the latter discards the result with `_ = hasRedundantWarning`
- `TestFossilArchiveAction_Execute_MissingParams` (coverage_gap_test.go:378) vs `TestFossilArchiveAction_Execute_MissingProjectName` (coverage_gap7_test.go:363) and `TestFossilArchiveAction_Execute_MissingRepo` (coverage_gap11_test.go:899) -- same error paths, fragmented across 3 files
- `TestGitHubArchiveAction_Execute_MissingRepo` appears in both coverage_gap2_test.go:59 and coverage_gap7_test.go:476
- `TestGitHubFileAction_Execute_NoRepo` (coverage_gap2_test.go:103) vs `TestGitHubFileAction_Execute_MissingRepo` (coverage_gap7_test.go:546)
- `TestInstallBinariesAction_ValidateBinaryPath_*` tests appear in both coverage_gap7_test.go and coverage_gap11_test.go
- `TestRegisteredNames_NotEmpty` (coverage_gap_test.go:157) duplicates `TestRegisteredNames_Sorted` (coverage_gap7_test.go:808)

This duplication is a direct consequence of the grab-bag file pattern. When tests live in the corresponding source test file, you see at a glance what's already covered.

**Fix**: Deduplicate during redistribution. Keep the version with stronger assertions.

### 3. IsDeterministic tests duplicate existing table-driven coverage -- Advisory

`decomposable_test.go` already contains `TestIsDeterministic` which tests all actions in a table-driven loop. The coverage_gap files add 11 individual `TestFooAction_IsDeterministic` functions: `TestDownloadArchiveAction_IsDeterministic`, `TestGitHubArchiveAction_IsDeterministic`, `TestGitHubFileAction_IsDeterministic`, `TestFossilArchiveAction_IsDeterministic_Direct`, `TestDownloadAction_IsDeterministic`, `TestApplyPatchAction_IsDeterministic`, `TestExtractAction_IsDeterministic`, `TestHomebrewAction_IsDeterministic_Direct`, `TestHomebrewRelocateAction_IsDeterministic`, `TestSetRpathAction_IsDeterministic_Direct`, `TestInstallGemDirectAction_IsDeterministic`.

These are redundant with the existing pattern. They test that a one-liner returns a hardcoded boolean.

**Fix**: Delete them. If the table-driven test in `decomposable_test.go` doesn't cover a new action, add it to the table.

### 4. Tests that can't fail (`_ = err`) -- Blocking

Instances where error results are explicitly discarded:

| Location | Pattern |
|----------|---------|
| `coverage_gap2_test.go:198` | `_ = err` on ConfigureMake Execute with comment "we're just exercising the code path" |
| `coverage_gap10_test.go:505` | `_ = hasRedundantWarning` with comment "May or may not have this warning" |
| `coverage_gap12_test.go:359` | `_ = err` on AptInstall Execute with comment "Either way, we're exercising the code path" |

These exist solely to execute code paths without asserting anything. From an architectural perspective, they create false coverage signals: a later developer sees 75% coverage on a function and assumes it's tested, when the "test" would pass even if the function panicked. This breaks the contract between the coverage metric and actual test safety.

**Fix**: Either assert the expected error or delete the test.

### 5. Tests that depend on DNS resolution -- Advisory

`TestDownloadFileAction_Execute_DownloadFailure` (coverage_gap12_test.go:139) and ~15 other `Execute` tests use `nonexistent.invalid` URLs. These rely on DNS failing for `.invalid` TLD (RFC 6761). While `.invalid` is reserved, corporate DNS resolvers with wildcard records can intercept it.

This is advisory because the tests still assert `err != nil` (they don't check error type), so they'll pass in most environments. But they're testing "does HTTP fail when DNS fails" rather than "does the action validate its inputs correctly."

### 6. Stub action tests -- Advisory

`TestAptRepoAction_Execute_Stub`, `TestAptPPAAction_Execute_Stub`, `TestDnfRepoAction_Execute_Stub` (coverage_gap12_test.go) test that stub functions return nil. When these stubs get real implementations, the tests will silently pass or fail unpredictably depending on system state.

### 7. Testing stdlib behavior -- Advisory

`TestCpanInstall_Min` (coverage_gap10_test.go:407) tests Go's built-in `min()` function. `TestComputeSHA256_NonexistentFile` and `TestCopyFile_NonexistentSource` (coverage_gap5_test.go) test that `os.Open` fails on missing files. Not architecturally harmful but adds noise.

---

## What fits the architecture well

Several test categories genuinely test action behavior through the public interface (`Preflight`, `Execute`, `Decompose`) and would be valuable if placed in the correct files:

1. **Preflight validation tests** -- table-driven tests for ConfigureMake, ApplyPatch, Download, GitHubArchive, GitHubFile, DownloadArchive, Chmod, RunCommand, RequireSystem, InstallBinaries, AppBundle. These test the recipe validation pipeline through the action interface. High value.

2. **Decompose error path tests** -- missing required params in GitHubArchive, GitHubFile, DownloadArchive, ApplyPatch, FossilArchive, NpmInstall, GemInstall, CargoInstall, PipxInstall, NixInstall. These protect the decomposition pipeline. High value.

3. **Security boundary tests** -- `TestIsPathWithinDirectory_PartialMatch`, `TestValidateSymlinkTarget_*`, `TestCargoBuildAction_Execute_WithPathTraversalFeature`, `TestValidateBinaryPath_*`, `TestValidateRpath_*`, `TestValidateBinaryName_*`, `TestGemInstallAction_Execute_ControlCharInExecutable`, `TestGemInstallAction_Execute_ShellMetacharInExecutable`. These catch real security bugs and should absolutely be kept.

4. **Download cache round-trip tests** -- Save/Check/Invalidate/Clear/Info lifecycle. Real filesystem state management with proper assertions.

5. **Binary format detection** -- Mach-O magic number variants. Protects cross-platform binary detection edge cases.

6. **Build environment construction** -- `TestBuildAutotoolsEnv_*`, `TestBuildDeterministicCargoEnv_*`. Tests that dependency paths are correctly wired into environment variables.

---

## Dependency Direction Assessment

All tests are in the `actions` package (same package as production code, using `package actions` not `package actions_test`). This means they access unexported functions directly: `cacheKey`, `cachePaths`, `writeMeta`, `readMeta`, `buildAutotoolsEnv`, `buildDeterministicCargoEnv`, `touchAutogeneratedFiles`, `findMake`, `linkCargoRegistryCache`, `createGemWrapper`, `formulaToGHCRPath`, `ghcrHTTPClient`, `extractBundlerVersion`, `fixPythonShebang`, `countRequirementsPackages`, `isValidPyPIPackage`, `containsPlaceholder`, `detectBinaryFormat`, `validatePathWithinDir`, `validateRpath`, `validateBinaryName`, `createLibraryWrapper`, `isPathWithinDirectory`, `validateSymlinkTarget`, `extractSourceFiles`, `checkEvalDepsInDir`, `computeFileSHA256`, `findLibraryDirectories`, `buildRpathFromLibDirs`, `resolveAssetName`.

Some of these (security validators like `validateRpath`, `validateBinaryName`, `isPathWithinDirectory`) are correctly tested as unit functions -- they have complex edge cases that deserve direct testing. Others (like `cacheKey`, `cachePaths`, `ghcrHTTPClient`) are implementation details that would break on any internal refactoring.

**This is advisory, not blocking.** The existing test files in this package already use the same `package actions` pattern, so this is consistent with established convention.

---

## Per-File Recommendations

| File | Recommendation | Rationale |
|------|---------------|-----------|
| `coverage_gap_test.go` | **Rework**: redistribute to 10 source test files | Contains tests for 10 different source files |
| `coverage_gap2_test.go` | **Rework**: redistribute; delete `_ = err` on line 198 | Good extract tests, bad ConfigureMake test |
| `coverage_gap3_test.go` | **Rework**: redistribute | Dependencies/RequiresNetwork tests, linkCargoRegistryCache |
| `coverage_gap4_test.go` | **Rework**: redistribute | buildAutotoolsEnv, buildDeterministicCargoEnv, CopyDirectory, Decompose tests |
| `coverage_gap5_test.go` | **Rework**: redistribute; delete stdlib tests | Download cache tests (high value), stdlib tests (no value) |
| `coverage_gap6_test.go` | **Rework**: redistribute | Preflight tests, mostly high value |
| `coverage_gap7_test.go` | **Rework**: redistribute; deduplicate Execute param tests; keep Mach-O tests | Binary format detection valuable; many duplicates with gap2 |
| `coverage_gap8_test.go` | **Rework**: redistribute; keep filesystem tests | CopyDirectoryExcluding, CopySymlink, CopyFile are high value |
| `coverage_gap9_test.go` | **Rework**: redistribute | Execute validation, Decompose tests, mostly high value |
| `coverage_gap10_test.go` | **Rework**: redistribute; delete `_ = hasRedundantWarning`; delete `TestCpanInstall_Min`; deduplicate | Security tests high value, duplicates and stdlib no value |
| `coverage_gap11_test.go` | **Rework**: redistribute; deduplicate validateBinaryPath tests | Many duplicates with gap7 |
| `coverage_gap12_test.go` | **Rework**: redistribute; delete stub tests and `_ = err` test | Lowest value file -- mostly stubs and network-dependent tests |
| `recipe/coverage_test.go` | **Keep**: rename to match source file or merge into existing test file | Tests a specific feature, naming is the only issue |
| `verify/coverage2_test.go` | **Rework**: redistribute to corresponding verify test files | Grab-bag for verify internals |
| `verify/header_coverage_test.go` | **Advisory**: rename to match source file | Better than gap files but still "coverage" framed |
| `verify/deps_coverage_test.go` | **Advisory**: rename to match source file | Same |
| `verify/dltest_coverage_test.go` | **Advisory**: rename to match source file | Same |
| `verify/external_coverage_test.go` | **Advisory**: rename to match source file | Same |
| `discover/chain_coverage_test.go` | **Advisory**: rename to match source file | Same |

---

## Overall Assessment

The test code is structurally sound at the function level -- it uses the right testing patterns (`t.Parallel`, `t.TempDir`, table-driven tests, `t.Helper`) and mostly tests through stable interfaces (`Preflight`, `Execute`, `Decompose`). The architectural problem is entirely organizational: 12 grab-bag files that introduce a parallel naming pattern and hide tests from the developers who need to maintain them.

The PR does not introduce other architectural violations (no action dispatch bypass, no provider inline instantiation, no state contract drift). The problem is purely a "parallel pattern" violation -- two locations for the same concern.

**Recommendation**: Redistribute all tests into their corresponding `_test.go` files, deduplicate, delete the `_ = err` tests and the stdlib tests, then delete all `coverage_gap*.go` files. This preserves the ~70% of tests that are genuinely valuable while eliminating the organizational debt. Block on it because the pattern will be copied by the next coverage push if left in place.
