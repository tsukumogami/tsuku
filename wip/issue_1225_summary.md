# Issue 1225 Summary

## What Was Implemented

Fixed cmake recipe OpenSSL rpath drift and added go.mod to Build Essentials CI path triggers to prevent similar gaps.

## Changes Made
- `internal/recipe/recipes/cmake.toml`: Replaced hardcoded `openssl-3.6.0` in rpath with `{deps.openssl.version}` template variable
- `.github/workflows/build-essentials.yml`: Added `go.mod` to both push and pull_request path triggers
- `testdata/golden/plans/embedded/cmake/`: Regenerated at v4.2.3 (v4.2.1 bottles expired from Homebrew)
- `testdata/golden/plans/embedded/ninja/`: Regenerated to pick up cmake rpath template change

## Key Decisions
- Used `{deps.openssl.version}` template variable instead of updating to a new hardcoded version: This matches the pattern used by the curl recipe and eliminates the class of bug entirely. The variable is expanded at execution time using the resolved dependency version.
- Regenerated golden files at v4.2.3 instead of keeping v4.2.1: Homebrew no longer serves bottles for v4.2.1, making the old golden files unvalidatable.

## Root Cause Analysis

**Immediate cause**: The cmake recipe hardcoded `openssl-3.6.0` in its rpath (commit 612224a8, Dec 22 2025). When Homebrew updated openssl@3 to 3.6.1, the rpath pointed to a nonexistent directory. The cmake binary then fell back to the system's libssl.so.3, which on GitHub Actions ubuntu-latest provides only OPENSSL_3.0.x symbols -- too old for cmake 4.2.3's requirement for OPENSSL_3.2.0.

**Why it wasn't caught**: The Go version was bumped from 1.25.5 to 1.25.6 (commit 07ee15dc), which only changed `go.mod`. Since `go.mod` wasn't in the Build Essentials path filter, the workflow didn't run on that PR or its merge to main. The OpenSSL version drift happened between the last successful Build Essentials run and the next time the workflow was triggered (PR #1223, which happened to touch `scripts/*.sh`).

**Prevention**: Adding `go.mod` to path triggers ensures Go version bumps (which change the runner image via `go-version-file`) trigger integration tests. The weekly schedule already serves as a catch-all for upstream Homebrew changes.

## Trade-offs Accepted
- Golden files bumped from cmake v4.2.1 to v4.2.3: Necessary because Homebrew expired the old bottles. This is expected behavior for recipes tracking latest versions.

## Test Coverage
- No new Go tests needed -- the fix is in recipe TOML and CI workflow configuration
- Golden file regeneration validates the recipe change produces correct plan output
- Build Essentials CI (on the PR) is the integration test

## Known Limitations
- The weekly Build Essentials schedule remains the only catch for upstream Homebrew bottle changes that don't correspond to any repo file change
