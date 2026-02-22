# Exploration Summary: Embedded Recipe musl Coverage

## Problem (Phase 1)
Six embedded recipes download glibc-only binaries and fail on Alpine/musl. CI and static analysis don't catch it.

## Decision Drivers (Phase 1)
- All six tools have Alpine packages available
- Eight recipes already demonstrate the libc-aware pattern
- Infrastructure (WhenClause.Libc, DetectLibc, apk_install) exists
- Static analysis gap means this class of bug can recur

## Research Findings (Phase 2)
- Pattern: cmake.toml uses three paths (glibc Homebrew, musl apk_install, darwin Homebrew)
- Coverage test only checks when clauses, not os_mapping semantics
- test-recipe-changes.yml already triggers for embedded paths
- All six tools have direct Alpine package equivalents

## Options (Phase 3)
- 1A: apk_install with Alpine packages (chosen)
- 1B: Download musl-specific binaries (rejected: more work, no clear benefit)
- 1C: Skip musl with supported_libc constraint (rejected: loses musl support entirely)
- 2A: Extend AnalyzeRecipeCoverage to flag glibc os_mapping (chosen)
- 2B: Require when clauses on all downloads (rejected: over-broad)
- 3A: Add embedded paths to CI triggers (chosen)

## Decision (Phase 5)

**Problem:**
Six embedded recipes (rust, python-standalone, nodejs, ruby, perl, patchelf) download glibc-linked binaries unconditionally. On Alpine Linux and other musl-based systems, these binaries can't execute because the glibc dynamic linker doesn't exist. Eight other embedded recipes already handle this correctly with libc-aware when clauses, but the remaining six were never updated. CI doesn't catch it because the static analysis treats unconditional steps as universally compatible, and PR-time recipe testing doesn't trigger for embedded recipe paths.

**Decision:**
Add musl fallback paths to all six recipes using apk_install with Alpine system packages, matching the pattern established by cmake, openssl, and other already-fixed recipes. Extend the static analysis in AnalyzeRecipeCoverage to flag recipes that use os_mapping with glibc-specific values but lack libc when clauses. Add embedded recipe paths to the test-recipe.yml trigger so PR-time Alpine testing covers them.

**Rationale:**
The apk_install pattern is proven across eight existing recipes and requires no new infrastructure. Alpine packages exist for all six tools. Fixing the static analysis catches this class of bug at unit test time rather than waiting for weekly validation runs. Adding the CI trigger path is a one-line change that closes the coverage gap without restructuring workflows.

## Current Status
**Phase:** 7 - Security (complete)
**Last Updated:** 2026-02-22
