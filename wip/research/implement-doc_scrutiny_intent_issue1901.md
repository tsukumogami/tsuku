# Scrutiny Review: Intent - Issue #1901

## Issue
#1901: refactor(sandbox): create containerimages package and centralized config

## Design Doc
docs/designs/DESIGN-sandbox-image-unification.md

## Sub-check 1: Design Intent Alignment

### container-images.json at repo root

**Design intent**: "Create a `container-images.json` file at the repo root that maps family names to container images" with five entries (debian, rhel, arch, alpine, suse) using current CI image versions to fix drift.

**Implementation**: `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/container-images.json` exists at the repo root with exactly the five families and the corrected image versions (alpine:3.21, opensuse/tumbleweed). Matches design.

### internal/containerimages/ package

**Design intent**: "New Go package with three responsibilities: 1. Embed container-images.json via go:embed, 2. Parse the JSON into a map[string]string on init, 3. Export ImageForFamily(family string) (string, bool) and DefaultImage() string."

**Implementation**: `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/internal/containerimages/containerimages.go` implements all three responsibilities. The `//go:generate cp ../../container-images.json container-images.json` directive copies the root file. The `//go:embed container-images.json` directive embeds the local copy. `init()` parses JSON and validates the required "debian" entry. `ImageForFamily` and `DefaultImage` match the specified signatures.

Additionally exports `Families() []string` which is not in the design doc's key interfaces section. This function has no callers outside its own test file. **Advisory** -- not harmful, but it's an exported API surface that isn't specified. If downstream issues need it (e.g., #1902 iterating over families), it's forward-looking. If not, it's dead API surface. Minor concern.

Note: `Families()` doc comment says "sorted" but the implementation does not sort. This is a code defect, not an intent issue. Out of scope for this review.

### Sandbox code updates

**Design intent**: "container_spec.go replaces familyToBaseImage lookups with containerimages.ImageForFamily(). requirements.go replaces DefaultSandboxImage with containerimages.DefaultImage(). SourceBuildSandboxImage stays as a constant."

**Implementation**:
- `container_spec.go` line 113: `baseImage, ok := containerimages.ImageForFamily(family)` -- matches design. The old `familyToBaseImage` map is fully removed. A comment at line 34-35 explains where the mapping now lives.
- `requirements.go` line 79: `defaultImage := containerimages.DefaultImage()` -- matches design. `DefaultSandboxImage` constant is removed.
- `requirements.go` line 16: `const SourceBuildSandboxImage = "ubuntu:22.04"` -- unchanged, as design specifies.

All matches design intent.

### Makefile integration

**Design intent**: "A go generate directive or build-time copy step keeps the embedded copy in sync with the repo root file."

**Implementation**: Makefile `build` target runs `go generate ./internal/containerimages/...` before `go build`. This ensures the local development workflow keeps the embedded copy fresh.

Gap: `build-test` target does NOT run `go generate`. If a developer edits the root JSON and runs `make build-test` without running `make build` first, the test binary will have a stale embedded copy. However, the embedded copy is committed to git, so `build-test` will use whatever is in the repo. The `go generate` step is only needed when the root file changes and the developer hasn't committed yet. **Advisory** -- not a design intent violation, but a small workflow gap.

Gap: `.goreleaser.yaml` has no `before.hooks` to run `go generate`. Release builds rely on the committed embedded copy being in sync. This is acceptable because CI (planned in #1903) will verify sync with `go generate && git diff --exit-code`, but until #1903 ships, there's no automated check. **Advisory** -- acceptable because the embedded copy is committed.

### Test coverage

**Design intent**: The design doc doesn't specify test details, but the test plan (scenarios 1-9) covers the package tests, removal of old constructs, and go vet.

**Implementation**: `containerimages_test.go` tests all five families, unknown family behavior, DefaultImage, embedded JSON validity, all-entries-non-empty, and the Families function. Sandbox tests in `requirements_test.go` and `container_spec_test.go` are updated with new image values and use `containerimages.DefaultImage()` instead of the removed constant. Integration test in `sandbox_integration_test.go` imports `containerimages`. All test files consistent with the refactoring.

### Overall design intent alignment

The implementation closely follows the design doc's Solution Architecture section. The data flow (JSON at root -> go:generate copy -> go:embed -> parsed map -> exported functions -> sandbox imports) matches the described architecture exactly. The scope boundaries are respected: SourceBuildSandboxImage stays hardcoded, the PPA override in container_spec.go stays unchanged, CI workflow changes are deferred to #1902.

No blocking findings for design intent alignment.

---

## Sub-check 2: Cross-Issue Enablement

### #1902: ci: migrate workflow container images to centralized config

**Needs**: container-images.json at repo root with all 5 families readable by jq.

**Provided**: container-images.json exists at the repo root. It's a flat JSON object with string keys and string values -- the simplest format for jq consumption. All five families are present. `jq -r '.alpine' container-images.json` would return `alpine:3.21`. The format exactly matches what #1902 needs.

No issues.

### #1903: ci: add Renovate config and drift-check CI job

**Needs**:
1. `go generate` to work (for the drift-check `go generate && git diff --exit-code`)
2. `container-images.json` format stable for regex matching

**Provided**:
1. `go generate ./internal/containerimages/...` runs the `cp` command that copies the root file to the package directory. The drift-check pattern will work: modifying root -> running go generate -> git diff will show changes if someone forgot to run generate.
2. The JSON format uses `"image:tag"` patterns that match the Renovate regex in the design doc: `"(?<depName>[a-z][a-z0-9./-]+):\s*(?<currentValue>[a-z0-9][a-z0-9._-]+)"`. All five entries follow this pattern.

No issues.

### #1904: chore: add container-images.json to CODEOWNERS

**Needs**: container-images.json at repo root.

**Provided**: File exists at the expected location.

No issues.

### Cross-issue assessment

The foundation is solid for all three downstream issues. The JSON format is simple, the file location is at the repo root as all downstream issues expect, and the go:generate mechanism is in place for the drift-check. No fields, functions, or structural elements are missing that downstream issues would need.

---

## Backward Coherence

First issue in the sequence. No previous summary to check against. Skipped.

---

## Summary of Findings

### Blocking: 0

### Advisory: 2

1. **Exported `Families()` function not in design doc's key interfaces.** The function has no callers outside its own test file. It's not harmful and might be useful for downstream issues, but it's undocumented API surface. Minor concern.

2. **`build-test` Makefile target and `.goreleaser.yaml` don't run `go generate`.** The embedded copy is committed to git so this works in practice, and #1903 will add the drift-check CI job. But until then, a developer workflow gap exists.
