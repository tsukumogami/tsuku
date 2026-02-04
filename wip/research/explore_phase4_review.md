# Design Review: Batch PR CI Validation

**Design Document**: `docs/designs/DESIGN-batch-pr-ci-validation.md`
**Review Date**: 2026-02-04
**Review Phase**: Problem Statement and Options Analysis

---

## 1. Problem Statement Evaluation

### Clarity and Specificity

**Rating**: Good - adequately specific for solution evaluation

The problem statement identifies four root causes:
1. GitHub auto-merge only waits for *required* status checks
2. Making recipe-specific checks required would block unrelated PRs
3. Fallback logic merged immediately without waiting
4. Golden file validation fails for new recipes (chicken-and-egg)

**Strengths**:
- Cites specific evidence (PRs #1457, #1454, #1453, #1452)
- Names specific failing checks (`Validate: swiftformat`, `opentofu`, `terragrunt`)
- Clearly scopes what's in vs out of scope

**Gaps identified**:
- Does not quantify the problem severity (how many PRs affected? What percentage?)
- Does not specify *which* checks are failing - are they golden file checks or other types?
- The relationship between "failing checks" and "golden file validation" is unclear. Are all the cited failures due to golden files, or are there other failure modes?

### Testable Success Criteria

**Issue**: The problem statement lacks explicit success criteria.

Suggested additions:
- Batch PRs with all CI checks passing should auto-merge
- Batch PRs with failing CI checks should not merge automatically
- New recipe PRs should not fail golden file validation (expected path)
- Modified recipe PRs should still validate golden files (regression detection)

---

## 2. Missing Alternatives Analysis

### Decision 1: How should batch workflow wait for CI?

**Potentially missing alternatives**:

1. **GitHub Actions Workflow Conclusion Events**
   - Use `workflow_run` triggers to coordinate between workflows
   - Wait for workflow completion events rather than polling checks
   - Rejection rationale needed: complexity, timing issues, or why `gh pr checks` is simpler

2. **Branch Protection with Path-Conditional Required Checks**
   - Some CI systems support "required when run" via status checks that report as skipped/neutral when not triggered
   - GitHub may have evolved - worth confirming this is still unavailable

3. **Status Check Aggregator Action**
   - Third-party actions exist that aggregate check status (e.g., Autotask status checks)
   - Could provide a single required check that gates on all dynamic checks
   - Rejection rationale needed: dependency risk, maintenance burden, or feature gaps

4. **Required Check + Skip Logic**
   - Make all recipe checks required but have them skip (report success) when not triggered
   - Uses `jobs.<job_id>.if` conditions with early success exit
   - Rejection rationale: every workflow must be modified, coordination complexity

**Assessment**: The chosen solution (`gh pr checks --watch`) is pragmatic and simple. However, the alternatives section could be more complete by explicitly addressing workflow coordination options.

### Decision 2: How should golden file validation handle new recipes?

**Potentially missing alternatives**:

1. **Label-Based Gating**
   - Apply a `new-recipe` label to batch PRs
   - Golden validation checks for this label and skips
   - Simpler than R2 existence checks, but requires batch workflow to apply label

2. **Allow-List in PR Description**
   - Batch workflow includes list of new recipes in PR body
   - Golden validation parses PR description to determine which recipes are new
   - No R2 dependency, but couples validation to PR metadata

3. **Git-Based New Recipe Detection**
   - Compare against base branch to determine if recipe file is new vs modified
   - If file doesn't exist in base branch, skip golden validation
   - Works entirely within git, no external dependencies

4. **Two-Phase Commit**
   - First commit adds recipe without triggering golden validation (e.g., to `recipes-staging/`)
   - Post-merge workflow generates golden files
   - Second commit/PR moves recipe to final location
   - Rejected because: complexity, doubles PR count

**Assessment**: The R2 existence check is reasonable, but the git-based detection (option 3) would be simpler and avoid network dependencies. The design should explain why R2 is preferred over git-based detection.

---

## 3. Rejection Rationale Evaluation

### Decision 1 Alternatives

| Alternative | Rejection Rationale | Fair? |
|-------------|---------------------|-------|
| Rollup job across workflows | "GitHub Actions doesn't support cross-workflow job dependencies. Would require complex polling/webhook coordination." | **Fair** - accurate technical limitation |
| Explicit check list | "Maintenance burden - easy to forget adding new checks. Also fragile if check names change." | **Fair** - valid operational concern |

**Assessment**: Rejection rationales are specific and fair. Not strawmen.

### Decision 2 Alternatives

| Alternative | Rejection Rationale | Fair? |
|-------------|---------------------|-------|
| Pre-generate golden files in batch workflow | "Requires R2 write credentials in batch workflow, adds complexity, and could upload incorrect golden files if recipe has bugs." | **Partially fair** - the "incorrect golden files" concern applies to post-merge generation too. The credential concern is valid. |
| Advisory mode | "Doesn't actually solve the problem - we still need to decide whether to merge, and 'advisory' status is confusing." | **Fair** - advisory checks don't provide clear signals |
| Defer validation to post-merge | "Loses early feedback. Modified recipes (regressions) wouldn't be caught until after merge." | **Fair** - correctly identifies the regression detection gap |

**Assessment**: Rejections are reasonable, but the "pre-generate" alternative's bug concern could be addressed (batch workflow already validates recipes on multiple platforms before creating PR).

---

## 4. Unstated Assumptions

The following assumptions are implicit and should be made explicit:

### Technical Assumptions

1. **`gh pr checks --watch` behavior**: Assumes this command waits for ALL checks, including those from external workflows triggered by the PR. Need to verify behavior with dynamically-triggered workflows.

2. **R2 availability**: Assumes R2 is generally available. The design mentions conservative failure mode, but doesn't specify expected availability SLA.

3. **Check naming stability**: Assumes check names remain stable enough that `gh pr checks` can identify them. What if checks are renamed?

4. **Timing of check registration**: Assumes all checks register their pending status before `gh pr checks --watch` starts polling. If a check registers late, it might be missed.

### Process Assumptions

1. **Batch workflow has exclusive merge rights**: The design assumes batch PRs are merged by the workflow, not humans. What if a maintainer manually merges?

2. **Single batch PR at a time**: The concurrency group prevents parallel batch runs, but doesn't prevent overlap with manually-created recipe PRs.

3. **Golden file generation reliability**: Assumes post-merge golden generation succeeds. What if it fails? The new recipe would be in the registry without golden files, and future modifications would fail validation.

### Scope Assumptions

1. **Only batch PRs need this fix**: The design explicitly scopes to batch PRs, but the same problem could affect manually-created recipe PRs. Is that acceptable?

---

## 5. Strawman Analysis

**Verdict**: No obvious strawmen detected.

Each rejected alternative:
- Addresses a real aspect of the problem
- Has a plausible implementation path
- Is rejected for specific, technical reasons

The chosen solutions are not artificially compared to obviously inferior alternatives. The alternatives considered represent reasonable engineering approaches that have genuine tradeoffs.

---

## 6. Implementation Gap Analysis

Comparing the design to the actual implementation:

### Current Implementation (`batch-generate.yml`)

```yaml
- name: Enable auto-merge or post review notice
  run: |
    if [ "$EXCLUDED_COUNT" -eq 0 ]; then
      if ! gh pr merge "$PR_NUMBER" --auto "${MERGE_ARGS[@]}" 2>&1; then
        echo "Auto-merge unavailable - waiting for CI checks to complete"
        if gh pr checks "$PR_NUMBER" --watch --fail-fast; then
          echo "All CI checks passed - merging"
          gh pr merge "$PR_NUMBER" "${MERGE_ARGS[@]}"
        else
          echo "::error::CI checks failed - not merging automatically"
          gh pr comment "$PR_NUMBER" --body "..."
          exit 1
        fi
      fi
    fi
```

**Observation**: The implementation already has the `gh pr checks --watch --fail-fast` fallback. The design may be documenting/formalizing existing behavior rather than proposing changes.

### Current Implementation (`validate-golden-recipes.yml`)

```yaml
# Registry recipes use R2 when available
export TSUKU_GOLDEN_SOURCE=r2

# R2 unavailable - skip registry recipe validation with warning
echo "::warning::Skipping $RECIPE validation - R2 unavailable (registry recipes require R2 golden files)"
exit 0
```

**Observation**: The implementation already skips validation when R2 is unavailable for registry recipes. However, it doesn't distinguish between "R2 unavailable" and "golden files don't exist in R2 for this new recipe."

### Gap Identified

The design proposes:
```bash
if ! golden_files_exist_in_r2 "$RECIPE"; then
  echo "::notice::New recipe '$RECIPE' - skipping golden validation"
  exit 0
fi
```

But the current implementation conflates R2 availability with golden file existence. The design's solution is more precise.

---

## 7. Design Consistency Check

### Internal Consistency

| Section | Claim | Consistent? |
|---------|-------|-------------|
| Problem Statement | "batch workflow's fallback merged immediately without waiting" | **Needs verification** - current implementation has wait logic |
| Decision 1 | "Skip auto-merge entirely" | **Inconsistent** - design says skip auto-merge, but implementation tries auto-merge first |
| Decision 2 | "Skip validation when golden files don't exist in R2" | **Consistent** - aligns with proposed script |

### Cross-Section Consistency

- The problem statement mentions 4 root causes, but only 2 are addressed by the decisions
- Root cause 1 (auto-merge waits for required checks only) is addressed by Decision 1
- Root cause 2 (can't make checks required) is addressed by Decision 1 (doesn't use required checks)
- Root cause 3 (fallback merged immediately) is addressed by Decision 1
- Root cause 4 (golden validation fails for new recipes) is addressed by Decision 2

All root causes are addressed.

---

## 8. Recommendations

### High Priority

1. **Verify current implementation state**: The design may be describing the existing implementation. Clarify whether this is a new proposal or documentation of current behavior.

2. **Add git-based new recipe detection as considered alternative**: This would avoid R2 dependency for determining if a recipe is new.

3. **Specify `gh pr checks --watch` behavior for late-registering checks**: Add uncertainty section or test plan to verify this edge case.

### Medium Priority

4. **Add success criteria**: Define measurable outcomes (e.g., "zero batch PRs merged with failing checks over 30 days").

5. **Address post-merge golden generation failure**: What happens if the publish-golden-to-r2 workflow fails? The recipe is now in the registry without golden files.

6. **Clarify scope on non-batch PRs**: Explicitly state whether manually-created recipe PRs have the same protections.

### Low Priority

7. **Consider workflow conclusion events**: Document why `workflow_run` coordination wasn't considered, for completeness.

8. **Add timing diagram**: Show the sequence of events from PR creation through merge, including all check registrations.

---

## Summary

The design is generally sound with clear problem articulation and reasonable solutions. The main gaps are:

1. **Implementation alignment uncertainty** - the current implementation appears to already have some proposed features
2. **Missing alternative** - git-based new recipe detection would be simpler than R2 existence checks
3. **Unstated assumptions** - particularly around `gh pr checks` timing behavior with dynamically-triggered workflows
4. **No explicit success criteria** - makes it hard to verify the design achieved its goals

The alternatives are not strawmen - they represent genuine engineering options with real tradeoffs. The rejection rationales are specific and fair.
