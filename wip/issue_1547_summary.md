# Issue 1547 Summary

## What Was Implemented

Added GitHub Actions workflow `.github/workflows/coverage-update.yml` that automatically regenerates `website/coverage/coverage.json` when recipe files change. This ensures the coverage dashboard at tsuku.dev/coverage always reflects the current recipe catalog without manual intervention.

## Changes Made

- `.github/workflows/coverage-update.yml`: New workflow file
  - Triggers on push to main when recipes/**/*.toml changes
  - Also supports manual workflow_dispatch
  - Runs cmd/coverage-analytics to generate fresh coverage.json
  - Commits changes using github-actions bot account
  - Includes [skip ci] to prevent CI loops
  - Only commits when coverage.json actually changes (git diff check)

## Key Decisions

- **Action versions**: Used same pinned versions as other workflows (checkout@v6.0.2, setup-go@v6.2.0) for consistency
- **Bot account**: Used github-actions[bot] for commits to clearly identify automated updates
- **Skip CI**: Added [skip ci] to commit message to prevent workflow from triggering itself recursively
- **Empty commit prevention**: Added git diff check before committing to avoid no-op commits
- **Tool defaults**: Used coverage-analytics with default arguments since they match required paths
- **Trigger paths**: Included both recipes/ and internal/recipe/recipes/ to catch all recipe changes

## Trade-offs Accepted

- **Post-merge testing**: Workflow won't appear in GitHub UI until merged to main, so full validation requires merge. This is a GitHub Actions limitation - workflows must be on default branch to appear in UI.
- **Push-to-main pattern**: Workflow commits directly to main rather than creating PRs. This is appropriate for automated data regeneration but means no manual review of coverage.json changes.
- **No retry logic**: If coverage-analytics fails, workflow fails without retry. This is intentional - if the tool fails, the issue needs investigation, not automatic retries.

## Test Coverage

No new tests added - this is a CI workflow file addition. The cmd/coverage-analytics tool it invokes was already validated in #1545 and #1546.

Validation approach:
- YAML syntax manually verified (indentation, structure, field names)
- Action versions match existing workflows (consistency check)
- Tool invocation tested locally (coverage-analytics --help confirms defaults)
- git diff logic tested in other workflows (update-dashboard.yml pattern)

## Known Limitations

- Workflow only triggers on recipe TOML file changes, not on coverage-analytics tool changes. If the tool logic changes, coverage.json won't auto-regenerate until next recipe change.
- No notification if workflow fails - contributors need to check Actions tab
- Assumes GITHUB_TOKEN has sufficient permissions to push to main

## Future Improvements

- Could add Slack/Discord notification on workflow failure (blocked on notification infrastructure)
- Could trigger on coverage-analytics code changes (would need path: 'cmd/coverage-analytics/**')
- Could add workflow status badge to dashboard (requires badge generation workflow)
- Documentation in #1548 will cover how to monitor and manually trigger this workflow
