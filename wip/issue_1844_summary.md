# Issue 1844 Summary

## Problem
Two workflows (test-changed-recipes, validate-golden-recipes) created ghost failure
runs on every push event due to stale GitHub Actions workflow registrations. These
runs had zero jobs, showed the file path as the workflow name, and reported immediate
failure.

## Root Cause
PR #1811 modified both workflow files, which caused GitHub Actions to create stale
internal registrations that fire on push events even though neither file defines a
push trigger.

## Fix
Added an explicit no-op push trigger to both workflows with an unmatchable path filter
(`.github/non-existent-path-do-not-create`). This replaces the stale registration with
an explicit one that always evaluates to "skip."

## Files Changed
- `.github/workflows/test-changed-recipes.yml` - added no-op push trigger
- `.github/workflows/validate-golden-recipes.yml` - added no-op push trigger

## Verification
After merge, verify ghost runs stop by checking for new push-event runs:
```bash
gh api "repos/tsukumogami/tsuku/actions/workflows/212962093/runs?per_page=3&event=push" \
  --jq '.workflow_runs[] | {conclusion, event, created_at}'
```

## Risk Assessment
Low. The no-op push trigger can never match real file changes. PR triggers and
workflow_dispatch are unchanged.
