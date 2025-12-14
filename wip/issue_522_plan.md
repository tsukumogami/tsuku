# Issue 522 Implementation Plan

## Summary

Add `apply_patch` and `text_replace` to the primitives and deterministic actions maps, then integrate patch processing into the plan generator so that `recipe.Patches` entries are converted to `apply_patch` steps during plan generation.

## Approach

The `apply_patch` action already exists with full implementation and tests. The missing pieces are:
1. Registering `apply_patch` and `text_replace` as primitives in `decomposable.go`
2. Adding logic in `plan_generator.go` to convert `recipe.Patches` to `apply_patch` steps

This is a focused integration task rather than new action development.

### Alternatives Considered
- **Integrate patches into composite actions**: Would require changes to multiple composites (github_archive, homebrew_bottle, etc.) - rejected as it couples download/extraction logic with patching unnecessarily
- **Keep patches as pre-processing in executor**: Would skip plan generation, meaning patches wouldn't be captured in reproducible plans - rejected as it breaks determinism goals

## Files to Modify

- `internal/actions/decomposable.go` - Add `apply_patch` and `text_replace` to primitives and deterministicActions maps
- `internal/actions/decomposable_test.go` - Update expected primitive count and add new primitives to test assertions
- `internal/executor/plan_generator.go` - Add patch-to-step conversion logic before processing recipe steps
- `internal/executor/plan_generator_test.go` - Add tests for patch step generation

## Files to Create

None required - all infrastructure exists.

## Implementation Steps

- [x] Add `apply_patch` and `text_replace` to primitives map in decomposable.go
- [x] Add `apply_patch` and `text_replace` to deterministicActions map (as deterministic)
- [x] Update decomposable_test.go to expect 19 primitives (was 17) and include new actions in assertions
- [x] Add patch processing in plan_generator.go after step resolution setup but before recipe step iteration
- [x] Add unit tests for patch step generation in plan_generator_test.go
- [x] Run full test suite to verify no regressions

Mark each step [x] after it is implemented and committed. This enables clear resume detection.

## Testing Strategy

- **Unit tests**: Verify primitives registration (count and membership), patch-to-step conversion
- **Integration tests**: Generate plan from recipe with patches, verify apply_patch steps appear
- **Manual verification**: Create recipe with patches, run `tsuku plan`, verify output shows apply_patch steps

## Risks and Mitigations

- **Test count mismatch**: Tests assert exact primitive count - will update in same commit to avoid CI failure
- **Patch ordering**: Patches should be applied after extraction but before build steps - will insert at correct position in plan

## Success Criteria

- [ ] `apply_patch` and `text_replace` appear in `actions.Primitives()` output
- [ ] `actions.IsPrimitive("apply_patch")` returns true
- [ ] `actions.IsDeterministic("apply_patch")` returns true
- [ ] Plan generation converts `recipe.Patches` to `apply_patch` steps
- [ ] All existing tests pass

## Open Questions

None - implementation path is clear.
