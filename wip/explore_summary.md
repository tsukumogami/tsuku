# Exploration Summary: Native Binary Release Workflow

## Problem (Phase 1)

The current release workflow uses goreleaser with CGO_ENABLED=0, cross-compiling all 4 platform binaries from a single ubuntu-latest runner. This approach breaks when adding native binaries (like the Rust-based tsuku-dltest helper) because:

1. CGO cross-compilation to macOS requires osxcross + macOS SDK (only legally available on macOS hardware)
2. Rust's cross-rs also can't cross-compile to macOS from Linux
3. Go binaries built on newer glibc may fail on older systems

This blocks the Level 3 library verification work (issue #1020) and will become more common as tsuku adds native helpers.

## Decision Drivers (Phase 1)

- **Correctness**: All binaries must be tested on their target platform before release
- **All-or-nothing releases**: Don't publish partial releases (broken platforms)
- **Maintainability**: Keep the workflow understandable and debuggable
- **Cost**: Prefer free GitHub runners where possible
- **Compatibility**: Support as many target systems as reasonable
- **Version coherence**: Go and Rust binaries must share version at build time

## Research Findings (Phase 2)

**GoReleaser capabilities:**
- Supports split/merge pattern for multi-runner builds
- Run `goreleaser release --split` per-platform, then `goreleaser continue --merge` to aggregate
- Can build Rust projects as of recent versions
- Version injection via ldflags works well

**GitHub Actions runners (as of Jan 2025):**
- Linux ARM64: `ubuntu-24.04-arm` now free for public repos
- macOS x86_64: `macos-13` (or newer `macos-15-intel`)
- macOS ARM64: `macos-latest`

**glibc compatibility:**
- Ubuntu 20.04: glibc 2.31 (broadest compatibility)
- Ubuntu 22.04: glibc 2.35
- Ubuntu 24.04: glibc 2.39
- Build on oldest target for widest support

**Draft release pattern:**
- Create draft first
- Upload artifacts from parallel jobs
- Finalize only when all succeed
- If any job fails, release stays draft

**macOS signing:**
- Ad-hoc signing (`codesign -s -`) works without Apple Developer Program
- Avoids "app is damaged" errors but Gatekeeper may still warn
- Full notarization requires Apple Developer account ($99/year)

**Existing tsuku patterns:**
- Version injection: `internal/buildinfo` package with ldflags
- Current goreleaser config: `CGO_ENABLED=0`, single ubuntu-latest runner
- Output: plain binaries with checksums.txt

## Options (Phase 3)

1. **Workflow Architecture:**
   - 1A: Unified Split-Merge (goreleaser split/merge)
   - 1B: Staged Build-Then-Release (separate Go/Rust/publish jobs)
   - 1C: Parallel Matrix with Draft Release (draft first, parallel upload)

2. **glibc Compatibility:**
   - 2A: Ubuntu 20.04 (glibc 2.31)
   - 2B: Ubuntu 22.04 (glibc 2.35)
   - 2C: musl static linking

3. **macOS Code Signing:**
   - 3A: Ad-hoc signing
   - 3B: No signing
   - 3C: Defer to future

## Decision (Phase 5)

**Problem:** The current release workflow can't handle native binaries like the Rust dlopen helper because cross-compilation to macOS requires the macOS SDK, which is only legally available on macOS hardware.

**Decision:** Use parallel matrix builds on native runners with a draft-then-publish pattern, Ubuntu 22.04 for glibc 2.35 compatibility, and ad-hoc code signing for macOS.

**Rationale:** Draft releases provide atomic all-or-nothing semantics. Ubuntu 22.04 balances compatibility with support longevity. Ad-hoc signing eliminates common macOS friction without Apple Developer costs.

## Current Status

**Phase:** 5 - Decision complete
**Last Updated:** 2025-01-18
