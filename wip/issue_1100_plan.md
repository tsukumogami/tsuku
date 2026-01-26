# Issue 1100 Implementation Plan

## Overview

Remove git-based registry golden files from the repository, completing the R2 migration.

## Current State

- Total golden files: 418 (6.5MB)
- Embedded golden files: 40 (288KB) - PRESERVE
- Registry golden files: 378 (~6.2MB) - DELETE

Registry golden files are in `testdata/golden/plans/[a-z]/` directories.
Embedded golden files are in `testdata/golden/plans/embedded/` directory.

## Implementation Steps

### Step 1: Delete Registry Golden Files

Delete all single-letter directories under `testdata/golden/plans/`:
- a/, b/, c/, d/, e/, f/, g/, h/, i/, j/, k/, l/, m/, n/, p/, r/, s/, t/, v/, w/

Preserve:
- embedded/ directory

### Step 2: Update .gitignore (Optional)

Consider adding entries to prevent accidental re-commit of registry golden files. However, since the CI now generates them to R2, this may not be necessary.

### Step 3: Verify CI Compatibility

CI workflows were updated in #1099 to use R2 for registry recipes:
- `validate-golden-recipes.yml`: Uses TSUKU_GOLDEN_SOURCE=r2 for registry
- `validate-golden-execution.yml`: Uses TSUKU_GOLDEN_SOURCE=r2 for registry

Both workflows should pass after deletion since they no longer reference git-based registry golden files.

### Step 4: Test Locally

Run tests to ensure no code references the deleted files.

## Risk Mitigation

1. **Preserve embedded files**: Only delete single-letter directories, not `embedded/`
2. **Git history retention**: Files remain in git history for recovery if needed
3. **R2 has copies**: All registry golden files were migrated to R2 in #1097

## Files Modified

- testdata/golden/plans/[a-z]/* - DELETED (378 files)
- .gitignore - potentially updated
