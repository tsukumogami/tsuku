# Architecture Review: #1789 - nvidia-driver and cuda-runtime dependency recipes

## Scope

Two new TOML recipe files:
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/recipes/n/nvidia-driver.toml`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/recipes/c/cuda-runtime.toml`

No Go code changes.

## Design Alignment

Both recipes align with the design doc (DESIGN-gpu-backend-selection.md, Decision 5: "GPU runtimes are standard recipes; action types match distribution method"). The design specifies:
- nvidia-driver uses system PM actions (apt_install / dnf_install / pacman_install / zypper_install)
- cuda-runtime uses download_archive from NVIDIA redistributable tarballs, depends on nvidia-driver
- cuda-runtime provides libcudart.so for running pre-built CUDA binaries

Both recipes follow these specifications.

## Pattern Consistency

### nvidia-driver.toml

Follows the `docker.toml` pattern exactly: multiple system PM action steps without explicit `when` clauses, relying on the `ImplicitConstraint()` mechanism in `internal/actions/system_action.go` to filter per linux_family at plan generation time. This is the correct architectural pattern -- the plan generator's two-stage filtering (Stage 1: action implicit constraint, Stage 2: explicit when clause) handles distro selection automatically.

Includes `require_command` verification step and a `[verify]` section with `mode = "output"`, both following established conventions. The `pattern = "."` regex is intentional per the `reason` field: driver versioning is system-managed, so any valid output suffices.

`supported_os = ["linux"]` matches the constraint that GPU kernel drivers are only relevant on Linux.

### cuda-runtime.toml

Follows the library recipe pattern (type = "library") established by `libcurl.toml`, `brotli.toml`, `zstd.toml`. Metadata-level `dependencies = ["nvidia-driver"]` matches the existing convention where dependencies are declared on the `[metadata]` section.

Uses `download_archive` action with explicit `when` clauses for architecture filtering (`when = { os = ["linux"], arch = "amd64" }` and `arch = "arm64"`). This matches the `golang.toml` pattern for architecture-specific download URLs.

The `install_binaries` step with `install_mode = "directory"` and `outputs` listing `.so` files follows the library output registration pattern from `brotli.toml` and `libcurl.toml`.

No `[verify]` section, consistent with other library-type recipes (brotli, zstd have no verify sections either).

### Relationship with existing cuda.toml

The existing `cuda.toml` (full toolkit, manual install) has been updated with a cross-reference comment pointing users to `cuda-runtime` for runtime-only needs. Clean separation: cuda = development toolkit (manual), cuda-runtime = runtime libraries only (tsuku-managed).

## Findings

### Advisory: Pinned version without version provider (cuda-runtime.toml)

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/recipes/c/cuda-runtime.toml`, lines 43, 50

The download URLs hardcode version `12.6.77`:
```
url = "https://developer.download.nvidia.com/compute/cuda/redist/cuda_cudart/linux-x86_64/cuda_cudart-linux-x86_64-12.6.77-archive.tar.xz"
```

There is no `[version]` section, so the recipe has no version provider. This means `tsuku update cuda-runtime` won't detect newer versions. The design doc acknowledges this implicitly ("shipping multiple CUDA versions" is deferred), and the header comment explains the version coupling with the tsuku-llm CI pipeline build. This is a deliberate choice, not an oversight, but it means version updates require manually editing the recipe TOML.

This is consistent with the architecture -- many library recipes (brotli, zstd, libcurl) also lack `[version]` sections. **Advisory** because it's a known trade-off documented in the recipe comments, not a pattern violation.

### Advisory: ARM64 URL uses "sbsa" platform identifier (cuda-runtime.toml)

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/recipes/c/cuda-runtime.toml`, line 50

The ARM64 download URL uses `linux-sbsa` (Server Base System Architecture):
```
url = "https://developer.download.nvidia.com/compute/cuda/redist/cuda_cudart/linux-sbsa/cuda_cudart-linux-sbsa-12.6.77-archive.tar.xz"
```

This is NVIDIA's naming convention for server/datacenter ARM64. Consumer ARM64 devices (e.g., Jetson) use a different identifier (`linux-aarch64`). The recipe comment documents the URL format and notes both identifiers. For the tsuku-llm use case (server-class GPU inference), `sbsa` is likely correct, but this is worth flagging since the `when` clause simply says `arch = "arm64"` without distinguishing server vs consumer ARM64. **Advisory** because this is a correctness concern outside architecture scope; the pattern itself is structurally sound.

## Overall Assessment

Both recipes fit the existing architecture cleanly. No new patterns introduced -- nvidia-driver follows the docker.toml system PM pattern, cuda-runtime follows the library recipe pattern with download_archive. The dependency chain (cuda-runtime -> nvidia-driver) uses the standard metadata-level `dependencies` field. Action dispatch, implicit constraints, when clause filtering, and install_binaries output registration all follow established conventions. No blocking findings.
