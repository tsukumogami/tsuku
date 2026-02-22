# Review: pragmatic focus -- Issue #1901

## Findings

### 1. Speculative export: `Families()` has zero callers (Advisory)

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/internal/containerimages/containerimages.go:52-58`

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

`Families()` is exported but has zero callers outside its own test. The key decisions note says "A Families() export was added for downstream issues" (presumably #1902 or #1903). This is speculative generality -- add it when #1902 actually needs it. The function is small and inert, so it won't cause problems if left, but it's dead weight today.

Additionally, the godoc says "sorted list" but the function doesn't sort. It iterates a map, producing non-deterministic order. If kept, either add `sort.Strings(fams)` or fix the comment.

**Severity:** Advisory. Small, inert, but the doc/implementation mismatch is a latent bug if a future caller depends on sorting.

**Suggestion:** Delete `Families()` and its test. Add it in the PR that needs it. If kept, add `sort.Strings(fams)` before the return.

---

### 2. Impossible-case panic in `DefaultImage()` (Advisory)

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/internal/containerimages/containerimages.go:44-50`

```go
func DefaultImage() string {
	img, ok := images["debian"]
	if !ok {
		panic("containerimages: embedded container-images.json missing required \"debian\" entry")
	}
	return img
}
```

`init()` on line 28 already panics if `"debian"` is missing. By the time any caller reaches `DefaultImage()`, the entry is guaranteed to exist. The check is impossible-case handling.

**Severity:** Advisory. Defensive code in a small function. Not harmful, just redundant. The comment on line 41-43 even acknowledges this ("the init function validates this so a panic here means the binary was built with a corrupt embed").

**Suggestion:** Could simplify to `return images["debian"]` since init guarantees the key exists. Low priority.

---

### 3. `build-test` Makefile target doesn't run `go generate` (Advisory)

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/Makefile:18-19`

```makefile
build-test:
	CGO_ENABLED=0 go build -ldflags "-X main.defaultHomeOverride=.tsuku-test" -o tsuku-test ./cmd/tsuku
```

The `build` target runs `go generate ./internal/containerimages/...` before building, but `build-test` does not. This is safe because the embedded copy is committed to the repo, so `build-test` will use whatever's in git. But the inconsistency means someone who edits `container-images.json` and runs `make build-test` (skipping `make build`) gets a stale embedded copy.

**Severity:** Advisory. The committed-copy strategy makes this safe in practice. CI and goreleaser don't run `go generate` either, relying on the committed copy.

**Suggestion:** Either add `go generate` to `build-test` for consistency, or add a comment explaining why only `build` needs it.

---

## Summary Assessment

The implementation correctly matches the design doc's intent for issue #1901. The `containerimages` package is minimal and well-structured: `go:embed` with `go:generate` copy, `init()` validation, two clean exported functions. The sandbox code changes (`container_spec.go` and `requirements.go`) are surgical replacements of hardcoded values with `containerimages.ImageForFamily()` and `containerimages.DefaultImage()` calls. Tests are thorough and cover all families, unknown families, and the integration between sandbox and containerimages.

No blocking findings. The `Families()` doc/implementation mismatch (claims sorted, isn't sorted) is the most actionable advisory item.
