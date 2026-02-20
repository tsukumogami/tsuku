# Test Plan: GPU Backend Selection

Generated from: docs/designs/DESIGN-gpu-backend-selection.md
Issues covered: 14
Total scenarios: 22

---

## Scenario 1: GPU detection returns valid value on Linux
**ID**: scenario-1
**Category**: infrastructure
**Testable after**: #1773
**Commands**:
- `go test ./internal/platform/ -run 'TestDetectGPU$' -v -count=1`
**Expected**: `DetectGPU()` returns one of the valid values: "nvidia", "amd", "intel", "apple", "none". On the CI runner (Linux without a discrete GPU), it returns either "intel" (integrated graphics) or "none". Test passes without error.
**Status**: passed

---

## Scenario 2: GPU detection with mock sysfs for each vendor
**ID**: scenario-2
**Category**: infrastructure
**Testable after**: #1773
**Commands**:
- `go test ./internal/platform/ -run 'TestDetectGPUWithRoot' -v -count=1`
**Expected**: `DetectGPUWithRoot()` correctly identifies nvidia, amd, intel, and none from mock sysfs structures under `internal/platform/testdata/gpu/`. Dual-GPU scenarios (nvidia+intel, amd+intel) return the discrete GPU vendor (nvidia or amd).
**Status**: passed

---

## Scenario 3: platform.Target carries GPU value
**ID**: scenario-3
**Category**: infrastructure
**Testable after**: #1773
**Commands**:
- `go test ./internal/platform/ -run 'TestTarget.*GPU' -v -count=1`
**Expected**: `NewTarget("linux/amd64", "debian", "glibc", "nvidia")` creates a Target whose `GPU()` method returns "nvidia". Targets constructed with empty gpu string return "".
**Status**: passed

---

## Scenario 4: Matchable interface includes GPU on both implementations
**ID**: scenario-4
**Category**: infrastructure
**Testable after**: #1773
**Commands**:
- `go build ./...`
- `go vet ./...`
**Expected**: Build succeeds with GPU() on both `platform.Target` and `recipe.MatchTarget`. All ~76 callsites updated. No compile errors.
**Status**: passed

---

## Scenario 5: WhenClause GPU matching logic
**ID**: scenario-5
**Category**: infrastructure
**Testable after**: #1774
**Commands**:
- `go test ./internal/recipe/ -run 'TestWhenClause.*GPU' -v -count=1`
**Expected**: WhenClause with `GPU: ["nvidia"]` matches a target with GPU()="nvidia", does not match GPU()="amd" or GPU()="none". WhenClause with `GPU: ["amd", "intel"]` matches both "amd" and "intel" targets. WhenClause with empty GPU matches all targets. AND semantics with other fields: `{os=["linux"], gpu=["nvidia"]}` only matches linux+nvidia, not darwin+nvidia.
**Status**: passed

---

## Scenario 6: WhenClause IsEmpty includes GPU check
**ID**: scenario-6
**Category**: infrastructure
**Testable after**: #1774
**Commands**:
- `go test ./internal/recipe/ -run 'TestWhenClause.*IsEmpty' -v -count=1`
**Expected**: A WhenClause with only GPU set (`GPU: ["nvidia"]`) returns `false` for `IsEmpty()`. A WhenClause with all fields empty (including GPU) returns `true`.
**Status**: passed

---

## Scenario 7: TOML unmarshal parses gpu field from recipe
**ID**: scenario-7
**Category**: infrastructure
**Testable after**: #1774
**Commands**:
- `go test ./internal/recipe/ -run 'TestUnmarshalTOML.*gpu' -v -count=1`
**Expected**: A TOML step with `when = { gpu = ["nvidia"] }` unmarshals correctly into a WhenClause with `GPU: ["nvidia"]`. Single-string value `when = { gpu = "nvidia" }` also parses (converted to single-element array, matching libc pattern).
**Status**: passed

---

## Scenario 8: ToMap round-trips GPU field
**ID**: scenario-8
**Category**: infrastructure
**Testable after**: #1774
**Commands**:
- `go test ./internal/recipe/ -run 'TestToMap.*GPU' -v -count=1`
**Expected**: A WhenClause with `GPU: ["nvidia", "amd"]` serialized via ToMap produces `{"gpu": ["nvidia", "amd"]}` in the when map. A WhenClause with empty GPU omits the gpu key from the map.
**Status**: passed

---

## Scenario 9: PlanConfig GPU auto-detection and override
**ID**: scenario-9
**Category**: infrastructure
**Testable after**: #1775
**Commands**:
- `go test ./internal/executor/ -run 'TestGeneratePlan.*GPU' -v -count=1`
**Expected**: With `PlanConfig.GPU = ""`, GeneratePlan calls `platform.DetectGPU()` to auto-detect. With `PlanConfig.GPU = "nvidia"`, the detected value is overridden. A recipe with a `gpu = ["nvidia"]` step is included in the plan when GPU is "nvidia" and excluded when GPU is "none".
**Status**: passed

---

## Scenario 10: GPU propagates through dependency plan generation
**ID**: scenario-10
**Category**: infrastructure
**Testable after**: #1775
**Commands**:
- `go test ./internal/executor/ -run 'TestDep.*GPU' -v -count=1`
**Expected**: When a parent recipe step has `dependencies = ["cuda-runtime"]` and matches only for `gpu = ["nvidia"]`, the dependency plan generation also receives the GPU value "nvidia" through depCfg. The dependency chain filters correctly.
**Status**: passed

---

## Scenario 11: nvidia-driver recipe parses and validates
**ID**: scenario-11
**Category**: infrastructure
**Testable after**: #1789
**Commands**:
- `go build ./...`
- `go test ./... -run 'TestRecipe.*nvidia' -v -count=1`
**Expected**: `recipes/n/nvidia-driver.toml` is valid TOML with correct metadata (name, supported_os). Contains apt_install, dnf_install, pacman_install, zypper_install steps and a require_command step for nvidia-smi. Passes CI recipe validation.
**Status**: pending

---

## Scenario 12: cuda-runtime recipe parses and validates
**ID**: scenario-12
**Category**: infrastructure
**Testable after**: #1789
**Commands**:
- `go build ./...`
- `go test ./... -run 'TestRecipe.*cuda' -v -count=1`
**Expected**: `recipes/c/cuda-runtime.toml` is valid TOML with correct metadata (name, type=library, dependencies=["nvidia-driver"]), download_archive steps for linux amd64 and arm64 pointing to NVIDIA redist URLs, and install_binaries step for libcudart.so.
**Status**: pending

---

## Scenario 13: Vulkan dependency recipes parse and validate
**ID**: scenario-13
**Category**: infrastructure
**Testable after**: #1790
**Commands**:
- `go build ./...`
- `go test ./... -run 'TestRecipe.*vulkan' -v -count=1`
**Expected**: `recipes/v/vulkan-loader.toml` and `recipes/m/mesa-vulkan-drivers.toml` are valid TOML. vulkan-loader depends on mesa-vulkan-drivers. Both have system PM steps for at least 5 distro families (apt, dnf, pacman, apk, zypper).
**Status**: pending

---

## Scenario 14: tsuku-llm recipe has correct GPU-filtered steps
**ID**: scenario-14
**Category**: infrastructure
**Testable after**: #1776
**Commands**:
- `grep -c 'action = "github_file"' recipes/t/tsuku-llm.toml`
- `grep 'gpu = ' recipes/t/tsuku-llm.toml`
**Expected**: Recipe has 11 github_file steps total (2 macOS + 6 Linux GPU-filtered + 2 Linux no-GPU + 1 Windows). GPU conditions are present: `gpu = ["nvidia"]` on CUDA steps, `gpu = ["amd", "intel"]` on Vulkan steps, `gpu = ["none"]` on CPU steps. CUDA steps have `dependencies = ["cuda-runtime"]`, Vulkan steps have `dependencies = ["vulkan-loader"]`.
**Status**: pending

---

## Scenario 15: llm.backend config key get/set validation
**ID**: scenario-15
**Category**: infrastructure
**Testable after**: #1777
**Commands**:
- `tsuku config set llm.backend cpu`
- `tsuku config get llm.backend`
- `tsuku config set llm.backend invalid`
**Expected**: Setting "cpu" succeeds and returns confirmation. Getting returns "cpu". Setting "invalid" returns exit code 2 with an error listing valid values. Setting empty string clears the override. The key appears in `tsuku config` output.
**Status**: pending

---

## Scenario 16: CI validation tests for GPU when clauses
**ID**: scenario-16
**Category**: infrastructure
**Testable after**: #1792
**Commands**:
- `go test ./internal/recipe/... -run 'GPU|gpu' -v -count=1`
- `go test ./internal/executor/... -run 'GPU|gpu' -v -count=1`
**Expected**: All GPU-specific unit tests pass. Multi-step recipe filtering produces exactly one download step per GPU target (nvidia, amd, none). Dependency chain tests (tsuku-llm -> cuda-runtime -> nvidia-driver, tsuku-llm -> vulkan-loader -> mesa-vulkan-drivers) resolve without cycles.
**Status**: pending

---

## Scenario 17: Addon migration removes legacy code and uses recipe system
**ID**: scenario-17
**Category**: infrastructure
**Testable after**: #1778
**Commands**:
- `test ! -f internal/llm/addon/manifest.go`
- `test ! -f internal/llm/addon/manifest.json`
- `test ! -f internal/llm/addon/platform.go`
- `test ! -f internal/llm/addon/download.go`
- `test ! -f internal/llm/addon/verify.go`
- `grep -q 'type Installer interface' internal/llm/addon/manager.go`
- `go build ./...`
- `go test ./...`
**Expected**: All five legacy files are deleted. manager.go contains an Installer interface. No remaining references to GetManifest, PlatformKey, GetCurrentPlatformInfo, or cachedManifest in non-test Go files. Build and all tests pass.
**Status**: pending

---

## Scenario 18: Install tsuku-llm on Linux with NVIDIA GPU selects CUDA variant
**ID**: scenario-18
**Category**: use-case
**Environment**: manual -- requires Linux host with NVIDIA GPU and nvidia-smi working
**Testable after**: #1776, #1778
**Commands**:
- `tsuku install tsuku-llm`
- `ls $TSUKU_HOME/tools/ | grep tsuku-llm`
- `tsuku-llm --version`
**Expected**: tsuku detects the NVIDIA GPU via sysfs. The plan selects the CUDA variant step (`linux-amd64-cuda` or `linux-arm64-cuda`). cuda-runtime dependency is installed to `$TSUKU_HOME`. The installed binary runs successfully and reports its version. No manual GPU configuration needed.
**Status**: pending

---

## Scenario 19: Install tsuku-llm on Linux without GPU selects CPU variant
**ID**: scenario-19
**Category**: use-case
**Environment**: manual -- requires Linux host without a discrete GPU (or CI runner without GPU)
**Testable after**: #1776, #1778
**Commands**:
- `tsuku install tsuku-llm`
- `ls $TSUKU_HOME/tools/ | grep tsuku-llm`
- `tsuku-llm --version`
**Expected**: tsuku detects no GPU (DetectGPU returns "none"). The plan selects the CPU variant step. No GPU runtime dependencies are installed. The binary runs and reports its version.
**Status**: pending

---

## Scenario 20: llm.backend=cpu override forces CPU variant on NVIDIA hardware
**ID**: scenario-20
**Category**: use-case
**Environment**: manual -- requires Linux host with NVIDIA GPU
**Testable after**: #1777, #1778
**Commands**:
- `tsuku config set llm.backend cpu`
- `tsuku install tsuku-llm`
- `ls $TSUKU_HOME/tools/ | grep tsuku-llm`
**Expected**: Despite NVIDIA GPU being detected, the config override forces GPU to "none" before plan generation. The CPU variant is installed instead of the CUDA variant. No cuda-runtime dependency is installed.
**Status**: pending

---

## Scenario 21: GPU variant performance exceeds CPU on shipped models
**ID**: scenario-21
**Category**: use-case
**Environment**: manual -- requires NVIDIA GPU (CUDA) or AMD/Intel GPU (Vulkan) hardware
**Testable after**: #1780
**Commands**:
- `tsuku install tsuku-llm`
- Run benchmark on 0.5B, 1.5B, 3B models measuring tokens/second
- `tsuku config set llm.backend cpu`
- `tsuku install tsuku-llm`
- Run same benchmark with CPU variant
**Expected**: GPU variant tokens/second is meaningfully higher than CPU variant for all three model sizes. Results recorded in `docs/designs/benchmarks/`. Hardware details (GPU model, driver version, CUDA/Vulkan version) documented.
**Status**: pending

---

## Scenario 22: Unsupported platform gets clear error message
**ID**: scenario-22
**Category**: use-case
**Environment**: manual -- requires musl/Alpine Linux or non-amd64/arm64 architecture
**Testable after**: #1786
**Commands**:
- `tsuku install tsuku-llm` (on Alpine Linux or unsupported arch)
**Expected**: `SupportsPlatformRuntime()` rejects the platform before plan generation. Error message includes what IS supported (glibc, amd64/arm64, linux/darwin) and explains why the current platform is unsupported. Exit code is non-zero. The error mentions the libc constraint for musl users.
**Status**: pending
