# Explore Scope: unified-release-versioning

## Core Question

How should tsuku unify all its artifacts (CLI, dltest, llm) into a single release
tag, enable their recipes to resolve versions from repo tags, and enforce same-version
constraints so these binaries always ship and run in lockstep?

## Context

Issues #2108 and #2109 both propose integrating tsuku-llm into the main `release.yml`
pipeline (currently released independently via `llm-release.yml` with `tsuku-llm-v*`
tags). tsuku-dltest is already built alongside the Go CLI. The user wants to go further:
version resolution for the tsuku-dltest and tsuku-llm recipes should derive from the
same `v*` tags, and tsuku should enforce that its companion binaries match its own
version -- eliminating any need for backward compatibility between these binaries.

## In Scope

- Unified `v*` tag release pipeline for all artifacts
- Version resolution for tsuku-dltest and tsuku-llm recipes using repo tags
- Version constraint mechanism in tsuku CLI (require same-version companions)
- Retiring `llm-release.yml` and independent `tsuku-llm-v*` tags

## Out of Scope

- Artifact naming alignment (#1791 tracks separately)
- GPU variant selection UX (#1776)
- General recipe version resolution changes for external tools

## Research Leads

1. **How does the current release pipeline build and version tsuku-dltest, and what would extending it to tsuku-llm require?**
   Issues #2108/#2109 propose following the `build-rust` pattern. Need to understand the actual pipeline structure, version injection, and artifact verification to assess feasibility and effort.

2. **How do the tsuku-dltest and tsuku-llm recipes currently resolve versions, and can they use GitHub tags from the tsuku repo?**
   These are internal tools whose recipes should resolve versions from the same `v*` tags used for the Go CLI. Need to understand what version providers are available and whether existing ones support tag-based resolution from the tsuku repo.

3. **Does tsuku have a mechanism to enforce version constraints between itself and companion binaries?**
   The user wants tsuku to require dltest/llm at its own exact version. Need to determine whether dependency constraints or version pinning already exists, or if a new mechanism is needed.

4. **How does tsuku currently discover and invoke dltest and llm at runtime?**
   Understanding the current integration points reveals where version checking should go and what happens when there's a version mismatch.

5. **What does retiring `llm-release.yml` look like, and are there consumers of the separate `tsuku-llm-v*` tags?**
   Need to ensure nothing depends on the independent release flow before removing it.
