---
status: Proposed
problem: |
  Tsuku has no self-update path. Users must re-run the installer script to get
  new versions, which creates friction and lets the binary fall behind silently.
  When Feature 2's background checks detect a newer tsuku release, there's no
  mechanism to act on that information.
decision: |
  TBD
rationale: |
  TBD
---

# DESIGN: Self-Update Mechanism

## Status

Proposed

## Context and Problem Statement

Tsuku has no self-update path today. Users must re-run the installer script to
get new versions, which creates friction and means tsuku can silently fall behind.
Feature 2's background update check infrastructure can detect when a newer tsuku
release exists, but there's no command to act on that information.

The self-update mechanism must be separate from the managed tool system (PRD
decision D5) to avoid bootstrap risk -- a broken updater that can't fix itself.
The rename-in-place pattern is well-understood and keeps the self-update path
simple.

GoReleaser produces platform-specific binaries (`tsuku-{os}-{arch}`) with SHA256
checksums, published as GitHub releases on `tsukumogami/tsuku`.

## Decision Drivers

- **Atomic replacement**: A failed update must never leave the user without a
  working tsuku binary
- **Simplicity**: PRD D5 explicitly chose a separate code path (~30 lines) over
  treating tsuku as a managed tool, to avoid bootstrap risk
- **Verification**: Downloaded binaries must be checksum-verified before
  replacement
- **Integration**: Should work with Feature 2's existing update cache so
  `tsuku outdated` can report tsuku's own staleness
- **No version pinning**: tsuku always tracks latest stable (PRD R7)
