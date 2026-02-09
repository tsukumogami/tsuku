# Exploration Summary: musl-library-testing

## Problem (Phase 1)

The musl library dlopen tests fail because the CI workflow doesn't install the system packages that recipes require. The `apk_install` action only verifies packages exist - it doesn't install them. The workflow hardcodes build tools but not library-specific packages, so tests for zlib and libyaml fail with "missing system packages" errors.

## Decision Drivers (Phase 1)

- Recipes should be the source of truth for required packages
- Tests should work consistently across glibc, musl, and sandbox modes
- Adding new library recipes shouldn't require workflow changes
- Solution should align with how sandbox mode already handles this

## Current Status
**Phase:** 1 - Problem
**Last Updated:** 2026-02-08
