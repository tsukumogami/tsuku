# Architecture Review: #1791 - fix(ci): align tsuku-llm release artifacts with recipe asset patterns

## Scope

One new file:
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/.github/workflows/llm-release.yml`

No Go code changes. No recipe changes.

## Design Alignment

The design doc (DESIGN-gpu-backend-selection.md) specifies issue #1791 as: "Updates the tsuku-llm release pipeline so artifact filenames include version and match the `asset_pattern` values in the recipe (e.g., `tsuku-llm-v{version}-linux-amd64-cuda`). Without this, the recipe can't find release assets."

The implementation delivers exactly this. The artifact naming convention `tsuku-llm-v{version}-{os}-{arch}[-{backend}]` matches the `asset_pattern` values shown in the design doc's recipe sketch (lines 310-367 of the design doc). The pipeline produces all 8 platform variants listed in the design doc's platform table plus corresponding integration tests for CPU/Metal variants.

## Pattern Consistency

### Follows release.yml structure

The new `llm-release.yml` follows the existing `release.yml` pipeline structure:

1. **Build matrix** -> **Integration test** -> **Create release (draft)** -> **Finalize release (verify + publish)**

This matches the `release.yml` pattern of build -> test -> verify -> publish. The main structural differences are deliberate adaptations for the LLM binary's specifics:

- **Matrix uses `artifact_suffix` instead of `artifact`**: The existing `release.yml` uses `artifact: tsuku-dltest-linux-amd64` where the full artifact name is a static string in the matrix. The new workflow uses `artifact_suffix: linux-amd64-cuda` and computes the versioned name dynamically via a `Compute artifact name` step (`tsuku-llm-v${VERSION}-${suffix}`). This is the correct approach because tsuku-llm artifact names embed the version (required by the recipe's `asset_pattern = "tsuku-llm-v{version}-..."` convention), while tsuku-dltest names do not. The two naming schemes are driven by their consumers: tsuku-dltest is looked up by platform key, while tsuku-llm is looked up by asset pattern with version interpolation.

- **Separate tag namespace**: Triggers on `tsuku-llm-v*` tags vs `v*` for the main release. Clean separation -- no risk of cross-triggering.

- **Draft-then-publish pattern**: `llm-release.yml` creates a draft release, uploads all artifacts, verifies completeness, then publishes. This matches the `release.yml` pattern (GoReleaser creates draft, finalize-release publishes after verification).

- **Artifact upload via `actions/upload-artifact` + later `actions/download-artifact`**: The main `release.yml` uploads directly to the GitHub release in the build step (`gh release upload`). The `llm-release.yml` first uploads to GitHub Actions artifacts, then downloads and attaches to the release in a separate job. This is a slightly different pattern but structurally sound -- it decouples build from release creation, allowing the release creation to happen atomically after all builds complete.

### CUDA version coupling documentation

The header comment (lines 19-25) documents the CUDA version coupling between the CI pipeline (CUDA 12.4 toolkit) and the `cuda-runtime` recipe (CUDA 12.6.77 runtime). This is a cross-file coupling that's explicitly called out -- CUDA 12.x maintains forward compatibility within the major version. The inline comment at line 129-130 reinforces the coupling point. This documentation approach is appropriate; the coupling is inherent to the CUDA ecosystem and the comments make it explicit rather than discoverable only by debugging.

### Finalize release verification

The `finalize-release` job (lines 306-337) enumerates all 9 expected artifacts (8 binaries + checksums.txt) and verifies each is present before publishing. This matches the verification pattern in `release.yml`'s finalize-release job. The expected artifact list matches the design doc's platform table exactly.

### macOS backend suffix omission

macOS artifacts use `darwin-arm64` and `darwin-amd64` without a backend suffix, while Linux artifacts include `-cuda`, `-vulkan`, or `-cpu`. This matches the design doc's recipe sketch (lines 308-316), where macOS steps have no backend suffix because Metal is the only backend per architecture. The naming convention comment at line 16-17 documents this decision explicitly.

## Findings

No blocking findings.

### Advisory: Integration tests only cover CPU and Metal variants (lines 195-213)

The integration test matrix covers `darwin-arm64`, `darwin-amd64`, `linux-amd64-cpu`, and `linux-arm64-cpu`. The CUDA and Vulkan variants are built but not integration-tested. This is a reasonable trade-off since GPU hardware isn't available on standard CI runners, and the test only validates basic binary execution (`--help`, `--version`), not GPU functionality. However, the CUDA and Vulkan binaries could still be tested for basic execution (they should print help/version even without GPU hardware). This is a pragmatic concern rather than an architectural one -- the pattern of testing a subset of the build matrix is consistent with the reality of CI runner limitations.

### Advisory: `actions/upload-artifact@v6` and `actions/download-artifact@v7` use unpinned major versions (lines 189, 226, 252)

The `release.yml` pins action versions to specific commit SHAs (e.g., `actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd`). The new `llm-release.yml` pins `checkout` to the same SHA but uses unpinned major version tags (`@v6`, `@v7`) for `upload-artifact` and `download-artifact`. This is an inconsistency in the supply chain security posture. Not architecturally blocking -- the pipeline works correctly either way -- but the existing convention is SHA pinning for all actions. Aligning to the SHA-pinned convention would be more consistent.

## Overall Assessment

The workflow follows existing CI patterns and aligns precisely with the design doc's artifact naming requirements. The `artifact_suffix` + computed name approach is a clean adaptation of the existing matrix pattern for versioned artifact names. The CUDA version coupling documentation is well-placed. No structural violations or parallel patterns introduced. The two advisory findings (integration test coverage and action version pinning) are minor consistency items that don't affect architecture.
