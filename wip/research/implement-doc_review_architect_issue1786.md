# Architect Review: Issue #1786

**Commit**: 166ea571ba16d817ac043128094c2eb597c9db6e
**Focus**: Structural fit of recipe test file, metadata addition, and recipe changes.

## Summary

This commit adds `unsupported_reason` metadata to the tsuku-llm recipe, removes a Windows step that had no pipeline artifact, adds a dedicated test file validating recipe/pipeline alignment, and updates the design doc's "What Ships" section.

## Findings

### No blocking issues found.

### Advisory: Recipe-loading helper duplicates existing infrastructure (contained)

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/recipe/tsuku_llm_recipe_test.go:18-51`

The test file introduces `findRepoRoot()` and `loadTsukuLLMRecipe()` to load and parse the recipe TOML from disk. The `internal/recipe` package already has a loader in `loader.go` and embedded recipes in `embedded.go`.

However, this is a test helper that reads a specific file from the repo's `recipes/` directory (not the embedded registry), so it has a different purpose: validating the committed recipe file against the release pipeline. The existing loaders work with the embedded filesystem or user-specified paths through a different API. The helper is small (30 lines), limited to test code, and has no other callers.

If more recipe-specific test files follow this pattern (e.g., one per complex recipe), the `findRepoRoot()` + direct TOML parse pattern would benefit from extraction into a shared test helper. For a single file, this is contained. **Advisory.**

### Advisory: Hardcoded pipeline matrix in test code

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/recipe/tsuku_llm_recipe_test.go:58-69`

The `pipelineBuildMatrix()` function hardcodes the 8 artifact suffixes from `.github/workflows/llm-release.yml`. If the pipeline matrix changes, this function must be updated manually. The comment on line 54-55 documents the source, which is good.

This is an inherent trade-off: the test exists precisely to catch recipe/pipeline drift, and one side has to be the source of truth. Parsing the YAML programmatically would add complexity without much benefit. The approach is pragmatic. **Advisory -- the comment should be sufficient to keep this in sync.**

## Structural Assessment

### `unsupported_reason` metadata field

The `UnsupportedReason` field already exists in `MetadataSection` (types.go:168), is consumed by `UnsupportedPlatformError.Error()` (platform.go:68-70), displayed by `FormatPlatformConstraints()` (platform.go:332-334), and shown in the `info` command (cmd/tsuku/info.go). The recipe simply populates an existing field. No state contract drift.

### Test structure

The test file lives in `internal/recipe/` (same package), uses the existing `Recipe` struct and methods (`ValidatePlatformConstraints`, `SupportsPlatformWithLibc`, `NewMatchTarget`, `WhenClause.Matches`), and reuses the `contains()` helper from `types_test.go`. This follows the established test pattern in the package.

All 6 tests exercise existing public API surface:
- `TestTsukuLLMRecipeAssetPatternsMatchPipeline` -- validates recipe steps match pipeline build matrix
- `TestTsukuLLMRecipePlatformConstraints` -- calls `ValidatePlatformConstraints()`
- `TestTsukuLLMRecipePlatformMetadata` -- asserts metadata field values
- `TestTsukuLLMRecipeUnsupportedPlatformError` -- exercises `UnsupportedPlatformError.Error()`
- `TestTsukuLLMRecipeMuslNotSupported` -- calls `SupportsPlatformWithLibc()`
- `TestTsukuLLMRecipeStepCoverage` -- uses `NewMatchTarget()` and `WhenClause.Matches()`

No new types, interfaces, or dispatch paths are introduced.

### Recipe changes

Removing the Windows step aligns the recipe with the pipeline (no Windows artifact exists). Adding `unsupported_reason` uses the existing metadata field. Both changes are data-level, not structural.

### Design doc update

The "What Ships" section now includes a platform support matrix with explicit rows for unsupported platforms. This documents the same constraints declared in the recipe metadata.

## Verdict

The commit fits the existing architecture. No new patterns introduced. The test file is a recipe-specific integration test that validates data consistency between the recipe TOML and the release pipeline -- it exercises existing API and doesn't bypass any dispatch or loading infrastructure.

**Blocking: 0 | Advisory: 2**
