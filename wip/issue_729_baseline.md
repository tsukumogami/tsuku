# Issue 729 Baseline

## Environment
- Date: 2025-12-28
- Branch: chore/729-refactor-recipe-copy-pattern
- Base commit: adf50dbc71c06406a0ecb7349814e91d14f4c69c

## Test Results
- All packages: 22 tested
- Passed: 22
- Failed: 0

## Build Status
Build successful - no errors or warnings

## Pre-existing Issues
None identified. All tests pass cleanly.

## Issue Summary
Refactor test workflows to use `--recipe` flag instead of copying recipes to `internal/recipe/recipes/` before building. This simplifies the test pattern and removes the need for recipe embedding during tests.

### Locations to Refactor
1. `.github/workflows/build-essentials.yml` - 5 occurrences
2. `scripts/test-zig-cc.sh` - 1 occurrence

### User Request
User requested: "please enlist agents to find every place in source where this pattern can be applied in order to simplify testing complexity and reduce code surface"

This requires a comprehensive search beyond the known locations.
