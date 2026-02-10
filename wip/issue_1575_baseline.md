# Issue 1575 Baseline

## Environment
- Date: 2026-02-09
- Branch: feat/recipe-driven-ci-deps (shared with #1573, #1574)
- Base commit: 612f21222dca4ca3962139b4150686c7a8f632fc

## Test Results
- No Go code changes expected - workflow file only
- Pre-existing failures: internal/sandbox, internal/validate (documented in #1574 baseline)

## Build Status
Pass

## Notes

This issue modifies `.github/workflows/integration-tests.yml` only. It's a simple find-and-replace of the `tsuku deps` command with `tsuku info --deps-only --system`.
