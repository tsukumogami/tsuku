---
status: Proposed
problem: |
  The tsuku-llm addon builds 10 platform variants across GPU backends (CUDA, Vulkan, Metal, CPU), but the addon manifest and download system map each OS-architecture pair to a single binary. There's no mechanism to select which GPU variant to download, no hardware probing before download, and no fallback when a GPU backend fails at runtime. Users with NVIDIA GPUs get the wrong binary, and CUDA driver mismatches fail silently.
decision: |
  Expand the manifest schema to a two-level lookup (platform then backend variant) and add Go-side GPU library probing before download to select the right variant. Vulkan is the preferred GPU backend on Linux to avoid CUDA driver version coupling, with CUDA available as a user override. Automatic runtime fallback is deferred to a follow-up design after measuring detection accuracy in practice.
rationale: |
  Go-side library probing is fast, requires no extra downloads, and catches the common case (GPU libraries present on disk). Vulkan as default avoids the CUDA toolkit/driver version matrix that caused silent failures on development machines. Separate per-backend binaries (already built by CI) keep downloads small and supply chain verification simple. Deferring automatic runtime fallback keeps the initial scope tight while the config override provides a manual escape hatch for detection errors.
---

# Design Document: GPU Backend Selection

## Status

Proposed

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

This creates several concrete problems:

**Wrong binary for the hardware.** On Linux, there's no way to specify whether to download the CUDA, Vulkan, or CPU variant. The manifest points to one URL per platform. A user with an NVIDIA GPU gets the same binary as a user with no GPU at all.

**Detection happens too late.** Hardware detection (`hardware.rs`) runs inside the Rust binary at server startup. By that point, the binary is already downloaded. If it was compiled with CUDA but the system only has Vulkan, the detection code correctly identifies Vulkan but the binary can't use it because it was compiled with `--features cuda`.

**CUDA version coupling.** The CI builds against CUDA 12.4. Users need a compatible NVIDIA driver (>= 525.60). If the driver is older, CUDA initialization fails and the binary silently falls back to CPU with no indication of what happened. We hit this on our own development workstation: `ggml_cuda_init: failed to initialize CUDA: forward compatibility was attempted on non supported HW`.

**Vulkan VRAM detection returns zero.** `detect_vulkan()` in `hardware.rs` always returns `vram_bytes=0`. The model selector then picks the smallest model (0.5B) for all Vulkan users regardless of actual VRAM. This is a separate bug (tracked independently) but it compounds the backend selection issue: even when Vulkan is correctly selected, the model is under-provisioned.

### Scope

**In scope:**
- Manifest schema expansion to support multiple variants per platform
- Go-side GPU hardware detection before addon download
- Backend variant selection logic (automatic + user override)
- Cleanup of deprecated `AddonPath()` and related legacy functions that will break with the new directory layout

**Out of scope:**
- Automatic runtime fallback when GPU backend fails (deferred until detection accuracy is measured in practice; manual override via `llm.backend` config is the escape hatch)
- Vulkan VRAM detection fix (standalone Rust bug, separate issue)
- Windows GPU support (only CPU variant exists today)
- Shipping multiple CUDA versions (e.g., CUDA 11 + CUDA 12)
- Dynamic backend loading within a single binary (llama.cpp's `GGML_BACKEND_DL` mode)
- Model selection changes (handled by existing `model.rs` logic once backend/VRAM are correct)

## Decision Drivers

- **Supply chain security**: Manifest is embedded at compile time. Each variant needs its own SHA256 checksum. Verification flow (download → checksum → chmod → atomic rename) must stay intact.
- **Self-contained philosophy**: Users shouldn't need to know their GPU backend or install SDKs. Detection should be automatic.
- **Existing CI pipeline**: 10 variants are already built. The solution should use what exists rather than restructure the build.
- **Download size**: Binaries are 50-200MB each. Bundling all variants would mean 500MB+ downloads for Linux users.
- **Correctness over performance**: Better to download the CPU variant that works than the CUDA variant that silently falls back to CPU anyway.
- **macOS simplicity**: Apple Silicon always uses Metal. Intel Macs use Metal too. No variant selection needed on macOS.

## Considered Options

### Decision 1: Distribution Model

The release pipeline already builds separate binaries per backend (one for CUDA, one for Vulkan, one for CPU on each Linux architecture). The question is whether to keep shipping separate binaries or consolidate into a single binary that loads backends at runtime.

This matters because it determines whether we need manifest schema changes at all, and it directly affects download size and supply chain verification complexity.

#### Chosen: Separate per-backend binaries

Keep the current model where each variant is a distinct binary compiled with exactly one feature flag (`--features cuda`, `--features vulkan`, or no features for CPU). The manifest expands to list all variants. Users download only the one they need.

This fits naturally with the existing CI pipeline, which already produces these separate binaries. Each binary gets its own SHA256 checksum, so the verification flow doesn't change structurally. Download size stays reasonable (one binary per user, not all ten).

The compile-time guarantee is valuable: a CUDA binary links `libggml-cuda` statically, a Vulkan binary links `libggml-vulkan`. There's no ambiguity about which backend is active. If initialization fails, we know exactly which backend failed.

#### Alternatives Considered

**Single bundled binary (Ollama model)**: Ship one binary per platform that includes all backend libraries. At runtime, probe each backend subprocess-style and use the first one that works.
Rejected because download size would be 500MB+ for Linux (CUDA libs alone are large), and tsuku-llm isn't a persistent server like Ollama where amortizing a slow first-run probe makes sense. tsuku-llm starts on demand for inference and shuts down after idle timeout. The probe cost would be paid repeatedly.

**Dynamic backend loading**: Compile the main binary without GPU features, ship backend `.so` files separately, load them at runtime via `GGML_BACKEND_DL`.
Rejected because this adds path management complexity (where do `.so` files live? how are they verified?), it's not the default mode for llama.cpp releases, and it blurs the supply chain story. Each `.so` would need separate checksum verification and the extraction flow becomes multi-file.

### Decision 2: Manifest Schema

The current manifest maps `"GOOS-GOARCH"` to a single `{url, sha256}` pair. We need to add a backend dimension. The schema change affects `manifest.json`, the Go types (`Manifest`, `PlatformInfo`), and every function that looks up platform info.

This is the foundational change that everything else builds on. Get it wrong and every downstream component gets more complex.

#### Chosen: Nested variant map with default

Expand each platform entry to contain a map of backend variants plus a default backend name:

```json
{
  "version": "0.2.0",
  "platforms": {
    "darwin-arm64": {
      "default": "metal",
      "variants": {
        "metal": { "url": "...", "sha256": "..." }
      }
    },
    "linux-amd64": {
      "default": "cpu",
      "variants": {
        "cuda": { "url": "...", "sha256": "..." },
        "vulkan": { "url": "...", "sha256": "..." },
        "cpu": { "url": "...", "sha256": "..." }
      }
    }
  }
}
```

The Go types become:

```go
type Manifest struct {
    Version   string                    `json:"version"`
    Platforms map[string]PlatformEntry  `json:"platforms"`
}

type PlatformEntry struct {
    Default  string                    `json:"default"`
    Variants map[string]VariantInfo    `json:"variants"`
}

type VariantInfo struct {
    URL    string `json:"url"`
    SHA256 string `json:"sha256"`
}
```

Lookup becomes two-step: `platforms[platformKey].variants[backend]`. The `default` field provides a safe fallback when GPU detection fails or returns an unsupported backend. For macOS, there's only one variant (metal) so the nesting is minimal overhead.

The schema version bump from the existing format signals to any parsing code that the structure has changed.

#### Alternatives Considered

**Flat composite keys** (`"linux-amd64-cuda"`, `"linux-amd64-vulkan"`): Extend the platform key to include the backend.
Rejected because it requires prefix scanning to enumerate available backends for a platform, and there's no natural place for the `default` field. The nested schema makes the platform-to-variants relationship explicit in the data structure, which is clearer for both the detection code and human readers of the manifest.

**Priority-ordered variant list**: Map each platform to an array of `{backend, url, sha256}` entries, ordered by preference.
Rejected because arrays require linear scan for lookup by backend name, and the ordering conflates download priority with data structure. Priority should be determined by the detection logic, not baked into the manifest schema.

### Decision 3: Pre-download Backend Selection

The Go-side addon manager needs to pick a backend variant before downloading. Currently, all hardware detection lives in `hardware.rs` (Rust), which runs after the binary is already on disk. We need some form of GPU detection in Go.

The detection doesn't need to be perfect. It just needs to pick a reasonable variant. If it picks wrong, the runtime fallback chain (Decision 4) handles it.

#### Chosen: Go-side library file probing with config override

Port the library-probing logic from `hardware.rs` to Go. Check for GPU libraries at known filesystem paths:

**CUDA detection (Linux)**:
- Probe standard library paths across distros: Debian/Ubuntu (`/usr/lib/x86_64-linux-gnu/`), Fedora/RHEL (`/usr/lib64/`), Arch (`/usr/lib/`), and CUDA toolkit (`/usr/local/cuda/lib64/`)
- Check for `libcuda.so.1` or `libcuda.so`
- If found: CUDA is available

**Vulkan detection (Linux)**:
- Probe same distro-specific paths for `libvulkan.so.1` or `libvulkan.so`
- If found: Vulkan is available

**Limitations**: Library probing checks file existence, not driver functionality. In containers with GPU passthrough, libraries may be bind-mounted at non-standard paths. The `llm.backend` config override handles these edge cases.

**Metal detection (macOS)**:
- ARM64: always available (Apple Silicon)
- AMD64: always available (Metal supported on Intel Macs)

**Windows**:
- CPU only for now (no GPU variants built)

The detection returns a list of available backends. The selection priority for Linux is: **Vulkan > CUDA > CPU**. This is deliberately different from the Rust-side priority (which puts CUDA first) because:

1. Vulkan works across NVIDIA, AMD, and Intel GPUs without driver version coupling
2. CUDA requires a compatible driver version (>= 525.60 for CUDA 12)
3. For most users, Vulkan performance is close enough to CUDA for 1-3B parameter models
4. Users who specifically want CUDA can set `llm.backend = cuda` in config

The user override via `llm.backend` in `$TSUKU_HOME/config.toml` lets power users force a specific backend. This follows the existing config pattern from the secrets manager.

The selection flow:
1. Check `llm.backend` config override. If set, use it.
2. Run library probing for the current platform.
3. Pick the highest-priority available backend.
4. Look up that backend in the manifest's variant map.
5. If the backend isn't in the manifest, fall back to `default`.

#### Alternatives Considered

**Shell out to nvidia-smi / vulkaninfo**: Run GPU query tools and parse their output.
Rejected because these tools may not be installed even when the libraries are. `nvidia-smi` ships with NVIDIA drivers but `vulkaninfo` is a separate package. Subprocess spawning adds latency and error handling complexity. Library file probing is faster and sufficient for "is this backend likely to work?"

**Download a tiny probe binary**: Ship a small static binary in the manifest that runs GPU detection and prints JSON to stdout.
Rejected because it introduces a two-phase download (probe first, then real binary). The probe binary itself needs checksum verification, version management, and a manifest entry. It adds complexity without much accuracy improvement over library probing.

**User configuration only**: Require users to set `llm.backend` manually.
Rejected as the sole mechanism because it violates tsuku's self-contained philosophy. But it's included as an override alongside auto-detection for users who know what they want.

### Decision 4: Runtime Failure Handling

Even with good pre-download detection, things can go wrong. A CUDA library might exist on disk but the driver version might be incompatible. Vulkan might be installed but the GPU might not support the required Vulkan version.

#### Chosen: Informative error + manual override (automatic fallback deferred)

When the Rust binary can't initialize its compiled-in GPU backend, it logs a clear error message to stderr explaining what failed and suggesting the config override: `tsuku config set llm.backend cpu`. The Go side surfaces this message to the user.

This is deliberately simple. Automatic fallback (detecting the failure, downloading the CPU variant, and relaunching) adds `BackendFailedError` types, Rust exit code changes, process exit monitoring in `waitForReady()`, and retry logic in `LocalProvider.Complete()`. That's significant complexity for a case that should be uncommon once library probing works. Ship detection + manual override first, measure how often detection picks wrong in practice, then add automatic fallback if needed.

The Rust binary should still distinguish backend initialization failure from other crashes in its stderr output, so the error message is actionable rather than a generic "server failed to start."

#### Alternatives Considered

**Automatic CPU fallback**: Detect backend failure via exit code 78, auto-download and launch CPU variant.
Deferred (not rejected). This is the right long-term answer but adds complexity that isn't justified until we know how accurate library probing is. If probing has a >5% false positive rate, automatic fallback becomes worth the complexity. Tracked as a future enhancement.

**Pre-download CPU alongside GPU**: Always download both the GPU variant and CPU variant.
Rejected because it doubles download size for every Linux user to cover an uncommon failure case.

## Decision Outcome

### Summary

The manifest schema expands from a flat platform-to-URL mapping to a nested structure where each platform contains a map of backend variants (cuda, vulkan, metal, cpu) and a default. The Go types change from `PlatformInfo{URL, SHA256}` to `PlatformEntry{Default, Variants}` with `VariantInfo{URL, SHA256}`.

Before downloading the addon, a new `DetectBackend()` function in the `addon` package probes for GPU libraries at known filesystem paths across Linux distros (Debian, Fedora, Arch). On macOS, Metal is always the answer. On Windows, CPU is the only option. The detection returns a list of available backends, with Vulkan preferred over CUDA on Linux to avoid driver version coupling.

Users can override auto-detection by setting `llm.backend` in `$TSUKU_HOME/config.toml`. This follows the same config pattern used by the secrets manager and LLM provider settings.

The download flow changes from `GetCurrentPlatformInfo()` (one step) to `PlatformKey()` + `DetectBackend()` + manifest lookup (three steps). The atomic download, SHA256 verification, and file-lock coordination stay exactly as they are today. The binary path gains a backend segment (`$TSUKU_HOME/tools/tsuku-llm/<version>/<backend>/tsuku-llm`) so multiple variants can coexist on disk.

The directory structure encodes the variant (`<version>/<backend>/tsuku-llm`), so `verifyBinary()` derives the backend from the path and looks up the correct SHA256 checksum. No separate tracking file is needed.

When a GPU variant fails at runtime, the Rust binary logs a clear error message to stderr with the failed backend name and suggests `tsuku config set llm.backend cpu` as a workaround. Automatic fallback (downloading CPU variant and relaunching) is deferred until detection accuracy is measured in practice.

The deprecated `AddonPath()` function and its callers must be cleaned up in Phase 1, since it constructs paths without the new backend segment and will silently break.

### Rationale

These decisions reinforce each other. Separate binaries mean the manifest can list each variant with its own checksum, and download size stays small. The nested manifest schema makes it natural to enumerate backends per platform, which the Go-side detection needs. Library file probing is fast because it's just `stat()` calls, so it doesn't add noticeable latency to the download flow.

Vulkan as the default Linux GPU backend simplifies the CUDA version problem without sacrificing it entirely. Users who want CUDA performance can opt in via config, at which point they're explicitly accepting the driver compatibility risk. This is better than auto-selecting CUDA and having it silently fail.

Deferring automatic runtime fallback keeps the initial implementation scope tight. The manual override via `llm.backend` is a sufficient escape hatch while we gather data on detection accuracy. If library probing turns out to have a high false positive rate (>5%), automatic fallback becomes the next iteration.

## Solution Architecture

### Component Changes

```
internal/llm/addon/
├── manifest.go       # Updated types: PlatformEntry, VariantInfo
├── manifest.json     # Expanded with variant maps
├── platform.go       # Updated: PlatformKey() unchanged, new BackendKey()
├── detect.go         # NEW: DetectBackend() + config override logic
├── detect_linux.go   # NEW: Linux library probing (Debian, Fedora, Arch paths)
├── detect_darwin.go  # NEW: macOS (always Metal)
├── detect_windows.go # NEW: Windows (always CPU)
├── manager.go        # Updated: uses DetectBackend(), backend in path
├── download.go       # Unchanged
└── verify.go         # Updated: derive backend from path, verify variant checksum
```

### Manifest Schema

Current schema (v1):
```json
{
  "version": "0.1.0",
  "platforms": {
    "linux-amd64": { "url": "...", "sha256": "..." }
  }
}
```

New schema (v2):
```json
{
  "version": "0.2.0",
  "platforms": {
    "linux-amd64": {
      "default": "cpu",
      "variants": {
        "cuda": { "url": "https://github.com/.../tsuku-llm-linux-amd64-cuda", "sha256": "..." },
        "vulkan": { "url": "https://github.com/.../tsuku-llm-linux-amd64-vulkan", "sha256": "..." },
        "cpu": { "url": "https://github.com/.../tsuku-llm-linux-amd64-cpu", "sha256": "..." }
      }
    },
    "darwin-arm64": {
      "default": "metal",
      "variants": {
        "metal": { "url": "https://github.com/.../tsuku-llm-darwin-arm64", "sha256": "..." }
      }
    }
  }
}
```

### Go Type Changes

```go
type Manifest struct {
    Version   string                     `json:"version"`
    Platforms map[string]PlatformEntry   `json:"platforms"`
}

type PlatformEntry struct {
    Default  string                     `json:"default"`
    Variants map[string]VariantInfo     `json:"variants"`
}

type VariantInfo struct {
    URL    string `json:"url"`
    SHA256 string `json:"sha256"`
}
```

Key methods: `GetVariantInfo(platform, backend)` for direct lookup, `GetDefaultVariant(platform)` for fallback when detection returns an unknown backend.

### GPU Detection

`DetectBackend(configOverride string) string` checks the user config override first, then calls platform-specific `probeBackends()` which returns available backends in priority order. The `configOverride` is a plain string passed by the caller (e.g., `local.go` or `factory.go`). The `addon` package must not import `userconfig` to load it directly -- that would be a dependency direction violation (lower-level package importing higher-level one).

When a config override is provided, validate it against a known allowlist (`cuda`, `vulkan`, `metal`, `cpu`) before using it in path construction or manifest lookup.

Linux probe paths must cover the three major library layouts:
- Debian/Ubuntu: `/usr/lib/x86_64-linux-gnu/`, `/usr/lib/aarch64-linux-gnu/`
- Fedora/RHEL: `/usr/lib64/`
- Arch: `/usr/lib/`
- CUDA toolkit: `/usr/local/cuda/lib64/`

Priority order on Linux: Vulkan > CUDA > CPU. macOS: always Metal. Windows: always CPU.

### Variant Tracking

The directory structure encodes the variant: `$TSUKU_HOME/tools/tsuku-llm/<version>/<backend>/tsuku-llm`. The `verifyBinary()` method derives the backend name from the path via `filepath.Base(filepath.Dir(binaryPath))` and looks up the corresponding checksum in the manifest. No separate tracking file is needed.

### Directory Layout

Binary paths gain a backend segment:

```
$TSUKU_HOME/tools/tsuku-llm/
└── 0.2.0/
    ├── vulkan/
    │   └── tsuku-llm
    └── cpu/
        └── tsuku-llm         # only if user overrides to CPU
```

Multiple variants can coexist on disk. A user who switches backends via config keeps both binaries.

### Legacy Cleanup

The deprecated `AddonPath()` at `manager.go:232` and its caller in `lifecycle.go` build paths without a backend segment. These must be updated or removed in Phase 1, before the directory layout changes. `NewServerLifecycleWithManager()` should accept the binary path from `EnsureAddon()` instead of computing it independently.

### Rust-side Error Reporting

When the Rust binary detects that its compiled-in backend doesn't match available hardware, it logs a structured error to stderr:

```
ERROR: Backend "vulkan" failed to initialize.
  Detected hardware supports: None
  Suggestion: tsuku config set llm.backend cpu
```

This is an informational message, not a protocol change. The Go side doesn't parse it. Automatic fallback (exit code 78, re-download, relaunch) is deferred to a follow-up design.

## Implementation Approach

### Phase 1: Manifest and Types + Legacy Cleanup

1. Remove deprecated `AddonPath()` and `IsInstalled()` package-level functions
2. Update `ServerLifecycle` to accept binary path at `EnsureRunning()` time, not construction time
3. Update `manifest.json` with the new nested schema
4. Update Go types (`Manifest`, `PlatformEntry`, `VariantInfo`)
5. Replace `GetPlatformInfo` with `GetVariantInfo` and `GetDefaultVariant`
6. Update `verifyBinary()` to derive backend from directory path and look up variant-specific checksum
7. Automate manifest generation in the release CI (10 SHA256 entries are too error-prone to copy manually)

### Phase 2: GPU Detection

1. Add `detect.go` with `DetectBackend()` function (accepts config override string from caller)
2. Add platform-specific probe files (`detect_linux.go`, `detect_darwin.go`, `detect_windows.go`)
3. Linux probes must cover Debian, Fedora, and Arch library paths
4. Add `llm.backend` config key to userconfig, `LLMConfig` interface, and `AvailableKeys()`
5. Config override validated against allowlist (`cuda`, `vulkan`, `metal`, `cpu`)
6. Wire detection into `AddonManager.EnsureAddon()` (caller loads config and passes override)
7. Update binary path to include backend segment

### Phase 3: Rust Error Reporting

1. Add structured stderr error when compiled-in backend doesn't match hardware
2. Include suggestion for `tsuku config set llm.backend` in the message
3. Ensure the existing hardware detection in `hardware.rs` correctly reports when a backend is unavailable vs degraded

### Phase 4: Testing and Documentation

1. Unit tests for manifest parsing and variant lookup
2. Unit tests for detection logic (mock filesystem paths)
3. Integration test: verify correct variant selection on current host
4. Update `tsuku llm download` command to respect variant selection
5. Developer documentation for building with GPU support locally

### Benchmark Gate

Before shipping Vulkan as the default Linux GPU backend, benchmark Vulkan vs CUDA performance on the models we ship (0.5B, 1.5B, 3B). If the gap exceeds 25% on tokens/second, reconsider the default priority order. This benchmark can be informal (run both variants on the same machine, compare throughput) but must happen before this design is marked Current.

## Security Considerations

### Download verification

Each variant has its own SHA256 checksum in the manifest. The verification flow (download to temp → verify checksum → chmod → atomic rename) doesn't change. Adding more variants to the manifest increases the attack surface of the manifest itself, but since it's embedded at compile time via `//go:embed`, the manifest can't be tampered with without rebuilding the tsuku binary.

### Execution isolation

No change. The addon binary runs with the same permissions as the tsuku process. GPU library probing uses `os.Stat()` calls only, never loads or executes the probed libraries. This is important: we detect GPU presence by file existence, not by dlopen.

All probed paths (`/usr/lib/...`, `/usr/lib64/...`, `/usr/local/cuda/...`) require root write access. An attacker who can plant a fake `libcuda.so` to influence variant selection has already compromised the system. The `llm.backend` config override is validated against a known allowlist to prevent path traversal via config manipulation.

### Supply chain risks

The release pipeline produces signed binaries with checksums. Adding more variants (from 5 to 10 entries in the manifest) doesn't change the signing or checksum infrastructure. Each variant's URL points to the same GitHub release page. The risk profile is the same as today, just with more entries.

One new risk: if the Go-side detection picks the wrong variant, the user downloads a binary they can't fully use. The Rust binary will report the mismatch via stderr, and the user can override via `llm.backend` config. No binary is executed without SHA256 verification first.

### User data exposure

No change. GPU library probing reads only file metadata (existence, not contents). No new network calls are introduced beyond the existing download flow. The `llm.backend` config value is stored in `$TSUKU_HOME/config.toml` alongside other settings.

## Consequences

### Positive

- Users with GPUs automatically get GPU-accelerated inference without manual configuration.
- Download size stays small (one variant per user, not all ten).
- CUDA driver version mismatches no longer cause silent CPU fallback with the wrong model. The system downloads the right variant for the hardware.
- Power users can force a specific backend via config.
- Cleaning up deprecated `AddonPath()` removes a source of path-construction bugs.

### Negative

- The manifest schema change is a breaking change. Old tsuku versions can't parse the new manifest. This is fine because the manifest is embedded at compile time, not fetched remotely.
- Go-side library probing may give wrong results in unusual environments (containers, custom library paths, GPU passthrough). Users must use `llm.backend` config override in these cases until automatic fallback ships.
- Vulkan as the Linux default means NVIDIA users get slightly less performance than CUDA would provide. They can opt in to CUDA via config if they want.
- The binary path gains a backend segment, so existing installations re-download the addon on the first run after upgrading.

### Risks

- We haven't benchmarked Vulkan vs CUDA performance for the models we ship. The benchmark gate (25% threshold) must pass before marking this design Current.
- GPU detection on ARM64 Linux is less well-tested than x86_64. The library paths may differ across distributions. Coverage of the three major distro families (Debian, Fedora, Arch) should catch most users.
- Without automatic runtime fallback, a false positive from library probing (library file exists but backend can't actually init) leaves the user with a broken addon until they set `llm.backend` manually. The error message from the Rust side must be clear enough that users know what to do.
