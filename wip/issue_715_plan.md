# Implementation Plan: Issue #715 - Batch Golden File Scripts

## Overview

Create two batch scripts that wrap the single-recipe scripts from #714:
- `scripts/validate-all-golden.sh` - validates all recipes with golden files
- `scripts/regenerate-all-golden.sh` - regenerates all golden files

## Analysis

### Existing Infrastructure (from #714)

The single-recipe scripts establish these patterns:
- Path constants: `SCRIPT_DIR`, `REPO_ROOT`, `GOLDEN_BASE`
- Auto-build tsuku if not present
- First-letter directory structure: `testdata/golden/plans/{letter}/{recipe}/`
- Exit codes: 0=success, 1=mismatch, 2=error

### Design from Design Doc

From the design doc section on batch scripts:

```bash
# scripts/validate-all-golden.sh
# Iterates over all recipes in testdata/golden/plans/
# Calls validate-golden.sh for each
# Reports which recipes failed so you can investigate and selectively regenerate
# Exit codes: 0 = all match, 1 = any mismatch
```

## Implementation Steps

### Step 1: Create `scripts/validate-all-golden.sh`

**Features:**
- Iterate over `testdata/golden/plans/{letter}/{recipe}/` directories
- Call `./scripts/validate-golden.sh <recipe>` for each
- Track failed recipes in an array
- Report failed recipes with specific regeneration commands
- Exit code 0 if all match, 1 if any mismatch

### Step 2: Create `scripts/regenerate-all-golden.sh`

**Features:**
- Iterate over `testdata/golden/plans/{letter}/{recipe}/` directories
- Call `./scripts/regenerate-golden.sh <recipe>` for each
- Report which recipes are being processed
- Exit code 0 on success

### Step 3: Test both scripts

- Verify validate-all works with existing fzf golden file
- Verify regenerate-all works

## File Changes

| File | Change Type | Description |
|------|-------------|-------------|
| `scripts/validate-all-golden.sh` | Create | Batch validation script |
| `scripts/regenerate-all-golden.sh` | Create | Batch regeneration script |

## Acceptance Criteria Mapping

| Criterion | Implementation |
|-----------|----------------|
| `validate-all-golden.sh` iterates over all recipes | Loop over `testdata/golden/plans/*/*/` |
| Reports list of failed recipes with regeneration commands | FAILED array, actionable output at end |
| Exit code 0 if all match, 1 if any mismatch | Check FAILED array length |
| `regenerate-all-golden.sh` regenerates all recipes | Loop and call single-recipe script |
| Scripts call single-recipe scripts internally | Direct script invocation |
| Output includes which recipes are being processed | Echo recipe name before processing |
