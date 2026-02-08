# Implementation Plan: Issue #1548

## Summary

Create comprehensive documentation for the coverage dashboard, explaining how it works, how to use it, and how to maintain it.

## Files to Create/Modify

### New Files
- `website/docs/coverage-dashboard.md` - Main documentation file

### Modified Files
- `website/README.md` - Add coverage dashboard section

## Implementation Approach

### Documentation Structure

Create `website/docs/coverage-dashboard.md` with sections:

1. **Overview**
   - What the dashboard shows
   - Link to live dashboard
   - Purpose and audience

2. **Dashboard Usage**
   - Three views: Coverage Matrix, Gap List, Category Breakdown
   - How to filter and sort
   - How to interpret indicators (supported, partial, unsupported)
   - What each view shows

3. **Data Sources**
   - Coverage data from static analysis of recipe files
   - CI workflow runs analyzer on every merge to main
   - coverage.json is committed to repository
   - Dashboard reads this file at page load

4. **Manual Regeneration**
   - When you might need to regenerate (testing recipe changes, local development)
   - Command: `go run cmd/coverage-analytics/main.go`
   - Output location: `website/coverage/coverage.json`
   - How to verify the generated file is valid JSON

5. **Platform Support Indicators**
   - **Supported**: Recipe has explicit steps for this platform (glibc, musl, darwin)
   - How the analyzer determines support (checks `when` clauses in recipe steps)
   - Examples of supported vs unsupported recipes

6. **CI Integration**
   - Workflow file: `.github/workflows/coverage-update.yml`
   - Triggers: push to main when recipes change, manual dispatch
   - What happens: analyzer runs, commits coverage.json if changed
   - How to check if data is stale (check commit date in coverage.json)

### README Update

Add to `website/README.md`:
- Link to coverage dashboard page
- Link to documentation
- Brief description of what it shows
- Command for manual regeneration

## Implementation Steps

1. Create `website/docs/` directory if it doesn't exist
2. Write `website/docs/coverage-dashboard.md` with all sections
3. Update `website/README.md` with coverage dashboard section
4. Verify all links work
5. Test manual regeneration command to ensure it's accurate
6. Commit with message: `docs(website): add coverage dashboard documentation`

## Content Guidelines

- Keep language accessible for first-time contributors
- Include concrete examples (specific recipes, actual commands)
- Link to relevant files in GitHub (workflow file, tool source code)
- Explain "why" not just "how" (why we analyze recipes, why CI auto-updates)
- Use consistent terminology matching the dashboard UI

## Success Criteria

- [ ] `website/docs/coverage-dashboard.md` exists with all 6 sections
- [ ] Documentation explains all three dashboard views
- [ ] Manual regeneration steps are accurate (tested)
- [ ] Platform support indicators are clearly explained
- [ ] CI workflow integration is documented
- [ ] `website/README.md` links to coverage dashboard docs
- [ ] All links in documentation are valid

## Related Issues

- Builds on: #1544 (dashboard HTML), #1545 (coverage matrix), #1546 (gap list/breakdown), #1547 (CI workflow)
- Completes: Milestone 2 (Coverage Dashboard)
- Part of: PR #1529 (design/recipe-coverage-system branch)
