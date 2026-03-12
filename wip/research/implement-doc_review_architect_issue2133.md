# Architect Review: #2133 (test: cover internal/discover and internal/verify to 75%)

## Summary

This issue adds ~80 unit tests across 6 new `*_coverage_test.go` files to bring `internal/discover` and `internal/verify` above 75% coverage. All changes are test-only; no production code was modified.

## Findings

### Advisory: Parallel test file naming convention

**Files:** All 6 new files (`chain_coverage_test.go`, `coverage2_test.go`, `deps_coverage_test.go`, `dltest_coverage_test.go`, `external_coverage_test.go`, `header_coverage_test.go`)

The new files use a `*_coverage_test.go` naming pattern that doesn't exist elsewhere in the codebase. Existing tests are named after the source file they test (e.g., `chain_test.go` tests `chain.go`, `deps_test.go` tests `deps.go`). Adding separate `_coverage` files creates a second organizational pattern for tests in the same packages.

This is structurally contained -- Go treats all `_test.go` files in a package equally, so the tests compile and run identically regardless of which file they're in. The naming convention doesn't create a dispatch or registration problem. However, it may encourage future contributors to create `_coverage_test.go` files instead of adding tests to the existing test file for the relevant source file. For example, `header_coverage_test.go` tests functions from `header.go` that could have been added to `header_test.go`.

**Impact:** Low. The files could be merged into existing test files, but splitting won't cause divergence in behavior or imports. **Advisory.**

### No blocking findings

The tests:

1. **Reuse existing test infrastructure.** `mockRecipeLoader`, `mockActionLookup`, `mockResolver`, and `mockProber` are all defined in existing test files (`deps_test.go`, `chain_test.go`, `ecosystem_probe_test.go`) and reused by the new coverage files. No mock duplication.

2. **Don't introduce new dependencies or imports.** All imports are from packages already used by the existing tests in each package. No new cross-package dependencies.

3. **Don't bypass architectural boundaries.** Tests call exported and unexported functions within the same package (standard Go white-box testing). No action dispatch bypass, no provider inline instantiation, no state contract changes.

4. **Add one new helper (`testLogger`) in `chain_coverage_test.go`.** This creates a `log.Logger` backed by a `bytes.Buffer` for output verification. It's specific to the logging coverage tests and doesn't duplicate any existing helper.

5. **Don't modify CLI surface, state contract, or template interpolation.** Pure test additions with zero production code changes.

6. **`containsStr` helper in `dltest_coverage_test.go`** reimplements `strings.Contains` without importing `strings`. This is a style observation, not a structural concern -- it's contained to one test file and doesn't affect architecture.
