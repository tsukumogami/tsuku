# Issue 768 Baseline

## Environment
- Date: 2026-01-01 20:11:20 UTC
- Branch: feature/768-container-spec-derivation
- Base commit: 60a1a5c22ca683f52bb3a5847e58fd96e9abe420

## Test Results
- Total: 19 packages tested
- Passed: 18 packages
- Failed: 1 package (internal/actions)

## Pre-existing Issues
- TestCargoInstallAction_Decompose fails with "cargo not found: install Rust first"
  - This is a pre-existing failure not related to issue #768
  - Package: github.com/tsukumogami/tsuku/internal/actions
  - Error: "Decompose() failed: cargo not found: install Rust first (tsuku install rust)"

## Build Status
Build succeeds - binary created successfully at ./tsuku

## Coverage
Coverage not measured in baseline (will be checked during finalization if needed)
