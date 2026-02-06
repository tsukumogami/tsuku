# Issue 1507 Summary

## What Was Implemented
Created a post-merge GitHub Actions workflow that automatically generates and commits the pipeline dashboard after state files merge to main. This eliminates dashboard.json as a conflict source in batch PRs by moving generation from PR creation time to post-merge.

## Changes Made
- `.github/workflows/update-dashboard.yml`: New workflow file (53 lines)
  - Triggers on push to main when state files change (data/queues/**, data/metrics/**, data/failures/**)
  - Builds queue-analytics and regenerates dashboard
  - Commits dashboard.json with github-actions[bot] identity
  - Includes retry logic with exponential backoff for concurrent push handling

- `.github/workflows/batch-generate.yml`: Removed dashboard generation (38 lines removed)
  - Deleted "Generate dashboard" step that built and ran queue-analytics during batch workflow
  - Removed dashboard.json from git add commands in two locations (persist state step and PR commit step)

- `docs/designs/DESIGN-batch-pr-coordination.md`: Updated implementation status (9 lines changed)
  - Strikethrough issue #1507 in implementation issues table
  - Updated dependency graph: I1507 changed from ready to done class
  - Unblocked issue #1508 by marking it as ready (no longer blocked by #1507)

## Key Decisions
- **Concurrency group**: Used `update-dashboard` group with `cancel-in-progress: false` to serialize dashboard updates, preventing race conditions when multiple PRs merge quickly
- **Retry logic**: Implemented 3-attempt retry with exponential backoff (2s, 4s, 8s) and git pull --rebase to handle concurrent pushes gracefully
- **No-op handling**: Check `git diff --cached --quiet` before committing to avoid empty commits when dashboard unchanged
- **Bot identity**: Use github-actions[bot] for commits instead of PAT for better security and audit trail
- **Path filters**: Restrictive paths filter (only state files) to avoid unnecessary workflow triggers

## Trade-offs Accepted
- **Dashboard staleness window**: Dashboard shows stale data for ~60 seconds between PR merge and workflow completion. Acceptable because dashboard is not user-facing real-time data.
- **Potential workflow failure**: If post-merge workflow fails, dashboard stays stale until next PR. Mitigated by workflow design (no error suppression, fails loudly on errors) and acceptable because dashboard eventually updates with next state file change.
- **File separation**: Dashboard now updated in separate commit from state file changes. This is intentional architectural decision to eliminate conflicts, provides clearer audit trail.

## Test Coverage
- No new tests added (workflow file changes, not Go code)
- Existing tests verified: internal/dashboard and internal/batch pass
- Integration testing will happen after merge to main (see Validation section below)

## Validation After Merge
Once this PR merges to main:
1. Make a trivial change to a state file to trigger the workflow
2. Verify workflow runs via `gh run list --workflow=update-dashboard.yml`
3. Confirm dashboard updates within 60 seconds
4. Verify commit message format matches "chore(dashboard): update pipeline dashboard"
5. Test concurrent merge scenario (merge two PRs quickly, verify both trigger workflow and second rebases successfully)
6. Verify batch-generate workflow still functions correctly without dashboard generation

## Known Limitations
- Workflow requires state files to change to trigger. If someone manually wants to regenerate dashboard without state changes, they need to either:
  - Make a trivial change to a state file
  - Manually run queue-analytics locally and commit
  - Trigger workflow_dispatch if we add that capability later
- Dashboard lag of ~60 seconds is acceptable for this use case but could be problematic if dashboard needs to be real-time in the future
- Workflow uses github-actions[bot] which has elevated permissions. Future security hardening could use GitHub App with more restricted scopes.

## Context
This issue is part of Milestone 3: State File Refactoring and Branch Protection from DESIGN-batch-pr-coordination.md. It completes the final piece of Phase 2 (State File Refactoring):
- #1505: Split priority queue by ecosystem (✓ complete)
- #1506: Timestamp metrics and failures files (✓ complete)
- **#1507: Post-merge dashboard updates (✓ complete - this issue)**
- #1508: Enable branch protection (ready to proceed)

Before this change: Dashboard was the last remaining conflict source. When multiple batch PRs were open, each had a different dashboard version based on speculative PR state, causing merge conflicts.

After this change: Zero conflict sources remain for batch PRs. Dashboard always reflects actual merged state, enabling true parallel batch execution across ecosystems and unblocking #1508 (branch protection).
