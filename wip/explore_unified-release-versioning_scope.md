# Explore Scope: unified-release-versioning

## Core Question

How should tsuku unify all its artifacts (CLI, dltest, llm) into a single release
tag, standardize artifact naming, enable their recipes to resolve versions from repo
tags, and enforce same-version constraints so these binaries always ship and run in
lockstep?

## Context

Issues #2108 and #2109 both propose integrating tsuku-llm into the main `release.yml`
pipeline (currently released independently via `llm-release.yml` with `tsuku-llm-v*`
tags). tsuku-dltest is already built alongside the Go CLI. The user wants to go further:
version resolution for the tsuku-dltest and tsuku-llm recipes should derive from the
same `v*` tags, and tsuku should enforce that its companion binaries match its own
version -- eliminating any need for backward compatibility between these binaries.

Issue #1791 tracks aligning artifact naming between the release pipeline and recipe
asset patterns. Currently the pipeline produces `tsuku-llm-linux-amd64-cuda` (no
version) but the recipe expects `tsuku-llm-v{version}-linux-amd64-cuda`. This naming
alignment is now in scope as part of the unified release design.

## In Scope

- Unified `v*` tag release pipeline for all artifacts
- Artifact naming standardization across CLI, dltest, and llm (#1791)
- Version resolution for tsuku-dltest and tsuku-llm recipes using repo tags
- Version constraint mechanism in tsuku CLI (require same-version companions)
- Retiring `llm-release.yml` and independent `tsuku-llm-v*` tags

## Out of Scope

- GPU variant selection UX (#1776)
- General recipe version resolution changes for external tools

## Research Leads

### Round 1 (completed)

1. **How does the current release pipeline build and version tsuku-dltest, and what would extending it to tsuku-llm require?**
2. **How do the tsuku-dltest and tsuku-llm recipes currently resolve versions, and can they use GitHub tags from the tsuku repo?**
3. **Does tsuku have a mechanism to enforce version constraints between itself and companion binaries?**
4. **How does tsuku currently discover and invoke dltest and llm at runtime?**
5. **What does retiring `llm-release.yml` look like, and are there consumers of the separate `tsuku-llm-v*` tags?**

### Round 2

6. **What artifact naming convention should be standardized across all three binaries, and what does changing it break?**
   #1791 identifies a mismatch between pipeline artifact names and recipe patterns. With unified releases, we need one naming convention. Need to evaluate options against existing recipes, GoReleaser config, and downstream consumers.

7. **How should the dltest compile-time version pinning pattern be extended to llm, given llm's gRPC daemon architecture?**
   Round 1 found dltest already has ldflags-based version pinning with auto-reinstall. llm has none. Need to understand the llm addon manager's lifecycle and where version checking fits.

8. **What does the GPU build dependency setup (CUDA toolkit, Vulkan SDK) add to the release pipeline, and how do GPU artifact variants affect the unified naming scheme?**
   Merging llm-release.yml into release.yml brings GPU dependencies. Need to understand build time impact and how backend variants (cuda/vulkan/metal) fit the naming convention.
