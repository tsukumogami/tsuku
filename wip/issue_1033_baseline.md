# Issue 1033 Baseline

## Environment
- Date: 2026-01-19
- Branch: refactor/1033-migrate-registry-recipes
- Base commit: 7268a4007660a13b21bb00b603c9a5f09f513680

## Test Results
- Total packages: 22
- Passed: 21
- Failed: 1 (internal/actions - pre-existing)

### Pre-existing Failures (on main)
The `internal/actions` package has failing tests related to toolchain resolution. These failures exist on main and are unrelated to this issue:

- `TestResolveGo_*` (7 tests) - Go resolution tests
- `TestResolveCargo_*` (5 tests) - Cargo resolution tests
- `TestResolvePerl_*` / `TestResolveCpanm_*` (2 tests) - Perl resolution tests
- `TestCpanInstallAction_*` (7 tests) - CPAN action tests
- `TestGoBuildAction_*` / `TestGoInstallAction_*` (14 tests) - Go build/install action tests

These tests appear to have environment-specific dependencies that aren't satisfied in the current test environment.

## Build Status
- `go build -o tsuku ./cmd/tsuku`: PASS (no warnings)

## Coverage
Not tracked for this issue (toolchain resolution tests are environment-dependent).

## Pre-existing Issues
- `internal/actions` test failures are present on main branch
- Tests related to Go/Cargo/Perl resolution fail due to missing toolchain setup in test environment
