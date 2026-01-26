---
summary:
  constraints:
    - Only remove registry golden files (testdata/golden/plans/[a-z]/), not embedded (testdata/golden/plans/embedded/)
    - This is the "point of no return" for R2 migration - signals full commitment to R2 storage
    - CI workflows must pass after removal (validate-golden-recipes.yml and validate-golden-execution.yml now use R2 for registry)
  integration_points:
    - testdata/golden/plans/[a-z]/ - files to delete
    - .gitignore - may need update to prevent re-commit of registry golden files
    - CI workflows - should already be using R2 for registry recipes (from #1099)
  risks:
    - Accidental deletion of embedded golden files (testdata/golden/plans/embedded/) - must preserve these
    - CI workflows might still have references to deleted paths - verify workflows pass
    - Git history will retain the files but working tree should shrink by ~5MB
  approach_notes: |
    Simple file deletion task:
    1. Delete all directories under testdata/golden/plans/ that are single letters [a-z]
    2. Keep testdata/golden/plans/embedded/ intact
    3. Optionally update .gitignore to prevent re-adding registry golden files
    4. Verify CI passes
---

# Implementation Context: Issue #1100

**Source**: docs/designs/DESIGN-r2-golden-storage.md

## Key Points

- This is Phase 4 of the R2 migration - the final step that removes git-based storage for registry golden files
- Registry golden files are stored at `testdata/golden/plans/[a-z]/` (organized by first letter of recipe name)
- Embedded golden files at `testdata/golden/plans/embedded/` must be preserved (they stay in git)
- Issue #1099 updated PR validation to use R2 for registry recipes, so CI should pass after deletion
- Expected working tree reduction: ~5MB (418 files)
