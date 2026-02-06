# Implementation Plan: Issue #1507

## Summary
Add post-merge dashboard update workflow to eliminate the final conflict source in batch PRs. This moves dashboard generation from PR creation time to post-merge, ensuring the dashboard always reflects actual merged state rather than speculative PR state.

## Approach
Create a new GitHub Actions workflow that triggers on pushes to main when state files change, then build and run queue-analytics to regenerate the dashboard and commit it back to main. Remove the corresponding dashboard generation from the batch-generate workflow's merge job.

## Files to Modify

### New Files
1. `.github/workflows/update-dashboard.yml` - New workflow file

### Modified Files
1. `.github/workflows/batch-generate.yml` - Remove dashboard generation from merge job
2. `docs/designs/DESIGN-batch-pr-coordination.md` - Update implementation status

## Implementation Steps

### Step 1: Create update-dashboard workflow
Create `.github/workflows/update-dashboard.yml` with:
- Trigger: `on: push` to main, `paths` filter for `data/queues/**`, `data/metrics/**`, `data/failures/**`
- Permissions: `contents: write`
- Concurrency: `group: update-dashboard`, `cancel-in-progress: false`
- Job steps:
  1. Checkout code
  2. Setup Go (use go-version-file: go.mod)
  3. Build queue-analytics binary
  4. Run queue-analytics (generates website/pipeline/dashboard.json)
  5. Configure git user as github-actions[bot]
  6. Stage dashboard.json
  7. Check if dashboard changed (`git diff --cached --quiet`)
  8. If changed: commit with message "chore(dashboard): update pipeline dashboard"
  9. Push with retry logic (3 attempts with exponential backoff)

Key requirements from design:
- Retry logic: `for attempt in {1..3}; do git push || (sleep $((2**attempt)); git pull --rebase); done`
- No-op handling: exit 0 if no changes to avoid empty commits
- Bot identity: `github-actions[bot]` for commits

### Step 2: Remove dashboard generation from batch-generate workflow
Find where batch-generate.yml currently generates the dashboard:
- Search for "queue-analytics" or "dashboard" in the file
- Likely in the merge job (after recipes are merged)
- Remove the steps that build and run queue-analytics
- Remove any git operations related to committing dashboard changes
- Keep any steps that reference dashboard for PR comments/status (read-only operations)

Expected location: Lines related to dashboard update in the merge job that runs after PR merge.

### Step 3: Update design doc
Mark issue #1507 as complete in the implementation issues table:
- Strikethrough the issue row
- Update dependency graph: change I1507 from `ready` class to `done` class
- Update I1508 from `blocked` to `ready` (no longer blocked by #1507)

## Testing Strategy

### Workflow validation
```bash
# Validate workflow YAML syntax
yamllint .github/workflows/update-dashboard.yml

# Check workflow can be parsed by GitHub Actions
gh workflow list | grep "Update Dashboard"
```

### Integration testing (after merge to main)
1. Make a trivial change to a state file
2. Commit and push to main
3. Verify workflow triggers (check gh run list)
4. Verify dashboard is updated within 60 seconds
5. Verify commit message matches expected format

### Regression testing
1. Verify batch-generate workflow still works after removing dashboard steps
2. Check that batch PRs no longer include dashboard.json changes
3. Confirm dashboard updates only happen post-merge

## Risks and Mitigations

### Risk: Workflow triggers on non-state file changes
Mitigation: paths filter is restrictive (only data/queues/**, data/metrics/**, data/failures/**)

### Risk: Concurrent pushes cause failures
Mitigation: Retry logic with exponential backoff (2s, 4s, 8s) and git pull --rebase

### Risk: Dashboard generation fails silently
Mitigation: Workflow should fail loudly if queue-analytics returns non-zero exit code. No error suppression.

### Risk: Workflow creates empty commits
Mitigation: Check `git diff --cached --quiet` before committing, exit 0 if no changes

### Risk: Breaking batch-generate by removing wrong steps
Mitigation: Carefully identify dashboard-related steps, test batch workflow still functions

## Success Criteria
- [ ] update-dashboard.yml workflow file created with all required elements
- [ ] Dashboard generation removed from batch-generate.yml
- [ ] Design doc updated to reflect completion
- [ ] Workflow YAML is valid (yamllint passes)
- [ ] No syntax errors in workflow file
- [ ] All acceptance criteria from issue #1507 met

## Rollback Plan
If issues are discovered after merge:
1. Revert the commit that added update-dashboard.yml
2. Revert the commit that removed dashboard from batch-generate.yml
3. Dashboard generation returns to in-PR behavior
4. No data loss (git history preserves all dashboard versions)

## Notes
- This is the final piece of Phase 2 (State File Refactoring) from the design
- After this is deployed and stable, #1508 (branch protection) can proceed
- Dashboard will show a ~60 second lag between PR merge and update (acceptable trade-off)
- The workflow uses github-actions[bot] identity, not a PAT, for better security
