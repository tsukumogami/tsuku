# Maintainer Review: Issue #1792

**Issue**: test(ci): add recipe validation for GPU when clauses and dependency chains
**File**: `internal/executor/plan_generator_test.go` (lines 1902-2835)
**Commit**: 72ea98eb

## Summary

The commit adds 7 new test functions and a `mockRecipeLoader` helper type to the plan generator test file. The tests cover:

1. GPU step filtering (`TestGeneratePlan_GPUFiltering`)
2. GPU auto-detection passthrough (`TestGeneratePlan_GPUAutoDetection`)
3. GPU propagation through dependency resolution (`TestGeneratePlan_GPUPropagationThroughDependencies`)
4. LinuxFamily propagation through depCfg (`TestGeneratePlan_DepCfgLinuxFamilyPropagation`)
5. GPU-filtered download step selection via `FilterStepsByTarget` (`TestGeneratePlan_GPUFilteringDownloadSteps`)
6. NVIDIA CUDA dependency chain: tsuku-llm -> cuda-runtime -> nvidia-driver (`TestGeneratePlan_GPUDependencyChain_NvidiaCUDA`)
7. AMD Vulkan dependency chain: tsuku-llm -> vulkan-loader -> mesa-vulkan-drivers (`TestGeneratePlan_GPUDependencyChain_AMDVulkan`)
8. No-GPU path with zero GPU dependencies (`TestGeneratePlan_GPUDependencyChain_NoneNoDeps`)

## Findings

### 1. Duplicated tsuku-llm recipe fixture across three tests (Advisory)

**File**: `internal/executor/plan_generator_test.go`, lines 2467-2498, 2624-2655, 2754-2785

The `tsukuLLMRecipe` variable is defined identically (same 4 steps: cuda chmod, vulkan chmod, cpu chmod, install_binaries) in three separate test functions: `TestGeneratePlan_GPUDependencyChain_NvidiaCUDA`, `TestGeneratePlan_GPUDependencyChain_AMDVulkan`, and `TestGeneratePlan_GPUDependencyChain_NoneNoDeps`.

This is a divergent-twins risk. If someone updates the recipe shape in one test (e.g., adds an arm64 step) but misses the other two, the tests silently diverge in what they're validating. A next developer won't know whether the differences (currently there are none) are intentional.

**Suggestion**: Extract a `newTestTsukuLLMRecipe()` helper that returns the shared fixture. The tests only differ in the loader contents and the GPU target value, not in the recipe itself.

**Severity**: Advisory. The tests are currently identical, so no misread risk today, but the duplication is a maintenance trap for the next person who modifies the recipe shape.

### 2. Test name includes "DepCfg" implementation detail (Advisory)

**File**: `internal/executor/plan_generator_test.go`, line 2207

`TestGeneratePlan_DepCfgLinuxFamilyPropagation` uses the internal variable name `depCfg` in the test name. The next developer reading the test list won't know what "DepCfg" means without reading the plan generator implementation. The test's actual behavior -- verifying that LinuxFamily is propagated to dependency plan generation -- is clear from the test body.

**Suggestion**: Rename to `TestGeneratePlan_LinuxFamilyPropagationThroughDependencies`, which mirrors the naming pattern of `TestGeneratePlan_GPUPropagationThroughDependencies` right above it.

**Severity**: Advisory. The test body and comment explain what's tested, so the name won't cause a misread, just a minor cognitive bump.

### 3. Comments are accurate and well-targeted (Positive)

Each test function has a leading comment that explains:
- What's being tested (the behavior, not the implementation)
- Why a specific approach was chosen (e.g., `FilterStepsByTarget` instead of `GeneratePlan` for download steps because `github_file` is a composite action needing network)
- What the mock recipes model (e.g., "modeled after the real recipe structure")

The `mockRecipeLoader` comment is minimal and sufficient. The comment on `TestGeneratePlan_GPUDependencyChain_NvidiaCUDA` lines 2421-2423 correctly explains why non-composite actions are used (avoiding network access during decomposition). This is exactly the kind of non-obvious decision that needs a comment.

### 4. mockRecipeLoader error path could mislead during debugging (Advisory)

**File**: `internal/executor/plan_generator_test.go`, lines 2066-2071

When `mockRecipeLoader.GetWithContext` doesn't find a recipe, it returns a generic `fmt.Errorf("recipe %q not found", name)`. Looking at the call chain, `generateSingleDependencyPlan` at `plan_generator.go:727-731` swallows this error and returns `nil, nil`, silently skipping the dependency.

This means if a test has a typo in the loader's recipe map key (e.g., `"cuda-runtme"` instead of `"cuda-runtime"`), the dependency silently disappears from the plan. The test would then pass the "no further dependencies" check but fail on a later assertion with an unhelpful error like "expected cuda-runtime dependency, found: []".

The existing tests have assertions that catch this (e.g., line 2538 checks for cuda-runtime explicitly), so this won't cause a silent false pass. But the debugging path is indirect.

**Severity**: Advisory. The existing assertions protect against false passes, but the next person debugging a typo would need to trace through two layers of indirection.

### 5. Test assertions match real recipe structure (Positive)

The mock recipes in the dependency chain tests accurately reflect the actual recipe files:
- `cuda-runtime.toml` has `dependencies = ["nvidia-driver"]` -- the test's `cudaRuntimeRecipe` has `Dependencies: []string{"nvidia-driver"}` (line 2446)
- `vulkan-loader.toml` has `dependencies = ["mesa-vulkan-drivers"]` -- the test's `vulkanLoaderRecipe` has `Dependencies: []string{"mesa-vulkan-drivers"}` (line 2611)
- `nvidia-driver.toml` has no metadata dependencies -- the test's `nvidiaDriverRecipe` has no `Dependencies` field (line 2426)
- `tsuku-llm.toml` steps use `dependencies = ["cuda-runtime"]` and `dependencies = ["vulkan-loader"]` at the step level -- the test mirrors this with step-level `Dependencies` (lines 2480, 2486)

The dependency chains in the tests match the real recipes. A developer maintaining these tests can trust that they reflect production.

### 6. FilterStepsByTarget approach for download tests is well-documented (Positive)

**File**: `internal/executor/plan_generator_test.go`, lines 2296-2304

The comment at `TestGeneratePlan_GPUFilteringDownloadSteps` clearly explains why it uses `FilterStepsByTarget` instead of the full `GeneratePlan` path: composite actions like `github_file` decompose during plan generation and require network access. The filtering logic runs before decomposition, so testing it at the `FilterStepsByTarget` level exercises the same code path without network dependencies.

This is a good design decision that the next developer needs to understand. The comment provides that understanding.

## Overall Assessment

The tests are well-structured, well-commented, and accurately reflect the real recipe dependency chains. The test names follow the established `TestGeneratePlan_*` convention in the file. The `t.Skipf` pattern for network-dependent failures follows the existing convention used by 14+ other tests in the same file.

The `mockRecipeLoader` is a clean test helper that correctly implements the `actions.RecipeLoader` interface. It's used consistently across 5 tests.

No blocking findings. The advisory items are minor quality improvements: extracting the duplicated recipe fixture and aligning the `DepCfg` test name with the naming pattern of its neighboring test.
