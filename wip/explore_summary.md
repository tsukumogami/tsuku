# Exploration Summary: Sandbox Image Unification

## Problem (Phase 1)

Container image versions for Linux families are defined in three independent locations: `familyToBaseImage` in Go source, CI workflow YAML files, and test scripts. These definitions have already drifted (alpine:3.19 vs 3.21, opensuse/leap:15 vs tumbleweed). Each location must be updated manually and there's no mechanism to detect or prevent drift between them.

## Decision Drivers (Phase 1)

- Single source of truth: image versions must be declared exactly once
- Automated updates: a tool like Dependabot or Renovate should be able to propose version bumps
- Sandbox readiness: determine whether `--sandbox` can replace direct docker calls in CI
- Minimal disruption: solution shouldn't require major CI workflow restructuring
- Correctness: sandbox and CI must test against the same container images

## Research Findings (Phase 2)

- Sandbox can't replace CI docker calls yet: 3 critical gaps (no verification, no env passthrough, no structured results)
- 12 of 22 CI container usages are recipe validation that could eventually use --sandbox
- Dependabot can't update Go source or workflow container refs; Renovate can via regex custom manager
- Simplest approach: single config file read by both Go and CI, updatable by Renovate or manually

## Options (Phase 3)

- **JSON config file** (chosen): Single `container-images.json` at repo root, embedded in Go, read by CI with jq
- **Go source + Renovate**: Keep map in Go, use Renovate annotations; CI still can't read it
- **Proxy Dockerfile**: Dummy Dockerfile for Dependabot; confusing and requires sync step
- **Full sandbox migration**: Replace all CI docker calls with --sandbox; not ready (3 critical gaps)

## Decision (Phase 5)

**Problem:**
Container image versions for Linux families are defined independently in Go source, CI workflow files, and test scripts. These have already drifted: alpine:3.19 vs 3.21, opensuse/leap:15 vs tumbleweed. There's no mechanism to detect or prevent this drift, and with PR #1886 adding --target-family support, sandbox testing now depends on these images being correct and matching what CI validates against.

**Decision:**
Create a `container-images.json` file at the repo root as the single source of truth. A new `internal/containerimages/` Go package embeds this file and exports the parsed map, replacing the hardcoded `familyToBaseImage` in `container_spec.go` and the `DefaultSandboxImage`/`SourceBuildSandboxImage` constants. CI workflows and test scripts read the same file with `jq`. Renovate is configured with a regex custom manager to propose automated version bumps.

**Rationale:**
JSON is the simplest format that every consumer can parse natively: Go's standard library, jq on CI runners, and Renovate's regex engine. The approach eliminates drift by construction rather than detection, doesn't require Renovate to function (manual edits work fine), and doesn't force a premature sandbox migration. Research showed sandbox has three critical gaps (no verification, no env passthrough, no structured results) that make it unsuitable for replacing CI docker calls today. The config file works with both direct docker calls and --sandbox, preserving the migration path for later.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-02-22
