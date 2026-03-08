---
status: Proposed
problem: |
  Tsuku's three binaries (CLI, dltest, llm) are released independently with inconsistent versioning, naming, and enforcement. The CLI and dltest share a `v*` tag in the main repo, but llm has a separate workflow (`llm-release.yml`) triggered by `tsuku-llm-v*` tags that has never been used. Artifact naming varies across all three (GoReleaser default, simple, version-prefixed). Only dltest has compile-time version pinning; llm accepts any installed version with no compatibility checking. This creates risk of version mismatches between binaries that share internal protocols.
---

# DESIGN: Unified Release Versioning

## Status

Proposed

## Context and Problem Statement

Tsuku ships three companion binaries that communicate via internal protocols: the Go CLI (main binary), tsuku-dltest (dlopen verification, invoked as subprocess), and tsuku-llm (LLM inference, gRPC daemon). These binaries are currently released through two independent pipelines with separate version namespaces.

The CLI and dltest already share a `v*` tag release via `release.yml`, producing 10 artifacts. The llm binary has a separate `llm-release.yml` triggered by `tsuku-llm-v*` tags -- but no such tags have ever been pushed, meaning the workflow has never run. The llm recipe currently points to a separate `tsukumogami/tsuku-llm` repo for version resolution.

This creates several problems:

1. **No version lockstep.** The CLI can run with any version of llm, risking protocol incompatibilities. Only dltest has compile-time version pinning with auto-reinstall.

2. **Inconsistent artifact naming.** The CLI uses GoReleaser's default (`tsuku-{os}-{arch}_{version}_{os}_{arch}`), dltest uses `tsuku-dltest-{os}-{arch}`, and llm uses `tsuku-llm-v{version}-{platform}`. The version-in-filename for CLI and llm is redundant since `github_file` already resolves within a tagged release.

3. **Recipe version resolution gap.** The llm recipe has no `[version]` section and falls back to InferredGitHubStrategy (priority 10), pulling from the wrong repo. Adding explicit version configuration would promote it to GitHubRepoStrategy (priority 90) and point it at the main repo's `v*` tags.

4. **No version constraint mechanism.** Dependencies in the recipe schema are version-agnostic string arrays. There's no way to express "this tool requires companion binaries at its own version."

Exploration across 8 research leads confirmed that the separate llm release has no consumers, the dltest version pinning pattern generalizes cleanly to llm, GPU build dependencies add ~10 minutes but parallelize fully, and artifact naming can be standardized without breaking existing users.

## Decision Drivers

- **Version safety:** Eliminate risk of running mismatched binary versions that share internal protocols (subprocess args for dltest, gRPC for llm)
- **Single release tag:** All artifacts should ship under one `v*` tag so a release is atomic
- **Recipe consistency:** Both companion recipes should resolve versions from the same source (`tsukumogami/tsuku` tags) using the same mechanism
- **Naming clarity:** Artifact filenames should follow a consistent pattern; version in filename is redundant given how `github_file` works
- **Pipeline simplicity:** One release workflow instead of two, with clear job dependency graph
- **GPU build constraints:** CUDA toolkit (5-8 min) and Vulkan SDK (3-5 min) add infrastructure but parallelize with cargo compilation; macOS Metal has zero setup overhead
- **Backward compatibility:** No external consumers of `tsuku-llm-v*` tags exist, making migration zero-risk
- **Backend suffix convention:** macOS omits GPU backend suffix (Metal is implicit); Linux includes cuda/vulkan to differentiate variants. This asymmetry is intentional and should be preserved.

## Considered Options

### Decision 1: Implementation strategy

**Context:** The unified release problem spans four dimensions (recipe resolution, version pinning, pipeline consolidation, artifact naming). These can be addressed all at once or incrementally.

**Chosen: Incremental Migration.**

Each dimension ships as an independent PR validated before the next begins. This gives the smallest blast radius per change and allows progressive validation across release cycles. If pipeline consolidation introduces a regression, it can be reverted without affecting version pinning work that already landed. Reviewers focus on one concern per PR instead of understanding all dimensions simultaneously.

The true implementation order accounts for dependencies between phases: dltest recipe fix first (works today), then llm version pinning (independent of pipeline), then pipeline merge (enables llm artifacts under `v*` tags), then llm recipe fix (depends on pipeline), then naming standardization.

*Alternative rejected: Full Consolidation.* Addresses all dimensions in 1-3 tightly-coupled PRs for an atomic transition with no intermediate inconsistency. Rejected because the high review burden and all-or-nothing rollback cost aren't justified -- the dimensions are separable enough that incremental delivery is cleaner. The naming-pipeline coupling (changing artifact names requires coordinated pipeline and recipe updates) is real but manageable within a single phase rather than requiring all phases to land together.

*Alternative rejected: Minimal (Pinning + Recipe Fix).* Solves the core safety problem with ~8 files changed. Rejected because keeping two workflows that both fire on `v*` tags introduces a release creation race condition (both try to create a GitHub release for the same tag). This fragility would need its own fix, and the naming inconsistency would remain as acknowledged-but-unaddressed debt. The incremental approach delivers version safety just as quickly (pinning is an early phase) while also committing to the full cleanup.

### Decision 2: Artifact naming convention

**Context:** Three different naming patterns exist across the binaries. The `github_file` action resolves assets within a tagged release, making version in filenames redundant.

**Chosen: No version in filenames.** Convention: `{tool}-{os}-{arch}[-{backend}]`.

Examples: `tsuku-linux-amd64`, `tsuku-dltest-linux-amd64`, `tsuku-llm-linux-amd64-cuda`. Backend suffix is asymmetric by design: omitted on macOS (Metal is the only option), included on Linux (cuda/vulkan differentiate variants). This matches how dltest already names its artifacts and aligns with how `github_file` resolves assets.

*Alternative rejected: Version in all filenames.* Format `{tool}-{version}-{os}-{arch}[-{backend}]`. More explicit but redundant since the release tag already identifies the version. Would require all recipes using `github_file` to include `{version}` in their asset patterns for tsuku's own binaries.

*Alternative rejected: Keep current mix.* Most fragile, requires recipes to handle heterogeneous patterns across the three binaries. Adds cognitive load for contributors.

### Decision 3: Version enforcement mechanism

**Context:** Only dltest has compile-time version pinning. llm accepts any installed version with no compatibility checking.

**Chosen: Compile-time ldflags pinning (same pattern as dltest) with optional gRPC handshake follow-up.**

Add `pinnedLlmVersion` to `internal/verify/version.go`, inject via `.goreleaser.yaml` ldflags, enforce in `addon/manager.go` with auto-reinstall on mismatch. Dev mode accepts any version; release mode requires exact match. A follow-up phase adds `addon_version` to the gRPC StatusResponse proto for runtime version visibility, improving diagnostic error messages.

*Alternative rejected: Recipe-level version constraints.* A new recipe field like `version_lock = "self"` would pin at install time. Rejected because it requires recipe schema changes for a special case (only companion binaries need this), and compile-time embedding is simpler and more reliable -- the constraint is guaranteed at build time rather than depending on install-time resolution.

## Decision Outcome

Unify tsuku's release versioning through an incremental migration in 5 phases:

1. **Recipe fix (dltest)** -- Add explicit `github_repo` to dltest recipe's `[version]` section
2. **Compile-time llm pinning** -- Extend dltest's ldflags pattern to llm with auto-reinstall
3. **Pipeline merge** -- Move llm builds into `release.yml`, extend finalize-release to 16+ artifacts, delete `llm-release.yml`
4. **Recipe fix (llm) + naming standardization** -- Update llm recipe to resolve from main repo, standardize all artifact names to `{tool}-{os}-{arch}[-{backend}]`
5. **gRPC version handshake** -- Add `addon_version` to StatusResponse proto for runtime diagnostics

Key properties:
- All three binaries ship under a single `v*` tag
- Version lockstep enforced at compile time (release mode requires exact match, dev mode permissive)
- Artifact naming follows `{tool}-{os}-{arch}[-{backend}]` with no version in filename
- Backend suffix is asymmetric: omitted on macOS (Metal implicit), included on Linux (cuda/vulkan)
- Each phase is independently deployable and validated before the next begins
