# Exploration Summary: Platform Compatibility Verification

## Problem (Phase 1)

tsuku claims support for multiple platforms and Linux families but testing doesn't verify actual compatibility - tests simulate environments (e.g., running "Alpine" tests on Ubuntu) rather than running on real targets. This was exposed when musl-based systems couldn't load embedded libraries that link against glibc.

## Decision Drivers (Phase 1)

- Accuracy over speed: Real tests catch real issues
- Release parity: Every released binary needs integration tests
- Fail-fast discovery: Catch incompatibilities in CI, not production
- Maintainability: Sustainable test infrastructure
- CI resource constraints: Limited ARM64 runners and container support
- Self-contained philosophy: Should not mean unnecessary duplication

## Research Findings (Phase 2 + Extended Research)

**Codebase patterns:**
- Container-based family testing exists via test scripts with `family` parameter
- Platform detection via /etc/os-release parsing with ID_LIKE fallback
- Homebrew bottles with platform tags and patchelf RPATH fixup
- dlopen verification via tsuku-dltest Rust helper with batching and retry
- Package manager actions (apt_install, apk_install, etc.) already exist with ImplicitConstraint() for mutual exclusion

**Extended research findings:**
- Homebrew bottle versions are NOT fresher than distro packages (distros often lead)
- System packages are security-reviewed by institutional distro teams
- Only 4 embedded library recipes exist (zlib, libyaml, openssl, gcc-libs)
- Dependency graph is shallow (max depth 2)
- "Self-contained" value is about no build tools needed, not about duplicating system libs

**Industry patterns:**
- asdf/mise: Assume system has deps, don't manage them
- Homebrew: Manages deps within Homebrew ecosystem
- Nix: Complete isolation (over-engineered for tsuku's use case)
- mise specifically: Auto-detects glibc vs musl, falls back to source, honest about deps

**Alpine APK research:**
- APK files are extractable tar.gz archives (three concatenated gzip streams)
- CDN provides versioned packages at `dl-cdn.alpinelinux.org/alpine/v{version}/{repo}/{arch}/`
- APKINDEX.tar.gz contains checksums (SHA1) and dependency info
- Could implement "Homebrew for musl" by downloading and extracting APKs directly

## Options (Phase 3)

Three decisions with options:

1. **Musl compatibility**:
   - 1A: Document glibc requirement only
   - 1B: Provide musl-specific binaries (doubles maintenance)
   - 1C: Runtime detect and skip gracefully
   - **1D: System packages via existing actions (apk_install, etc.)**
   - 1E (new): Alpine APK extraction (hermetic alternative)

2. **Real environment testing**: Containers (2A), native runners (2B), hybrid (2C)

3. **Verification matrix scope**: Match release (3A), representative subset (3B), full matrix (3C)

## Current Decision (Phase 5)

**Problem:** tsuku claims support for multiple platforms and Linux families but testing doesn't verify actual compatibility, as discovered when musl-based systems couldn't load embedded libraries.

**Decision:** Adopt "self-contained tools, system-managed dependencies" philosophy. Tools remain self-contained binaries, but library dependencies use system package managers via existing actions. Combined with hybrid testing (native runners + containers) and test coverage matching the release matrix.

**Rationale:** System packages are available on all Linux families (including Alpine/musl), are security-reviewed by distro teams, and eliminate the glibc/musl incompatibility. This trades hermetic version control for working tools across all platforms - a worthwhile trade given that distros backport security fixes and version differences rarely cause compatibility issues.

**Open alternative:** Alpine APK extraction could preserve hermetic version control for musl while using Homebrew on glibc. This adds complexity but addresses concern about "giving up an important feature."

## Reviews Completed (Phase 8)

- **Architecture Review:** Design is clear enough to implement with minor clarifications. Phases can run in parallel (ARM64 testing doesn't depend on musl detection).
- **Security Review:** Design is security-sound. Mitigations are sufficient. No blockers.

## Current Status

**Phase:** 5 - Decision (awaiting final musl approach selection)
**Last Updated:** 2026-01-24
**Full synthesis:** `wip/research/explore_full_synthesis.md`
