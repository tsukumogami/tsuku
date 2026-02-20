# Maintainer Review: Issue #1786

**Focus**: Maintainability -- can the next developer understand and change this code with confidence?

**Commit**: `166ea571ba16d817ac043128094c2eb597c9db6e`

## Summary

The commit adds a new test file (`internal/recipe/tsuku_llm_recipe_test.go`) with 6 tests that validate the `tsuku-llm.toml` recipe against the release pipeline's build matrix, platform constraints, step coverage, and user-facing error messages. It also removes the Windows step from the recipe (no pipeline artifact exists) and adds `unsupported_reason` metadata.

The tests are well-structured and clearly named. Each test covers a distinct concern, the table-driven tests follow Go conventions, and the helper functions have appropriate `t.Helper()` calls.

## Findings

### 1. Pipeline build matrix is duplicated across three locations with no cross-reference enforcement

**Files**:
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/recipe/tsuku_llm_recipe_test.go:58` -- `pipelineBuildMatrix()` returns hardcoded suffixes
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/.github/workflows/llm-release.yml:44-89` -- the actual CI build matrix
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/.github/workflows/llm-release.yml:308-319` -- the finalize-release verification list

The comment on `pipelineBuildMatrix()` says "Sourced from .github/workflows/llm-release.yml build matrix" but there's no mechanism to detect drift. If someone adds a FreeBSD build to the CI matrix, this Go function still returns the old list, and the test passes -- silently guarding an outdated contract. The same suffixes appear a third time in the `finalize-release` job's `EXPECTED_ARTIFACTS` array.

The test's stated purpose is to validate that the recipe matches the pipeline, but it actually validates that the recipe matches a hardcoded Go slice that someone manually keeps in sync with the pipeline. The next developer will assume this test catches pipeline/recipe drift. It won't catch pipeline changes that aren't also reflected in the Go code.

**Severity**: Advisory. The comment accurately describes the source, so the next developer knows where to look. But the name `TestTsukuLLMRecipeAssetPatternsMatchPipeline` promises pipeline validation that it can't deliver. A comment like "NOTE: if the build matrix changes in llm-release.yml, update this list" would make the manual sync obligation explicit.

### 2. Custom `contains` helper instead of `strings.Contains`

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/recipe/tsuku_llm_recipe_test.go:228`

The test calls `contains(errMsg, substr)` which resolves to a custom implementation in `types_test.go:1719` that reimplements substring search manually. The next developer will wonder why this exists instead of `strings.Contains`. The custom version has identical semantics -- it's just a hand-rolled substring search with no additional behavior.

This is a minor readability speed bump. The next developer will spend a moment finding the definition and verifying it does what `strings.Contains` does.

**Severity**: Advisory. Small scope, same package, no risk of misread leading to a bug.

### 3. Test names accurately describe what they test

`TestTsukuLLMRecipeStepCoverage` tests that every platform/GPU combination matches exactly one step. `TestTsukuLLMRecipeMuslNotSupported` tests that musl is rejected. `TestTsukuLLMRecipePlatformMetadata` tests metadata field values. `TestTsukuLLMRecipeUnsupportedPlatformError` tests the error message content. The assertions match the names. No issues here.

### 4. `findRepoRoot` uses `runtime.Caller` -- novel pattern in this codebase

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/recipe/tsuku_llm_recipe_test.go:19-32`

This is the only test file in `internal/` that uses `runtime.Caller(0)` to locate the repo root. Other tests in the codebase use `testdata/` directories (relative to the test file) or inline fixtures. This approach is more fragile -- it breaks if the test file is moved without updating the walk-up logic, and it assumes `go.mod` is at the repo root.

That said, this test has a genuine need: it must read a real recipe file from `recipes/t/tsuku-llm.toml`, which is outside the `internal/recipe/` directory tree. The `runtime.Caller` approach is a reasonable solution for this constraint.

**Severity**: Advisory. The function is well-documented, self-contained, and has a clear error path. If it becomes a pattern, it should be extracted to a shared test helper. As a one-off, it's fine.

### 5. Step coverage test doesn't verify the asset_pattern content of the matched step

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/recipe/tsuku_llm_recipe_test.go:279-297`

`TestTsukuLLMRecipeStepCoverage` verifies that each target matches exactly one step, but doesn't verify *which* step matched. For example, target `{"linux-amd64-nvidia", "linux", "amd64", "nvidia"}` should match the CUDA step, not the Vulkan step. The test only checks `matchCount == 1`.

The next developer might assume this test catches a step-level regression (e.g., swapping the GPU arrays on two steps). It wouldn't -- both steps would still match exactly one target each, just the wrong ones.

**Severity**: Advisory. The `TestTsukuLLMRecipeAssetPatternsMatchPipeline` test partially covers this by checking that the right suffixes exist, but doesn't link suffixes to targets. A combined check ("target X matches the step with asset_pattern Y") would be stronger. However, the current recipe structure makes accidental swaps unlikely since the `when` clauses and asset patterns are adjacent in the TOML.

### 6. Metadata assertion uses order-dependent comparison

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/recipe/tsuku_llm_recipe_test.go:134-167`

`TestTsukuLLMRecipePlatformMetadata` compares `supported_os`, `supported_arch`, and `supported_libc` by index position. If someone reorders `supported_os = ["darwin", "linux"]` in the TOML, the test breaks even though the semantics are identical. This is fine if the order is intentional (TOML preserves array order), but the test doesn't explain whether order matters.

**Severity**: Advisory. The order doesn't matter for platform support logic (`containsString` does membership checks), so the test is stricter than the code it protects. A set comparison would be more resilient, but this is a minor concern for a list with 2 elements.

## What's Clear

- Test file organization: one helper section at top, then one test per concern. Easy to navigate.
- Recipe comments explain the variant selection logic clearly. A developer unfamiliar with the GPU backend design can understand which step serves which platform.
- The `unsupported_reason` field in the recipe gives users a complete explanation of why musl and Windows aren't supported, and the test verifies the key terms are present.
- `loadTsukuLLMRecipe` using `t.Helper()` correctly so failures point to the calling test.

## Verdict

No blocking findings. The code is readable and the tests cover meaningful properties of the recipe. The main advisory concern is that `TestTsukuLLMRecipeAssetPatternsMatchPipeline` promises more than it delivers -- it validates against a hardcoded list, not the actual pipeline -- but the comment makes the source clear enough that the next developer can follow the chain.
