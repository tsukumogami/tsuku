# Maintainer Review: Issue #1789 - nvidia-driver and cuda-runtime dependency recipes

## Review Focus: Maintainability (clarity, readability, duplication)

## Files Changed

- `recipes/n/nvidia-driver.toml` (new)
- `recipes/c/cuda-runtime.toml` (new)
- `recipes/c/cuda.toml` (modified -- cross-reference added)

---

## Finding 1: Hardcoded CUDA version in URLs without `{version}` template (Advisory)

**File**: `recipes/c/cuda-runtime.toml`, lines 43 and 50

The download URLs contain a hardcoded version `12.6.77`:
```toml
url = "https://developer.download.nvidia.com/compute/cuda/redist/cuda_cudart/linux-x86_64/cuda_cudart-linux-x86_64-12.6.77-archive.tar.xz"
```

Other download recipes (e.g., `golang.toml`) use `{version}` template variables:
```toml
url = "https://go.dev/dl/go{version}.{os}-{arch}.tar.gz"
```

The next developer will wonder: is the version hardcoded because there's no version provider, or was the `{version}` template accidentally omitted?

**Mitigating factor**: The file header (lines 8-12) explicitly explains this is intentional -- the component version (12.6.77) differs from the CUDA toolkit version (12.6.3), and the recipe pins a specific version to ensure ABI compatibility with tsuku-llm's CI build. The header comment is excellent and thoroughly explains the reasoning. This is well-documented enough that the next developer won't misunderstand it.

**Severity**: Advisory. The header comment fully explains the decision. A future improvement could add a version provider for the NVIDIA redist JSON manifest, but that's out of scope.

---

## Finding 2: Two `download_archive` steps differ only by platform string (Advisory)

**File**: `recipes/c/cuda-runtime.toml`, lines 41-52

The two download steps are near-identical:
```toml
# Linux x86_64
url = "https://...cuda_cudart/linux-x86_64/cuda_cudart-linux-x86_64-12.6.77-archive.tar.xz"
when = { os = ["linux"], arch = "amd64" }

# Linux aarch64 (sbsa)
url = "https://...cuda_cudart/linux-sbsa/cuda_cudart-linux-sbsa-12.6.77-archive.tar.xz"
when = { os = ["linux"], arch = "arm64" }
```

The difference is intentional and necessary: NVIDIA uses `linux-x86_64` and `linux-sbsa` (not `linux-aarch64`) as platform identifiers in their CDN paths. These can't be consolidated with `{arch}` template mapping because both the directory name and filename component differ in non-standard ways.

The next developer updating the CUDA version will need to update both URLs. With only two steps this is manageable, and the NVIDIA naming convention comment makes the divergence clear.

**Severity**: Advisory. The divergence is inherent to NVIDIA's CDN URL scheme. No action needed. If a third architecture were added, a comment noting "update all download URLs together" would help.

---

## Finding 3: nvidia-driver verify uses `pattern = "."` (Advisory)

**File**: `recipes/n/nvidia-driver.toml`, lines 52-56

```toml
[verify]
command = "nvidia-smi --query-gpu=driver_version --format=csv,noheader"
mode = "output"
pattern = "."
reason = "Driver versioning is managed by the system package manager; any valid output confirms the driver is functional"
```

The single-dot regex `"."` matches any character. The next developer might think this is a placeholder or mistake. However, the `reason` field immediately explains why: the recipe doesn't manage driver versions, so any output from nvidia-smi confirms the driver works.

The design doc's sketch (line 643) used `pattern = "{version}"`, which would be wrong here since the recipe has no `[version]` section. The implementation correctly deviated from the sketch.

**Severity**: Advisory. The `reason` field makes this self-documenting. Good use of a non-obvious pattern with an inline explanation.

---

## Finding 4: Header comment quality is strong

**File**: `recipes/n/nvidia-driver.toml`, lines 1-19
**File**: `recipes/c/cuda-runtime.toml`, lines 1-32

Both files have header comments that explain:
- What the recipe provides and what it doesn't (nvidia-driver: verification, not version management; cuda-runtime: runtime only, not full toolkit)
- Distro-specific gotchas (RPM Fusion requirement for Fedora, G06 driver series for openSUSE)
- URL format and how to check available versions
- Driver compatibility requirements (CUDA 12.x needs driver >= 525.60)
- Cross-reference to the related issue (#1791 for release pipeline coordination)

This is notably better documentation than most existing recipes in the codebase. The next developer maintaining these recipes has everything they need in the file itself.

---

## Finding 5: cuda.toml cross-reference is helpful and accurate

**File**: `recipes/c/cuda.toml`, lines 8-10

```toml
# NOTE: If you only need to run pre-built CUDA binaries (not compile them),
# use the "cuda-runtime" recipe instead. It provides only libcudart.so
# from NVIDIA's redistributable archive, without requiring manual installation.
```

The next developer searching for "cuda" in recipes will find `cuda.toml` first (the full toolkit recipe). This cross-reference prevents them from thinking `cuda.toml` is the only option and potentially replacing the `action = "manual"` with a download when all they need is the runtime.

---

## Finding 6: Missing `[verify]` on cuda-runtime is correct but worth noting

**File**: `recipes/c/cuda-runtime.toml`

The recipe has no `[verify]` section. This is consistent with other library-type recipes (zstd, readline, libnghttp2, etc.) that provide `.so` files without a command-line tool. The `install_binaries` step with `outputs` lists the expected library files, which serves as implicit verification.

The design doc sketch (line 673-675) also omitted `[verify]`, so this matches intent.

**Severity**: Not a finding. Consistent with existing patterns.

---

## Overall Assessment

The implementation is clean, well-documented, and follows codebase conventions closely. The nvidia-driver recipe follows the docker.toml pattern (system PM actions per distro). The cuda-runtime recipe is the first recipe to use `download_archive` with a non-GitHub CDN URL, and the extensive header comments explain the NVIDIA-specific URL format, version coupling, and how to check for updates.

No blocking findings. All three advisory items are cases where the code is already well-documented; the notes are for awareness rather than action.
