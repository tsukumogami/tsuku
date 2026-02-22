# Issue 1841 Summary

## What Was Implemented

Fixed the overly aggressive `checkExistingRecipe` guard in `runCreate` that blocked exact name matches, and added `@critical` tags to 4 representative `create.feature` scenarios so they run in every PR CI.

## Changes Made
- `cmd/tsuku/create.go`: Changed call-site condition from blocking all matches to only blocking satisfies-alias matches (`canonicalName != toolName`)
- `test/functional/features/create.feature`: Added `@critical` tags to 4 scenarios covering npm, pypi, discovery (deterministic-only), and discovery registry paths

## Key Decisions
- Tagged 4 scenarios (not all 18): Covers the main code paths without bloating critical-only CI runs
- Selected scenarios cover both `--from` (ecosystem) and no-`--from` (discovery) paths, plus error handling

## Trade-offs Accepted
- Not all create scenarios are `@critical`: Acceptable because the 4 tagged scenarios exercise the `checkExistingRecipe` code path that caused the regression

## Test Coverage
- New tests added: 0 (existing unit tests already cover `checkExistingRecipe` helper; `@critical` tags are the guardrail)
- All 37 packages pass

## Requirements Mapping

| AC | Status | Evidence / Reason |
|----|--------|-------------------|
| Exact name match no longer blocked | Implemented | `create.go:486` condition now requires `canonicalName != toolName` |
| Satisfies-alias match still blocked | Implemented | Same condition; when names differ, `exitWithCode` fires |
| CI guardrail for future regressions | Implemented | 4 `@critical` tags in `create.feature` ensure scenarios run on every PR |
