# Pragmatic Review: Issue #1792

**Issue**: test(ci): add recipe validation for GPU when clauses and dependency chains
**File changed**: `internal/executor/plan_generator_test.go`
**Tests added**: 4

## Tests Added

1. `TestGeneratePlan_GPUFilteringDownloadSteps` (line 2295) -- FilterStepsByTarget with github_file steps and GPU when clauses
2. `TestGeneratePlan_GPUDependencyChain_NvidiaCUDA` (line 2412) -- tsuku-llm -> cuda-runtime -> nvidia-driver chain
3. `TestGeneratePlan_GPUDependencyChain_AMDVulkan` (line 2583) -- tsuku-llm -> vulkan-loader -> mesa-vulkan-drivers chain
4. `TestGeneratePlan_GPUDependencyChain_NoneNoDeps` (line 2746) -- gpu=none produces no GPU dependencies

## Findings

No blocking or advisory findings.

### Analysis

**Correctness**: All 4 tests exercise the paths described in the issue requirements. GPU when clause matching is tested via `FilterStepsByTarget` (test 1) and via `GeneratePlan` (tests 2-4). Step-level dependency resolution is tested for both the NVIDIA/CUDA chain and AMD/Vulkan chain, including verification that filtered-out steps don't pull in their dependencies (test 4). Cycle detection is implicitly exercised since the dependency resolution code uses a `processed` map and these tests go through the full `GeneratePlan` path.

**Test design**: Using `FilterStepsByTarget` for test 1 is a sound decision. `github_file` is a composite action requiring network access for decomposition; testing pre-decomposition filtering validates the same logic without the network dependency. The comment at line 2301-2304 explains this explicitly.

**Mock recipes**: The dependency chain tests use mock recipes with non-composite actions (chmod, apt_install, install_binaries) to avoid network access. This is correct -- the dependency resolution logic doesn't depend on action type. The mock recipe structures match the real recipe topology (cuda-runtime depends on nvidia-driver, vulkan-loader depends on mesa-vulkan-drivers).

**Edge cases**: Test 4 uses an empty `mockRecipeLoader` and verifies no GPU dependencies are requested when gpu="none". This catches a potential bug where filtered-out step dependencies might leak into the plan.

**Error handling**: Tests use `t.Skipf` for network-related failures from version resolution (nodejs_dist source), which is consistent with all other tests in the file. This is appropriate since these tests target GPU filtering logic, not version resolution.

**Missing TOML validation**: The issue description mentions "TOML syntax for all new recipe files" but no TOML parsing tests were added. However, the existing CI workflow (`test.yml` line 365) runs `./tsuku validate --strict` on every recipe file in the registry, which covers TOML syntax validation for nvidia-driver.toml, cuda-runtime.toml, vulkan-loader.toml, and mesa-vulkan-drivers.toml. The Go-level tests in this commit focus on the logic that consumes those recipes, not parsing. Not a gap.

**No scope creep**: Only one file changed. No refactoring, no new abstractions, no documentation. The `mockRecipeLoader` type was already present from #1775.
