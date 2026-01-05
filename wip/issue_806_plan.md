# Issue 806 Implementation Plan

## Summary

Expand the CI multi-family sandbox testing matrix from limited configuration (debian + simple tools) to full coverage (5 families + complex tools).

## Type

Simplified plan (CI configuration change)

## Files to Modify

1. `.github/workflows/build-essentials.yml` - Expand matrix configuration

## Approach

Modify the `test-sandbox-multifamily` job matrix:

**Current state (lines 371-374):**
```yaml
family: [debian]
tool: [make, pkg-config]
```

**Target state:**
```yaml
family: [debian, rhel, arch, alpine, suse]
tool: [cmake, ninja]
```

This produces 5 × 2 = 10 test combinations validating the full multi-family infrastructure.

## Tool Selection Rationale

- **cmake**: Has declared dependencies (openssl, patchelf, zlib) - validates dependency embedding in plans
- **ninja**: Uses `cmake_build` action and has transitive dependencies - validates complex build toolchains

Both tools were already validated in the local test script `test/scripts/test-cmake-provisioning.sh`.

## Steps

- [x] Update matrix `family` array: `[debian]` → `[debian, rhel, arch, alpine, suse]`
- [x] Update matrix `tool` array: `[make, pkg-config]` → `[cmake, ninja]`
- [x] Remove TODO comment about adding recipes
- [x] Update job comments if needed

## Verification

- Push to trigger CI
- Verify all 10 matrix combinations pass
