# Issue 38 Baseline

## Environment
- Date: 2025-11-28
- Branch: chore/38-raise-coverage-threshold
- Base commit: f615a0149946d7a9c88606e5829cdbba4e8b4e61

## Test Results
- Total: 11 packages tested
- Passed: All tests pass
- Failed: None

## Build Status
Pass - no warnings

## Coverage

**Overall: 33.1%** (target: 50%)

| Package | Coverage |
|---------|----------|
| internal/recipe | 97.0% |
| internal/buildinfo | 90.0% |
| internal/config | 86.7% |
| internal/registry | 78.3% |
| internal/version | 49.7% |
| internal/executor | 31.4% |
| internal/actions | 25.8% |
| internal/install | 12.5% |
| cmd/tsuku | 3.2% |

**Excluded from coverage (per codecov.yml):**
- cmd/**/*
- internal/install/**/*
- internal/testutil/**/*

**Coverage gap:** Need to increase from 33.1% to 50% (+16.9%)

**Priority packages for improvement:**
1. internal/actions (25.8%) - largest impact potential
2. internal/executor (31.4%) - significant gap
3. internal/version (49.7%) - close to 50%, easy wins

## Pre-existing Issues
None - all tests pass
