# Issue 549 Baseline

## Environment
- Date: 2025-12-21 18:54:40 UTC
- Branch: feature/549-cmake-recipe
- Base commit: 938915c fix(recipe): update curl recipe for dynamic versioning (#657)

## Test Results
- Total: Multiple test packages
- Build status: ✅ Pass
- Pre-existing test failures: 1

### Pre-existing Failure
- `TestCargoInstallAction_Decompose` in `internal/actions`
  - Error: `cargo not found: install Rust first (tsuku install rust)`
  - This is a pre-existing failure unrelated to cmake work

## Build Status
✅ Pass - `go build -o tsuku ./cmd/tsuku` succeeds with no errors or warnings

## Coverage
Not measured for baseline (Go coverage not typically run for recipe additions)

## Pre-existing Issues
- One test failure in cargo_install_test.go related to Rust/cargo not being installed in test environment
- This failure exists on main branch and is unrelated to the cmake recipe work
