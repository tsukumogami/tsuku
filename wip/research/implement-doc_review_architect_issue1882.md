# Architect Review: #1882 (test(homebrew): validate library recipe generation against pipeline data)

## Review Scope

Issue #1882 is a validation-only issue: end-to-end tests that run the generator against known library packages (bdw-gc, tree-sitter) and non-library packages, verifying the generated recipe structure and failure subcategory tagging. The changes are test-only, adding four `TestEndToEnd_*` functions in `internal/builders/homebrew_test.go` and one dashboard test case in `internal/dashboard/failures_test.go`.

## Findings

### Finding 1: Tests correctly use the established mock infrastructure
**Severity**: N/A (positive observation)
**Files**: `internal/builders/homebrew_test.go:3458-3920`

The end-to-end tests use `newMockGHCRBottleServer` and `createBottleTarballBytes` -- the same test helpers established in #1879 for `scanMultiplePlatforms` tests. The tests construct `HomebrewBuilder` with an injected `httpClient` that routes GHCR requests through the mock, matching the pattern already used by `checkBottleAvailability` tests and `scanMultiplePlatforms` tests earlier in the same file.

This is the correct integration point. The tests call `generateDeterministicRecipe` (the real dispatch entry point), which internally calls `inspectBottleContents` -> `getGHCRToken` -> `fetchGHCRManifest` -> `downloadBottleBlob` -> `extractBottleContents`. All HTTP calls route through the mock. No production methods are bypassed or replaced with test doubles.

### Finding 2: Tests validate TOML serialization round-trip, matching design doc intent
**Severity**: N/A (positive observation)
**Files**: `internal/builders/homebrew_test.go:3653-3685`, `3804-3835`

Both bdw-gc and tree-sitter tests serialize the generated recipe via `recipe.WriteRecipe()` and then validate the TOML output string for structural markers (`type = "library"`, `install_mode = "directory"`, `outputs =`, absence of `[verify]`, presence of `when`). This validates the full path from bottle inspection through recipe construction to serialization -- the exact pipeline described in the design doc's Phase 4.

### Finding 3: Failure classification tests use classifyDeterministicFailure correctly
**Severity**: N/A (positive observation)
**Files**: `internal/builders/homebrew_test.go:3841-3920`

`TestEndToEnd_NonLibraryPackage_StillFailsComplexArchive` creates a Python-like bottle layout, runs it through `generateDeterministicRecipe` (which correctly fails), then passes the error to `classifyDeterministicFailure` and checks: (a) category is `complex_archive`, (b) the `[library_only]` tag is absent. `TestEndToEnd_LibraryOnly_SubcategoryOnGenerationFailure` validates the inverse: a "library recipe generation failed" error gets the `[library_only]` tag. Both tests go through the actual `HomebrewSession.classifyDeterministicFailure` method, respecting the dispatch chain.

### Finding 4: Dashboard test validates subcategory recognition
**Severity**: N/A (positive observation)
**Files**: `internal/dashboard/failures_test.go:55-60`

The `library_only` subcategory test case is added to the existing `TestExtractSubcategory_bracketedTag` table-driven test. This is the right place -- it validates that `extractSubcategory()` parses the `[library_only]` tag from the error message, which is the contract between `classifyDeterministicFailure` (in `internal/builders/`) and the dashboard (in `internal/dashboard/`). No new test function or parallel pattern introduced.

### Finding 5: No structural concerns
**Severity**: N/A

The changes are test-only and don't introduce any new production code patterns. Specifically:
- No action dispatch bypass: tests call `generateDeterministicRecipe` which goes through the full inspection pipeline.
- No dependency direction violations: test files import only the packages they test (`internal/builders`, `internal/recipe`, `internal/dashboard`).
- No state contract changes: no new fields added to any struct.
- No CLI surface changes.
- No parallel patterns: the `newMockGHCRBottleServer` helper was introduced in #1879 and is reused here, not duplicated.

The only potential concern would be if the end-to-end tests duplicated verification already done by unit tests for `generateLibraryRecipe`, but the value here is different: the E2E tests validate the integration from `generateDeterministicRecipe` (the top-level dispatch) through GHCR download, bottle scanning, library detection, multi-platform scanning, recipe generation, and TOML serialization. Unit tests for `generateLibraryRecipe` (from #1878/#1879) only test the recipe construction step in isolation.

## Overall Assessment

The implementation fits the existing architecture cleanly. Tests use the established mock GHCR infrastructure, go through the production dispatch path, and validate at the right abstraction levels. The dashboard test integrates into existing table-driven test structure. No blocking or advisory findings.
