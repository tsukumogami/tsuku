# Pragmatic Review: Issue #1791 (align tsuku-llm release artifacts with recipe asset patterns)

## Scope

Files changed:
- `.github/workflows/llm-release.yml` (modified)

Key changes: Renamed matrix field `artifact` to `artifact_suffix`, added "Compute artifact name" step that constructs versioned filenames from `GITHUB_REF_NAME`, added CUDA version coupling documentation, added artifact verification in finalize-release job.

## Findings

### Blocking Findings

#### 1. CUDA toolkit install uses x86_64 repo for ARM64 runners

**File**: `.github/workflows/llm-release.yml:131`
**Severity**: Blocking

The CUDA toolkit installation step downloads from `ubuntu2204/x86_64/`:

```yaml
wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2204/x86_64/cuda-keyring_1.1-1_all.deb
```

This step runs for all CUDA builds (`matrix.backend == 'cuda' && startsWith(matrix.runner, 'ubuntu')`), including the `linux-arm64-cuda` entry on `ubuntu-24.04-arm` (line 76). The `cuda-keyring` .deb is architecture-independent (`_all.deb`), but the repo it configures (`ubuntu2204/x86_64`) will have x86_64 packages. On an ARM64 runner, `apt-get install cuda-toolkit-12-4` would either fail (no matching architecture) or install x86_64 packages that can't be used.

The ARM64 CUDA build needs the `ubuntu2204/sbsa` or `ubuntu2404/sbsa` CUDA repo. Fix: branch the CUDA install step on runner architecture, or add separate steps with appropriate `if:` conditions.

Note: this may be a pre-existing issue if the ARM64 CUDA matrix entry existed before this commit. However, the `artifact_suffix` rename and artifact name computation in this commit wire up the ARM64-cuda artifact path end-to-end, so a broken build for that variant means the finalize-release verification step (line 308-333) would fail when checking for `tsuku-llm-v${VERSION}-linux-arm64-cuda`.

### Advisory Findings

#### 2. Vulkan SDK install uses `apt-key` (deprecated) and jammy-specific source list

**File**: `.github/workflows/llm-release.yml:142-143`
**Severity**: Advisory

```bash
wget -qO - https://packages.lunarg.com/lunarg-signing-key-pub.asc | sudo apt-key add -
sudo wget -qO /etc/apt/sources.list.d/lunarg-vulkan-jammy.list \
  https://packages.lunarg.com/vulkan/lunarg-vulkan-jammy.list
```

`apt-key` is deprecated since Ubuntu 22.04 and may be removed in future runner images. The `jammy` source list is specific to Ubuntu 22.04; the ARM64 Vulkan builds run on `ubuntu-24.04-arm` which is `noble`. This could fail when the runner image updates or on the ARM64 runners. Same category as finding #1 -- may be pre-existing, but wired up by this commit.

#### 3. CUDA version coupling docs expand scope slightly

**File**: `.github/workflows/llm-release.yml:19-25`
**Severity**: Advisory

The CUDA version coupling header comment (lines 19-25) and inline comment (lines 128-130) go beyond "align artifact names." This is a minor scope expansion. The documentation is accurate and useful for the next person who touches the CUDA version, so it's net positive. No action needed.

## Overall Assessment

The core artifact naming mechanism is correct and well-structured. The `artifact_suffix` field plus the "Compute artifact name" step cleanly constructs versioned filenames matching the recipe's `asset_pattern` values. The finalize-release verification step is a good addition that catches missing artifacts before publish.

The blocking finding is that ARM64 CUDA builds will fail because the CUDA toolkit install step configures an x86_64 repo on ARM64 runners. The Vulkan SDK install has a similar portability concern (deprecated `apt-key`, jammy-specific source for noble runners) but is less likely to break immediately.
