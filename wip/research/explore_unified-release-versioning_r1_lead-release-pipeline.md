# Release Pipeline Architecture & Unified Versioning

## Executive Summary

**Current State**: tsuku-dltest builds alongside Go CLI in single `release.yml` with unified `v*` tags and version injection. tsuku-llm builds independently via separate `llm-release.yml` with dedicated `tsuku-llm-v*` tags and separate artifact naming (`tsuku-llm-v{version}-{os}-{arch}[-{backend}]`). tsuku-dltest recipe resolves versions from `tsukumogami/tsuku` repo tags, while tsuku-llm recipe lacks a `[version]` section, preventing version resolution and recipe-based installation with automatic version discovery.

**Key Gap**: Bringing tsuku-llm into unified release requires four changes: (1) merge `llm-release.yml` jobs into `release.yml` to build all artifacts under single `v*` tag, (2) add `[version]` section to `tsuku-llm.toml` recipe to enable version resolution from `tsukumogami/tsuku` tags (not separate `tsuku-llm-v*` tags), (3) modify artifact naming convention to match unified scheme or introduce version-specific filtering in asset patterns, and (4) introduce enforcement mechanism to validate matching versions across CLI, dltest, and llm binaries.

## Current Build Pipelines

### release.yml: Go CLI + tsuku-dltest

**Trigger**: Tags matching `v*` (e.g., `v0.9.0`, `v1.0.0-rc.1`)

**Jobs**:
1. **release** (GoReleaser): Builds Go binaries for Linux amd64/arm64 and macOS amd64/arm64. Outputs versioned names: `tsuku-{os}-{arch}_{version}_{os}_{arch}` (e.g., `tsuku-linux-amd64_0.9.0_linux_amd64`).

2. **build-rust** (native runners): Parallel glibc builds across 4 platforms (linux-amd64, linux-arm64, darwin-amd64, darwin-arm64) using Rust 1.75. Version injected into `cmd/tsuku-dltest/Cargo.toml` by sed before `cargo build --release`. Outputs: `tsuku-dltest-{os}-{arch}` (e.g., `tsuku-dltest-linux-amd64`).

3. **build-rust-musl** (Alpine containers): Parallel musl builds for linux-amd64 and linux-arm64 using Alpine image from `container-images.json`. Version injected same way. Outputs: `tsuku-dltest-{os}-{arch}-musl` (e.g., `tsuku-dltest-linux-amd64-musl`).

4. **integration-test**: Runs on 4 platforms. Downloads both `tsuku-{os}-{arch}_{version}_{os}_{arch}` and `tsuku-dltest-{os}-{arch}` binaries. **Validates version match**: both binaries must report version matching `EXPECTED_VERSION` (derived from git tag by stripping "v" prefix). Integration tests only run on macOS (darwin-amd64, darwin-arm64) for tsuku-llm equivalent, but here they run on all 4 Linux and macOS platforms.

5. **finalize-release**: After integration-test and build-rust-musl pass. Verifies presence of exactly 10 artifacts: 4 Go binaries (versioned), 4 glibc Rust binaries, 2 musl Rust binaries. Generates unified `checksums.txt` (sha256sum of all `tsuku-*` files). Publishes release (removes draft flag).

### llm-release.yml: tsuku-llm (Independent)

**Trigger**: Tags matching `tsuku-llm-v*` (e.g., `tsuku-llm-v0.3.0`)

**Jobs**:
1. **build-llm** (6 platform variants): Matrix builds for darwin-arm64 (metal), darwin-amd64 (metal), linux-amd64-cuda, linux-amd64-vulkan, linux-arm64-cuda, linux-arm64-vulkan. Version extracted by `VERSION="${TAG#tsuku-llm-v}"` and injected into `tsuku-llm/Cargo.toml`. Feature flags set per backend (metal/cuda/vulkan). Outputs: `tsuku-llm-v{version}-{platform}` (e.g., `tsuku-llm-v0.3.0-darwin-arm64`, `tsuku-llm-v0.3.0-linux-amd64-cuda`).

2. **integration-test** (macOS only): Runs on macOS platforms only (darwin-arm64, darwin-amd64). Downloads artifacts and runs `--help` and `--version` commands. **Warning-only version check**: logs if version not found in output, does not fail.

3. **create-release**: Downloads all artifacts from build-llm jobs, flattens directory structure, generates `checksums.txt`, creates GitHub release (draft), uploads all binaries and checksums.

4. **finalize-release**: Verifies 7 expected artifacts present (6 binaries + checksums), publishes release.

**Key Difference**: Uses GitHub Artifacts API (upload-artifact/download-artifact actions) to pass binaries between jobs, rather than releasing to draft release immediately like release.yml does.

## Version Resolution in Recipes

### tsuku-dltest.toml (Current)

```toml
[metadata]
name = "tsuku-dltest"
description = "dlopen verification helper for tsuku verify"
homepage = "https://github.com/tsukumogami/tsuku"
supported_libc = ["glibc"]

[version]
tag_prefix = "v"

[[steps]]
action = "github_file"
repo = "tsukumogami/tsuku"
asset_pattern = "tsuku-dltest-{os}-{arch}"
binaries = [{src = "tsuku-dltest-{os}-{arch}", dest = "bin/tsuku-dltest"}]

[verify]
command = "tsuku-dltest --version"
pattern = "tsuku-dltest v{version}"
```

**Resolution Logic**:
- `[version]` section present with `tag_prefix = "v"`
- No `source` field → triggers **GitHubRepoStrategy** (priority 90: PriorityExplicitHint) via `github_repo = "tsukumogami/tsuku"`
- Creates `GitHubProviderWithPrefix(resolver, "tsukumogami/tsuku", "v")`
- Provider calls `resolver.ListGitHubVersions("tsukumogami/tsuku")` → fetches all tags from `tsukumogami/tsuku` repo, filters those starting with "v", strips "v" prefix, returns sorted list
- `ResolveLatest()` returns first stable (non-prerelease) version
- `asset_pattern` uses version placeholder: `tsuku-dltest-{os}-{arch}` (no version in filename per release.yml naming)

**Asset Pattern Mismatch**: Recipe asset_pattern doesn't include version, but Go binaries use versioned naming (`tsuku-{os}-{arch}_{version}_{os}_{arch}`). For dltest the pattern is correct (release.yml outputs `tsuku-dltest-{os}-{arch}`), but for future unified releases the pattern would need adjustment.

### tsuku-llm.toml (Current)

```toml
[metadata]
name = "tsuku-llm"
description = "Local LLM inference engine"
homepage = "https://github.com/tsukumogami/tsuku-llm"
supported_os = ["linux", "darwin"]
supported_arch = ["amd64", "arm64"]
supported_libc = ["glibc"]
unsupported_reason = "All variants are built against glibc. musl/Alpine Linux is not supported because llama.cpp and CUDA runtime libraries require glibc. Windows is not supported because the release pipeline does not produce Windows artifacts."

[[steps]]
action = "github_file"
when = { os = ["darwin"], arch = "arm64" }
repo = "tsukumogami/tsuku-llm"
asset_pattern = "tsuku-llm-v{version}-darwin-arm64"
binary = "tsuku-llm"
# ... 5 more steps for other platforms ...

[verify]
command = "tsuku-llm --version"
pattern = "{version}"
```

**No [version] section** → Triggers **InferredGitHubStrategy** (priority 10: PriorityInferred) from `repo = "tsukumogami/tsuku-llm"` in github_file action. Creates `NewGitHubProvider(resolver, "tsukumogami/tsuku-llm")` with **no tag prefix filtering**.

**Result**: Version resolution pulls tags from `tsukumogami/tsuku-llm` repo (the separate LLM repo), not from `tsukumogami/tsuku` main repo. This decouples versioning and prevents recipe installation from finding matching main-repo versions.

**Asset Pattern Matches llm-release.yml**: All 6 step actions use `asset_pattern = "tsuku-llm-v{version}-{platform}"` which exactly matches `llm-release.yml` artifact naming.

## finalize-release Artifact Verification

**release.yml finalize-release** (lines 251-286):

```bash
# Verifies these 10 artifacts (by exact name):
# Go binaries (4): tsuku-linux-amd64_${VERSION}_linux_amd64, tsuku-linux-arm64_${VERSION}_linux_arm64, tsuku-darwin-amd64_${VERSION}_darwin_amd64, tsuku-darwin-arm64_${VERSION}_darwin_arm64
# glibc Rust (4): tsuku-dltest-linux-amd64, tsuku-dltest-linux-arm64, tsuku-dltest-darwin-amd64, tsuku-dltest-darwin-arm64
# musl Rust (2): tsuku-dltest-linux-amd64-musl, tsuku-dltest-linux-arm64-musl
```

Loop checks `gh release view "$TAG" --repo "$GITHUB_REPOSITORY" --json assets -q '.assets[].name'` and ensures each expected artifact is present. **Fails release if any artifact missing** (non-zero exit code).

**llm-release.yml finalize-release** (lines 279-315):

```bash
# Verifies these 7 artifacts:
# Binaries (6): tsuku-llm-v${VERSION}-darwin-arm64, tsuku-llm-v${VERSION}-darwin-amd64, tsuku-llm-v${VERSION}-linux-amd64-cuda, tsuku-llm-v${VERSION}-linux-amd64-vulkan, tsuku-llm-v${VERSION}-linux-arm64-cuda, tsuku-llm-v${VERSION}-linux-arm64-vulkan
# Checksums (1): checksums.txt
```

Same verification loop structure. **Also fails if any artifact missing**.

## Key Findings: Unified Release Requirements

### 1. Tag Strategy Decision

**Current**: 
- Main repo uses `v*` tags (single namespace)
- LLM repo uses `tsuku-llm-v*` tags (separate namespace)

**To Unify**:
- All three artifacts (CLI, dltest, llm) must ship under single `v*` tag in main repo
- This requires moving llm-release.yml jobs into release.yml or restructuring to trigger off single tag
- **Breaking change for current tsuku-llm users**: Existing installations using `tsuku-llm-v*` tags would break; would need recipe migration script or deprecation period

### 2. Artifact Naming Alignment

**Current**:
- Go CLI: `tsuku-{os}-{arch}_{version}_{os}_{arch}` (GoReleaser default)
- tsuku-dltest glibc: `tsuku-dltest-{os}-{arch}` (release.yml lines 121)
- tsuku-dltest musl: `tsuku-dltest-{os}-{arch}-musl` (release.yml line 167)
- tsuku-llm: `tsuku-llm-v{version}-{platform}[-{backend}]` (llm-release.yml line 95)

**Issue**: No unified convention. Go binaries include version in filename; dltest doesn't; llm includes "v" prefix.

**Options to Unify**:
- **Option A**: Keep current naming, update recipes to handle different patterns (complex, fragile)
- **Option B**: Standardize to `{tool}-{os}-{arch}[-{backend}]` without version in filename for all, update GoReleaser config (breaking change)
- **Option C**: Standardize to `{tool}-v{version}-{os}-{arch}[-{backend}]` with version for all (breaking change, large renaming for thousands of existing Go tool recipes)

**Recommendation**: Option A is least disruptive. Recipes already handle heterogeneous asset patterns via template engine (`{os}`, `{arch}`, `{version}` placeholders).

### 3. Version Section Configuration

**tsuku-dltest.toml**: Has explicit `[version]` section with `tag_prefix = "v"` and resolves from main repo via `github_repo` field (PriorityExplicitHint).

**tsuku-llm.toml**: **Missing `[version]` section entirely**. This must be added to:
- Declare explicit version source instead of inferring from steps
- Point to main repo (`tsukumogami/tsuku`) instead of separate `tsukumogami/tsuku-llm` repo
- Use same `tag_prefix = "v"` as dltest

**Proposed Addition to tsuku-llm.toml**:

```toml
[version]
github_repo = "tsukumogami/tsuku"
tag_prefix = "v"
```

This switches version resolution from InferredGitHubStrategy (priority 10, pulls from `tsukumogami/tsuku-llm` in step params) to GitHubRepoStrategy (priority 90, pulls from main repo). **Result**: Both dltest and llm recipes now resolve versions from same source, enabling single-source version management.

### 4. Version Enforcement Mechanism

**Current State**: Integration tests validate individual binary versions but don't enforce cross-binary matching.

**release.yml integration-test** (lines 218-244):
- Downloads both `tsuku` and `tsuku-dltest` binaries
- Validates each independently against `EXPECTED_VERSION` (derived from tag)
- Doesn't prevent mismatched versions if build timestamps differ

**llm-release.yml integration-test** (lines 210-223):
- Only runs on macOS (missing linux-amd64, linux-arm64)
- Version check is warning-only (lines 220-222): `if ! echo "$BINARY_VERSION" | grep -q "$VERSION"; then echo "WARNING: Version string not found"`
- Does not fail the job

**To Enforce Unified Versioning**:
1. **Add cross-binary version comparison** to release.yml integration-test:
   ```bash
   TSUKU_VERSION=$(./artifacts/${TSUKU_BINARY} --version 2>&1)
   DLTEST_VERSION=$(./artifacts/${{ matrix.dltest_binary }} --version 2>&1)
   LLM_VERSION=$(./artifacts/${{ matrix.llm_binary }} --version 2>&1)
   if ! echo "$TSUKU_VERSION" "$DLTEST_VERSION" "$LLM_VERSION" | sort -u | wc -l | grep -q "^1$"; then
     echo "ERROR: Version mismatch across binaries"
     exit 1
   fi
   ```

2. **Extend integration-test to all platforms** (not just macOS for llm): Run llm tests on Linux variants that have CUDA/Vulkan builds.

3. **Add finalize-release validation**: Before publishing, verify all three binary version strings match expected tag version.

### 5. Artifact Count and finalize-release

**Current release.yml finalize-release** expects 10 artifacts:
- 4 Go binaries
- 4 glibc dltest binaries
- 2 musl dltest binaries

**With llm integrated**, finalize-release would need to expect 16+ artifacts (adding 6 llm variants). The verification loop (lines 279-285) would expand:

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
  # llm (6) — NEW
  tsuku-llm-v${VERSION}-darwin-arm64
  tsuku-llm-v${VERSION}-darwin-amd64
  tsuku-llm-v${VERSION}-linux-amd64-cuda
  tsuku-llm-v${VERSION}-linux-amd64-vulkan
  tsuku-llm-v${VERSION}-linux-arm64-cuda
  tsuku-llm-v${VERSION}-linux-arm64-vulkan
)
```

Checksum generation (`sha256sum tsuku-*`) would naturally include all artifacts.

## Implementation Roadmap

### Phase 1: Prepare Main Repo (No Breaking Changes)
1. Add `[version]` section to `recipes/t/tsuku-llm.toml` pointing to main repo
2. Add second `llm-release.yml` trigger: in addition to `tsuku-llm-v*`, also trigger on main `v*` tags (requires `if: startsWith(github.ref_name, 'tsuku-llm-v') || startsWith(github.ref_name, 'v')`)
3. Extract version parsing logic to handle both tag formats (`v*` → use directly; `tsuku-llm-v*` → strip prefix)

### Phase 2: Validate Version Matching
1. Extend integration-test to validate version consistency across all three binaries
2. Update finalize-release to verify all binaries present and versions match

### Phase 3: Consolidate to Single release.yml (Breaking Change)
1. Merge `llm-release.yml` build-llm jobs into `release.yml` as new parallel stage after release job (depends on `release` job for tag consistency)
2. Consolidate finalize-release jobs
3. Deprecate `tsuku-llm-v*` tags; document migration to main `v*` tags
4. Update release documentation

### Phase 4: Enforce Version Consistency at Runtime
1. Add `--check-versions` flag to `tsuku verify` command
2. Validate that installed `tsuku`, `tsuku-dltest`, and `tsuku-llm` all report matching version strings
3. Warn or error if versions mismatch

## Conclusion

Bringing tsuku-llm into unified release is achievable and recommended. The main challenge is not technical but organizational: consolidating two separate version namespaces (`v*` and `tsuku-llm-v*`) and artifact repositories (main vs. llm). Adding the `[version]` section to tsuku-llm recipe is the minimal change to enable version resolution from the main repo; merging workflows and enforcing version consistency follow naturally.
