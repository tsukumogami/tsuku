# Issue 1079 Implementation Plan

## Summary

Add a validation step to `validate-recipe-structure.yml` that checks each registry recipe's filename starts with its parent directory letter.

## Approach

Add a new step after "Validate registry recipes use letter directories" that iterates through all `recipes/<letter>/*.toml` files and verifies `basename` starts with `<letter>`. Report all mismatches at once for easy correction.

## Files to Modify

- `.github/workflows/validate-recipe-structure.yml` - Add new validation step

## Files to Create

None

## Implementation Steps

- [ ] Add "Validate recipes are in correct letter directory" step after line 83
- [ ] Find all `.toml` files in `recipes/` with depth 2 (`recipes/<letter>/<name>.toml`)
- [ ] For each file, extract parent directory name and file basename
- [ ] Check if filename starts with directory letter
- [ ] Collect mismatches and report all at once with clear error message
- [ ] Test with intentionally misplaced recipe

## Success Criteria

- [ ] CI fails when `recipes/a/fzf.toml` exists (misplaced)
- [ ] CI passes with existing correct structure
- [ ] Error message shows recipe path and expected directory
