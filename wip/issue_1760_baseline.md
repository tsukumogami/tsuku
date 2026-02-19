# Issue 1760 Baseline

## Environment
- Date: 2026-02-19
- Branch: fix/1760-coverage-update-permissions
- Base commit: 538a89c3

## Scope
Workflow-only change (.github/workflows/coverage-update.yml). No Go code changes, no tests to run.

## Current State
- coverage-update.yml has never succeeded (7/7 runs failed)
- Missing: permissions block, App token, concurrency, push retry
- Reference workflow: sync-disambiguations.yml (same pattern needed)
