# Architect Review: #2134 (test: cover internal/actions to 66%)

## Scope

16 new test files added to `internal/actions/`: `coverage_gap_test.go` through `coverage_gap12_test.go`, `dependencies_shadow_test.go`, and `extract_formats_test.go`. All files are test-only (`_test.go`). No production code was modified.

## Findings

### 1. Parallel naming pattern: `coverage_gap{N}_test.go` files diverge from established convention

**Severity: Advisory**

The existing codebase names test files after the source file they test: `chmod.go` -> `chmod_test.go`, `download.go` -> `download_test.go`, `composites.go` -> `composites_test.go`. The new `coverage_gap{N}_test.go` files break this convention by grouping tests from multiple source files into arbitrarily numbered files.

For example, `coverage_gap_test.go` contains tests for `configure_make.go`, `decomposable.go`, `download_cache.go`, `preflight.go`, `composites.go`, `download.go`, `apply_patch.go`, `extract.go`, `gem_common.go`, and `fossil_archive.go` -- all in one file. `coverage_gap3_test.go` similarly mixes tests for `cargo_build.go`, `go_build.go`, `gem_exec.go`, `install_gem_direct.go`, `homebrew.go`, `homebrew_relocate.go`, `eval_deps.go`, and `set_rpath.go`.

This means when a contributor modifies `homebrew.go`, they need to look in both `homebrew_test.go` (existing) AND `coverage_gap3_test.go` to find all tests. There's no discoverability from the filename. The numbering is sequential with no semantic meaning -- `coverage_gap7_test.go` doesn't correspond to any particular source file.

This doesn't compound in the usual sense (nobody will add new production code and name its test file `coverage_gap13_test.go`), but it does make the test suite harder to navigate. The better approach would have been to append these tests to the existing `*_test.go` files or create new ones following the `{source_file}_test.go` convention.

Not blocking because the tests are functionally correct and contained -- reorganizing them later requires no changes to production code.

### 2. `dependencies_shadow_test.go` follows the naming convention correctly

No finding -- this file tests `DetectShadowedDeps` which lives in the `dependencies.go` family of code. The naming is descriptive and discoverable.

### 3. Test helper `newTestExecCtx` defined locally but `ExecutionContext{}` is constructed inline 417 times across 48 test files

**Severity: Advisory**

`coverage_gap7_test.go` defines `newTestExecCtx()` -- a test helper that creates a minimal `ExecutionContext`. This is used only within that file, while every other test file constructs `ExecutionContext{}` inline. The helper is a good idea but introducing it in a single `coverage_gap` file means it won't be discovered or adopted by other test writers.

If this helper were in a shared file like `test_helpers_test.go`, it could reduce boilerplate across the package. As-is, it's contained and doesn't create a structural problem.

### 4. `buildTarGz` helper defined in `coverage_gap2_test.go`

**Severity: Advisory**

A reusable test helper (`buildTarGz`) that creates tar.gz archives is defined in `coverage_gap2_test.go`. This helper would be useful to other test files (e.g., `extract_test.go`, `extract_formats_test.go`) but is hidden behind the `coverage_gap` naming. No duplication exists today -- it's only defined once -- but its discoverability is poor.

### 5. No production code changes -- no action dispatch, state contract, or dependency direction concerns

The change is test-only. All tests instantiate action types directly (e.g., `&ConfigureMakeAction{}`, `&DownloadArchiveAction{}`), which is the correct pattern for unit testing action implementations. Tests exercise the public API of each action (`Execute`, `Preflight`, `Name`, `IsDeterministic`, `Dependencies`, `RequiresNetwork`) without bypassing the registry or introducing new abstractions.

No new packages, no new interfaces, no import direction violations. The `dependencies_shadow_test.go` file imports `internal/recipe` to construct test fixtures, which follows the existing dependency direction (actions depend on recipe, not the reverse).

## Summary

No blocking architectural issues. The tests are structurally sound -- they test the right interfaces, follow the right dependency direction, and don't modify production code. The `coverage_gap{N}` naming convention is the only structural concern: it introduces a parallel file organization pattern that makes tests harder to locate. This is an advisory finding because it doesn't compound (future test authors will follow the established `{source}_test.go` convention, not the `coverage_gap` pattern) and reorganizing later is a mechanical refactor with no production code impact.
