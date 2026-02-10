---
summary:
  constraints:
    - Golden files must use v4 format (no recipe_hash field)
    - Format version must be 4, not 3
    - All ~600 files in testdata/golden/plans/ must be regenerated
    - Tests must pass after regeneration
  integration_points:
    - scripts/regenerate-golden.sh - script to regenerate individual golden files
    - scripts/regenerate-all-golden.sh - batch regeneration script
    - testdata/golden/plans/ - location of all golden files
    - internal/executor/plan.go - defines PlanFormatVersion (now 4)
  risks:
    - Large file count (~600) may cause slow regeneration
    - Recipe network errors could cause incomplete regeneration
    - Need to verify no recipe_hash remains in any file
  approach_notes: |
    This is a simple chore issue - run the regeneration script to update
    all golden files from v3 to v4 format. The code changes are already
    complete (issues #1585, #1586, #1587). Validation script was prepped
    in #1584 to handle the transition.
---

# Implementation Context: Issue #1588

**Source**: docs/designs/DESIGN-plan-hash-removal.md (Phase 4)

## Issue Goal

Regenerate all local golden files (~600 files) to use the v4 plan format without the `recipe_hash` field.

## Acceptance Criteria

- [ ] `regenerate-golden.sh` produces v4 format plans (no `recipe_hash` field)
- [ ] `./scripts/regenerate-all-golden.sh` runs successfully
- [ ] All golden files in `testdata/golden/plans/` are updated to v4 format
- [ ] No golden file contains `recipe_hash` field
- [ ] `go test ./...` passes with regenerated golden files

## Validation

```bash
# Verify no golden files contain recipe_hash
if grep -rq '"recipe_hash"' testdata/golden/plans/; then
    echo "FAIL: recipe_hash still present in golden files"
    exit 1
fi
echo "PASS: No recipe_hash in golden files"

# Verify format_version is 4
if grep -rq '"format_version": 3' testdata/golden/plans/; then
    echo "FAIL: Some golden files still have format_version 3"
    exit 1
fi
echo "PASS: All golden files use format_version 4"

# Run tests
go test ./...
```
