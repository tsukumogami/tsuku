# Architecture Review: Issue #1901

## Summary

Issue #1901 creates a new `internal/containerimages/` package as the Go-side single source of truth for Linux family-to-container-image mappings. It replaces the hardcoded `familyToBaseImage` map in `container_spec.go` and the `DefaultSandboxImage` constant in `requirements.go` with calls to the new package.

## Findings

### 1. GoReleaser and build-test do not run `go generate` (Blocking)

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/.goreleaser.yaml`
**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/Makefile` (line 19)

The `go:generate` directive in `containerimages.go` copies the root `container-images.json` into the package directory before embedding. The `make build` target runs this (Makefile line 7). However:

1. **`.goreleaser.yaml`** has no `before:` hooks to run `go generate`. GoReleaser runs `go build` directly. Release builds will embed whatever `internal/containerimages/container-images.json` happens to be committed, NOT the canonical root file. If someone edits the root file and commits without running `go generate`, the release binary ships with stale data.

2. **`make build-test`** (Makefile line 19) does not run `go generate` either. QA testing via `make build-test` would use stale embedded data.

The design doc acknowledges this risk ("CI runs `go generate ./... && git diff --exit-code` to catch this") but that CI check is Phase 3 (#1903), not this issue. Meanwhile, the `go:generate` + commit pattern creates a window where the embedded copy can silently drift from the canonical file.

**This is a structural issue** because the correctness of the embedded data depends on a build step that two of the three build paths skip. Fix options:
- Add a `before:` hook to `.goreleaser.yaml` that runs `go generate ./internal/containerimages/...`
- Add `go generate` to the `build-test` target
- Or: commit the `internal/containerimages/container-images.json` to git and add a CI check that it matches the root file (which is what the design already plans, making the generate step a convenience rather than a correctness requirement -- but it needs to be clearly documented that the committed copy IS the embedded source, not a derived artifact)

**Severity**: Blocking. The release pipeline silently ships stale image data if the root file is edited.

### 2. `Families()` claims sorted output but does not sort (Advisory)

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/internal/containerimages/containerimages.go`, line 52

The doc comment says "Families returns a sorted list" but the implementation iterates a map (nondeterministic order) and returns the unsorted slice. The function is not currently called outside tests, so this doesn't break anything yet. However, exporting a function with a documented guarantee that it doesn't fulfill will cause problems when callers depend on the sort order.

Fix: add `sort.Strings(fams)` before the return, or change the doc to say "unordered list."

**Severity**: Advisory. No current callers depend on order, and the test (`TestFamilies`) only checks set membership, not order.

### 3. Design alignment is correct

The implementation matches the design doc's architecture:

- **New package placement**: `internal/containerimages/` is a leaf package with zero internal imports. It depends only on stdlib (`embed`, `encoding/json`, `fmt`). This is the correct dependency direction -- a lower-level data package imported by the higher-level `sandbox` package.

- **API surface**: `ImageForFamily(family) (string, bool)` and `DefaultImage() string` match the design doc's specified interfaces exactly.

- **`container_spec.go` migration**: The old `familyToBaseImage` map is fully removed. Line 113 uses `containerimages.ImageForFamily(family)` as a drop-in replacement. The `pmToFamily` map (package manager to family mapping) correctly stays in the sandbox package since it's sandbox-specific logic, not image configuration.

- **`requirements.go` migration**: `DefaultSandboxImage` constant is removed and replaced with `containerimages.DefaultImage()` at line 79. `SourceBuildSandboxImage` stays as a local constant, matching the design doc's explicit decision that it's a build variant, not a family-level image.

- **`go:embed` pattern**: Consistent with the existing embed in `internal/recipe/embedded.go` (recipes embed from a subdirectory). The `go:generate cp` approach is the standard workaround for `go:embed`'s parent-directory restriction.

### 4. No parallel pattern introduction

The implementation does not create a second way to look up container images. All consumers within the diff go through `containerimages.ImageForFamily()` or `containerimages.DefaultImage()`. The PPA override (`ubuntu:24.04` for PPA repositories) correctly stays inline in `container_spec.go` since it's conditional logic, not a family-level default -- consistent with the design doc's scope exclusion.

### 5. Test updates are consistent

Tests in `requirements_test.go` and `container_spec_test.go` were updated to use `containerimages.DefaultImage()` and explicit image strings (matching the JSON config values) instead of the old `DefaultSandboxImage` constant. The new `containerimages_test.go` tests exercise all five families, unknown families, the default image, JSON validity, and the `Families()` function.
