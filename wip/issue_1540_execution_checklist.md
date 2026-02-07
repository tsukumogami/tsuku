# Issue #1540 Execution Checklist

**Prerequisite**: PR #1529 must be merged to main before this issue can be completed.

## Phase 1: Initial Validation Run (Review Mode)

### Steps to Execute

1. [ ] Verify PR #1529 is merged to main
2. [ ] Navigate to https://github.com/tsukumogami/tsuku/actions/workflows/validate-all-recipes.yml
3. [ ] Click "Run workflow" button
4. [ ] Select parameters:
   - Branch: `main`
   - `auto_constrain`: `false`
5. [ ] Click "Run workflow" to start
6. [ ] Wait for workflow completion (estimated: 30-60 minutes)

### Analysis Tasks

7. [ ] Review workflow summary showing pass/fail by platform
8. [ ] Document failure patterns:
   - [ ] Count recipes failing on Alpine (musl)
   - [ ] Count recipes failing on specific architectures
   - [ ] Count recipes failing on macOS only
   - [ ] Identify any surprising failures (recipes expected to work)
9. [ ] Post findings as comment on #1540 with format:
   ```
   ## Phase 1 Results

   Workflow run: [link to run]

   **Summary**:
   - Total recipes tested: X
   - Recipes passing all platforms: Y
   - Recipes with failures: Z

   **Failure breakdown**:
   - Alpine/musl failures: N recipes
   - arm64-specific failures: N recipes
   - macOS-specific failures: N recipes
   - Other patterns: [describe]

   **Notable failures**:
   - [recipe-name]: fails on [platforms] - [brief reason if obvious]
   ```

## Phase 2: Auto-Constraint PR Generation

### Steps to Execute

10. [ ] Navigate to same workflow URL
11. [ ] Click "Run workflow" button
12. [ ] Select parameters:
    - Branch: `main`
    - `auto_constrain`: `true`
13. [ ] Click "Run workflow" to start
14. [ ] Wait for workflow completion (estimated: 30-60 minutes)

### Verification Tasks

15. [ ] Verify PR was created successfully
16. [ ] Check PR contains only recipe TOML file changes
17. [ ] Spot-check 3-5 constrained recipes to ensure constraints match Phase 1 failures
18. [ ] Verify PR description explains what was constrained
19. [ ] Post comment on #1540 with format:
    ```
    ## Phase 2 Results

    Workflow run: [link to run]
    Auto-generated PR: [link to PR]

    **PR Summary**:
    - Recipes modified: X
    - Platforms constrained: [list unique patterns]

    **Spot-check verification**:
    - [recipe-1]: constrained to [platforms] ✓ matches Phase 1
    - [recipe-2]: constrained to [platforms] ✓ matches Phase 1
    - [recipe-3]: constrained to [platforms] ✓ matches Phase 1

    Ready for review in #1543.
    ```

20. [ ] Link the PR in #1540's issue description
21. [ ] Leave PR open (do not merge - merging happens in #1543)

## Success Criteria

- [ ] Phase 1 workflow completed successfully
- [ ] Phase 1 findings documented in issue comment
- [ ] Phase 2 workflow completed successfully
- [ ] Auto-generated PR exists and is linked in #1540
- [ ] PR validation confirms constraints match failures
- [ ] Issue #1543 can proceed with PR review

## Timeline

- **Phase 1**: 30-60 minutes (workflow execution) + 15 minutes (analysis)
- **Phase 2**: 30-60 minutes (workflow execution) + 10 minutes (verification)
- **Total**: ~2 hours active time

## Troubleshooting

If Phase 1 fails:
- Check individual job logs for errors
- Common causes: network issues, GitHub Actions capacity
- Retry the workflow run

If Phase 2 doesn't create PR:
- Check workflow logs for error in PR creation step
- Verify no existing PR from previous auto-constraint run
- Check repository permissions allow PR creation
- Manual fallback: run `scripts/write-platform-constraints.sh` locally

If constraints seem wrong:
- Do NOT merge the PR
- Investigate specific recipes with unexpected constraints
- May need to fix validation logic or script
- Report findings in #1540 for investigation
