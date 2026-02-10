# Issue 1588 Baseline

## Environment
- Date: 2026-02-10
- Branch: docs/plan-hash-removal (continuing from PR #1582)
- Base commit: 99beaa7e60ae93803bf95d9aa37251c11e2b7233

## Test Results
- Executor tests: All pass (100+)
- Overall: Some sandbox/validate tests fail (infrastructure issues, not related to this work)
  - TestSandboxIntegration_* failures: Container/Docker issues
  - TestEvalPlanCacheFlow: GitHub API 404 (infrastructure)

## Build Status
Pass (no warnings)

## Coverage
Not tracked for this simple chore issue

## Pre-existing Issues
- Sandbox integration tests failing due to container infrastructure
- GitHub API returning 404 for some test files
- These are unrelated to golden file regeneration

## Issue Requirements
- Regenerate all ~600 golden files to v4 format
- No `recipe_hash` field in any golden file
- All `format_version` must be 4
- Tests must pass after regeneration
