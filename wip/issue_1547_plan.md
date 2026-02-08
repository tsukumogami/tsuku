# Implementation Plan: Issue #1547

## Summary

Add GitHub Actions workflow `.github/workflows/coverage-update.yml` that automatically regenerates `website/coverage/coverage.json` when recipe files change.

## Files to Create/Modify

### New Files
- `.github/workflows/coverage-update.yml` - Workflow file for automatic coverage.json regeneration

### Modified Files
None (only adding a new workflow file)

## Implementation Approach

### Workflow Structure

Create a GitHub Actions workflow with:
1. **Triggers**:
   - `push` to `main` branch when `recipes/**/*.toml` changes
   - `workflow_dispatch` for manual regeneration

2. **Steps**:
   - Checkout code
   - Setup Go environment (using go.mod for version)
   - Run `go run cmd/coverage-analytics/main.go website/coverage/coverage.json`
   - Check if coverage.json changed (git diff)
   - Commit and push if changed
   - Use github-actions bot account

3. **Commit convention**:
   - Message: `chore(website): regenerate coverage.json [skip ci]`
   - Committer: `github-actions[bot] <github-actions[bot]@users.noreply.github.com>`
   - `[skip ci]` prevents CI loop

### Action Versions

Use same versions as other workflows in the repo:
- `actions/checkout@de0fac2e4500dabe0009e67214ff5f5447ce83dd` (v6.0.2)
- `actions/setup-go@7a3fe6cf4cb3a834922a1244abfce67bcef6a0c5` (v6.2.0)

### Edge Cases Handled

1. **No changes**: Check `git diff` before committing to avoid empty commits
2. **Tool failures**: Workflow fails if coverage-analytics exits non-zero
3. **CI loops**: `[skip ci]` prevents workflow from triggering itself
4. **Concurrent pushes**: GitHub's push protection handles automatically

## Implementation Steps

1. Create `.github/workflows/coverage-update.yml`
2. Add workflow YAML with proper structure:
   - name: "Update Coverage Dashboard"
   - Trigger paths matching recipes directory
   - Job with ubuntu-latest runner
   - Checkout, setup Go, run tool
   - Conditional commit based on changes
3. Test workflow file syntax (yaml linting)
4. Commit with message: `feat(ci): add workflow to regenerate coverage.json on recipe changes`

## Testing

Since workflows only appear in GitHub UI when on the default branch, full testing requires merge. However, we can validate:
- YAML syntax is valid
- Paths and commands are correct
- Action versions match repo standards
- Tool invocation is correct (`go run cmd/coverage-analytics/main.go website/coverage/coverage.json`)

Post-merge validation (documented for #1548):
- Workflow appears in Actions tab
- Manual dispatch works
- Recipe change triggers workflow
- coverage.json updates automatically

## Success Criteria

- [ ] Workflow file created at `.github/workflows/coverage-update.yml`
- [ ] Triggers configured for recipe changes and manual dispatch
- [ ] coverage-analytics tool invocation correct
- [ ] Bot account used for commits with [skip ci]
- [ ] Empty commit prevention (git diff check)
- [ ] YAML syntax valid

## Related Issues

- Depends on: cmd/coverage-analytics tool (completed in #1545)
- Blocks: #1548 (dashboard documentation will cover this workflow)
- Part of: PR #1529 (design/recipe-coverage-system branch)
