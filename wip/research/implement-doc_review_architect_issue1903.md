# Architect Review: Issue #1903 (ci: add Renovate config and drift-check CI job)

## Files Changed

- `renovate.json` (new)
- `.github/workflows/drift-check.yml` (new)

## Findings

### 1. Parallel dependency update system (Dependabot + Renovate)

**File:** `renovate.json`, `.github/dependabot.yml`
**Severity:** Advisory

The repo already uses Dependabot for GitHub Actions version updates (`.github/dependabot.yml`). This PR adds Renovate as a second dependency update system for container images. Running two bot-driven update tools on the same repo is a recognized operational pattern when each handles a different concern that the other cannot -- Dependabot lacks custom file regex matching, and Renovate lacks Dependabot's native GitHub Actions ecosystem support. The design doc explicitly documents this rationale (Dependabot cannot parse Go source or arbitrary JSON).

This is not a parallel pattern violation because the two tools cover disjoint scopes with no overlap: Dependabot manages GitHub Actions versions, Renovate manages container image versions in JSON. There's no risk of them proposing conflicting PRs for the same dependency. However, it does mean the team needs to be aware that two different bots will be opening PRs, potentially with different auto-merge policies. The existing `dependabot-auto-merge.yml` workflow only handles Dependabot PRs -- there's no equivalent for Renovate.

**Verdict:** Architecturally acceptable. The scope separation is clean. Worth noting in the repo's contributing guide that two update tools are in use, but not blocking.

### 2. Drift-check workflow follows established CI patterns

**File:** `.github/workflows/drift-check.yml`
**Severity:** N/A (positive finding)

The drift-check workflow follows the same structural patterns as other CI jobs in this repo:
- Uses pinned action SHAs with version comments (matching `actions/checkout@de0fac...  # v6.0.2`)
- Uses `actions/setup-go` with `go-version-file: go.mod` (consistent with `test.yml`, `validate-embedded-deps.yml`)
- Uses `push` + `pull_request` triggers with path filters (consistent with other validation workflows)
- Two separate jobs for two separate concerns (embedded copy freshness, hardcoded reference detection)

The embedded-copy-freshness job (`go generate && git diff --exit-code`) is structurally parallel to the existing `go mod tidy && git diff --exit-code` check in `test.yml` (line 64-69). This is the right pattern -- not a duplicate, but the same approach applied to a different generated artifact.

### 3. Hardcoded reference check covers the right scope

**File:** `.github/workflows/drift-check.yml:55`
**Severity:** Advisory

The PATTERN regex checks for the five managed families (debian, fedora, archlinux, alpine, opensuse). It intentionally excludes `ubuntu:` because `SourceBuildSandboxImage` and the Ubuntu PPA override are out of scope per the design doc. The `DefaultValidationImage` in `internal/validate/executor.go` (which IS `debian:bookworm-slim`) is handled as a known exception with an explanatory comment.

One gap: the exception `'internal/validate/executor\.go:.*DefaultValidationImage'` is fragile. If `DefaultValidationImage` is referenced from a different file (say a new validation package), the exception won't cover it. But this is minor -- the drift-check would correctly flag it as a new hardcoded reference that needs attention, which is the safer failure mode.

### 4. Renovate config is minimal and correctly scoped

**File:** `renovate.json`
**Severity:** N/A (positive finding)

The Renovate config contains only the custom manager for `container-images.json`. It does not extend to other concerns (no Go module management, no npm dependency tracking, etc.). This is disciplined -- it would be easy to add additional Renovate managers while setting it up, but the PR correctly limits it to the specific problem from the design doc.

The `matchStrings` regex `"(?<depName>[a-z][a-z0-9./-]+):\s*(?<currentValue>[a-z0-9][a-z0-9._-]+)"` correctly handles the current image references including multi-segment names like `opensuse/tumbleweed`. The `datasourceTemplate: "docker"` tells Renovate to check Docker Hub for updates.

### 5. Design doc alignment

The implementation matches Phase 3 of the design doc:
- Renovate config with regex custom manager: matches design section "Decision 2" exactly
- CI drift-check with two concerns (embedded copy freshness + hardcoded reference detection): matches the design's Phase 3 description
- The `go generate ./internal/containerimages/... && git diff --exit-code` approach matches the design's "CI runs go generate ./... && git diff --exit-code to catch this" mitigation

One minor deviation: the design doc says "Greps all workflow files, Go source, and shell scripts for hardcoded image references, failing if any are found outside container-images.json." The implementation adds a nuanced exception system that the design doc didn't specify. This is a reasonable implementation detail -- the design's intent was "catch regressions" and a pure grep with no exceptions would create false positives on test files and comments.

## Summary

The implementation fits the existing architecture. The two new files introduce no new structural patterns -- `renovate.json` is a standard config file for an external tool, and `drift-check.yml` follows the same CI workflow conventions as the other 50+ workflow files in `.github/workflows/`. The Dependabot + Renovate coexistence is the one area to watch: the tools cover disjoint scopes (GitHub Actions vs container images), so there's no overlap, but the repo now has two different PR-proposing bots with potentially different auto-merge and review policies.

No blocking findings.
