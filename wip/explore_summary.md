# Exploration Summary: Container Image Digest Pinning

## Problem (Phase 1)
Container images in container-images.json use mutable tags (e.g., `alpine:3.21`) that can resolve to different content over time. This means the same tag pulled today and next week may contain different packages, security patches, or breaking changes. The sandbox cache key includes the tag string but not the actual image content, so a stale cached image won't be rebuilt when the upstream tag updates. Digest pinning (`alpine:3.21@sha256:...`) makes image references immutable and automatically improves cache invalidation.

## Decision Drivers (Phase 1)
- Immutability: same reference must always resolve to the same image content
- Automated maintenance: Renovate must be able to update digests and tags
- Transparent to consumers: all jq and Go consumers should work without code changes
- Rolling releases: opensuse/tumbleweed has no version tag, needs special handling
- Cache correctness: ContainerImageName() should invalidate when image content changes

## Research Findings (Phase 2)
- Docker natively supports `image:tag@sha256:digest` in FROM directives
- Renovate regex can capture digest via optional `currentDigest` group + `autoReplaceStringTemplate`
- No image name parsing in production Go code — string is fully opaque
- ContainerImageName() hash automatically benefits from digest changes
- opensuse/tumbleweed has no version tag (rolling release), can use :latest explicitly

## Options (Phase 3)
- **Decision 1 (Format)**: Inline tag@digest strings (chosen) vs structured JSON objects vs separate digest file
- **Decision 2 (Tumbleweed)**: Add explicit :latest tag (chosen) vs digest-only without tag vs skip pinning
- **Decision 3 (Version Strategy)**: Switch openSUSE to Leap (chosen) vs keep Tumbleweed with date tags vs accept inconsistency

## Decision (Phase 5)

**Problem:**
Container images in container-images.json use mutable tags like alpine:3.21
that can resolve to different content over time. The sandbox cache key
includes the tag string but not a content identifier, so the cache won't
invalidate when an upstream tag gets a new push. This causes silent staleness
and makes builds non-reproducible across machines and time.

**Decision:**
Add SHA256 digest suffixes to every image reference in container-images.json
using Docker's native tag@digest format. Switch openSUSE from Tumbleweed
(rolling) to Leap (versioned) so four of five images have meaningful version
tags. Update the Renovate regex to capture and maintain digests automatically.
No Go code changes are needed since the image string is opaque to all consumers.

**Rationale:**
The inline tag@digest format is the standard Docker reference syntax, supported
natively by FROM directives, docker pull, and podman. Keeping the tag alongside
the digest preserves human readability and lets Renovate track both version
bumps and digest rotations through a single regex pattern. Switching openSUSE
from Tumbleweed to Leap standardizes the version strategy so that Arch is the
only remaining rolling-release image, and it has no versioned alternative.

## Current Status
**Phase:** 8 - Final Review
**Last Updated:** 2026-02-22
