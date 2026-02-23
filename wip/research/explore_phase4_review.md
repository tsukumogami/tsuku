# Architect Review: Container Image Digest Pinning Design

**Reviewer:** Architect
**Document:** `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/docs/designs/DESIGN-container-image-digest-pinning.md`
**Branch:** `docs/sandbox-image-unification`

---

## 1. Problem Statement Assessment

The problem statement is specific and well-grounded. It identifies two concrete symptoms (non-reproducible builds and stale cache hits), traces them to the root cause (mutable tags in `container-images.json`), and points to the existing code that acknowledges the gap (`container_spec.go:371-373` comment referencing issue #799).

One thing the problem statement doesn't address: the scope boundary. The design covers the five images in `container-images.json`, but there are two other hardcoded image references in production code that have the same mutability problem:

- `internal/sandbox/requirements.go:16` -- `SourceBuildSandboxImage = "ubuntu:22.04"`
- `internal/validate/executor.go:20` -- `DefaultValidationImage = "debian:bookworm-slim"`

These constants are NOT read from `container-images.json`. They're hardcoded strings that suffer from the same tag-mutability problem the design describes. The design should either explicitly scope them out with rationale (e.g., "these are covered by DESIGN-sandbox-image-unification") or include them.

**Verdict:** Problem statement is clear and evaluable. Missing explicit scope boundary for out-of-config hardcoded images.

---

## 2. Alternatives Analysis

### 2a. Missing Alternatives

No significant missing alternatives. The three considered approaches (inline `tag@digest`, structured JSON objects, separate digest file) cover the realistic design space. A fourth option -- using a lockfile pattern (like `container-images.lock.json` generated from `container-images.json`) -- is sometimes seen in package managers, but it's effectively a variant of the "separate digest file" option and would have the same sync/merge problems.

### 2b. Rejection Rationale

**Structured JSON objects** -- rejection rationale is fair and specific. It correctly identifies that every consumer (`map[string]string` Go type, `jq -r` in CI, test assertions) would break. The reconstruction logic point is strong: splitting a string that Docker already parses from a single reference creates work with no functional benefit.

**Separate digest file** -- rejection rationale is fair. The sync/drift problem is real. The "doubles the Renovate configuration" claim is accurate since you'd need one manager for tags and one for digests.

**Digest-only without tag for tumbleweed** -- rejection rationale is fair. Maintaining two regex patterns for a single file is unnecessary complexity.

**Keep tumbleweed untagged** -- rejection rationale is strong. Tumbleweed is the most volatile image, making it the worst candidate for an exception.

No strawmen detected. Each rejected alternative has genuine trade-offs; they aren't set up to fail.

---

## 3. Renovate Regex Analysis

### Current regex (from `renovate.json` line 8):
```
"(?<depName>[a-z][a-z0-9./-]+):\s*(?<currentValue>[a-z0-9][a-z0-9._-]+)"
```

### Proposed regex:
```
"(?<depName>[a-z][a-z0-9./-]+):(?<currentValue>[a-z0-9][a-z0-9._-]+)(@(?<currentDigest>sha256:[a-f0-9]+))?"
```

### Issues Found

**3a. Outer quotes in the regex.** The existing regex has `\"` at both ends to match the JSON double-quote wrapping each value. The proposed regex in the design doc (line 269) also has `\"` at both ends. Good -- this is correct. Renovate needs to match the full `"debian:bookworm-slim@sha256:..."` string including quotes so the `autoReplaceStringTemplate` can reconstruct it with quotes.

**3b. The `\s*` removal.** The design notes this change at line 277: "Removed `\s*` after the first colon (no whitespace in image refs)." This is correct. The original `\s*` after `:` would never match in a Docker image reference. Removing it is a cleanup.

**3c. Digest capture group.** `(@(?<currentDigest>sha256:[a-f0-9]+))?` is correct syntax for an optional digest capture. The `[a-f0-9]+` pattern matches hex characters. A SHA256 digest is exactly 64 hex characters; using `+` instead of `{64}` is more permissive but acceptable -- Renovate will get the actual digest from the registry, so the regex just needs to match the existing string.

**3d. autoReplaceStringTemplate.** The proposed template:
```
"{{depName}}:{{newValue}}{{#if newDigest}}@{{newDigest}}{{/if}}"
```

This handles three cases:
- **Tag + digest update:** `"{{depName}}:{{newValue}}@{{newDigest}}"` -- correct
- **Digest-only update (same tag):** `{{newValue}}` equals `{{currentValue}}`, digest changes -- correct
- **Tag-only update (no digest):** If `newDigest` is empty/undefined, the `{{#if}}` block is skipped, producing `"{{depName}}:{{newValue}}"` -- this would strip the digest from a previously-pinned reference

This third case is a concern **in theory**. However, in practice Renovate's docker datasource always resolves a digest when it resolves a new tag. So `newDigest` should never be empty for a docker datasource update. The conditional is a safety net, not a normal code path. Acceptable as-is.

**3e. Potential regex matching issue with JSON structure.** The regex matches across the full file content. In `container-images.json`, each entry looks like:
```json
  "debian": "debian:bookworm-slim@sha256:abc..."
```

The regex `"(?<depName>...)..."` starts matching at the opening `"` of the value string. But it could also accidentally match the *key* string `"debian"` since `debian` matches `[a-z][a-z0-9./-]+`. However, the key string doesn't contain `:`, so the regex requires a `:` after `depName`, which prevents matching keys. This is fine.

**3f. The `opensuse/tumbleweed:latest` case.** With `latest` as `currentValue`, Renovate's docker datasource will track digest changes for the `latest` tag. For a rolling release like tumbleweed, this is the correct behavior -- new digests signal new content. The `latest` tag itself won't "bump" to a new version, so Renovate will only open digest-update PRs. This is exactly what the design intends.

One subtle point: Renovate treats `latest` specially. By default, Renovate may skip `latest` tags because they're not considered "versioned." However, when a `currentDigest` is captured, Renovate should still track digest changes. The `autoReplaceStringTemplate` with `{{#if newDigest}}` handles this correctly. Worth verifying in a Renovate dry-run (which the design lists as step 6), but the configuration looks correct.

---

## 4. "No Go Production Code Changes" Claim

### Claim verification

The design claims at lines 109-114 and 203-206 that no Go production code changes are needed because the image string is opaque to all consumers.

**Verified code paths:**

1. **`containerimages.ImageForFamily()`** (`/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/internal/containerimages/containerimages.go:36-38`): Returns `images[family]` directly from the JSON map. The string is passed through unchanged. **No change needed.** Confirmed.

2. **`containerimages.DefaultImage()`** (`containerimages.go:45-51`): Same -- returns the raw string. **No change needed.** Confirmed.

3. **`generateDockerfile()` in `runtime.go:18-23`**: Produces `"FROM " + baseImage`. Docker/Podman accept `FROM image:tag@sha256:digest`. **No change needed.** Confirmed.

4. **`podmanRuntime.Build()` and `dockerRuntime.Build()`** (`runtime.go:345-361`, `485-501`): Both call `generateDockerfile(baseImage, buildCommands)` and pipe it to `build -f -`. The `FROM` line with digest is valid. **No change needed.** Confirmed.

5. **`ContainerImageName()` in `container_spec.go:368-422`**: Includes `spec.BaseImage` in the hash input as `"base:debian:bookworm-slim@sha256:..."`. This is a change in hash value (which is the point), but no code change is needed. The digest becomes part of the hash input automatically. **No change needed.** Confirmed.

6. **`ComputeSandboxRequirements()` in `requirements.go:79-86`**: Uses `containerimages.DefaultImage()` and `containerimages.ImageForFamily()`, which return the full string. This string is assigned to `reqs.Image` and eventually used in `Executor.Sandbox()`. **No change needed.** Confirmed.

7. **`Executor.Sandbox()` in `executor.go:168`**: Uses `reqs.Image` as the container image for `runtime.Build()` or `runtime.Run()`. The image string with digest is valid for both operations. **No change needed.** Confirmed.

8. **CI workflows** (`recipe-validation-core.yml:143`, `test-recipe.yml:136`, `platform-integration.yml:96-97`, `validate-golden-execution.yml:639`): All use `jq -r '.family' container-images.json` which returns the raw string. When passed to `docker run --rm ... "$IMAGE" ...`, the `image:tag@sha256:digest` format is valid. **No change needed.** Confirmed.

**The claim is correct for the stated scope.** All consumers of `container-images.json` treat the image string as opaque and pass it to Docker/Podman, which natively support the `tag@digest` format.

### Out-of-scope hardcoded images (not covered by the claim)

These are production code constants that remain mutable and unpinned:
- `SourceBuildSandboxImage = "ubuntu:22.04"` in `requirements.go:16`
- `DefaultValidationImage = "debian:bookworm-slim"` in `validate/executor.go:20`
- `SourceBuildValidationImage = "ubuntu:22.04"` in `validate/source_build.go:19`

The design doesn't claim to address these. They're separate from `container-images.json`. But their existence means the "zero production code changes" framing is slightly misleading -- it's zero changes for the `container-images.json` consumer path, but the broader reproducibility goal remains incomplete for these constants. This is not a blocking issue for this design, but should be called out.

---

## 5. opensuse/tumbleweed:latest Handling

### Is adding `:latest` safe?

Yes. Docker treats `opensuse/tumbleweed` and `opensuse/tumbleweed:latest` identically. The OCI spec defines that omitting a tag implies `latest`. Adding it explicitly changes nothing at the runtime level.

### Will Renovate handle it correctly?

Renovate's docker datasource resolves `latest` as a valid tag and tracks its digest. When the upstream manifest changes, Renovate detects the new digest and opens a PR. For a rolling release, this is the desired behavior.

One consideration: Renovate's default `extends: ["config:recommended"]` configuration includes a rule that ignores `latest` tags for version updates. However, since we're using a custom regex manager (not the built-in Docker manager), this default doesn't apply. The regex manager matches whatever the regex captures. Since `currentValue` would be `latest`, Renovate won't try to "bump" it to a newer version tag (there is no newer version), but it will detect digest changes because `currentDigest` is captured.

### Test impact

Tests at `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/internal/containerimages/containerimages_test.go:16` assert `"suse": "opensuse/tumbleweed"`. This must change to `"opensuse/tumbleweed:latest@sha256:..."`. Tests at `/home/dangazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/internal/sandbox/container_spec_test.go:63-68` assert `wantBaseImage: "opensuse/tumbleweed"`. Same update needed.

The design correctly identifies this at line 163-165 and in the consumer impact table at line 298.

---

## 6. Multi-arch Manifest Edge Cases

The design mentions at lines 248-249: "The digest is the SHA256 hash of the multi-arch manifest (or the platform-specific manifest if multi-arch isn't available). We use `linux/amd64` as the reference platform since that's what CI and sandbox builds target."

### Potential issue

There are two types of manifests:
1. **Manifest list (multi-arch):** Points to platform-specific manifests. `docker manifest inspect` returns the list manifest.
2. **Platform-specific manifest:** The actual image for one platform.

When you pin to the manifest list digest, `docker pull image:tag@sha256:list-digest` works on any platform -- Docker resolves the correct platform-specific image from the list. When you pin to a platform-specific digest, it only works on that platform.

The design's implementation step (lines 328-337) uses `docker manifest inspect` which returns the manifest list if one exists. The `jq` command extracts `.[0].Descriptor.digest` from the verbose output. For multi-arch images, `.[0]` would be the *first platform entry*, not the manifest list itself. This is a platform-specific digest, which would break on non-amd64 platforms.

**Corrected approach for multi-arch images:** To get the manifest list digest:
```bash
docker manifest inspect --verbose "$img" | jq -r '
  if type == "array" then
    # Multi-arch: need the list digest, not individual platform digests
    input_filename  # not available in jq this way
  else
    .Descriptor.digest
  end'
```

Actually, `crane digest` is the cleaner approach and is already mentioned as an alternative. `crane digest debian:bookworm-slim` returns the manifest list digest directly when a manifest list exists. This is the correct digest to pin because:
- It works on all platforms (Docker resolves the right sub-manifest)
- Renovate's docker datasource compares manifest list digests

The `docker manifest inspect -v` approach in the implementation section (lines 331-337) would give the wrong digest for multi-arch images. The `crane digest` approach (lines 342-344) gives the correct digest.

**Recommendation:** Use `crane digest` as the primary method in the implementation section. The `docker manifest inspect -v` fallback with the current `jq` expression needs correction if kept.

### CI/build platform

The design says CI and sandbox builds target `linux/amd64` (line 249). Looking at CI workflows, this is mostly true, but `platform-integration.yml` also uses `arm64` runners (line 88-89: `runner: ubuntu-24.04-arm`). The arm64 runner uses `docker run` with the Alpine image (line 97). If the digest is a manifest list digest, this works fine on arm64 because Docker resolves the correct platform manifest from the list. If it were a platform-specific amd64 digest, it would fail on arm64.

This reinforces the need to use manifest list digests, not platform-specific ones.

---

## 7. Unstated Assumptions

1. **Renovate has access to all five registries.** The images come from Docker Hub (debian, fedora, archlinux, alpine) and Docker Hub (opensuse/tumbleweed). This is a single registry, so this assumption holds.

2. **All images have stable manifest list digests.** Some registries re-sign manifests, which changes the digest without changing content. This is rare for Docker Hub official images but worth noting. Renovate would detect it as a digest change and open a PR, which is the correct behavior (fail open).

3. **The `ubuntu:24.04` PPA override in `container_spec.go:121` still works.** When the debian family is selected but the plan has PPA repositories, `DeriveContainerSpec` overrides the base image to `"ubuntu:24.04"`. This string is hardcoded, not read from `container-images.json`. It won't get a digest suffix. This is an existing issue outside the scope of this design, but the design should note it as a known gap if completeness is a goal.

4. **The `go generate` step produces an identical copy.** The design correctly lists `go generate ./internal/containerimages/...` as a required step. The embedded copy at `internal/containerimages/container-images.json` must match the root `container-images.json` exactly, including digest suffixes. The existing drift-check CI (lines 26-41 of `drift-check.yml`) validates this. No gap here.

---

## 8. Drift-Check Workflow Impact

The design discusses the hardcoded-references job at lines 301-323 and concludes that no pattern update is needed because the `container-images.json:` exception already excludes the source file and its embedded copy.

Looking at the actual `drift-check.yml` (line 55):
```
PATTERN='(debian:(bookworm|bullseye|buster|sid|stretch)|fedora:[0-9]|archlinux:(base|latest)|alpine:[0-9]|opensuse/(leap|tumbleweed))'
```

This pattern matches the image name and tag prefix. After digest pinning, the image strings in code would be `"debian:bookworm-slim@sha256:..."`. The pattern `debian:(bookworm|...)` matches `bookworm` in `bookworm-slim`, so it would still match even with the digest suffix. The exceptions (`container-images.json:`, `_test.go:`, etc.) filter out legitimate uses.

The design's conclusion is correct: no drift-check changes needed. The existing exceptions already cover the source files, and the pattern doesn't need to explicitly account for digests.

---

## 9. Summary of Findings

### Blocking Issues

None. The design is architecturally sound and fits the existing codebase structure.

### Advisory Issues

1. **Multi-arch digest resolution (lines 328-337).** The `docker manifest inspect -v` + `jq` command in the implementation section extracts a platform-specific digest, not the manifest list digest. Use `crane digest` instead, or fix the `jq` expression. This matters because `platform-integration.yml` runs on arm64 hosts. The design already mentions `crane` as an alternative -- just make it the primary recommendation.

2. **Scope boundary for hardcoded constants.** Three production constants (`SourceBuildSandboxImage`, `DefaultValidationImage`, `SourceBuildValidationImage`) have the same tag-mutability problem but aren't covered by this design. Adding a one-line note acknowledging these are out-of-scope (or covered by another design) would prevent confusion during implementation.

3. **Renovate `latest` tag handling.** The design should note that Renovate's default configuration may deprioritize `latest` tags. Since this uses a custom regex manager (not the built-in Docker manager), the default filtering rules don't apply, but a brief note would help future maintainers understand why this works.

### Confirmed Correct

- The "no Go production code changes" claim is verified across all eight consumer paths.
- The Renovate regex update is syntactically correct and handles all three update cases (tag+digest, digest-only, tag-only).
- Adding `:latest` to opensuse/tumbleweed is safe and enables uniform Renovate tracking.
- The `autoReplaceStringTemplate` with `{{#if newDigest}}` handles the edge case of digest absence gracefully.
- Cache key invalidation in `ContainerImageName()` works automatically because the digest becomes part of the hash input string.
- All CI workflows pass the image string opaquely to `docker run` or use it in variable assignments; the `tag@digest` format is valid in all these contexts.

---

## Recommendation

Accept the design. The inline `tag@digest` approach is the right call -- it's the OCI standard format, every consumer already treats the string as opaque, and the Renovate regex update is minimal. Address the advisory issues (multi-arch digest resolution command, scope boundary note) before implementation.
