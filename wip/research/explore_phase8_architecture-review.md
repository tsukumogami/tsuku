# Architecture Review: Container Image Digest Pinning

**Design document:** `docs/designs/DESIGN-container-image-digest-pinning.md`
**Review date:** 2026-02-22

## 1. Is the architecture clear enough to implement?

**Yes.** The design is implementable as written. The six implementation steps are
concrete and each produces a verifiable artifact. No ambiguity exists about what
the final `container-images.json` looks like, what the Renovate regex should be,
or which tests need updating.

Two minor gaps in implementation clarity:

**Gap 1: Manifest list vs platform-specific digest.** The design says "use `crane
digest`" and explains it returns the manifest list digest. This is correct. But
the design also says "We use `linux/amd64` as the reference platform since that's
what CI and sandbox builds target." These two statements are in tension. If we use
the manifest list digest (architecture-independent), the platform sentence is
misleading. If we use a platform-specific digest, `crane digest` without
`--platform` returns the manifest list digest by default, so the amd64 sentence
would require `crane digest --platform linux/amd64`. The design should clarify:
use the manifest list digest (which is what `crane digest` returns), and drop the
amd64 sentence, since the whole point is that the manifest list digest works
across platforms. **Advisory** -- the implementer can resolve this, and the choice
is unambiguous (manifest list digest is correct for multi-arch support).

**Gap 2: No mention of the `go:generate` step in CI.** Step 3 says "run
`go generate`" but doesn't mention whether CI already runs this or if the
developer must commit the regenerated embedded copy. In practice, the
drift-check CI workflow (`drift-check.yml`) already validates that the embedded
copy matches the root file, so CI will catch any omission. This is not a gap in
the design, just an implementation detail the implementer needs to know. The
existing drift-check workflow handles it.

## 2. Are there missing components or interfaces?

**No missing components.** The design correctly identifies all consumers and
correctly assesses that no Go production code changes are needed. I verified
this against the codebase:

### Claim verification: "Image string is opaque to all consumers"

Verified. Every consumer treats the image string as an opaque value:

- **`containerimages.ImageForFamily()`** (`internal/containerimages/containerimages.go:36-38`):
  Returns `images[family]` directly from the unmarshaled `map[string]string`. No
  parsing, no splitting. An `image:tag@sha256:digest` string flows through
  unchanged.

- **`DeriveContainerSpec()`** (`internal/sandbox/container_spec.go:113`): Calls
  `containerimages.ImageForFamily(family)` and assigns the result to
  `spec.BaseImage`. No transformation.

- **`ContainerImageName()`** (`internal/sandbox/container_spec.go:368-422`):
  Uses `spec.BaseImage` as a hash input via `fmt.Sprintf("base:%s", spec.BaseImage)`.
  The digest suffix becomes part of the hash, making the cache key
  content-aware. This is the exact behavior the design claims.

- **`generateDockerfile()` in `internal/validate/runtime.go:18-23`**: Produces
  `"FROM " + baseImage`. `FROM debian:bookworm-slim@sha256:abc...` is valid
  Docker/Podman syntax.

- **CI `jq` consumers** (7 workflow files, ~12 usage sites): All use
  `jq -r '.family'` or `jq -r --arg f "$f" '.[$f]'` to extract the full string
  value. The string is then used in `docker run "$IMAGE"` commands or passed to
  GitHub Actions `container:` directives. Both `docker run` and GitHub Actions
  `container:` support `image:tag@sha256:digest` natively.

### One exception: `ubuntu:24.04` hardcoded PPA override

At `internal/sandbox/container_spec.go:119-121`:

```go
if family == "debian" && hasPPARepository(reqs.Repositories) {
    baseImage = "ubuntu:24.04"
}
```

This overrides the family image with a hardcoded `ubuntu:24.04` when PPA
repositories are present. This image is NOT in `container-images.json` and
will not get digest pinning. The design's scope statement correctly identifies
only the five `container-images.json` entries as in-scope, but does not mention
this sixth hardcoded image at all. This is not a design flaw -- it's the same
class of out-of-scope hardcoded image as `SourceBuildSandboxImage` and
`DefaultValidationImage`. But it's worth noting for the sandbox image
unification design that three (not two) additional hardcoded images exist:

1. `SourceBuildSandboxImage = "ubuntu:22.04"` (`internal/sandbox/requirements.go:16`)
2. `DefaultValidationImage = "debian:bookworm-slim"` (`internal/validate/executor.go:20`)
3. `SourceBuildValidationImage = "ubuntu:22.04"` (`internal/validate/source_build.go:19`)
4. `"ubuntu:24.04"` PPA override (`internal/sandbox/container_spec.go:120`)

**Advisory** -- this is out of scope for this design but should be tracked.

### Renovate regex correctness

The proposed regex:

```
"(?<depName>[a-z][a-z0-9./-]+):(?<currentValue>[a-z0-9][a-z0-9._-]+)(@(?<currentDigest>sha256:[a-f0-9]+))?"
```

I verified this against the proposed `container-images.json` format:

| Entry | depName match | currentValue match | currentDigest match |
|-------|--------------|-------------------|-------------------|
| `"debian:bookworm-slim@sha256:abc..."` | `debian` | `bookworm-slim` | `sha256:abc...` |
| `"fedora:41@sha256:def..."` | `fedora` | `41` | `sha256:def...` |
| `"archlinux:base@sha256:ghi..."` | `archlinux` | `base` | `sha256:ghi...` |
| `"alpine:3.21@sha256:jkl..."` | `alpine` | `3.21` | `sha256:jkl...` |
| `"opensuse/tumbleweed:latest@sha256:mno..."` | `opensuse/tumbleweed` | `latest` | `sha256:mno...` |

All five entries match correctly. The `[a-z][a-z0-9./-]+` for depName correctly
captures `opensuse/tumbleweed` (includes `/`). The removal of `\s*` after the
colon is correct -- image references have no whitespace.

One subtlety: the current regex in `renovate.json` has `\s*` between the colon
and the tag: `"\\s*(?<currentValue>..."`. The design notes this is removed. This
is correct since the JSON has no whitespace there, but if any other file were
ever added to the `managerFilePatterns`, the `\s*` removal could matter. Since
the scope is only `container-images.json`, this is fine.

### autoReplaceStringTemplate correctness

The template:

```
"{{depName}}:{{newValue}}{{#if newDigest}}@{{newDigest}}{{/if}}"
```

This is correct Handlebars syntax for Renovate. The wrapping double quotes are
included in the template, which matches the JSON value format. When Renovate
replaces, it substitutes the entire matched string (including the outer quotes)
with the template output.

## 3. Are the implementation phases correctly sequenced?

**Yes, with one refinement needed.**

The six steps are:

1. Resolve current digests (using `crane`)
2. Update `container-images.json`
3. Regenerate embedded copy (`go generate`)
4. Update Renovate config
5. Update tests
6. Verify

This sequence is correct. Steps 1-3 could be done in a single commit. Steps
4-5 are independent of each other and could be parallel, but grouping them in
one commit with steps 1-3 is cleaner. This should all be a single PR.

**Refinement:** Step 5 says "update `containerimages_test.go` and
`container_spec_test.go` expected values." The specific test assertions that
need updating:

In `containerimages_test.go`:
- `TestImageForFamily_KnownFamilies` (line 11-17): hardcoded expected map
- `TestDefaultImage` (line 57): hardcoded `"debian:bookworm-slim"` assertion

In `container_spec_test.go`:
- `TestDeriveContainerSpec` (lines 37-62): `wantBaseImage` for each family
  (5 test cases with hardcoded image strings)

The design correctly identifies both files. The test updates are straightforward
string replacements -- no structural test changes needed.

## 4. Are there simpler alternatives we overlooked?

**No.** The design already considered and rejected the right alternatives. The
inline `tag@digest` format is the simplest option that achieves immutability
without any production code changes. The two rejected alternatives (structured
JSON objects, separate digest file) are strictly more complex with no functional
benefit.

One alternative not discussed but worth dismissing: **Renovate's built-in Docker
manager** instead of a custom regex manager. Renovate's Docker manager
automatically detects and pins digests in Dockerfiles and docker-compose files,
but `container-images.json` is not a file format it recognizes natively. The
custom regex manager is the correct approach here. The design implicitly makes
this choice by updating the existing regex config rather than switching manager
types.

### Design fitness assessment

This change fits the existing architecture well:

- **No parallel patterns introduced.** The image config remains a single
  `map[string]string` in a single file. The format changes (appending digests)
  but the structure and access patterns are identical.

- **No action dispatch bypass.** N/A -- this change doesn't touch the action
  or recipe systems.

- **No state contract violation.** N/A -- no state file changes.

- **No dependency direction violation.** N/A -- no new imports.

- **CLI surface unchanged.** No command or flag changes.

The change has zero Go production code modifications. It is purely a data
change (the JSON values) and a configuration change (Renovate regex). The Go
code's opaque treatment of image strings is a deliberate architectural decision
that pays off here -- the `containerimages` package abstraction makes digest
pinning invisible to all consumers.

## Findings Summary

### Blocking

None.

### Advisory

1. **Manifest list vs platform digest ambiguity.** The design says "use
   `linux/amd64` as the reference platform" but recommends `crane digest`
   (which returns the manifest list digest, not a platform-specific one). These
   are different things. Clarify that the manifest list digest is the target,
   since it works across architectures. The `linux/amd64` sentence should be
   removed or reworded to explain that manifest list digests are
   architecture-independent.
   Location: Solution Architecture section, paragraph below the JSON example.

2. **Undocumented hardcoded image: `ubuntu:24.04` PPA override.** At
   `internal/sandbox/container_spec.go:120`, `"ubuntu:24.04"` is hardcoded as
   a PPA override for the debian family. This image won't get digest pinning.
   The design's scope section mentions three out-of-scope hardcoded images but
   misses this fourth one. Not a blocker since it's out of scope, but worth
   adding to the scope boundary statement for completeness.

3. **drift-check self-contradiction.** The design's "drift-check Workflow
   Update" section first proposes updating the `PATTERN` regex, then
   concludes "the existing exception already excludes the source file" and
   the regex doesn't need to match digests. The section should be shortened
   to just the conclusion: no drift-check changes needed. The current text
   walks the reader through a false start that might confuse an implementer.

### Positive observations

- The "zero Go code changes" claim is accurate. Verified against all six
  consumer codepaths.
- The Renovate regex is correct for all five image entries, including the
  `opensuse/tumbleweed` case with a slash in the name.
- The `autoReplaceStringTemplate` correctly handles the conditional digest
  suffix using Handlebars `{{#if}}`.
- Leveraging the existing `ContainerImageName()` hash function for automatic
  cache invalidation is clean. The digest becomes part of the hash input with
  no code change, which is exactly how the abstraction was designed to work.
- The decision to add `:latest` to tumbleweed is the right call. One regex
  pattern is better than two.
