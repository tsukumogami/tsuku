# Maintainer Review: Issue #1901

**Review focus**: maintainability (clarity, readability, duplication)
**Reviewer perspective**: can someone who didn't write this understand it and change it with confidence?

## Files Reviewed

- `container-images.json` (repo root)
- `internal/containerimages/container-images.json` (embedded copy)
- `internal/containerimages/containerimages.go`
- `internal/containerimages/containerimages_test.go`
- `internal/sandbox/container_spec.go`
- `internal/sandbox/requirements.go`
- `internal/sandbox/requirements_test.go`
- `internal/sandbox/container_spec_test.go`
- `internal/sandbox/executor_test.go`
- `internal/sandbox/sandbox_integration_test.go`
- `Makefile`

## Findings

### 1. Stale comment on SandboxRequirements.Image -- Advisory

**File**: `internal/sandbox/requirements.go:60-62`

```go
// Image is the recommended container image based on requirements.
// Uses debian:bookworm-slim for binary-only, ubuntu:22.04 for source builds.
Image string
```

The comment hardcodes `debian:bookworm-slim` as if it's a fixed value, but the actual image now comes from `containerimages.DefaultImage()` and can change when someone edits `container-images.json`. A next developer reading this comment would think the value is always `debian:bookworm-slim`, but it's whatever the `debian` entry in the JSON file says.

**Suggestion**: Update to reference the source:
```go
// Image is the recommended container image based on requirements.
// Defaults to the debian image from container-images.json for binary-only,
// ubuntu:22.04 for source builds.
```

### 2. Families() docstring says "sorted" but implementation doesn't sort -- Advisory

**File**: `internal/containerimages/containerimages.go:52-59`

```go
// Families returns a sorted list of all known Linux family names.
func Families() []string {
	fams := make([]string, 0, len(images))
	for f := range images {
		fams = append(fams, f)
	}
	return fams
}
```

The doc comment says "sorted" but the function iterates over a map and returns keys in non-deterministic order. No `sort.Strings(fams)` call. The test at `containerimages_test.go:93-112` (`TestFamilies`) doesn't check sort order -- it only checks that all 5 families are present and the count is 5. This means a caller relying on sorted output will get a subtle bug that only manifests intermittently.

If no caller currently depends on sort order, the fix is to update the comment to remove "sorted". If callers should be able to depend on sort order, add `sort.Strings(fams)` before returning.

### 3. Makefile build-test doesn't run go generate -- Advisory

**File**: `Makefile:18-19`

```makefile
build-test:
	CGO_ENABLED=0 go build -ldflags "-X main.defaultHomeOverride=.tsuku-test" -o tsuku-test ./cmd/tsuku
```

The `build` target runs `go generate ./internal/containerimages/...` before compiling, but `build-test` does not. If someone edits `container-images.json` and runs `make build-test` for QA, the test binary will have the old embedded images. This is an easy trap because `build` does the right thing and `build-test` looks like it should too.

The embedded copy is committed to the repo, so this only matters when someone has edited the root JSON without running `go generate` first. But since `build` sets the precedent, `build-test` should follow it.

**Suggestion**: Add `go generate ./internal/containerimages/...` to `build-test`.

### 4. DefaultImage() double-validates what init() already guarantees -- Advisory

**File**: `internal/containerimages/containerimages.go:44-50`

```go
func DefaultImage() string {
	img, ok := images["debian"]
	if !ok {
		panic("containerimages: embedded container-images.json missing required \"debian\" entry")
	}
	return img
}
```

The `init()` function on line 28-30 already panics if `"debian"` is missing. So the `!ok` branch in `DefaultImage()` is unreachable unless someone modifies `images` after init (which isn't exported, so they can't from outside the package). The defensive check is fine -- the comment on line 42-43 even explains this -- but it's worth noting this is belt-and-suspenders, not a bug.

No action needed. The comment adequately explains the rationale.

### 5. Test values hardcode image strings that could drift from JSON -- Advisory

**File**: `internal/containerimages/containerimages_test.go:11-17`

```go
expected := map[string]string{
    "debian": "debian:bookworm-slim",
    "rhel":   "fedora:41",
    "arch":   "archlinux:base",
    "alpine": "alpine:3.21",
    "suse":   "opensuse/tumbleweed",
}
```

Also in `container_spec_test.go:38-67` (`TestDeriveContainerSpec`) and `requirements_test.go:326-331` (`TestComputeSandboxRequirements_TargetFamily`).

These test assertions hardcode the exact image strings. When someone updates `container-images.json` (the whole point of this centralization), they'll also need to update these test files. The `containerimages_test.go` case is intentional -- it validates the embedded JSON matches expectations. But `container_spec_test.go` and `requirements_test.go` could use `containerimages.ImageForFamily()` to derive expected values, similar to how some tests already use `containerimages.DefaultImage()`.

This is a judgment call. Hardcoded values in tests serve as regression detectors -- if the JSON changes and someone forgets to update the sandbox behavior, the tests catch it. But the design's intent is that changing the JSON should be a single-file edit. Having to update 3 test files on every image bump adds friction that partially undermines the centralization goal.

**Suggestion**: In `container_spec_test.go` and `requirements_test.go`, consider using `containerimages.ImageForFamily("alpine")` instead of `"alpine:3.21"` for the expected values, at least for tests that aren't specifically validating the image selection logic. The `containerimages_test.go` tests should keep their hardcoded values since they validate the JSON content itself.

## Overall Assessment

The implementation is clean and well-aligned with the design doc. The new `containerimages` package is small, focused, and easy to understand. The `go:generate` + `go:embed` pattern is documented in the package comment and the `Makefile`. The sandbox code changes are minimal -- swapping constant/map lookups for function calls -- with no API changes for callers.

The package structure is sound: clear exported API (`ImageForFamily`, `DefaultImage`, `Families`), good error messages in `init()`, and the JSON file is trivially parseable. Tests cover the key cases including unknown families and the embedded JSON validation.

The two things the next developer is most likely to stumble on are the stale comment about `debian:bookworm-slim` on the `Image` field (finding 1) and the `Families()` function claiming to return sorted output when it doesn't (finding 2). Neither will cause a bug today, but both will mislead someone reading the code for the first time.
