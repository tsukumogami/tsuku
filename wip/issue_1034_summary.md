# Issue 1034 Summary

## What Was Implemented

Reorganized golden file directory structure to separate embedded and registry recipes, enabling CI workflows to scope validation to just embedded recipes for faster feedback on code changes.

## Changes Made

- `testdata/golden/plans/embedded/`: Created new directory for embedded recipe golden files
- `testdata/golden/plans/{c,g,l,m,n,o,p,r,z}/`: Moved 14 embedded recipe golden directories to flat structure under `embedded/`
- `scripts/validate-golden.sh`: Added `detect_category()` function, `get_golden_dir()` function, and `--category` flag
- `scripts/validate-all-golden.sh`: Added `--category embedded|registry` flag with separate iteration functions
- `scripts/regenerate-golden.sh`: Added category detection and `--category` flag for output path selection
- `scripts/regenerate-all-golden.sh`: Added `--category` flag for scoped regeneration
- `docs/designs/DESIGN-recipe-registry-separation.md`: Marked M30 issues (#1033, #1034) as completed

## Key Decisions

- **Flat structure for embedded golden files**: Matches the flat embedded recipe layout after #1071, maintaining consistency between recipe and golden file organization
- **Auto-detection with override**: Scripts auto-detect category from recipe location but allow explicit `--category` flag for flexibility
- **Registry keeps letter-based structure**: Only embedded golden files changed; registry golden files remain in `<letter>/<recipe>/` pending future R2 migration

## Trade-offs Accepted

- **Two different directory structures**: Embedded (flat) vs registry (letter-based) may cause initial confusion, but mirrors the underlying recipe structure

## Test Coverage

- No new Go tests added (shell script changes only)
- Manual validation confirmed:
  - `validate-golden.sh go` finds embedded golden files
  - `validate-golden.sh fzf` finds registry golden files
  - `validate-all-golden.sh --category embedded` validates 14 embedded recipes
  - `validate-all-golden.sh --category registry` validates registry recipes

## Known Limitations

- Workflow trigger changes for scoped validation are deferred to #1036
- testdata recipes are treated as registry category (golden files in letter directories)

## Future Improvements

- #1036 will update workflows to use `--category embedded` for code change validation
- #1039 will design R2 storage for registry golden files (scaling solution)
