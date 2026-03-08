# Round 2: GPU Build Dependency Setup and Unified Naming Impact

## Executive Summary

GPU builds (CUDA toolkit 12.4 for NVIDIA, Vulkan SDK for AMD/Intel, Metal for macOS) add significant build infrastructure complexity to the release pipeline: CUDA toolkit download (~3 GB) and Vulkan SDK installation add ~5-10 minutes per Linux build (sequential dependency chains), while macOS Metal builds have zero setup overhead. The 6 llm-release.yml parallel jobs can run fully independently with no blocking on existing release.yml jobs, but merging into unified release.yml requires careful runner selection and dependency sequencing. Backend naming convention (`[darwin-{arch}|linux-{arch}-{backend}]`) is asymmetric by design: macOS omits backend because Metal is the only supported option per architecture, while Linux differentiates cuda/vulkan to route correct dependencies. Unified naming requires standardizing on this conditional suffix pattern across all three artifacts (CLI, dltest, llm).

## Part 1: GPU Build Dependency Analysis

### CUDA Toolkit Installation (Lines 114-125 in llm-release.yml)

```bash
wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/x86_64/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt-get update
sudo apt-get install -y cuda-toolkit-12-4
echo "/usr/local/cuda-12.4/bin" >> $GITHUB_PATH
echo "CUDA_PATH=/usr/local/cuda-12.4" >> $GITHUB_ENV
```

**Characteristics**:
- **Runner**: ubuntu-22.04 (amd64) and ubuntu-24.04-arm (arm64)
- **Steps**: (1) Download CUDA keyring (~1 MB), (2) Add to dpkg, (3) apt-get update, (4) apt-get install cuda-toolkit-12-4
- **Download Size**: CUDA 12.4 toolkit ~3 GB (uncompressed; downloads as compressed package)
- **Installation Time**: ~5-8 minutes on GitHub Actions runners (includes apt update and package resolution)
- **Dependencies**: None on protobuf (installed separately); runs after protobuf installation
- **Artifacts**: Places toolkit at `/usr/local/cuda-12.4/`; no persistent cache between jobs
- **Coupling Note** (lines 20-26): CUDA major version (12.4) must match cuda-runtime recipe (recipes/c/cuda-runtime.toml). CUDA 12.x maintains forward compatibility within the major version, so cuda-runtime's 12.6.77 libraries work with binaries built against 12.4. Changing CUDA major version requires updating cuda-runtime.toml.

### Vulkan SDK Installation (Lines 127-136 in llm-release.yml)

```bash
wget -qO - https://packages.lunarg.com/lunarg-signing-key-pub.asc | sudo apt-key add -
sudo wget -qO /etc/apt/sources.list.d/lunarg-vulkan-jammy.list \
  https://packages.lunarg.com/vulkan/lunarg-vulkan-jammy.list
sudo apt-get update
sudo apt-get install -y vulkan-sdk
echo "VULKAN_SDK=/usr" >> $GITHUB_ENV
```

**Characteristics**:
- **Runner**: ubuntu-22.04 (amd64) and ubuntu-24.04-arm (arm64)
- **Steps**: (1) Download LunarG signing key, (2) Add to apt sources, (3) apt-get update, (4) apt-get install vulkan-sdk
- **Download Size**: Vulkan SDK ~500 MB-1 GB (package manager install; only needed components downloaded)
- **Installation Time**: ~3-5 minutes on GitHub Actions runners
- **Dependencies**: None on protobuf or CUDA; runs independently
- **Artifacts**: Places SDK at `/usr/` (standard library path)
- **Symmetry**: Each Linux amd64 and arm64 variant needs either CUDA OR Vulkan, never both

### Protobuf Compiler Installation (Lines 105-112 in llm-release.yml)

```bash
if [[ "$OSTYPE" == "darwin"* ]]; then
  brew install protobuf
else
  sudo apt-get update
  sudo apt-get install -y protobuf-compiler
fi
```

**Characteristics**:
- **Runs for**: All 6 platform variants
- **macOS**: `brew install protobuf` (~1-2 minutes)
- **Linux**: `apt-get install protobuf-compiler` (~30 seconds)
- **Dependency**: Prerequisite for llama.cpp Rust bindings compilation (needed by llm binary)
- **Critical Path**: Not on critical path if GPU toolkit installation is sequential blocker

### Dependency Chain Analysis

**Current llm-release.yml Execution (Build-LLM Job)**:

```
checkout → set-up-rust → install-protobuf → [GPU setup in parallel] → build-release-binary → sign-binary → create-release-artifact
                                         ├─ CUDA (5-8 min)
                                         └─ Vulkan (3-5 min)
```

**Matrix Structure** (lines 44-79):
- 6 parallel matrix jobs (one per platform)
- CUDA builds (linux-amd64-cuda, linux-arm64-cuda) wait for protobuf, then install CUDA toolkit in parallel
- Vulkan builds (linux-amd64-vulkan, linux-arm64-vulkan) wait for protobuf, then install Vulkan SDK in parallel
- macOS builds (darwin-amd64, darwin-arm64) skip GPU setup entirely
- All 6 jobs can run simultaneously on different runners

**Wall-Clock Time Estimation for build-llm Job**:
- Checkout + Rust setup: ~2 minutes (parallel across all runners)
- Protobuf install: ~1-2 minutes (macOS 1-2 min, Linux ~30 sec; parallelized per runner)
- GPU toolkit install: ~3-8 minutes (CUDA at long end, Vulkan in middle, macOS zero)
- Cargo build: ~10-20 minutes (depends on llama.cpp compile time; likely dominates)
- **Total for build-llm**: ~15-30 minutes (GPU setup doesn't add much since it runs in parallel with cargo compile time)

## Part 2: Integration into Unified Release Pipeline

### Current release.yml Job Dependency Graph (Simplified)

```
release (GoReleaser)
├─ build-rust (glibc)
├─ build-rust-musl (Alpine)
└─ integration-test → finalize-release
```

**Current Timeline**:
- release: ~3-5 min (Go compilation and packaging)
- build-rust (parallel): ~10-15 min (Rust compile time)
- build-rust-musl (parallel): ~10-15 min (Docker startup + Alpine build)
- integration-test (needs build-rust): ~2-5 min (download + validation)
- finalize-release (needs integration-test + build-rust-musl): ~1-2 min (artifact verification + checksums)
- **Total Critical Path**: ~15-25 minutes

### Proposed Unified release.yml Structure

```
release (GoReleaser)
├─ build-rust (glibc) → integration-test (exists: validates tsuku + dltest)
├─ build-rust-musl (Alpine) → finalize-release
├─ build-llm-step-1 (protobuf + GPU setup)
│  └─ build-llm-step-2 (cargo build)
│     └─ build-llm-integration-test (NEW: validates llm binaries)
│        └─ finalize-release (NEW: added to validate llm artifacts)
```

**Key Insight**: llm builds can start immediately after `release` job completes (no blocking dependencies). GPU setup happens in parallel with Rust cargo compilation.

### Runner Requirements for GPU Builds

| Job | Runners | GPU Setup | Estimated Time |
|-----|---------|-----------|-----------------|
| build-llm darwin-arm64 (metal) | macos-latest | None | ~20-25 min |
| build-llm darwin-amd64 (metal) | macos-15-intel | None | ~20-25 min |
| build-llm linux-amd64-cuda | ubuntu-22.04 | CUDA 12.4 | ~25-30 min |
| build-llm linux-amd64-vulkan | ubuntu-22.04 | Vulkan SDK | ~23-25 min |
| build-llm linux-arm64-cuda | ubuntu-24.04-arm | CUDA 12.4 | ~25-30 min |
| build-llm linux-arm64-vulkan | ubuntu-24.04-arm | Vulkan SDK | ~23-25 min |

**Parallel Capacity**: Each runner type can run 1 job simultaneously (github actions matrices reserve runners per job). With 6 matrix variants and 2-4 available ubuntu runners, jobs may queue if release.yml has parallel build-rust jobs competing for runners.

**Solution Options**:
1. **Run llm builds on separate runners** (e.g., `runs-on: ubuntu-22.04` stays same, but accept queue if contention)
2. **Stagger llm builds** (sequential per arch) if runner capacity is limiting (adds ~10-15 minutes to critical path)
3. **Use self-hosted runners** for CUDA/Vulkan builds to avoid queue (infrastructure change)

## Part 3: Backend Naming Convention Analysis

### Current tsuku-llm Naming Scheme (llm-release.yml lines 6-18)

```
tsuku-llm-v{version}-{os}-{arch}[-{backend}]

Examples for tag tsuku-llm-v0.3.0:
  tsuku-llm-v0.3.0-darwin-arm64        (Metal)
  tsuku-llm-v0.3.0-linux-amd64-cuda    (NVIDIA)
  tsuku-llm-v0.3.0-linux-arm64-vulkan  (AMD/Intel)
```

**Asymmetry by Design**:
- **macOS**: Backend suffix OMITTED (Metal is the only option for all Macs)
  - darwin-arm64 → metal (implicit, not in artifact name)
  - darwin-amd64 → metal (implicit)
- **Linux**: Backend suffix REQUIRED (differentiates cuda/vulkan to route dependencies)
  - linux-amd64 → {cuda, vulkan} (explicit in artifact name to select correct binary)
  - linux-arm64 → {cuda, vulkan}

**Rationale** (from recipe step comments):
- Recipe `when` conditions use GPU dimension to select variant:
  ```toml
  when = { os = ["linux"], arch = "amd64", gpu = ["nvidia"] }
  asset_pattern = "tsuku-llm-v{version}-linux-amd64-cuda"
  dependencies = ["cuda-runtime"]
  ```
- Platform dimension added in commit c5b163c0 to enable GPU hardware detection at install time
- macOS steps omit `gpu` condition because Metal is built-in to all Macs

### Recipe Asset Pattern Matching (tsuku-llm.toml lines 20-70)

All 6 recipe steps use conditional asset patterns:

```toml
# macOS arm64: Metal variant
[[steps]]
action = "github_file"
when = { os = ["darwin"], arch = "arm64" }
asset_pattern = "tsuku-llm-v{version}-darwin-arm64"

# Linux amd64: CUDA variant
[[steps]]
action = "github_file"
when = { os = ["linux"], arch = "amd64", gpu = ["nvidia"] }
asset_pattern = "tsuku-llm-v{version}-linux-amd64-cuda"
dependencies = ["cuda-runtime"]

# Linux amd64: Vulkan variant
[[steps]]
action = "github_file"
when = { os = ["linux"], arch = "amd64", gpu = ["amd", "intel"] }
asset_pattern = "tsuku-llm-v{version}-linux-amd64-vulkan"
dependencies = ["vulkan-loader"]
```

**Pattern Characteristics**:
- Artifact name is used both in llm-release.yml output AND as recipe asset_pattern placeholder
- `{version}` placeholder resolved by `[version]` section (currently MISSING, points to wrong repo)
- Backend suffix in artifact name enables recipe to distinguish cuda vs vulkan downloads
- Dependencies are declared per-step so correct runtime libraries are installed

### Comparison to Other Artifact Naming

| Tool | Naming Pattern | Version in Name | Backend Suffix |
|------|---|---|---|
| Go CLI (tsuku) | tsuku-{os}-{arch}_{version}_{os}_{arch} | YES (middle + end) | NO |
| tsuku-dltest | tsuku-dltest-{os}-{arch}[-musl] | NO | YES (libc variant) |
| tsuku-llm | tsuku-llm-v{version}-{os}-{arch}[-{backend}] | YES (prefix) | YES (GPU) |

**Naming Heterogeneity**:
- Go CLI: GoReleaser default (versioned, no suffixes)
- dltest: Simple (unversioned, libc suffix only)
- llm: Semantic (versioned, backend suffix)

### Unified Naming Decision

**Option A: Keep Asymmetric Backend Suffix (Recommended for compatibility)**

Rationale:
- Matches current llm-release.yml artifact output exactly
- Recipe conditional selection is already implemented and working
- Minimal changes to existing pipelines
- Allows for future expansion (e.g., Windows variants with different backends)

Pattern:
```
tsuku-llm-v{version}-{os}-{arch}[-{backend}]
  where backend is omitted if unique per (os, arch) pair
```

**Option B: Always Include Backend Suffix**

Pattern:
```
tsuku-llm-v{version}-{os}-{arch}-{backend}
  darwin-arm64-metal
  darwin-amd64-metal
  linux-amd64-cuda
  linux-amd64-vulkan
  linux-arm64-cuda
  linux-arm64-vulkan
```

Pros:
- Explicit and symmetric
- Easier to understand artifact purpose
- Recipe selection logic becomes simpler (no conditional logic on backend presence)

Cons:
- Breaking change for existing tsuku-llm recipe users
- Artifact names become longer
- macOS suffixes seem redundant if Metal is only option

**Option C: Standardize All Artifacts with Backend Suffix**

Extend convention to dltest and Go CLI:
```
tsuku-{os}-{arch}[-{backend}]
tsuku-dltest-{os}-{arch}[-{libc}]
tsuku-llm-v{version}-{os}-{arch}[-{backend}]
```

Pros:
- Consistent naming across all tools
- Single recipe template pattern

Cons:
- Massive breaking change (thousands of Go CLI recipes would need updates)
- High risk of deployment issues
- GoReleaser config would need rewriting

**Recommendation**: Stick with **Option A** (asymmetric backend suffix). Current implementation is proven and backward-compatible. The macOS omission of suffix is a feature, not a bug—it signals that backend choice is automatic and requires no user selection.

## Part 4: Impact on Unified Release Pipeline

### Version Constraint Enforcement

**Current State**:
- release.yml validates tsuku and dltest versions independently (integration-test lines 218-244)
- llm-release.yml has warning-only version check (llm-release.yml lines 220-222)
- No cross-binary version matching requirement

**Required Changes for Unified Release**:
1. Merge llm-release.yml `build-llm` matrix into release.yml as new `build-llm` job
2. Extend integration-test to validate all three binaries:
   ```bash
   TSUKU_VERSION=$(./artifacts/${TSUKU_BINARY} --version)
   DLTEST_VERSION=$(./artifacts/${{ matrix.dltest_binary }} --version)
   LLM_VERSION=$(./artifacts/${{ matrix.llm_binary }} --version)
   # Enforce all three match EXPECTED_VERSION
   ```
3. Update finalize-release to expect 16+ artifacts (4 Go + 4 dltest glibc + 2 dltest musl + 6 llm)

### Critical Path Impact

Adding 6 llm parallel builds does NOT increase critical path because:
1. They run in parallel (matrix jobs)
2. They don't block on existing release.yml jobs (can start after `release` job completes)
3. GPU toolkit downloads (~3-8 min) overlap with cargo compile time (~10-20 min)

**New Critical Path** (if merged into release.yml):
```
release → build-llm (longest job in matrix, ~25-30 min) → integration-test → finalize-release
```

Compared to current release.yml critical path (~15-25 min), unified pipeline adds ~10 minutes.

### Artifact Verification Complexity

**Current finalize-release** verifies 10 artifacts.

**Unified finalize-release** would verify 16+ artifacts:
```bash
EXPECTED_ARTIFACTS=(
  # Go (4)
  "tsuku-linux-amd64_${VERSION}_linux_amd64"
  "tsuku-linux-arm64_${VERSION}_linux_arm64"
  "tsuku-darwin-amd64_${VERSION}_darwin_amd64"
  "tsuku-darwin-arm64_${VERSION}_darwin_arm64"
  # dltest glibc (4)
  tsuku-dltest-linux-amd64
  tsuku-dltest-linux-arm64
  tsuku-dltest-darwin-amd64
  tsuku-dltest-darwin-arm64
  # dltest musl (2)
  tsuku-dltest-linux-amd64-musl
  tsuku-dltest-linux-arm64-musl
  # llm (6)
  tsuku-llm-v${VERSION}-darwin-arm64
  tsuku-llm-v${VERSION}-darwin-amd64
  tsuku-llm-v${VERSION}-linux-amd64-cuda
  tsuku-llm-v${VERSION}-linux-amd64-vulkan
  tsuku-llm-v${VERSION}-linux-arm64-cuda
  tsuku-llm-v${VERSION}-linux-arm64-vulkan
)
```

Loop complexity increases but verification logic remains the same (check `gh release view "$TAG" --json assets`).

## Findings Summary

1. **GPU Dependency Cost**: CUDA toolkit adds 5-8 min per build, Vulkan SDK adds 3-5 min, but both can be parallelized with cargo compilation. No additional build machine or runner type needed beyond existing ubuntu-22.04 and ubuntu-24.04-arm runners.

2. **Job Parallelization**: All 6 llm builds can run independently in matrix format. GPU setup doesn't block other jobs. Zero contention with existing release.yml jobs if llm jobs start after `release` job.

3. **Naming Asymmetry Justified**: Backend suffix omission on macOS is intentional (Metal is the only option). Keeping asymmetric pattern minimizes breaking changes while enabling GPU selection via recipe conditionals. Do not standardize to always include suffix.

4. **Version Constraint**: Requires adding `[version]` section to tsuku-llm.toml pointing to main repo, plus extending integration-test to validate all three binaries match. Current llm-release.yml only does warning-level version check.

5. **Critical Path**: Merging llm into unified release.yml adds ~10 minutes due to cargo compilation time, not GPU setup. GPU dependencies are not the bottleneck.

6. **Recipe Compatibility**: Asset pattern naming already matches llm-release.yml output. No artifact renaming needed. Only need to fix `[version]` section in recipe to point to `tsukumogami/tsuku` instead of inferring from `tsukumogami/tsuku-llm`.
