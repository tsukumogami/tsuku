---
status: Accepted
problem: |
  The tsuku-llm addon builds 10 platform variants across GPU backends (CUDA, Vulkan, Metal, CPU),
  but the addon download system maps each OS-architecture pair to a single binary. There's no way
  to select which GPU variant to download, and more broadly, tsuku has no awareness of GPU hardware
  at the platform level. This prevents both the addon and any future recipes from filtering on GPU
  capabilities the way they already filter on OS, architecture, and libc.
decision: |
  Extend the platform detection system with GPU vendor identification (via PCI sysfs on Linux,
  system profiler on macOS) and add a gpu field to WhenClause for recipe step filtering. Then
  convert tsuku-llm from a custom addon-with-embedded-manifest into a standard recipe that uses
  when clauses for variant selection, with GPU driver packages as library recipe dependencies.
  The addon lifecycle code (server start/stop, gRPC, health checks) stays separate from the recipe.
rationale: |
  GPU detection follows the same pattern as libc detection: read a system file, return a
  string, match it in when clauses. Reusing the existing platform and recipe infrastructure
  means no new schema formats, no custom manifest, and any future tool that needs GPU filtering
  gets it for free. Converting tsuku-llm to a recipe also removes the only non-recipe binary
  distribution path in tsuku, simplifying the codebase. The addon lifecycle code stays because
  daemon management has no recipe equivalent, but binary installation moves to the standard flow.
---

# Design Document: GPU Backend Selection

## Status

Accepted

**Source Issue**: [#1769 - feat(llm): GPU backend selection and multi-variant addon distribution](https://github.com/tsukumogami/tsuku/issues/1769)

## Upstream Design Reference

This design implements part of [DESIGN-local-llm-runtime.md](current/DESIGN-local-llm-runtime.md) from the Production Ready milestone.

**Relevant sections:**
- Acknowledged as open question: "Haven't fully validated the CI infrastructure for this"
- Build pipeline (issue #1633) produces 10 platform variants but manifest maps only 5 platforms

## Context and Problem Statement

The tsuku-llm addon delivers local LLM inference by bundling llama.cpp in a Rust binary. The CI release pipeline already builds 10 variants across platforms and GPU backends:

| Platform | Variants |
|----------|----------|
| darwin-arm64 | metal |
| darwin-amd64 | metal |
| linux-amd64 | cuda, vulkan, cpu |
| linux-arm64 | cuda, vulkan, cpu |
| windows-amd64 | cpu |

But the addon download system has a 1:1 mapping from `GOOS-GOARCH` to a single binary URL. The manifest schema, embedded in the Go binary via `//go:embed`, has no backend dimension. `PlatformKey()` returns `runtime.GOOS + "-" + runtime.GOARCH` and that's the only key used to look up download info.

This creates concrete problems:

**Wrong binary for the hardware.** On Linux, there's no way to specify whether to download the CUDA, Vulkan, or CPU variant. A user with an NVIDIA GPU gets the same binary as a user with no GPU at all.

**Detection happens too late.** Hardware detection (`hardware.rs`) runs inside the Rust binary at server startup. By that point, the binary is already downloaded. If it was compiled with CUDA but the system only has Vulkan, the detection code correctly identifies Vulkan but the binary can't use it.

**CUDA version coupling.** The CI builds against CUDA 12.4. Users need a compatible NVIDIA driver (>= 525.60). If the driver is older, CUDA initialization fails silently. We hit this on our own development machine: `ggml_cuda_init: failed to initialize CUDA: forward compatibility was attempted on non supported HW`.

**No GPU awareness at the platform level.** Beyond the addon, tsuku has no concept of GPU hardware. The platform detection system (`platform.Target`) knows OS, architecture, Linux distribution family, and libc implementation. But it doesn't know whether the machine has an NVIDIA, AMD, or Intel GPU. This means no recipe can conditionally select download URLs or dependencies based on GPU hardware. The addon works around this with its own embedded manifest, but that's a one-off mechanism that doesn't help other tools or the broader recipe ecosystem.

### Scope

**In scope:**
- GPU vendor detection in the `platform` package (Linux sysfs, macOS system profiler)
- `gpu` field added to `Matchable` interface and `WhenClause`
- tsuku-llm converted from addon-with-embedded-manifest to standard recipe
- GPU driver packages (Vulkan loader, CUDA runtime) as library recipes
- Cleanup of addon manifest/download code (replaced by recipe system)
- Addon lifecycle code retained for daemon management

**Out of scope:**
- Automatic runtime fallback when GPU backend fails (deferred until detection accuracy is measured; manual override via `llm.backend` config is the escape hatch)
- Vulkan VRAM detection fix (standalone Rust bug, separate issue)
- Windows GPU support (only CPU variant exists today)
- Shipping multiple CUDA versions (e.g., CUDA 11 + CUDA 12)
- Dynamic backend loading within a single binary (llama.cpp's `GGML_BACKEND_DL` mode)
- General recipe "variant selection" mechanism (tsuku-llm handles override via LLM-specific config)
- CUDA variant selection (deferred until Vulkan vs CUDA benchmarks are complete; Vulkan serves all GPU vendors initially)

## Decision Drivers

- **Consistency**: tsuku already has a pattern for platform-specific filtering. Libc detection in the `platform` package feeds `WhenClause.Libc`, and recipes like `gcc-libs.toml` and `openssl.toml` use `when = { libc = ["glibc"] }` to pick the right binary. GPU detection should follow the same pattern rather than inventing a parallel mechanism in the addon package.
- **Ecosystem value**: Other tools need GPU awareness too. Machine learning frameworks, GPU compute libraries, and graphics tools all have platform-specific binaries. Making GPU a first-class platform dimension lets any recipe filter on it, not just tsuku-llm.
- **Supply chain security**: Manifest is currently embedded at compile time. Moving to the recipe system changes the security model. Each approach has trade-offs that need explicit analysis.
- **Self-contained philosophy**: Users shouldn't need to know their GPU vendor or install SDKs manually. Detection should be automatic.
- **Existing CI pipeline**: 10 variants are already built. The solution should use what exists.
- **Download size**: Binaries are 50-200MB each. Bundling all variants would mean 500MB+ downloads.

## Considered Options

### Decision 1: Where GPU Awareness Lives

GPU variant selection needs to happen somewhere. The question is whether it lives in the addon package (tool-specific) or the platform package (system-wide).

The addon currently has its own manifest, its own platform key format (`linux-amd64` vs the recipe system's `linux/amd64`), its own download and verification code, and zero overlap with the recipe system. Adding GPU detection to the addon would mean writing detection code that only benefits tsuku-llm. Adding it to the platform package makes it available to every recipe.

#### Chosen: Extend the platform package

Add GPU vendor detection alongside the existing OS, architecture, Linux family, and libc detection. The `platform.Target` struct gains a `gpu` field. `DetectTarget()` gains a GPU detection step. The `Matchable` interface gains a `GPU() string` method.

This follows the exact pattern established by libc detection:
- `DetectLibc()` reads an ELF binary to determine glibc vs musl
- `DetectGPU()` reads PCI sysfs to determine GPU vendor
- Both return a string that flows through `Matchable` into `WhenClause.Matches()`

The detection runs once per command invocation (same as libc), at `DetectTarget()` time. The result flows into plan generation, where `WhenClause` matching uses it to filter recipe steps.

#### Alternatives Considered

**Tool-specific detection in the addon package**: Keep detection in `internal/llm/addon/` with its own manifest schema and download code.
Rejected because it creates a parallel platform detection path that only benefits one tool. The addon already duplicates platform logic (`PlatformKey()` vs `platform.Target`). Adding GPU detection there deepens the divergence. Any future tool that needs GPU filtering would face the same problem.

**No detection (manual config only)**: Require users to set `llm.backend` explicitly.
Rejected as the sole mechanism because it violates tsuku's self-contained philosophy. Included as an override for edge cases.

### Decision 2: How to Detect GPU Hardware

The platform package needs to identify what GPU hardware is present. The detection should return the most basic, immutable fact about the system: what GPU vendor's hardware is installed. Not which drivers are loaded, not which APIs are available, just the hardware.

This matters because drivers can be installed or removed, but the physical GPU doesn't change. Detecting at the hardware level means the result is stable across driver installations and upgrades.

#### Chosen: PCI vendor identification via sysfs (Linux) and system profiler (macOS)

On Linux, read PCI device information from sysfs:
1. Scan `/sys/bus/pci/devices/*/class` for display controller class codes (`0x0300xx` for VGA, `0x0302xx` for 3D controller)
2. For each GPU-class device, read `/sys/bus/pci/devices/*/vendor` for the PCI vendor ID
3. Map vendor IDs: `0x10de` = NVIDIA, `0x1002` = AMD, `0x8086` = Intel

When multiple GPUs are present (common: Intel iGPU + NVIDIA dGPU), prefer discrete GPUs over integrated. Priority: NVIDIA > AMD > Intel. The result is a single vendor string.

On macOS, Apple Silicon always has an Apple GPU. Intel Macs may have AMD discrete GPUs or Intel integrated. Detection uses `system_profiler SPDisplaysDataType` or the IOKit framework.

On Windows, return `"none"` for now (only CPU variant exists).

This approach uses only stdlib filesystem reads. No drivers, no shared libraries, no subprocess spawning. The sysfs files are world-readable and available even before any GPU driver is installed.

**Values returned by `GPU()`**: `nvidia`, `amd`, `intel`, `apple`, `none`

#### Alternatives Considered

**Library file probing** (`os.Stat` on `/usr/lib/libcuda.so`): Check for GPU runtime libraries at known filesystem paths.
Rejected because it detects software, not hardware. A system with an NVIDIA GPU but no CUDA toolkit would probe as "no GPU." Path lists are distro-specific (Debian puts libraries in `/usr/lib/x86_64-linux-gnu/`, Fedora in `/usr/lib64/`, Arch in `/usr/lib/`). And library presence doesn't mean the library works; CUDA might be installed but the driver version incompatible.

**Shell out to `nvidia-smi` / `lspci`**: Run GPU query tools and parse output.
Rejected because these tools may not be installed. `nvidia-smi` ships with NVIDIA drivers (not present before driver install), and `lspci` requires the `pciutils` package. Subprocess spawning adds latency and error handling complexity. The sysfs approach gives the same information without external tools.

### Decision 3: How Tools Use GPU Information

With GPU detection in the platform package, the question is how recipes consume it. The existing pattern for platform-specific filtering is `WhenClause` matching: recipe steps declare conditions, the plan generator filters steps that don't match the current target.

The addon currently has its own manifest schema, its own download code, and no connection to the recipe system. There are two paths: extend the addon's custom system, or migrate to the recipe system.

#### Chosen: Extend WhenClause with `gpu` field; convert tsuku-llm to a recipe

Add a `gpu` field to `WhenClause` following the same pattern as `libc`:

```go
type WhenClause struct {
    Platform       []string `toml:"platform,omitempty"`
    OS             []string `toml:"os,omitempty"`
    Arch           string   `toml:"arch,omitempty"`
    LinuxFamily    string   `toml:"linux_family,omitempty"`
    PackageManager string   `toml:"package_manager,omitempty"`
    Libc           []string `toml:"libc,omitempty"`
    GPU            []string `toml:"gpu,omitempty"`            // NEW
}
```

Matching semantics: if `GPU` is non-empty and the target's `GPU()` returns a non-empty string, the target's GPU value must be in the list. Same AND semantics as other fields.

Then convert tsuku-llm from an addon with an embedded manifest into a standard recipe. The recipe uses `when` clauses with the `gpu` field for variant selection:

```toml
[metadata]
name = "tsuku-llm"
description = "Local LLM inference engine"

[version]
source = "github_releases"
github_repo = "tsukumogami/tsuku-llm"

# macOS: Metal variant (all Mac GPUs)
[[steps]]
action = "github_file"
when = { os = ["darwin"], arch = "arm64" }
repo = "tsukumogami/tsuku-llm"
asset_pattern = "tsuku-llm-v{version}-darwin-arm64"

[[steps]]
action = "github_file"
when = { os = ["darwin"], arch = "amd64" }
repo = "tsukumogami/tsuku-llm"
asset_pattern = "tsuku-llm-v{version}-darwin-amd64"

# Linux AMD64: Vulkan for any GPU (avoids CUDA driver coupling)
[[steps]]
action = "github_file"
when = { os = ["linux"], arch = "amd64", gpu = ["nvidia", "amd", "intel"] }
dependencies = ["vulkan-loader"]
repo = "tsukumogami/tsuku-llm"
asset_pattern = "tsuku-llm-v{version}-linux-amd64-vulkan"

# Linux AMD64: CPU for systems without a GPU
[[steps]]
action = "github_file"
when = { os = ["linux"], arch = "amd64", gpu = ["none"] }
repo = "tsukumogami/tsuku-llm"
asset_pattern = "tsuku-llm-v{version}-linux-amd64-cpu"

# Linux ARM64: same pattern
[[steps]]
action = "github_file"
when = { os = ["linux"], arch = "arm64", gpu = ["nvidia", "amd", "intel"] }
dependencies = ["vulkan-loader"]
repo = "tsukumogami/tsuku-llm"
asset_pattern = "tsuku-llm-v{version}-linux-arm64-vulkan"

[[steps]]
action = "github_file"
when = { os = ["linux"], arch = "arm64", gpu = ["none"] }
repo = "tsukumogami/tsuku-llm"
asset_pattern = "tsuku-llm-v{version}-linux-arm64-cpu"

# Windows: CPU only
[[steps]]
action = "github_file"
when = { os = ["windows"], arch = "amd64" }
repo = "tsukumogami/tsuku-llm"
asset_pattern = "tsuku-llm-v{version}-windows-amd64-cpu.exe"

[verify]
command = "tsuku-llm --version"
pattern = "{version}"
```

The `gpu` conditions are mutually exclusive because `GPU()` returns exactly one value. On a system with an NVIDIA GPU, `GPU()` returns `"nvidia"`, which matches the Vulkan step (since `"nvidia"` is in `["nvidia", "amd", "intel"]`). On a system with no GPU, `GPU()` returns `"none"`, which matches the CPU step. No step matches both.

The recipe maps all GPU vendors to the Vulkan variant on Linux. This is a product decision: Vulkan works across NVIDIA, AMD, and Intel without driver version coupling. Users who want CPU-only can override via `llm.backend = cpu` config.

Step-level `dependencies = ["vulkan-loader"]` pulls in the Vulkan loader library only when the Vulkan step matches. On no-GPU systems, no driver dependency is installed. This uses existing recipe dependency filtering (steps that don't match the target have their dependencies skipped).

The addon lifecycle code (server start/stop, gRPC socket, health checks, idle timeout) stays in `internal/llm/`. It doesn't move to the recipe system because daemon management has no recipe equivalent. What changes is how it finds the binary: instead of downloading via embedded manifest, it looks for the recipe-installed binary at the standard `$TSUKU_HOME/tools/tsuku-llm-<version>/` path.

#### Alternatives Considered

**Expand the addon's custom manifest schema**: Keep the embedded `manifest.json` and add a nested variant map with backend dimensions.
Rejected because it deepens the divergence between the addon and recipe systems. The addon already duplicates platform detection, download logic, and verification code. Adding a custom schema for GPU variants means maintaining two parallel distribution paths. Converting to a recipe eliminates this duplication and gives tsuku-llm the same install experience as every other tool.

**Add when-clause-style filtering to the addon manifest but keep it separate**: The manifest gains `when`-like conditions without fully becoming a recipe.
Rejected for the same reason as above, just more incrementally. If we're going to add conditional filtering, we should use the system that already has it rather than reimplementing it.

### Decision 4: Runtime Failure Handling

Even with good pre-download detection, things can go wrong. A Vulkan library might exist on disk but the GPU might not support the required Vulkan version. CUDA libraries might be present but the driver version incompatible.

#### Chosen: Informative error + manual override (automatic fallback deferred)

When the Rust binary can't initialize its compiled-in GPU backend, it logs a clear error message to stderr explaining what failed and suggesting the config override: `tsuku config set llm.backend cpu`. The Go side surfaces this message to the user.

This is deliberately simple. Automatic fallback (detecting the failure, reinstalling the CPU variant, and relaunching) adds `BackendFailedError` types, Rust exit code changes, process exit monitoring in `waitForReady()`, and retry logic in `Complete()`. That's significant complexity for a case that should be uncommon once hardware detection works. Ship detection + manual override first, measure how often detection picks wrong in practice, then add automatic fallback if needed.

#### Alternatives Considered

**Automatic CPU fallback**: Detect backend failure via exit code 78, auto-install and launch CPU variant.
Deferred (not rejected). This is the right long-term answer but adds complexity that isn't justified until we know how accurate sysfs detection is. Tracked as a future enhancement.

**Pre-download CPU alongside GPU**: Always download both the GPU variant and CPU variant.
Rejected because it doubles download size for every Linux user to cover an uncommon failure case.

## Decision Outcome

### Summary

The platform detection system gains GPU vendor identification as a new dimension alongside OS, architecture, Linux family, and libc. On Linux, detection reads PCI device class and vendor files from sysfs, returning one of `nvidia`, `amd`, `intel`, or `none`. On macOS, detection returns `apple`. The `Matchable` interface gains `GPU() string`, `platform.Target` gains a `gpu` field populated during `DetectTarget()`, and `WhenClause` gains a `gpu []string` filter.

tsuku-llm becomes a standard recipe with `when` clauses that filter on the `gpu` field. All GPU vendors get the Vulkan variant on Linux (avoiding CUDA driver coupling), no-GPU systems get the CPU variant, and macOS gets Metal. The Vulkan loader becomes a library recipe dependency, verified via `require_command` (following the same no-sudo pattern as the existing `cuda.toml` recipe).

The addon lifecycle code stays in `internal/llm/`. It still manages the gRPC server, socket, health checks, and idle timeout. But binary installation moves from the embedded manifest to the recipe system. `EnsureAddon()` checks whether `tsuku-llm` is installed via the recipe system, triggers installation if needed, and finds the binary at the standard tools path.

Users who want to force the CPU variant set `llm.backend = cpu` in `$TSUKU_HOME/config.toml`. The LLM lifecycle code handles this override by setting the target's GPU to `"none"` before plan generation, which selects the CPU recipe step. CUDA variant selection is deferred until benchmarks validate the Vulkan-default choice.

When a GPU variant fails at runtime, the Rust binary logs a clear error to stderr suggesting `tsuku config set llm.backend cpu` as a workaround. Automatic fallback is deferred.

### Rationale

These decisions reinforce each other. Platform-level GPU detection means the recipe system can filter on GPU without any special cases. The recipe's `when` clauses provide variant selection using the same mechanism that already handles libc, Linux family, and architecture filtering. Step-level dependencies mean driver packages are only installed when the matching step activates.

Converting tsuku-llm to a recipe eliminates the only non-recipe binary distribution path in tsuku. The addon package loses its embedded manifest, download code, platform key translation, and verification code. What remains is the daemon lifecycle, which is genuinely unique to tsuku-llm (no other recipe manages a long-running server).

The Vulkan-by-default choice for all Linux GPU vendors simplifies the recipe (one GPU step instead of one per vendor) and avoids CUDA driver version issues. The `llm.backend = cpu` override covers the main failure case (GPU detection correct but backend doesn't work), and CUDA variant selection can be added later once benchmarks and a recipe variant mechanism make it practical.

## Solution Architecture

### Platform Detection

New file `internal/platform/gpu.go`:

```go
// DetectGPU returns the primary GPU vendor for the current system.
// Returns one of: "nvidia", "amd", "intel", "apple", "none".
//
// On Linux, scans PCI devices via sysfs for display controllers.
// When multiple GPUs are present, prefers discrete over integrated
// (nvidia > amd > intel).
//
// On macOS, returns "apple" unconditionally (Apple GPU or Metal-capable Intel/AMD).
// On Windows, returns "none" (no GPU variants built yet).
func DetectGPU() string
```

With platform-specific implementations:

**`gpu_linux.go`**: Reads `/sys/bus/pci/devices/*/class` for GPU device classes (`0x0300xx` VGA, `0x0302xx` 3D controller). For each GPU device, reads `vendor` file and maps PCI vendor IDs: `0x10de` → `nvidia`, `0x1002` → `amd`, `0x8086` → `intel`. When multiple vendors are present, returns the highest-priority discrete GPU. Uses only `os.ReadFile` and `filepath.Glob`.

**`gpu_darwin.go`**: Returns `"apple"` unconditionally. Apple Silicon has Apple GPU; Intel Macs have Metal-capable GPUs. No variant selection needed on macOS.

**`gpu_windows.go`**: Returns `"none"`. Only CPU variant exists for Windows.

`DetectTarget()` in `family.go` gains a GPU detection step:

```go
func DetectTarget() (Target, error) {
    platform := runtime.GOOS + "/" + runtime.GOARCH
    gpu := DetectGPU()
    if runtime.GOOS != "linux" {
        return NewTarget(platform, "", "", gpu), nil
    }
    family, err := DetectFamily()
    if err != nil {
        return Target{}, err
    }
    libc := DetectLibc()
    return NewTarget(platform, family, libc, gpu), nil
}
```

### Matchable Interface and WhenClause Extension

`recipe/types.go` changes:

```go
type Matchable interface {
    OS() string
    Arch() string
    LinuxFamily() string
    Libc() string
    GPU() string  // NEW: "nvidia", "amd", "intel", "apple", "none"
}
```

Both implementations update:
- `platform.Target` gains a `gpu` field and `GPU() string` method
- `recipe.MatchTarget` gains a `gpu` field, updated constructor, and `GPU() string` method

`WhenClause.Matches()` gains a GPU check following the same pattern as libc:

```go
// In WhenClause.Matches():
if len(w.GPU) > 0 {
    gpu := target.GPU()
    gpuMatch := false
    for _, g := range w.GPU {
        if g == gpu {
            gpuMatch = true
            break
        }
    }
    if !gpuMatch {
        return false
    }
}
```

### Addon Lifecycle Refactor

The `internal/llm/addon/` package loses:
- `manifest.go` and `manifest.json` (embedded manifest, Go types, parsing)
- `platform.go` (`PlatformKey()`, `GetCurrentPlatformInfo()`)
- `download.go` (download, verify, atomic rename logic)
- `verify.go` (SHA256 verification, `VerifyBeforeExecution()`)

The addon package retains:
- `manager.go` (refactored: `EnsureAddon()` delegates to recipe system)
- `lifecycle.go` (server start/stop, socket, health checks, idle timeout)

`EnsureAddon()` changes from "download binary via embedded manifest" to:

```go
func (m *AddonManager) EnsureAddon(ctx context.Context) (string, error) {
    // Check if tsuku-llm is already installed
    binaryPath := m.findInstalledBinary()
    if binaryPath != "" {
        return binaryPath, nil
    }

    // Install via recipe system
    if err := m.installViaRecipe(ctx); err != nil {
        return "", fmt.Errorf("installing tsuku-llm: %w", err)
    }

    return m.findInstalledBinary()
}
```

`findInstalledBinary()` looks for the tsuku-llm binary at the standard recipe installation path. This could check `state.json` for the installed version, or look for the binary in `$TSUKU_HOME/tools/tsuku-llm-<version>/`.

`installViaRecipe()` loads the tsuku-llm recipe, generates a plan for the current target (applying `llm.backend` override if set), and executes it. This reuses the existing executor pipeline.

The `llm.backend` override works by modifying the target's GPU value before plan generation:

| Config value | Target GPU override | Effect |
|---|---|---|
| (unset) | (no override, use detected value) | Auto-selects Vulkan or CPU based on hardware |
| `cpu` | `none` | Forces CPU step |

Only the `cpu` override is needed initially because all GPU vendors map to Vulkan. CUDA variant selection is deferred until benchmarks validate the Vulkan-by-default choice. If CUDA support is added later, it will require a recipe variant mechanism (either a new recipe step with disambiguation, or a general variant selection feature) rather than simple GPU override.

**Edge case**: If a user sets `llm.backend = cpu` on a system that already has the Vulkan variant installed, the LLM code should reinstall with the CPU variant. If a user sets a value that doesn't map to any recipe step (e.g., `vulkan` on a no-GPU system where GPU is `"none"`), the LLM code should warn and fall back to the detected value.

### GPU Driver Library Recipes

GPU drivers become library recipes, installed as conditional dependencies of the tsuku-llm recipe's GPU steps.

**`recipes/v/vulkan-loader.toml`** (sketch):
```toml
[metadata]
name = "vulkan-loader"
description = "Vulkan ICD loader library"
type = "library"
supported_os = ["linux"]

# Verify Vulkan loader is installed (tsuku doesn't install system libraries)
[[steps]]
action = "require_command"
command = "ldconfig"
version_flag = "-p"
version_regex = "libvulkan\\.so\\.1"
note = "Install via your system package manager: apt install libvulkan1 (Debian/Ubuntu), dnf install vulkan-loader (Fedora), pacman -S vulkan-icd-loader (Arch)"

[verify]
command = "ldconfig -p"
mode = "output"
pattern = "libvulkan"
```

This follows the same pattern as the existing `cuda.toml` recipe: verify the dependency exists, tell the user how to install it if missing, but don't run sudo commands. Most Linux systems with a GPU already have the Vulkan loader installed via the desktop environment or GPU driver packages.

The tsuku-llm recipe's Vulkan steps declare `dependencies = ["vulkan-loader"]`. When the step matches (any GPU vendor), the loader is verified first. When the CPU step matches (no GPU), no driver dependency is checked.

### Rust-side Error Reporting

When the Rust binary detects that its compiled-in backend doesn't match available hardware, it logs a structured error to stderr:

```
ERROR: Backend "vulkan" failed to initialize.
  Detected hardware supports: None
  Suggestion: tsuku config set llm.backend cpu
```

This is an informational message, not a protocol change. The Go side doesn't parse it. Automatic fallback (exit code 78, reinstall, relaunch) is deferred to a follow-up design.

### Legacy Cleanup

The deprecated `AddonPath()` at `manager.go:232` and its caller in `lifecycle.go` build paths for the old directory layout. These are removed as part of the addon refactor. `NewServerLifecycleWithManager()` accepts the binary path from the recipe-installed location instead of computing it from the addon manifest.

The embedded `manifest.json` and all its supporting code are removed. The platform key translation (`linux-amd64` vs `linux/amd64`) is no longer needed because the recipe system uses the standard `platform.Target` format.

## Implementation Approach

### Phase 1: GPU Platform Detection + WhenClause Extension

This phase can ship independently. It adds a new platform dimension and recipe filtering capability without changing the addon system.

1. Add `gpu.go` with `DetectGPU()` function signature
2. Add `gpu_linux.go` with PCI sysfs scanning (accept test root parameter for mock sysfs)
3. Add `gpu_darwin.go` (returns `"apple"`)
4. Add `gpu_windows.go` (returns `"none"`)
5. Add `gpu` field to `platform.Target`, update `NewTarget()` and `DetectTarget()`
6. Add `GPU() string` to `Matchable` interface
7. Update `recipe.MatchTarget` with `gpu` field and `NewMatchTarget()` constructor

**Constructor cascade**: `NewTarget()` gains a `gpu` parameter. Production callsites that must update:
- `executor/plan_generator.go:111` and `:668` (plan generation)
- `cmd/tsuku/install_sandbox.go:94`, `verify.go:573`, `verify_deps.go:74` (commands)
- `internal/builders/orchestrator.go:260` (pipeline validation)
- `cmd/tsuku/info.go:447`, `sysdeps.go:212` (info display)

`NewMatchTarget()` gains a `gpu` parameter. Update all callers in `executor/` and `cmd/tsuku/`.

To minimize churn across test files, consider adding a `TargetWithGPU(target Target, gpu string) Target` helper or a `SetGPU` setter so existing tests that don't care about GPU can pass `""` without changing constructor calls everywhere.

8. Add `GPU []string` to `WhenClause`, update `Matches()`, `IsEmpty()`, `ToMap()`, and TOML unmarshaling
9. Update `MergeWhenClause()` for GPU constraint checking
10. Update `PlanConfig` with `GPU` field, pass through to target construction in `GeneratePlan()`
11. Unit tests for GPU detection (mock sysfs directory structure)
12. Unit tests for WhenClause GPU matching

### Phase 2: tsuku-llm Recipe + Addon Refactor

Depends on Phase 1 being validated (sysfs detection works correctly on target systems). Start only after Phase 1 ships and we have confidence in GPU detection accuracy.

1. Create `recipes/t/tsuku-llm.toml` with GPU-filtered steps (Vulkan + CPU only, no CUDA initially)
2. Add `llm.backend` config key to `userconfig` (Get/Set/AvailableKeys) — initially only `cpu` override
3. Add `LLMBackend() string` to `LLMConfig` interface in `factory.go`
4. Refactor `AddonManager`: remove manifest/download code, add recipe-based installation via injected `Installer` interface (not direct import of `internal/executor/`)
5. Update `EnsureAddon()` to check recipe installation state and trigger install if needed
6. Wire `llm.backend = cpu` override into plan generation (set target GPU to `"none"`)
7. Update `ServerLifecycle` to accept binary path from recipe installation
8. Remove embedded `manifest.json`, `platform.go`, `download.go`, `verify.go` from addon package
9. Integration test: verify correct variant selection on current host

### Phase 3: GPU Driver Library Recipes

1. Create `recipes/v/vulkan-loader.toml` (library recipe, `require_command` pattern — no sudo)
2. Test dependency chain: tsuku-llm → vulkan-loader on a GPU system
3. Test that missing Vulkan loader gives a clear error with installation instructions

### Phase 4: Testing and Benchmarking

1. End-to-end test: `tsuku install tsuku-llm` on systems with different GPU vendors
2. Test `llm.backend` config override path
3. Test no-GPU fallback to CPU variant
4. Benchmark Vulkan vs CUDA on NVIDIA hardware for shipped models

### Benchmark Gate

Before shipping Vulkan as the default Linux GPU backend, benchmark Vulkan vs CUDA performance on the models we ship (0.5B, 1.5B, 3B). If the gap exceeds 25% on tokens/second, reconsider the default. This benchmark can be informal (run both variants on the same machine, compare throughput) but must happen before this design is marked Current.

## Security Considerations

### Download verification

With the addon, SHA256 checksums are embedded in the Go binary at compile time via `//go:embed manifest.json`. This is tamper-proof: you can't change the checksums without rebuilding tsuku.

With the recipe approach, checksums live in the recipe file (`recipes/t/tsuku-llm.toml`). Recipe checksums are git-tracked and CI-validated, but they can be updated via `tsuku update-registry`. This is the same security model used by every other recipe in tsuku (all 200+ tools). If the registry is compromised, all tools are at risk, not just tsuku-llm.

The pre-execution SHA256 verification (`VerifyBeforeExecution`) can be retained by reading the plan's stored checksum and re-verifying before each server launch. This catches binary tampering after installation.

The net security change: tsuku-llm moves from "checksums embedded in binary" (stronger, specific to this one tool) to "checksums in recipe" (standard model, consistent with everything else). This is a deliberate trade-off for consistency and maintainability.

### Execution isolation

No change to the runtime model. The tsuku-llm binary runs with the same permissions as the tsuku process.

GPU detection uses `os.ReadFile` on sysfs files (`/sys/bus/pci/devices/*/class`, `*/vendor`). These files are world-readable. No special permissions needed, no libraries loaded, no processes spawned.

### Supply chain risks

The release pipeline produces signed binaries with checksums. Adding more variants (from 5 to 10 entries in the recipe) doesn't change the signing infrastructure. Each variant's URL points to the same GitHub release page. The `github_file` action computes checksums dynamically during plan generation (standard recipe behavior), and the plan is cached.

One new risk surface: the GPU driver library recipes (`vulkan-loader`, `cuda-runtime`) install system packages via `apt_install`, `dnf_install`, etc. These trust the system's package manager and its configured repositories. This is the same trust model used by existing system-dependency recipes.

### User data exposure

No change. GPU detection reads only PCI device metadata from sysfs (vendor ID, device class). No network calls beyond the existing download flow. The `llm.backend` config value is stored in `$TSUKU_HOME/config.toml` alongside other settings.

## Consequences

### Positive

- GPU becomes a first-class platform dimension. Any recipe can use `when = { gpu = ["nvidia"] }` to filter steps, not just tsuku-llm.
- Users with GPUs automatically get GPU-accelerated inference without manual configuration.
- Download size stays small (one variant per user).
- tsuku-llm becomes a regular recipe, removing the only non-recipe binary distribution path in tsuku.
- The addon package shrinks significantly. Manifest parsing, download logic, platform key translation, and verification code are removed.
- GPU driver packages become installable library recipes, bringing driver management into the same system as everything else.

### Negative

- The security model weakens slightly: embedded checksums (compile-time guarantee) are replaced by recipe checksums (git-tracked, CI-validated, but updatable). This is the standard model for all other tools. The plan cache is user-writable, so local tampering after installation is not caught unless pre-execution verification re-checks against a trusted source.
- The addon refactor is a significant change. `EnsureAddon()` changes from direct download to recipe system delegation. The addon package must use an injected installer interface rather than importing `internal/executor/` directly, to preserve dependency direction.
- PCI sysfs detection is Linux-specific. macOS returns a constant (`apple`), Windows returns `none`. If Windows GPU support is needed later, a new detection backend (DXGI or WMI) must be added.
- Existing tsuku-llm installations re-download on first run after upgrading, since the binary moves from the addon path to the recipe tools path.
- CUDA variant selection is deferred. Users who specifically want CUDA over Vulkan must wait for benchmarks and a follow-up design that adds a CUDA step without creating matching ambiguity in the recipe system.

### Risks

- We haven't benchmarked Vulkan vs CUDA for the models we ship. The benchmark gate (25% threshold) must pass before marking this design Current.
- PCI sysfs detection doesn't verify driver functionality. A system with an NVIDIA GPU but broken drivers would get the Vulkan variant, which might also fail. The manual `llm.backend cpu` override is the escape hatch.
- The addon-to-recipe migration touches the LLM initialization path, which is timing-sensitive (server startup, socket readiness, health checks). The refactor must preserve the existing lifecycle guarantees.
- Adding `GPU() string` to the `Matchable` interface is a breaking change. Both implementations are internal, but any code that constructs `MatchTarget` or `Target` must be updated.
