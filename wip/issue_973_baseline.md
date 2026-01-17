# Issue 973 Baseline

## Environment
- Date: 2026-01-17
- Branch: refactor/973-consolidate-verify-scripts
- Base commit: 5c32bc4d (main)

## Test Results
- All tests pass (short mode)
- No failures

## Build Status
- Build successful

## Pre-existing Issues
None identified

## Files to Modify
- test/scripts/verify-relocation.sh (151 lines) - to be removed
- test/scripts/verify-no-system-deps.sh (206 lines) - to be removed
- test/scripts/verify-binary.sh (new) - combined script
- .github/workflows/build-essentials.yml - update 8 call sites
