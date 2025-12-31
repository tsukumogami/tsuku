# Issue 761 Summary

## What Was Done

Implemented `FilterStepsByTarget(steps []recipe.Step, target platform.Target) []recipe.Step` function in `internal/executor/filter.go`.

The function performs two-stage filtering:
1. **Stage 1**: Check action's `ImplicitConstraint()` against target (only for SystemAction implementations)
2. **Stage 2**: Check step's explicit `when` clause against target platform

A step is included only if both stages pass.

## Files Changed

- `internal/executor/filter.go` - New file with `FilterStepsByTarget()` and `stepMatchesTarget()` functions
- `internal/executor/filter_test.go` - Comprehensive test suite with 23 test cases
- `docs/DESIGN-system-dependency-actions.md` - Updated dependency graph (#761 marked done)

## Acceptance Criteria

- [x] Function: `FilterStepsByTarget(steps, target)` - Named slightly differently to match Go conventions
- [x] Two-stage filtering: check action's `ImplicitConstraint()`, then check step's explicit `when` clause
- [x] Step included only if both checks pass
- [x] Actions without implicit constraint pass stage 1 automatically
- [x] Steps without explicit `when` pass stage 2 automatically
- [x] Integration test: `apt_install` step filtered out when target is `rhel`
- [x] Integration test: `brew_cask` step filtered out when target is `linux/amd64`

## Test Coverage

12 test cases in `TestFilterStepsByTarget`:
- Empty steps returns empty
- Steps with no constraints pass through
- apt_install filtered out for rhel target
- brew_cask filtered out for linux/amd64 target
- apt_install passes for debian target
- brew_cask passes for darwin target
- dnf_install passes for rhel target
- pacman_install filtered out for debian target
- Step with explicit when clause filtered correctly
- Step with explicit when clause passes when matched
- Mixed steps filtered correctly for debian target
- Mixed steps filtered correctly for darwin target

10 test cases in `TestStepMatchesTarget`:
- action without constraint matches any target
- apt_install matches debian / does not match rhel
- brew_install matches darwin / does not match linux
- when clause OS/platform mismatch and match cases
- implicit constraint passes but when clause fails
- both implicit constraint and when clause pass

## Design Notes

- Function returns `[]recipe.Step` (not `*Plan`) because we're filtering recipe steps, not resolved steps
- Uses type assertion to check if action implements `SystemAction` interface
- Helper function `stepMatchesTarget()` encapsulates single-step logic for clarity
