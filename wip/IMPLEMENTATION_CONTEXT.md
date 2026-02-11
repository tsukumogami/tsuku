## Goal

Validate that the complete batch PR coordination system from `docs/designs/DESIGN-batch-pr-coordination.md` is fully implemented and working end-to-end. This issue serves as the validation gate - it can only be closed when all 12 issues from the design are deployed AND the batch workflow successfully creates conflict-free PRs.

## Context

This is the final validation issue for the batch PR coordination improvements documented in `docs/designs/DESIGN-batch-pr-coordination.md`. The design addresses race conditions and conflicts in batch recipe generation by:

1. Splitting shared state files into per-ecosystem files (eliminates cross-ecosystem conflicts)
2. Adding preflight circuit breaker checks (prevents cascading failures)
3. Serializing operations per ecosystem via concurrency groups
4. Moving dashboard.json generation to post-merge workflow (removes non-deterministic file from PRs)
5. Enabling branch protection with auto-merge (enforces validation, enables automation)

**Why this is the validation gate:**

Branch protection can only be enabled after all infrastructure changes are deployed. Once enabled, it locks in the improvements and provides the enforcement layer. However, **branch protection alone doesn't validate the system works** - we must verify the batch workflow successfully creates PRs without conflicts.

**Recent discoveries:**

When testing the deployed system (post PR #1528 merge), we discovered the batch workflow has additional bugs preventing end-to-end validation:

1. **Artifact upload failure**: Failure log filename uses ISO 8601 timestamp format (`homebrew-2026-02-06T23:03:18Z.jsonl`) which contains colons. GitHub Actions artifacts don't allow colons in filenames, causing upload step to fail.

2. **jq processing errors**: The "Requeue unblocked packages" step fails with `Cannot iterate over null` errors when reading `/data/failures/homebrew.jsonl`, indicating malformed JSONL entries.

These issues were not caused by the design implementation - they're pre-existing bugs that only surfaced during end-to-end testing. They must be fixed for this issue to close.

## Acceptance Criteria

### Infrastructure Deployed (all 12 issues from design doc)

- [x] #1498 - State file refactoring (per-ecosystem files)
- [x] #1499 - Preflight circuit breaker checks
- [x] #1500 - Pin GitHub Actions to commit SHAs
- [x] #1501 - Concurrency groups per ecosystem
- [x] #1502 - Post-merge dashboard workflow
- [x] #1503 - Update batch workflow to use new state files
- [x] #1504 - Update seed-queue to use new state files
- [x] #1505 - Update batch-operations to use new state files
- [x] #1506 - Migrate data from legacy state.json
- [x] #1507 - Remove dashboard.json from batch PR creation
- [ ] #1508 - This issue (validation gate)

### Branch Protection Configuration

- [x] Branch protection enabled on `main` branch
- [x] Required status checks configured:
  - [x] `Check Artifacts` (was `check-artifacts` - fixed in PR #1528)
  - [x] `Lint Workflows` (was `lint-workflows` - fixed in PR #1528)
  - [x] `validate` (correct)
- [x] "Require branches to be up to date before merging" enabled (`strict: true`)
- [x] Other settings:
  - [x] `enforce_admins: false` (allow admin bypass for emergencies)
  - [x] `allow_force_pushes: false` (prevent force pushes)
  - [x] `allow_deletions: false` (prevent branch deletion)

### Workflow Bugs Fixed

- [x] Fixed invalid SHA for `actions/create-github-app-token` (PR #1528)
- [x] Fixed Dependabot auto-merge to use correct secrets (PR #1530)
- [ ] Fix artifact upload failure: Replace colons in failure log filenames
- [ ] Fix jq processing errors in requeue step

### End-to-End Validation

- [ ] Manually trigger batch workflow for homebrew ecosystem
- [ ] Verify workflow completes successfully (no artifact upload failures)
- [ ] Verify batch PR is created with label `batch:homebrew`
- [ ] Verify batch PR has zero conflicts (validates state file refactoring)
- [ ] Verify batch PR passes all required checks
- [ ] Verify auto-merge works (PR merges automatically after checks pass)
- [ ] Verify post-merge dashboard workflow triggers and updates `dashboard.json`

### Workflow Write Permissions Validated

- [x] Confirmed `seed-queue.yml` can commit to main (not tested but structure allows)
- [x] Confirmed `batch-operations.yml` can commit to main (not tested but structure allows)
- [x] Confirmed `update-dashboard.yml` can commit to main (verified in PR #1528 testing)

## Discovered Issues

### Issue 1: Artifact Filename Contains Colons

**Location**: `.github/workflows/batch-generate.yml` - "Upload passing recipes" step

**Symptom**:
```
##[error]The path for one of the files in artifact is not valid: /data/failures/homebrew-2026-02-06T23:03:18Z.jsonl. Contains the following character:  Colon :
```

**Root cause**: Failure log filename uses ISO 8601 timestamp format with colons. GitHub Actions artifacts don't allow the following characters in filenames: `" : < > | * ? \r \n`

**Fix needed**: Replace colons in timestamp with hyphens or underscores:
- Current: `homebrew-2026-02-06T23:03:18Z.jsonl`
- Proposed: `homebrew-2026-02-06T23-03-18Z.jsonl`

### Issue 2: jq Cannot Process Null Entries in Failure Log

**Location**: `.github/workflows/batch-generate.yml` - "Requeue unblocked packages" step

**Symptom**:
```
jq: error (at /home/runner/work/tsuku/tsuku/data/failures/homebrew.jsonl:2): Cannot iterate over null (null)
```

**Root cause**: The `data/failures/homebrew.jsonl` file contains null entries (empty lines or malformed JSON). The jq command expects valid JSONL where every line is a valid JSON object.

**Fix needed**: 
1. Filter out null/empty lines before processing: `grep -v '^null$' | jq ...`
2. Or investigate why null entries are written to failure logs

## Validation Script

Once bugs are fixed, run this to validate the complete system:

```bash
# 1. Trigger batch workflow
gh workflow run batch-generate.yml -f ecosystem=homebrew -f batch_size=5 -f tier=3

# 2. Wait for workflow to complete (should succeed)
WORKFLOW_RUN=$(gh run list --workflow=batch-generate.yml --limit 1 --json databaseId --jq '.[0].databaseId')
gh run watch $WORKFLOW_RUN

# 3. Verify PR was created
gh pr list --label "batch:homebrew" --limit 1

# 4. Verify PR has no conflicts
PR_NUM=$(gh pr list --label "batch:homebrew" --limit 1 --json number --jq '.[0].number')
gh pr view $PR_NUM --json mergeable --jq '.mergeable'
# Expected: "MERGEABLE"

# 5. Verify required checks run and pass
gh pr checks $PR_NUM --watch

# 6. Test auto-merge
gh pr merge $PR_NUM --auto --squash

# 7. Wait for PR to auto-merge
gh pr view $PR_NUM --json state --jq '.state'
# Expected: "MERGED"

# 8. Verify post-merge dashboard workflow ran
gh run list --workflow=update-dashboard.yml --limit 1 --json conclusion --jq '.[0].conclusion'
# Expected: "success"
```

## Dependencies

**Blocked by:**
- #1507 - Post-merge dashboard workflow (deployed in PR #1487) ✅

**Blocks:**
- Design doc transition to "Current" status
- Closing milestone for batch PR coordination

## Design Reference

See `docs/designs/DESIGN-batch-pr-coordination.md` for complete architecture and rationale.
