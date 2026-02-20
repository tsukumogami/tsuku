# Pragmatic Review: Issue #1789 (nvidia-driver and cuda-runtime dependency recipes)

## Scope

Files changed:
- `recipes/n/nvidia-driver.toml` (new)
- `recipes/c/cuda-runtime.toml` (new)
- `recipes/c/cuda.toml` (modified: added cross-reference comment)

## Findings

### No blocking issues found.

### Advisory Findings

#### 1. cuda-runtime hardcoded version means manual coordination required
**File**: `recipes/c/cuda-runtime.toml:43,50`
**Severity**: Advisory

The download URLs hardcode version `12.6.77`:
```
url = "https://developer.download.nvidia.com/compute/cuda/redist/cuda_cudart/linux-x86_64/cuda_cudart-linux-x86_64-12.6.77-archive.tar.xz"
```

This is intentional and well-documented in the header comments (lines 8-13). The version must match the CUDA version used to compile the consuming binary (tsuku-llm). There is no `[version]` section, so `tsuku update cuda-runtime` won't resolve newer versions automatically. This is the correct design for a tightly-coupled dependency, but means any CUDA version bump requires editing this file.

No action needed -- the header comments explain the coupling clearly.

#### 2. linux-sbsa vs linux-aarch64 diverges from design doc sketch
**File**: `recipes/c/cuda-runtime.toml:50`
**Severity**: Advisory

The design doc used `linux-aarch64` in the URL, but the implementation uses `linux-sbsa`:
```
url = "https://developer.download.nvidia.com/.../cuda_cudart/linux-sbsa/cuda_cudart-linux-sbsa-12.6.77-archive.tar.xz"
```

This is actually correct for server-grade ARM64 (SBSA = Server Base System Architecture, covering AWS Graviton, Ampere Altra, etc.). NVIDIA's `linux-aarch64` path is for Jetson/embedded ARM64. The implementation is more accurate than the sketch.

Worth noting: if a user has a Jetson device with NVIDIA GPU, this URL would be wrong for them. But Jetson is out of scope for tsuku's current user base. No action needed.

## Overall Assessment

Both recipes follow established codebase patterns exactly. `nvidia-driver.toml` mirrors `docker.toml` (system PM actions per distro, no `when` clauses needed since action types self-select). `cuda-runtime.toml` follows the library recipe pattern (`type = "library"`, `download_archive` + `install_binaries`). The `cuda.toml` modification is a minimal cross-reference.

The header comments are more extensive than most recipes, but justified given that these are the first GPU compute runtime recipes in the registry and contain non-obvious decisions (version coupling, sbsa vs aarch64, driver compatibility requirements).

No over-engineering detected. No dead code. No speculative generality. The recipes are the simplest correct approach for their purpose.
