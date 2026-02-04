---
status: Accepted
problem: |
  The batch-generate workflow merges PRs before all CI checks complete, or with failing checks.
  GitHub auto-merge only waits for required checks, but making recipe-specific checks required
  would block unrelated PRs. Additionally, golden file validation fails for new recipes because
  the golden files don't exist in R2 yet (chicken-and-egg problem).
decision: |
  Replace auto-merge with explicit `gh pr checks --watch --fail-fast` to wait for all checks
  before merging. Modify golden file validation to skip with success when golden files don't
  exist in R2 (new recipes), while still catching regressions in modified recipes.
rationale: |
  Waiting for all checks is simpler than cross-workflow coordination and ensures nothing merges
  with failures. Skipping validation for new recipes solves the chicken-and-egg problem without
  compromising regression detection for existing recipes.
---

# Design: Batch PR CI Validation

**Status**: Proposed

## Context and Problem Statement

The batch-generate workflow creates PRs that add new recipes to the registry. These PRs trigger multiple CI workflows, but the batch workflow merges PRs before all checks complete - or even when checks fail.

The root cause is a combination of factors:
1. GitHub auto-merge only waits for *required* status checks
2. Making recipe-specific checks required would block unrelated PRs that don't trigger those workflows
3. The batch workflow's fallback (when auto-merge fails) merged immediately without waiting
4. Golden file validation fails for new recipes because golden files don't exist in R2 yet

Recent batch PRs (#1457, #1454, #1453, #1452) were merged with failing checks, including `Validate: swiftformat`, `Validate: opentofu`, and `Validate: terragrunt`.

### Scope

**In scope:**
- Ensuring batch PRs wait for all relevant CI checks before merging
- Handling golden file validation for new recipes (chicken-and-egg problem)

**Out of scope:**
- Refactoring all 41 workflows into a single workflow
- Changing the batch generation process itself
- Branch protection for non-batch PRs
- Manually-created recipe PRs (they follow normal PR workflow)

### Success Criteria

- Zero batch PRs merged with failing CI checks
- Golden file validation passes (or skips with notice) for new recipes
- Golden file validation still catches regressions in modified recipes

## Decision Drivers

- Batch PRs must not merge with failing checks
- Non-batch PRs must not be blocked by recipe-specific required checks
- Golden file validation is expected to "fail" for new recipes (files don't exist in R2 yet)
- Solution must work within GitHub's branch protection model
- Avoid complex cross-workflow coordination

## Implementation Context

### GitHub Limitations

According to [GitHub community discussions](https://github.com/orgs/community/discussions/26092), there is no native "required if run" feature. Status checks are either always required or never required. The recommended workaround is the "rollup job" pattern - a job that runs `always()` and checks dependency results.

However, this pattern only works within a single workflow. With 41 separate workflow files, a cross-workflow rollup would require significant refactoring.

### Current Architecture

The batch-generate workflow:
1. Creates a PR with new recipes
2. Tries `gh pr merge --auto` (requires branch protection with required checks)
3. Falls back to direct merge if auto-merge fails

The `Validate Golden Files (Recipes)` workflow:
- Triggers on recipe file changes
- Fetches golden files from R2 and compares
- Fails if golden files don't exist (new recipes) or don't match (modified recipes)
- Error message acknowledges this: "Golden files in R2 don't match. The post-merge workflow will regenerate them after merge."

## Considered Options

### Decision 1: How should batch workflow wait for CI?

The batch workflow needs to ensure all CI checks pass before merging. The challenge is that GitHub auto-merge only waits for *required* checks, and we can't make all checks required without blocking unrelated PRs.

#### Chosen: Wait for all checks with `gh pr checks --watch`

Skip auto-merge entirely. Use `gh pr checks --watch --fail-fast` which waits for ALL checks from ALL workflows to complete, then only merge if all pass.

```bash
# Wait for all checks to complete
if gh pr checks "$PR_NUMBER" --watch --fail-fast; then
  gh pr merge "$PR_NUMBER" --squash ...
else
  echo "::error::CI checks failed"
  exit 1
fi
```

This is simple, requires no cross-workflow coordination, and ensures nothing is merged with failing checks.

#### Alternatives Considered

**Rollup job across workflows**: Create a meta-workflow that waits for all other workflows and reports a single status.
Rejected because GitHub Actions doesn't support cross-workflow job dependencies. Would require complex polling/webhook coordination.

**Explicit check list**: Batch workflow maintains a list of specific check names to wait for.
Rejected because it's a maintenance burden - easy to forget adding new checks. Also fragile if check names change.

### Decision 2: How should golden file validation handle new recipes?

Golden file validation currently fails for new recipes because golden files don't exist in R2 yet. This creates a chicken-and-egg problem: files can't exist until after merge, but we don't want to merge with failures.

#### Chosen: Skip validation when golden files don't exist in R2

Modify the validation script to detect whether golden files exist in R2. If they don't exist (new recipe), skip validation with success. If they exist but don't match (modified recipe), fail as before.

```bash
# In validate-golden.sh or the workflow
if ! golden_files_exist_in_r2 "$RECIPE"; then
  echo "::notice::Skipping validation for new recipe '$RECIPE' - golden files will be generated after merge"
  exit 0  # Success - not a failure
fi

# Existing validation logic for modified recipes
```

This distinguishes between:
- **New recipe**: No golden files in R2 → skip validation (expected)
- **Modified recipe**: Golden files exist but don't match → fail validation (regression)

#### Alternatives Considered

**Git-based new recipe detection**: Compare recipe files against the base branch to determine if a recipe is new (file doesn't exist in base) vs modified (file exists in base).
Rejected because it adds git operations to the validation workflow and may have edge cases with renames or moves. R2 existence check is more direct - it answers "do we have golden files to compare against" rather than inferring from git history.

**Pre-generate golden files in batch workflow**: Have batch workflow generate and upload golden files to R2 before creating the PR.
Rejected because it requires R2 write credentials in the batch workflow, adds complexity, and could upload incorrect golden files if the recipe has bugs.

**Advisory mode**: Run validation but don't fail the check, just warn.
Rejected because it doesn't actually solve the problem - we still need to decide whether to merge, and "advisory" status is confusing.

**Defer validation to post-merge**: Only run golden file validation after merge or in nightly runs.
Rejected because it loses early feedback. Modified recipes (regressions) wouldn't be caught until after merge.

### Uncertainties

- The R2 existence check needs to handle network errors gracefully (fail open vs fail closed)
- Unknown whether golden file generation occasionally fails for valid recipes (false negatives)
- If post-merge golden generation fails, the recipe will remain without golden files until manually regenerated (existing behavior, not addressed by this design)
- `gh pr checks --watch` timing: assumes all checks register their pending status before the watch command starts polling

## Decision Outcome

**Decision 1**: Use `gh pr checks --watch --fail-fast` to wait for all CI checks before merging. This is simpler than cross-workflow coordination and ensures nothing merges with failures.

**Decision 2**: Skip golden file validation when golden files don't exist in R2 (new recipes). This solves the chicken-and-egg problem while still catching regressions in modified recipes.

## Solution Architecture

### Component Changes

**1. batch-generate.yml**: Replace auto-merge with explicit check waiting

```yaml
- name: Wait for CI and merge
  run: |
    PR_NUMBER=$(echo "$PR_URL" | grep -o '[0-9]*$')
    echo "Waiting for all CI checks to complete..."

    if gh pr checks "$PR_NUMBER" --watch --fail-fast; then
      echo "All checks passed - merging"
      gh pr merge "$PR_NUMBER" --squash \
        --subject "feat(recipes): add batch ${{ env.ECOSYSTEM }} recipes" \
        --body "..."
    else
      echo "::error::CI checks failed - not merging"
      gh pr comment "$PR_NUMBER" --body "Batch auto-merge skipped: CI checks failed. Please review and merge manually."
      exit 1
    fi
```

**2. validate-golden-recipes.yml**: Add existence check before validation

In the validation step, check if golden files exist in R2 before attempting comparison:

```yaml
- name: Validate golden files
  run: |
    RECIPE="${{ matrix.item.recipe }}"

    # Check if golden files exist in R2
    if ! ./scripts/check-golden-exists.sh "$RECIPE"; then
      echo "::notice::New recipe '$RECIPE' - skipping golden validation (files will be generated after merge)"
      exit 0
    fi

    # Existing validation for modified recipes
    ./scripts/validate-golden.sh "$RECIPE" --category "$CATEGORY"
```

**3. New script: scripts/check-golden-exists.sh**

```bash
#!/bin/bash
# Check if golden files exist in R2 for a recipe
RECIPE="$1"
GOLDEN_PATH="golden/recipes/${RECIPE}/"

# Use HEAD request to check existence without downloading
if curl -s -I "${R2_BUCKET_URL}/${GOLDEN_PATH}" | grep -q "200 OK"; then
  exit 0  # Files exist
else
  exit 1  # Files don't exist
fi
```

### Data Flow

```
Batch PR Created
       │
       ▼
┌─────────────────────────────────────┐
│  Multiple CI Workflows Trigger      │
│  - Tests                            │
│  - Validate Golden Files (Recipes)  │
│  - Platform Integration Tests       │
│  - etc.                             │
└─────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────┐
│  Golden Validation Checks R2        │
│  - New recipe? Skip (success)       │
│  - Modified recipe? Validate        │
└─────────────────────────────────────┘
       │
       ▼
┌─────────────────────────────────────┐
│  Batch Workflow Waits               │
│  gh pr checks --watch --fail-fast   │
└─────────────────────────────────────┘
       │
       ▼
   All pass? ──No──► Comment + Exit
       │
      Yes
       │
       ▼
   Merge PR
       │
       ▼
┌─────────────────────────────────────┐
│  Post-merge Workflow                │
│  Generates golden files for new     │
│  recipes and uploads to R2          │
└─────────────────────────────────────┘
```

## Implementation Approach

### Phase 1: Fix batch-generate workflow (this PR)

1. Replace auto-merge logic with `gh pr checks --watch --fail-fast`
2. Remove the direct merge fallback that bypassed CI

### Phase 2: Update golden file validation

1. Add R2 existence check to `validate-golden-recipes.yml`
2. Create `scripts/check-golden-exists.sh` helper script
3. Update validation step to skip with notice for new recipes

### Testing

- Verify batch workflow waits for all checks before merge attempt
- Verify golden validation skips new recipes (no golden files in R2)
- Verify golden validation still fails for modified recipes with mismatched files

## Security Considerations

### R2 Access

The golden existence check only requires read access to R2 (HEAD request). No new write permissions are needed. The batch workflow already has appropriate permissions.

### Check Bypass

By skipping validation for new recipes, we trust that the batch workflow's own validation (which runs the recipe on multiple platforms) is sufficient. This is acceptable because:
1. Batch workflow already validates recipes work on Linux and macOS
2. Golden file validation is primarily for detecting regressions in *existing* recipes
3. Post-merge workflow generates golden files, enabling future regression detection

### Failure Modes

If R2 is unavailable during the existence check:
- Conservative approach: Fail the check (don't skip)
- This prevents false positives but may block valid PRs during R2 outages
- The workflow already has R2 health checks that can be extended

### Other Security Dimensions

**Download verification / Execution isolation / Supply chain**: Not applicable - this design doesn't change how recipes are downloaded or executed. It only changes *when* the batch workflow merges PRs.

**User data exposure**: Not applicable - no user data is involved in the CI merge workflow.

## Consequences

### Positive

- Batch PRs will no longer merge with failing checks
- New recipes won't cause spurious validation failures
- No changes needed to branch protection rules
- Minimal workflow changes required

### Negative

- Batch workflow will take longer (waits for all checks instead of just required)
- R2 outages could block batch PR merges (conservative failure mode)

### Neutral

- Branch protection remains simple (Lint Workflows + Check Artifacts)
- Non-batch PRs unaffected by these changes
