# Issue #1320 Implementation Plan

## Current State

All acceptance criteria are already implemented in `.github/workflows/batch-generate.yml` on main. The prototype was written in the previous session (PR #1303) and merged. The generate job builds 4 cross-compiled binaries, uploads passing-recipes and tsuku-binaries artifacts. Four validation jobs test across 11 environments with retry logic, JSON results, and artifact uploads.

## Gaps

None. Every AC item is satisfied by the current code.

## Plan

Since all code is already on main, this issue needs only verification:

1. Trigger workflow manually with `ecosystem=homebrew`, `batch_size=2`
2. Verify all 6 jobs run (generate + 4 validation + merge)
3. Check artifact uploads and result format
4. Close issue

## Files

No changes needed.
