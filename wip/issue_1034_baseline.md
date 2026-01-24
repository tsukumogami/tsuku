# Issue 1034 Baseline

## Environment
- Date: 2026-01-24
- Branch: refactor/1034-golden-file-reorg
- Base commit: 68413a7d7df1429aed93cd21f82d4a8ec0f2efe9

## Test Results
- Total: 25 packages
- Passed: All (some cached)
- Failed: 0

## Build Status
Pass - no warnings

## Pre-existing Issues
None - all tests pass on main

## Notes
- PR #1073 (issue #1071) was merged, flattening embedded recipes to `internal/recipe/recipes/*.toml`
- This changes the implementation approach - embedded recipes no longer use letter subdirectories
- Golden files currently still use letter subdirectories for all recipes
