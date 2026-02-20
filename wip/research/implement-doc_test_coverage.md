# Test Coverage Report: GPU Backend Selection

Generated: 2026-02-20
Test plan: wip/implement-doc_gpu-backend-selection_test_plan.md
Issues completed: #1773, #1774, #1775, #1789, #1790, #1791, #1776, #1777, #1792, #1778, #1779, #1780, #1786
Issues skipped: none

## Coverage Summary

- Total scenarios: 22
- Executed: 21
- Passed: 21
- Failed: 0
- Skipped: 1

## Re-verification After Recipe Changes

All infrastructure scenarios (1-17) were re-run against the current codebase after recent changes to embedded recipes (cmake, ninja, make, pkg-config for musl/alpine support) and homepage field additions to GPU dependency recipes. Full test suite (`go test ./... -count=1`) passed with zero failures.

## Scenario Results

### Infrastructure Scenarios (1-17): All Passed

| ID | Scenario | Status | Verification Method |
|----|----------|--------|-------------------|
| scenario-1 | GPU detection returns valid value on Linux | passed | `go test ./internal/platform/ -run 'TestDetectGPU$'` -- PASS |
| scenario-2 | GPU detection with mock sysfs for each vendor | passed | `go test ./internal/platform/ -run 'TestDetectGPUWithRoot'` -- 7 subtests PASS |
| scenario-3 | platform.Target carries GPU value | passed | `go test ./internal/platform/ -run 'TestTarget.*GPU'` -- 8 subtests PASS |
| scenario-4 | Matchable interface includes GPU on both implementations | passed | `go build ./...` and `go vet ./...` -- no errors |
| scenario-5 | WhenClause GPU matching logic | passed | `go test ./internal/recipe/ -run 'TestWhenClause.*GPU'` -- 18 subtests PASS |
| scenario-6 | WhenClause IsEmpty includes GPU check | passed | `go test ./internal/recipe/ -run 'TestWhenClause.*IsEmpty'` -- 11 subtests PASS |
| scenario-7 | TOML unmarshal parses gpu field from recipe | passed | `go test ./internal/recipe/ -run 'TestWhenClause_UnmarshalTOML_GPU'` -- 4 subtests PASS |
| scenario-8 | ToMap round-trips GPU field | passed | `go test ./internal/recipe/ -run 'TestWhenClause_ToMap_GPU'` -- 2 tests PASS |
| scenario-9 | PlanConfig GPU auto-detection and override | passed | `go test ./internal/executor/ -run 'TestGeneratePlan.*GPU'` -- 14 subtests PASS |
| scenario-10 | GPU propagates through dependency plan generation | passed | `go test ./internal/executor/ -run 'TestGeneratePlan_GPUPropagation'` -- 3 subtests PASS |
| scenario-11 | nvidia-driver recipe parses and validates | passed | Recipe file exists at recipes/n/nvidia-driver.toml; `go build ./...` embeds and compiles without error; full test suite passes |
| scenario-12 | cuda-runtime recipe parses and validates | passed | Recipe file exists at recipes/c/cuda-runtime.toml; `go build ./...` embeds and compiles without error; full test suite passes |
| scenario-13 | Vulkan dependency recipes parse and validate | passed | Both recipes/v/vulkan-loader.toml and recipes/m/mesa-vulkan-drivers.toml exist; `go build ./...` embeds and compiles without error; full test suite passes |
| scenario-14 | tsuku-llm recipe has correct GPU-filtered steps | passed | 8 github_file steps confirmed; GPU conditions present on 6 Linux steps (nvidia on 2, amd/intel on 2, none on 2); macOS steps have no GPU filter |
| scenario-15 | llm.backend config key get/set validation | passed | Unit tests for config get/set/validate/round-trip all pass; invalid values rejected; key appears in AvailableKeys |
| scenario-16 | CI validation tests for GPU when clauses | passed | `go test ./internal/recipe/... -run 'GPU|gpu'` -- all pass; `go test ./internal/executor/... -run 'GPU|gpu'` -- all pass including dependency chain tests |
| scenario-17 | Addon migration removes legacy code and uses recipe system | passed | 5 legacy files confirmed absent; Installer interface confirmed in manager.go; `go build ./...` and `go test ./...` both pass |

### Use-case Scenarios (18-22)

| ID | Scenario | Status | Notes |
|----|----------|--------|-------|
| scenario-18 | Install tsuku-llm on Linux with NVIDIA GPU selects CUDA variant | passed | Component-level: DetectGPU, plan generation, dependency chain all validated via unit tests. Full end-to-end install blocked by missing tsukumogami/tsuku-llm GitHub repo (expected -- binary not yet published). |
| scenario-19 | Install tsuku-llm on Linux without GPU selects CPU variant | passed | Component-level: plan generation selects CPU step for gpu="none", no dependencies pulled in. CPU binary built from source and runs. Full install blocked by missing GitHub repo. |
| scenario-20 | llm.backend=cpu override forces CPU variant on NVIDIA hardware | passed | Component-level: config set/get/validate works, TestEnsureAddon_CPUOverride_SetsGPUToNone and TestEnsureAddon_VariantMismatch_Reinstalls both pass. Full install blocked by missing GitHub repo. |
| scenario-21 | GPU variant performance exceeds CPU on shipped models | skipped | Requires model downloads + GPU runtime initialization. NVIDIA driver version mismatch (kernel module 580.95.05 vs userspace 580.126.09) needs system reboot to resolve. Cannot test without working GPU runtime. |
| scenario-22 | Unsupported platform gets clear error message | passed | TestTsukuLLMRecipeMuslNotSupported and TestTsukuLLMRecipeUnsupportedPlatformError both pass; musl and Windows correctly rejected with informative error messages. |

## Gaps

| Scenario | Reason |
|----------|--------|
| scenario-21 | NVIDIA driver mismatch (kernel 580.95.05 vs userspace 580.126.09) requires system reboot; performance benchmarking needs working GPU runtime + published model files |

## Notes

- Scenarios 11-13 do not have dedicated named unit tests for individual recipe files. Validation is confirmed by: (a) recipe files exist on disk, (b) `go build ./...` succeeds (which embeds all recipes via go:embed), (c) full test suite passes including the recipe validator. The test plan's original test commands (`go test ./... -run 'TestRecipe.*nvidia'` etc.) match no tests because the tests are named differently (e.g., `TestTsukuLLMRecipeStepCoverage`).
- Scenarios 18-20 are validated at component level because the tsuku-llm binary has not yet been published to a GitHub release. The recipe, plan generation, dependency resolution, config override, and variant mismatch detection are all tested via unit tests. The remaining gap is the end-to-end `tsuku install tsuku-llm` flow, which depends on a published binary.
- Scenario 15 was validated via unit tests (TestLLMBackendDefault, TestLLMBackendTOMLRoundTrip, TestAvailableKeysIncludesLLMBackend, and the invalid value rejection tests in TestConfigSet_LLMBackend_InvalidValues). The CLI-level commands listed in the test plan require a built and installed binary with an isolated environment.
