# Pragmatic Review: Issue #1786

Commit: `166ea571ba16d817ac043128094c2eb597c9db6e`

## Blocking Findings

### 1. Hardcoded pipeline matrix duplicates source of truth

`/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/recipe/tsuku_llm_recipe_test.go:58-69` -- `pipelineBuildMatrix()` returns a hardcoded list of artifact suffixes that must stay in sync with `.github/workflows/llm-release.yml`. This is a manually maintained copy of the pipeline's build matrix. When the pipeline changes, this test will either (a) silently pass if nobody updates it, or (b) fail with a misleading message about the *recipe* being wrong when the *test* is stale. The stated goal is "validate recipe against release pipeline" but the test validates recipe against a hardcoded Go slice, not the actual pipeline. Either parse the workflow YAML in the test, or acknowledge in a comment that this is a snapshot that must be updated manually (in which case the test's value is substantially reduced -- it's a change detector for the recipe TOML, not a pipeline-recipe consistency check). **Blocking** because it creates a false sense of safety.

### 2. TestTsukuLLMRecipePlatformMetadata is a snapshot test of TOML values

`/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/recipe/tsuku_llm_recipe_test.go:130-173` -- This test asserts that `supported_os = ["linux", "darwin"]`, `supported_arch = ["amd64", "arm64"]`, `supported_libc = ["glibc"]`, and `unsupported_reason != ""`. It's a change-detector that will break any time the recipe legitimately adds or removes a platform. The actual correctness check is already done by `TestTsukuLLMRecipePlatformConstraints` (validates structural consistency) and `TestTsukuLLMRecipeStepCoverage` (validates every target has a step). This test adds no safety beyond "did the TOML change?" -- which git already tells you. **Blocking** because it will cause false failures on legitimate recipe updates and must be maintained in lockstep with the TOML for no additional safety.

### 3. TestTsukuLLMRecipeUnsupportedReason is a snapshot of prose content

`/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/recipe/tsuku_llm_recipe_test.go:302-317` -- Asserts that `unsupported_reason` contains the strings "glibc" and "musl". This tests the content of a human-readable explanation string. If someone rewrites the reason to say "Alpine Linux (musl libc) is not supported" it still communicates the same information but could break this test depending on exact phrasing. The non-emptiness check in `TestTsukuLLMRecipePlatformMetadata` line 170 already ensures the field is populated. Testing prose content is gold-plated validation. **Advisory** -- it's small and the substrings are stable enough, but it adds maintenance cost for marginal value.

## Advisory Findings

### 4. TestTsukuLLMRecipeUnsupportedPlatformError tests the error formatter, not the recipe

`/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/recipe/tsuku_llm_recipe_test.go:177-234` -- This test constructs `UnsupportedPlatformError` manually with recipe metadata and checks the `.Error()` output contains expected substrings. It's really testing `UnsupportedPlatformError.Error()` formatting, which belongs in a unit test for that type (and likely already exists in `platform_test.go`). Putting it in a recipe-specific test file implies it's testing recipe behavior. Minor misplacement, not harmful.

### 5. Custom `contains` reimplements `strings.Contains`

`/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/recipe/types_test.go:1719-1731` -- Pre-existing, not introduced by this commit, but the new test file uses it. `strings.Contains` does the same thing. Not blocking since it's inherited technical debt.

## Summary

The recipe change (removing Windows, adding `unsupported_reason`) and the `StepCoverage` / `PlatformConstraints` / `MuslNotSupported` tests are well-scoped and useful. The pipeline matrix test gives false confidence since it's not actually reading the pipeline. The metadata snapshot test will break on legitimate changes. Consider removing or restructuring findings #1 and #2.
