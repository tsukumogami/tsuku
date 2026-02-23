---
status: Current
problem: |
  Container images in container-images.json use mutable tags like alpine:3.21
  that can resolve to different content over time. The sandbox cache key
  includes the tag string but not a content identifier, so the cache won't
  invalidate when an upstream tag gets a new push. This causes silent staleness
  and makes builds non-reproducible across machines and time.
decision: |
  Add SHA256 digest suffixes to every image reference in container-images.json
  using Docker's native tag@digest format. Switch openSUSE from Tumbleweed
  (rolling) to Leap (versioned) so four of five images have meaningful version
  tags. Update the Renovate regex to capture and maintain digests automatically.
  No Go code changes are needed since the image string is opaque to all consumers.
rationale: |
  The inline tag@digest format is the standard Docker reference syntax, supported
  natively by FROM directives, docker pull, and podman. Keeping the tag alongside
  the digest preserves human readability and lets Renovate track both version
  bumps and digest rotations through a single regex pattern. Switching openSUSE
  from Tumbleweed to Leap standardizes the version strategy so that Arch is the
  only remaining rolling-release image, and it has no versioned alternative.
---

# Container Image Digest Pinning

## Status

**Status:** Current

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
- **Consistent version strategy**: each image should pin to a meaningful
  version tag where one exists, so Renovate updates carry version context
  for reviewers.
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
  "suse": "opensuse/leap:15.6@sha256:qr78st90..."
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

### Decision 2: Handling the openSUSE Entry

`opensuse/tumbleweed` is a rolling release distribution with no version
tags. The current config uses the bare image name without any tag, which
Docker interprets as `:latest`. This creates a problem for both the
Renovate regex (which expects a `:tag` component to capture as
`currentValue`) and for the version tag strategy (rolling releases
provide no version context for reviewers).

Decision 3 (below) resolves this by switching from Tumbleweed to Leap.
Leap uses standard `MAJOR.MINOR` version tags (`15.6`), so the Renovate
regex matches uniformly and version bumps are meaningful. This decision
is kept as a record of the alternatives evaluated for the openSUSE entry
specifically.

#### Chosen: Switch to opensuse/leap (see Decision 3)

Replace `"opensuse/tumbleweed"` with `"opensuse/leap:15.6@sha256:..."`.
Leap is openSUSE's stable release with semver-style tags that Renovate
handles natively, uses the same zypper package manager, and provides
the same SUSE family coverage for sandbox testing.

#### Alternatives Considered

**Add explicit :latest tag to Tumbleweed**: Use
`"opensuse/tumbleweed:latest@sha256:..."`. Docker treats `image` and
`image:latest` identically. This would make the Renovate regex uniform,
but reviewers would see digest-only updates with no version context
for the most volatile image in the list. We use a custom regex manager
(not the built-in Docker manager), so Renovate's default `latest`
deprioritization doesn't apply.

**Keep untagged, skip digest pinning for tumbleweed**: Leave it as
`"opensuse/tumbleweed"` and accept that this one image isn't pinned.
Rejected because it undermines the goal. If one image can drift
silently, the reproducibility guarantee is incomplete. Tumbleweed is
the *most* volatile image in the list (rolling release), making it the
one that benefits most from pinning.

### Decision 3: Version Tag Strategy

The five images currently use inconsistent version granularity:

| Family | Current Tag | What It Means |
|--------|-------------|---------------|
| debian | `bookworm-slim` | Codename (major release series, gets security patches) |
| rhel | `41` | Major version (13-month lifecycle, gets all updates) |
| alpine | `3.21` | Minor version (gets patch updates within 3.21.x) |
| arch | `base` | Variant tag, no version (rolling release) |
| suse | (none) | Implied `:latest` (rolling release) |

Debian, Fedora, and Alpine pin to a release series and receive only
compatible updates within that series. Arch and openSUSE Tumbleweed are
rolling releases with no version tags at all, so any pull could bring
breaking changes. Digest pinning fixes the content-at-a-point-in-time
problem, but doesn't address the question of *which* content to pin:
a stable release series or whatever's newest.

This matters for Renovate. When Renovate bumps `alpine:3.21` to
`alpine:3.22`, the tag change signals a meaningful version update that
reviewers can evaluate. When it bumps a tumbleweed digest, there's no
version context, just a new hash.

#### Chosen: Switch openSUSE from Tumbleweed to Leap

Replace `opensuse/tumbleweed` with `opensuse/leap:15.6`. Leap is
openSUSE's stable release, with semver-style `MAJOR.MINOR` tags that
Renovate handles natively. It uses the same zypper package manager and
provides the same SUSE family coverage for sandbox testing.

Keep Arch Linux on the `base` tag. Arch has no versioned alternative:
it is rolling by design, and its Docker images only offer date-based
tags with CI job number suffixes (e.g., `base-20260215.0.490969`) that
Renovate can't parse without custom regex versioning. The digest pin
provides the content immutability that a version tag can't.

After this change, every image except Arch has a meaningful version tag.
Arch is the accepted exception: it's the only major distro family that
offers no stable release channel.

The resulting tag strategy:

| Family | Tag | Version Signal | Renovate Behavior |
|--------|-----|---------------|-------------------|
| debian | `bookworm-slim` | Codename (major) | Digest updates within Bookworm |
| rhel | `41` | Major version | Tag bump to `42` + digest updates within `41` |
| alpine | `3.21` | Minor version | Tag bump to `3.22` + digest updates within `3.21` |
| arch | `base` | None (rolling) | Digest updates only |
| suse | `15.6` | Minor version | Tag bump to `15.7` or `16.0` + digest updates within `15.6` |

#### Alternatives Considered

**Keep Tumbleweed with date-based tags**: Use
`opensuse/tumbleweed:20260220` instead of the rolling `latest` tag.
Rejected because Renovate can't compare YYYYMMDD tags without custom
regex versioning configuration, and the date tags provide no stability
guarantee. A date tag says "this was built on February 20th" but not
"this is compatible with what you had before." Leap's semver tags give
both signals.

**Pin Arch to date-based tags**: Use `archlinux:base-20260215.0.490969`
for explicit version tracking. Rejected because these tags include CI
job numbers that change every build, producing noise in Renovate diffs
without adding useful version information. The format also requires
custom regex versioning in Renovate. Since Arch is inherently rolling,
the digest pin already provides the content stability we need.

**Keep current tags unchanged**: Accept the inconsistency and rely
solely on digest pinning for all images. Rejected because Tumbleweed's
volatility makes digest-only updates problematic. With no version tag,
reviewers can't tell if a digest update brought a routine security patch
or a major package overhaul. Leap's versioned releases make this
distinction visible.

## Decision Outcome

**Chosen: 1A + 2A + 3A** (inline tag@digest, explicit tags, Leap replaces Tumbleweed)

### Summary

Every image reference in `container-images.json` gets a `@sha256:...`
digest suffix using Docker's native `image:tag@sha256:digest` format.
openSUSE switches from Tumbleweed (rolling) to Leap (versioned), giving
four of five images a meaningful version tag. Arch Linux stays on `base`
since no versioned alternative exists.

The resulting `container-images.json`:

```json
{
  "debian": "debian:bookworm-slim@sha256:<digest>",
  "rhel": "fedora:41@sha256:<digest>",
  "arch": "archlinux:base@sha256:<digest>",
  "alpine": "alpine:3.21@sha256:<digest>",
  "suse": "opensuse/leap:15.6@sha256:<digest>"
}
```

The Renovate regex custom manager in `renovate.json` gets two additions:
an optional `currentDigest` capture group at the end of the match pattern,
and an `autoReplaceStringTemplate` that reconstructs the full reference
on updates. Renovate's docker datasource handles both tag bumps (e.g.,
`alpine:3.21` to `3.22`) and digest rotations (same tag, new content)
through a single configuration.

No Go production code changes. `ImageForFamily()` returns the full
string as-is, which flows unchanged into `generateDockerfile()` as a
`FROM` argument and into `ContainerImageName()` as a hash input. The
cache key becomes content-aware automatically because the digest is
part of the hashed string.

Tests that assert exact image values need updates for the new strings.
The drift-check CI workflow needs its hardcoded-references regex updated
to match `opensuse/leap` instead of `opensuse/(leap|tumbleweed)`, and
the `container-images.json` exception already covers the digest suffix.

To get the initial digests, we query the registries using `crane digest`
and update `container-images.json`. After that, Renovate handles all
future updates.

### Rationale

The inline format works because Docker already defines a standard way to
combine tags and digests in a single string. Fighting that standard by
splitting into structured JSON would create work for every consumer
without adding any capability. The tag stays for readability and Renovate
version tracking. The digest stays for immutability and cache correctness.

Switching from Tumbleweed to Leap standardizes the version strategy: every
image except Arch now pins to a release series with a semver-compatible tag.
Renovate can propose both version bumps and digest updates with clear
version context. Arch remains the exception because no versioned Arch
release exists, and its date-based tags include CI job numbers that provide
no useful version signal.

## Solution Architecture

### container-images.json

After pinning, the file contains:

```json
{
  "debian": "debian:bookworm-slim@sha256:<64-char-hex>",
  "rhel": "fedora:41@sha256:<64-char-hex>",
  "arch": "archlinux:base@sha256:<64-char-hex>",
  "alpine": "alpine:3.21@sha256:<64-char-hex>",
  "suse": "opensuse/leap:15.6@sha256:<64-char-hex>"
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
| drift-check hardcoded-references | **Update needed.** Replace `tumbleweed` with `leap` in `PATTERN` regex. |

### drift-check Workflow Update

The hardcoded-references `PATTERN` regex needs one update: change
`opensuse/(leap|tumbleweed)` to reflect the switch from Tumbleweed to
Leap. The `@sha256:` suffix doesn't need special handling because the
existing exception `'container-images\.json:'` already excludes the
source file and its embedded copy, and the tag-matching regex catches
the tag portion regardless of what follows it.

## Implementation Approach

### Step 1: Resolve Current Digests

Query each registry for the manifest list digest using `crane` (from
go-containerregistry):

```bash
for img in "debian:bookworm-slim" "fedora:41" "archlinux:base" \
           "alpine:3.21" "opensuse/leap:15.6"; do
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

Replace each value with the tag@digest form. Switch `opensuse/tumbleweed`
to `opensuse/leap:15.6`.

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
