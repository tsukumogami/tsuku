# Issue 1098 Implementation Plan

## Overview

Enable parallel operation of R2 and git golden files during the migration transition period.

## Key Insight

The validation scripts already support `--golden-dir` for pointing at any directory. The nightly workflow (#1096) downloads from R2 to a temp directory and uses `--golden-dir`. So the infrastructure for using R2 is already in place.

What's missing:
1. A way to compare git vs R2 content for consistency verification
2. Environment variable control for which source to use
3. Documentation of the parallel operation state

## Deliverables

### 1. Consistency Check Script

Create `scripts/r2-consistency-check.sh`:
- Download files from R2 to temp directory
- Compare checksums against git files
- Report mismatches (expected: some files differ due to version regeneration)
- Exit 0 if no unexpected mismatches, exit 1 otherwise

**Note**: Files in R2 may have been regenerated with newer versions, so we should:
- Compare files that have the same version in both places
- Report (but don't fail on) files that exist in one place but not the other

### 2. Environment Variable Support

Add `TSUKU_GOLDEN_SOURCE` support to validation scripts:
- `git` (default): Use git-based golden files (testdata/golden/plans)
- `r2`: Download from R2 and use those files
- `both`: Validate against both and compare results (for verification)

The `r2` mode requires R2 credentials to be set.

### 3. Update Design Doc

Mark #1097 as done and #1098 as ready in the dependency graph.

## Implementation Steps

### Step 1: Create consistency check script

`scripts/r2-consistency-check.sh`:
- Parse arguments (--category, --recipe for filtering)
- Download R2 files to temp directory
- Transform R2 structure to git-compatible structure
- Compare checksums
- Report results

### Step 2: Add TSUKU_GOLDEN_SOURCE support

Update `scripts/validate-golden.sh`:
- Check TSUKU_GOLDEN_SOURCE environment variable
- If `r2`: download files from R2 first, then validate
- If `both`: validate both and compare results

Update `scripts/validate-all-golden.sh`:
- Pass through TSUKU_GOLDEN_SOURCE to validate-golden.sh

### Step 3: Update design doc dependency graph

Edit `docs/designs/DESIGN-r2-golden-storage.md`:
- Mark #1097 as done
- Mark #1098 as ready (will be done after this PR merges)

## Files to Create/Modify

1. **New**: `scripts/r2-consistency-check.sh`
2. **Modify**: `scripts/validate-golden.sh` - add TSUKU_GOLDEN_SOURCE
3. **Modify**: `scripts/validate-all-golden.sh` - add TSUKU_GOLDEN_SOURCE
4. **Modify**: `docs/designs/DESIGN-r2-golden-storage.md` - update dependency graph

## Testing

1. Run consistency check locally (requires R2 credentials)
2. Run validate-golden.sh with TSUKU_GOLDEN_SOURCE=git (existing behavior)
3. Verify existing CI workflows still pass
