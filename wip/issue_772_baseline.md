# Issue 772 Baseline

## Environment
- Date: 2026-01-12
- Branch: chore/772-migrate-recipes-typed-actions
- Base commit: 49351bb (origin/main)

## Test Results
- Total: 20 packages tested
- Passed: All
- Failed: 0

## Build Status
Pass - `go build -o tsuku ./cmd/tsuku` successful

## Migration Scope (from discovery #758)

The discovery script identified **3 recipes** requiring migration:

| Recipe | darwin | linux | Complexity |
|--------|--------|-------|------------|
| cuda | Not supported | Manual (NVIDIA) | Complex |
| docker | brew_install | Manual (docs) | Complex |
| test-tuples | brew_install | N/A | Simple |

## Pre-existing Issues
- `cuda` and `docker` have manual Linux instructions that may need special handling
- `test-tuples` appears to be a test recipe
