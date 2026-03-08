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
