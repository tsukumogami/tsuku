# Issue 818 Implementation Plan

## Summary

Upload only the recipe-specific subdirectory instead of all golden files, then reconstruct the correct path structure during merge.

## Approach

The root cause is that each platform uploads the entire `testdata/golden/plans/` directory, but only generates files for one platform. When merged, later platforms overwrite earlier platforms' correct files with stale versions.

The fix uploads only the recipe-specific subdirectory (e.g., `testdata/golden/plans/r/ruff/`) and reconstructs the path structure during merge by computing it from the recipe name.

## Files to Modify

- `.github/workflows/generate-golden-files.yml` - Change artifact upload path and merge logic

## Implementation Steps

- [ ] Compute recipe-specific path variables in generate job
- [ ] Upload only the recipe subdirectory as artifact
- [ ] Reconstruct target path in merge step from recipe name
- [ ] Test locally that path construction works correctly

## Success Criteria

- [ ] Each platform uploads only its generated files (not all golden files)
- [ ] Merge step correctly places files in `testdata/golden/plans/<first_letter>/<recipe>/`
- [ ] Workflow validates recipe-specific directory structure

## Open Questions

None - the path pattern is deterministic based on recipe name.
