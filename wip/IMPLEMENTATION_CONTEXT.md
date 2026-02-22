## Problem

Two workflows that only define `pull_request` and `workflow_dispatch` triggers are creating ghost failure runs on every `push` event:

- `test-changed-recipes.yml` (workflow ID 212962093)
- `validate-golden-recipes.yml` (workflow ID 219435193)

These ghost runs:
- Have **zero jobs** and **no logs** ("log not found")
- Show the file path as workflow name (e.g., `.github/workflows/test-changed-recipes.yml`) instead of the YAML `name:` property
- Report `conclusion: failure` immediately
- Trigger on every push to every branch (main, feature branches, bot commits)

The workflows' `pull_request` triggers work correctly -- all recent PR runs succeed with proper jobs.

## Timeline

- **2026-02-21 ~15:41 UTC**: First ghost push run appeared on `docs/recipe-ci-batching` branch when workflow files were modified (PR #1811 prep)
- **2026-02-21 ~17:06 UTC**: First ghost push run on `main` when PR #1811 was merged (commit `8e4d6f8b`)
- **All pushes after that**: Every push to any branch creates ghost failure runs for both workflows

Before Feb 21, these workflows had 521+ runs and zero push-triggered runs.

## Affected Workflows

Both workflow files define only these triggers:

```yaml
on:
  pull_request:
    branches: [main]
    paths:
      - 'internal/recipe/recipes/**/*.toml'
      - 'recipes/**/*.toml'
      # ... path filters
  workflow_dispatch:
```

No `push` trigger is defined. The push runs appear to be a GitHub Actions platform behavior triggered by modifying workflow files in commits.

## Impact

- Every push to main creates 2 extra failed workflow runs (noise in the Actions tab)
- Commit status shows these as failures, potentially confusing required status check configurations
- Three additional deleted workflows (`recipe-platform-validation.yml`, `test-dispatch-with-input.yml`, `validate-all-recipes.yml`) had the same issue and were disabled via API

## Possible Fixes

1. **Disable and re-enable** the stale workflow registrations via the API -- but since the same registration handles both ghost push runs and working PR runs, this may disrupt PR CI
2. **Add a no-op push trigger** that never matches, to satisfy GitHub's trigger resolution:
   ```yaml
   on:
     push:
       branches: [main]
       paths:
         - '.github/this-file-does-not-exist'
     pull_request:
       # ... existing config
   ```
3. **Wait for self-resolution**: Some users report this resolves after GitHub re-indexes workflow files
4. **Contact GitHub support** if the issue persists

## Verification

```bash
# Ghost push runs (all failures, zero jobs):
gh api "repos/tsukumogami/tsuku/actions/workflows/212962093/runs?per_page=5&event=push" \
  --jq '.workflow_runs[] | {conclusion, event, head_branch, created_at}'

# Working PR runs (successes with jobs):
gh api "repos/tsukumogami/tsuku/actions/workflows/212962093/runs?per_page=5&event=pull_request" \
  --jq '.workflow_runs[] | {conclusion, event, head_branch, created_at}'
```

## Notes

This is **not related** to PR #1824 or any specific code change. It's a GitHub Actions platform behavior that started when workflow files were modified in PR #1811 commits.
