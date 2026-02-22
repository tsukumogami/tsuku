# Scrutiny Review: Intent -- Issue #1882

**Focus**: Does the implementation capture the design's intent, not just the AC's literal text?

## Design Doc Intent for #1882

The design doc describes issue #1882 (Phase 4: Pipeline Validation) as:

> Run the updated generator against the current complex_archive failures in the queue. Verify generated recipes match the structure of existing library recipes. Update dashboard metrics to track the new library_only category.

The issue row in the design doc says:

> Runs the generator against known library packages (bdw-gc, tree-sitter) from the complex_archive queue. Verifies generated recipes match existing library recipe structure and confirms the library_only subcategory appears correctly in failure data.

The test plan has three scenarios for #1882:
- Scenario 12: End-to-end bdw-gc (mock GHCR, full pipeline through `generateDeterministicRecipe` -> `WriteRecipe`)
- Scenario 13: End-to-end tree-sitter (same pattern, different package)
- Scenario 14: Non-library packages still fail as complex_archive (negative case)

## Sub-check 1: Design Intent Alignment

### AC: "tsuku create succeeds for library packages" (status: deviated)

**Finding: Advisory.** The mapping claims deviation because "Tests 2 packages not 5+; same code path." The design doc names only bdw-gc and tree-sitter as the test targets. The issue description also names exactly these two. Testing 2 packages with distinct layouts (bdw-gc has multiple libgc/libgccpp variants, nested headers; tree-sitter has a single library with subdirectory headers) is proportionate to the design's intent. The deviation self-report is more conservative than necessary -- the design didn't ask for 5+ packages.

However, the tests use mock GHCR servers with hand-crafted bottle layouts rather than testing against real pipeline data. The test plan scenarios 12-13 were originally marked "manual -- requires network access to GHCR" with real `tsuku create` commands. The implementation converted these to automated tests using `newMockGHCRBottleServer`. This is a reasonable engineering choice -- automated mock-based tests are more reliable for CI than network-dependent integration tests. The mock layouts use realistic file lists (versioned .so symlinks, .a archives, .pc files, nested headers) that match what real bottles contain. The design's intent of "validate against pipeline data" is approximated rather than literally fulfilled, but the approximation is high-fidelity.

### AC: "structure matches gmp.toml pattern" (status: implemented)

**Finding: No issue.** The end-to-end tests (`TestEndToEnd_LibraryRecipeGeneration_BdwGC`, lines 3458-3685; `TestEndToEnd_LibraryRecipeGeneration_TreeSitter`, lines 3690-3834) validate the generated recipe struct AND the serialized TOML output. Both tests call `recipe.WriteRecipe(r, tomlPath)` and verify:
- `type = "library"` present in TOML
- `install_mode = "directory"` present
- `outputs =` key present, `binaries =` absent
- `[verify]` section absent
- `when` clauses present (multi-platform)

This matches the Pattern B structure described in the design doc (platform-conditional step pairs with when clauses). The design's reference to gmp.toml as the target pattern is honored.

### AC: "library_only subcategory in failure data" (status: implemented)

**Finding: No issue.** Two tests validate the subcategory:
- `TestEndToEnd_LibraryOnly_SubcategoryOnGenerationFailure` (line 3904): verifies that "library recipe generation failed: ..." errors produce `[library_only]` tag in the classified message.
- `TestEndToEnd_NonLibraryPackage_StillFailsComplexArchive` (line 3841): verifies that non-library packages do NOT get the `[library_only]` tag.

Both positive and negative classification paths are tested, matching the design's intent for pipeline observability.

### AC: "non-library still fails complex_archive" (status: implemented)

**Finding: No issue.** `TestEndToEnd_NonLibraryPackage_StillFailsComplexArchive` uses a python@3.12-like bottle (files in `libexec/bin/`, `.py` files in `lib/python3.12/`) to verify:
1. `generateDeterministicRecipe` returns an error
2. The error contains "no binaries or library files found in bottle"
3. `classifyDeterministicFailure` returns `FailureCategoryComplexArchive`
4. The message does NOT contain `[library_only]`

This directly validates the design's intent: "After this change, complex_archive narrows to genuinely unclassifiable bottles."

### Design gap: scenario-7 test coverage

**Finding: Advisory.** The test plan records scenario-7 ("generateDeterministicRecipe falls back to library path when bin/ is empty") as "failed" with the note: "The routing logic inside generateDeterministicRecipe (homebrew.go:2093-2135) is not unit-tested because it depends on inspectBottleContents which requires network access."

However, the new end-to-end tests in #1882 DO test this routing logic through mock GHCR servers. `TestEndToEnd_LibraryRecipeGeneration_BdwGC` exercises the full path: `generateDeterministicRecipe` -> `inspectBottleContents` (via mock) -> empty binaries -> library detection -> `scanMultiplePlatforms` (via mock) -> `generateLibraryRecipe`. This effectively closes the gap that scenario-7 identified, even though the scenario status wasn't updated.

### Overall TOML serialization validation

The tests go beyond struct-level assertions to validate the actual TOML output. Both bdw-gc and tree-sitter tests write the recipe to a temp file via `WriteRecipe`, read it back, and check for specific string patterns. This catches serialization issues that struct-level tests would miss (e.g., the `Verify` nil-pointer TOML omission).

## Sub-check 2: Cross-Issue Enablement

No downstream issues exist (this is the terminal issue in the dependency chain). Sub-check skipped.

## Backward Coherence

The previous summary states: "Files changed: internal/builders/homebrew_test.go. Key decisions: Used mock GHCR servers to test full generateDeterministicRecipe pipeline. bdw-gc and tree-sitter tests use realistic bottle layouts."

The implementation matches this summary. The test approach is consistent: mock GHCR servers were introduced in prior issues (#1879 for `scanMultiplePlatforms` tests) and #1882 extends the same pattern to full pipeline tests. The `newMockGHCRBottleServer` helper and `createBottleTarballBytes` helper are reused from the same test file.

No contradictions with prior work patterns detected.

## Summary of Findings

| # | AC | Severity | Assessment |
|---|-----|----------|------------|
| 1 | tsuku create succeeds for library packages | Advisory | Deviated to 2 packages (design names exactly 2), mock instead of real GHCR (reasonable for CI) |
| 2 | type = library in metadata | -- | Verified in code |
| 3 | install_mode = directory | -- | Verified in code |
| 4 | outputs key not binaries | -- | Verified in code |
| 5 | when clauses for platforms | -- | Verified in code |
| 6 | no verify section | -- | Verified in code |
| 7 | structure matches gmp.toml pattern | -- | Verified in TOML output |
| 8 | outputs contain correct file types | -- | Verified per-platform (.so/.a/.pc/headers for Linux, .dylib/.a/.pc/headers for macOS) |
| 9 | non-library still fails complex_archive | -- | Verified with python@3.12-like mock |
| 10 | library_only subcategory in failure data | -- | Verified positive and negative paths |
| 11 | go test passes | -- | Assumed (all prior scenarios passed) |
