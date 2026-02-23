---
status: Proposed
problem: |
  Container images in container-images.json use mutable tags like alpine:3.21
  that can resolve to different content over time. The sandbox cache key
  includes the tag string but not a content identifier, so the cache won't
  invalidate when an upstream tag gets a new push. This causes silent staleness
  and makes builds non-reproducible across machines and time.
decision: |
  Add SHA256 digest suffixes to every image reference in container-images.json
  using Docker's native tag@digest format. Update the Renovate regex to capture
  and maintain digests automatically. Add an explicit :latest tag to
  opensuse/tumbleweed so it matches the same regex pattern as all other entries.
  No Go code changes are needed since the image string is opaque to all consumers.
rationale: |
  The inline tag@digest format is the standard Docker reference syntax, supported
  natively by FROM directives, docker pull, and podman. Keeping the tag alongside
  the digest preserves human readability and lets Renovate track both version
  bumps and digest rotations through a single regex pattern. The alternative of
  splitting into structured JSON would break every consumer and add complexity
  for no functional gain.
---

# Container Image Digest Pinning

## Status

**Status:** Proposed

## Context and Problem Statement

Container images in `container-images.json` use mutable tags:

```json
{
  "debian": "debian:bookworm-slim",
  "alpine": "alpine:3.21",
  "suse": "opensuse/tumbleweed"
}
```

Tags are pointers, not identifiers. `alpine:3.21` pulled today might contain
different packages than `alpine:3.21` pulled next week after a security patch
lands. This creates two problems:

1. **Non-reproducible builds.** Two developers running `tsuku install` on the
   same recipe at the same time can get different sandbox environments if a
   registry push happened between their pulls. Sandbox test results that pass
   on one machine might fail on another.

2. **Stale cache hits.** `ContainerImageName()` hashes the tag string
   (`base:alpine:3.21`) into the cache key. When the upstream tag gets a new
   push, the hash doesn't change, so tsuku keeps using the cached image built
   from the old content. The comment at `container_spec.go:371-373` already
   flags this gap and references issue #799.

Digest pinning (`alpine:3.21@sha256:...`) solves both problems. The digest is
a SHA256 hash of the image manifest, making the reference immutable. When
Renovate detects a new digest for the same tag, it opens a PR to update
`container-images.json`, which automatically changes the `ContainerImageName()`
cache key and triggers a rebuild.

**Scope**: This design covers only the five images managed by
`container-images.json`. Four other hardcoded image constants exist in the
codebase (`SourceBuildSandboxImage` in `requirements.go`,
`DefaultValidationImage` in `validate/executor.go`,
`SourceBuildValidationImage` in `source_build.go`, and a PPA override
`ubuntu:24.04` in `container_spec.go`). Those are out of scope here;
migrating them to `container-images.json` is tracked separately by the
sandbox image unification design. Until that migration lands, these
constants have the same tag-mutability risk but no Renovate tracking or
CODEOWNERS protection.

## Decision Drivers

- **Immutability**: the same image reference must always resolve to the same
  image content, across machines and over time.
- **Automated maintenance**: Renovate must be able to update both tags and
  digests without manual intervention.
- **Transparent to consumers**: the Go code, CI workflows, and test fixtures
  that read `container-images.json` should need zero code changes. The image
  string is opaque to all consumers.
- **Rolling releases**: `opensuse/tumbleweed` has no version tag. Whatever
  format we pick must handle images that only have a digest, not a versioned
  tag.
- **Cache correctness**: `ContainerImageName()` should produce a different
  hash when image content changes, even if the tag string stays the same.

## Considered Options

### Decision 1: Image Reference Format

The central question is how to represent digest-pinned images in
`container-images.json`. The format must be parseable by Renovate's regex
custom manager, passable to `FROM` directives without transformation, and
compatible with the existing `map[string]string` Go type that all consumers
expect.

#### Chosen: Inline tag@digest strings

Keep `container-images.json` as a flat `map[string]string` and append the
digest to each value using Docker's native reference format:

```json
{
  "debian": "debian:bookworm-slim@sha256:ab12cd34...",
  "rhel": "fedora:41@sha256:ef56gh78...",
  "arch": "archlinux:base@sha256:ij90kl12...",
  "alpine": "alpine:3.21@sha256:mn34op56...",
  "suse": "opensuse/tumbleweed:latest@sha256:qr78st90..."
}
```

This format is the OCI standard for immutable image references. Docker and
Podman both parse `image:tag@sha256:digest` natively in `FROM` directives,
`docker pull`, and `podman pull`. When both a tag and digest are present,
the container runtime pins to the digest and ignores the tag for resolution
purposes. The tag is kept for human readability only.

Because the Go code treats image strings as opaque (passed straight through
to `FROM` in `generateDockerfile` and into the hash in
`ContainerImageName()`), no production code changes are needed. The digest
suffix automatically makes the cache key content-aware: when Renovate updates
the digest, the `base:debian:bookworm-slim@sha256:newdigest` hash input changes
and `ContainerImageName()` produces a new tag, forcing a cache rebuild.

The Renovate regex needs one update: an optional capture group for the digest
at the end of the pattern. The `autoReplaceStringTemplate` field tells Renovate
how to reconstruct the replacement string when both tag and digest change.

#### Alternatives Considered

**Structured JSON objects**: Replace strings with objects like
`{"image": "debian", "tag": "bookworm-slim", "digest": "sha256:ab12..."}`.
Rejected because it breaks every consumer. The Go embed unmarshals into
`map[string]string`, CI workflows use `jq -r .<family>` to get a single
string, and test fixtures assert exact string values. Every consumer would
need code changes to reconstruct the Docker reference from parts, and the
reconstruction logic (`image:tag@digest`) is exactly what Docker already
parses from a single string.

**Separate digest file**: Keep tags in `container-images.json` and digests
in `container-image-digests.json`, merged at build time. Rejected because
it creates a second file that must stay in sync, doubles the Renovate
configuration, and requires a merge step that doesn't exist today. The
drift-check CI job would need to validate cross-file consistency. All
this complexity exists to solve a formatting preference, not a functional
requirement.

### Decision 2: Handling opensuse/tumbleweed

`opensuse/tumbleweed` is a rolling release distribution with no version
tags. The current config uses the bare image name without any tag, which
Docker interprets as `:latest`. This creates a problem for both the
Renovate regex (which expects a `:tag` component to capture as
`currentValue`) and for digest pinning (which needs a consistent format
across all entries).

#### Chosen: Add explicit :latest tag

Change the entry from `"opensuse/tumbleweed"` to
`"opensuse/tumbleweed:latest@sha256:..."`. Docker treats `image` and
`image:latest` identically, so this is a no-op from the container runtime's
perspective.

The explicit tag has two benefits. First, it makes the Renovate regex
uniform: every entry matches `depName:currentValue@currentDigest`, with
no special case for tagless images. Second, Renovate's docker datasource
watches for new digests and opens update PRs when the manifest changes.
For tumbleweed, this is exactly what we want, since the only meaningful
update signal is a new digest.

Note: Renovate's built-in Docker manager deprioritizes `latest` tags by
default, but that doesn't apply here. We use a custom regex manager, not
the built-in Docker manager, so those defaults don't take effect. The
custom regex manager treats all matched entries equally regardless of tag
value.

The change does affect test assertions that currently expect
`"opensuse/tumbleweed"` without a tag. These tests need to be updated
to expect the new string, which is a one-time cost already covered by
updating the JSON values.

#### Alternatives Considered

**Digest-only without tag**: Use `opensuse/tumbleweed@sha256:...` with no
tag at all. Docker supports this format, but it requires a separate Renovate
regex pattern because the `:currentValue` capture group would be absent.
Rejected because maintaining two regex patterns for a single file adds
unnecessary configuration and makes the Renovate config harder to reason
about. The `latest` tag conveys the same semantic and keeps one regex.

**Keep untagged, skip digest pinning for tumbleweed**: Leave it as
`"opensuse/tumbleweed"` and accept that this one image isn't pinned.
Rejected because it undermines the goal. If one image can drift
silently, the reproducibility guarantee is incomplete. Tumbleweed is
the *most* volatile image in the list (rolling release), making it the
one that benefits most from pinning.

## Decision Outcome

**Chosen: 1A + 2A** (inline tag@digest, explicit :latest for tumbleweed)

### Summary

Every image reference in `container-images.json` gets a `@sha256:...`
digest suffix appended to its existing tag. For `opensuse/tumbleweed`,
an explicit `:latest` tag is added before the digest. The resulting file
looks like standard Docker image references that happen to include content
hashes.

The Renovate regex custom manager in `renovate.json` gets two additions:
an optional `currentDigest` capture group at the end of the match pattern,
and an `autoReplaceStringTemplate` that reconstructs the full
`depName:currentValue@currentDigest` string on updates. Renovate's docker
datasource already knows how to look up digests and detect when they change,
so these regex updates are all that's needed.

No Go production code changes. `ImageForFamily()` returns the full
`image:tag@sha256:...` string, which flows unchanged into
`generateDockerfile()` as a `FROM` argument and into
`ContainerImageName()` as a hash input. The cache key becomes
content-aware automatically because the digest is part of the hashed string.

Tests that assert exact image values need updates to include the digest suffix.
The drift-check CI workflow needs no changes since its existing exceptions
already cover the source files.

To get the initial digests, we query the registries using `crane digest`
and update `container-images.json`. After that, Renovate handles all
future updates.

### Rationale

The inline format works because Docker already defines a standard way to
combine tags and digests in a single string. Fighting that standard by
splitting into structured JSON would create work for every consumer
without adding any capability. The tag stays for readability and Renovate
version tracking. The digest stays for immutability and cache correctness.

Adding `:latest` to tumbleweed is the minimal change that makes all entries
follow the same pattern. One regex, one format, no special cases. The
alternative of maintaining two regex patterns saves zero characters in the
JSON file while doubling the Renovate configuration.

## Solution Architecture

### container-images.json

After pinning, the file contains:

```json
{
  "debian": "debian:bookworm-slim@sha256:<64-char-hex>",
  "rhel": "fedora:41@sha256:<64-char-hex>",
  "arch": "archlinux:base@sha256:<64-char-hex>",
  "alpine": "alpine:3.21@sha256:<64-char-hex>",
  "suse": "opensuse/tumbleweed:latest@sha256:<64-char-hex>"
}
```

Each value is a valid Docker image reference. The digest is the SHA256 hash
of the manifest list (multi-arch index), which is architecture-independent.
This means the same pinned reference works on both amd64 CI runners and
arm64 hosts used by `platform-integration.yml`. For images that don't
publish a multi-arch manifest, `crane digest` returns the single-platform
manifest digest instead.

### Renovate Configuration

The `renovate.json` regex updates from:

```json
{
  "matchStrings": [
    "\"(?<depName>[a-z][a-z0-9./-]+):\\s*(?<currentValue>[a-z0-9][a-z0-9._-]+)\""
  ],
  "datasourceTemplate": "docker"
}
```

To:

```json
{
  "matchStrings": [
    "\"(?<depName>[a-z][a-z0-9./-]+):(?<currentValue>[a-z0-9][a-z0-9._-]+)(@(?<currentDigest>sha256:[a-f0-9]+))?\""
  ],
  "datasourceTemplate": "docker",
  "autoReplaceStringTemplate": "\"{{depName}}:{{newValue}}{{#if newDigest}}@{{newDigest}}{{/if}}\""
}
```

Key changes:
- Removed `\s*` after the first colon (no whitespace in image refs)
- Added `(@(?<currentDigest>sha256:[a-f0-9]+))?` to optionally capture the
  digest after the tag
- Added `autoReplaceStringTemplate` using Handlebars syntax to reconstruct
  the full reference, conditionally including the digest

The Renovate docker datasource resolves both version updates (new tag) and
digest updates (same tag, new manifest). When a tag bumps (e.g.,
`alpine:3.21` to `alpine:3.22`), Renovate updates both `currentValue` and
`currentDigest`. When only the digest rotates (same tag, new content),
Renovate updates just `currentDigest`.

**Auto-merge must not be enabled** for `container-images.json` updates.
The security model depends on human review of digest changes via CODEOWNERS.
Auto-merging digest-only PRs would bypass that gate and allow a compromised
registry update to land without review.

### Consumer Impact

| Consumer | Change Required |
|----------|----------------|
| `containerimages.ImageForFamily()` | None. Returns the full string as-is. |
| `containerimages.DefaultImage()` | None. Returns the full string. |
| `generateDockerfile()` | None. `FROM image:tag@sha256:...` is valid Docker syntax. |
| `ContainerImageName()` | None. Hash input includes full string, so digest changes invalidate cache. |
| CI jq consumers | None. `jq -r .alpine` returns the full string. |
| Test assertions | **Update needed.** Expected values must include `@sha256:...` suffix. |
| drift-check hardcoded-references | None. Existing exceptions cover source files; tag pattern matches regardless of digest suffix. |

### drift-check Workflow Update

The hardcoded-references job doesn't need changes. Its scan pattern
matches tag strings like `alpine:[0-9]` and `debian:(bookworm|...)`, and
the existing exception `'container-images\.json:'` already excludes both
the source-of-truth file and its embedded copy. The `@sha256:` suffix
doesn't interfere because the regex matches the tag portion regardless
of what follows it. If a hardcoded reference elsewhere in the codebase
included a digest, the tag pattern would still catch it.

## Implementation Approach

### Step 1: Resolve Current Digests

Query each registry for the manifest list digest using `crane` (from
go-containerregistry):

```bash
for img in "debian:bookworm-slim" "fedora:41" "archlinux:base" \
           "alpine:3.21" "opensuse/tumbleweed:latest"; do
  digest=$(crane digest "$img")
  echo "$img@$digest"
done
```

Use `crane digest` rather than `docker manifest inspect -v`, which returns
platform-specific manifest digests instead of the manifest list digest. The
manifest list digest is what we want: it's architecture-independent, so the
same pinned reference works on both amd64 CI runners and arm64 hosts used
by `platform-integration.yml`.

### Step 2: Update container-images.json

Replace each value with the tag@digest form. For opensuse/tumbleweed,
add the `:latest` tag.

### Step 3: Regenerate Embedded Copy

```bash
go generate ./internal/containerimages/...
```

### Step 4: Update Renovate Config

Update `renovate.json` with the new regex and autoReplaceStringTemplate.

### Step 5: Update Tests

Update `containerimages_test.go` and `container_spec_test.go` expected
values to include digest suffixes.

### Step 6: Verify

- `go test ./...` passes
- `go vet ./...` clean
- Drift-check CI jobs pass
- Renovate dry-run shows correct detection of pinned images

## Security Considerations

### Download Verification

Digest pinning directly improves download verification. With tags alone,
`docker pull alpine:3.21` trusts the registry to return whatever content
the tag currently points to. With `alpine:3.21@sha256:abc...`, the
container runtime verifies the pulled content matches the digest. A
compromised registry that serves a different image for the same tag
would be caught by the digest mismatch.

### Execution Isolation

Not affected. Digest pinning changes how images are *identified*, not
how they're *executed*. Sandbox isolation (rootless containers, no
network by default, resource limits) remains unchanged.

### Supply Chain Risks

Digest pinning reduces supply chain risk by making the image reference
tamper-evident. Without pinning, a compromised registry could serve
malicious content under a trusted tag. With pinning, any content change
requires a corresponding digest change, which must go through a Renovate
PR reviewed by CODEOWNERS.

The Renovate update flow creates an auditable chain: registry publishes
new manifest, Renovate detects digest change, opens PR with new digest,
CODEOWNERS review and merge. At no point can image content change without
a tracked commit to `container-images.json`.

### User Data Exposure

Not affected. Container images are public base images (Debian, Fedora,
Alpine, etc.) pulled from public registries. No user data is involved
in the image reference or pull process.

## Consequences

### Positive

- **Immutable references.** The same `container-images.json` always resolves
  to the same image content, regardless of when or where it's pulled.
- **Automatic cache invalidation.** `ContainerImageName()` hash changes when
  the digest changes, so stale cached images are rebuilt without manual
  intervention.
- **Auditable updates.** Every image content change appears as a Renovate PR
  modifying the digest in `container-images.json`, reviewed by CODEOWNERS.
- **Zero production code changes.** The image string is opaque to all Go
  consumers; only test assertions need updating.

### Negative

- **Noisier diffs.** Digest strings are 64 hex characters, making
  `container-images.json` diffs harder to read at a glance. Reviewers
  need to focus on the tag portion to understand what changed.
- **More frequent Renovate PRs.** Digest rotations happen more often than
  tag bumps. Each base image might get a new digest weekly (or more for
  rolling releases like tumbleweed). This is additional review load, though
  the PRs are mechanical and low-risk. Reviewers should spot-check at
  least one digest per PR with `crane digest <image>` to verify Renovate
  proposed the correct value.
- **Initial test churn.** All test assertions that check exact image strings
  need a one-time update. This is a bounded cost.
