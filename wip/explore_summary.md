# Exploration Summary: Platform Compatibility Verification

## Problem (Phase 1)

tsuku claims support for multiple platforms and Linux families but testing doesn't verify actual compatibility - tests simulate environments (e.g., running "Alpine" tests on Ubuntu) rather than running on real targets. This was exposed when musl-based systems couldn't load embedded libraries that link against glibc.

## Decision Drivers (Phase 1)

- Accuracy over speed: Real tests catch real issues
- Release parity: Every released binary needs integration tests
- Fail-fast discovery: Catch incompatibilities in CI, not production
- Maintainability: Sustainable test infrastructure
- CI resource constraints: Limited ARM64 runners and container support

## Research Findings (Phase 2)

**Codebase patterns:**
- Container-based family testing exists via test scripts with `family` parameter
- Platform detection via /etc/os-release parsing with ID_LIKE fallback
- Homebrew bottles with platform tags and patchelf RPATH fixup
- dlopen verification via tsuku-dltest Rust helper with batching and retry

**Industry patterns:**
- GitHub Actions has free native ARM64 Linux runners (ubuntu-24.04-arm)
- musl compatibility typically requires separate binaries or static linking
- Projects like ripgrep distribute separate binaries per libc

**Gap analysis:**
- library-dlopen tests only run on Debian glibc
- No ARM64 Linux integration tests despite releasing binaries
- Alpine/musl disabled due to glibc library incompatibility

## Options (Phase 3)

Three decisions with options:
1. **Musl compatibility**: Document requirement (1A), provide musl binaries (1B), static libs (1C)
2. **Real environment testing**: Containers (2A), native runners (2B), hybrid (2C)
3. **Verification matrix scope**: Match release (3A), representative subset (3B), full matrix (3C)

## Decision (Phase 5)

**Problem:** tsuku claims support for multiple platforms and Linux families but testing doesn't verify actual compatibility, as discovered when musl-based systems couldn't load embedded libraries.
**Decision:** Implement runtime musl detection with user guidance, hybrid testing (native runners + containers), and test coverage matching the release matrix exactly.
**Rationale:** This balances accuracy with maintainability by using native runners for platform verification and containers for family-specific testing, while runtime detection provides fail-fast behavior for Alpine users without requiring immediate musl binary support.

## Current Status

**Phase:** 5 - Decision (complete)
**Last Updated:** 2026-01-24
