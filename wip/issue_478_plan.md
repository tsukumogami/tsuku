# Issue 478 Implementation Plan

## Summary

Wire up `installWithDependencies()` to use the new plan-based flow (`getOrGeneratePlan()` + `ExecutePlan()`), replacing direct recipe execution.

## Approach

Follow the design from `docs/DESIGN-deterministic-execution.md`. The key changes:
1. Replace `exec.Execute(ctx)` with `getOrGeneratePlan()` + `exec.ExecutePlan(ctx, plan)`
2. Replace `convertExecutorPlan()` with `executor.ToStoragePlan()` (the canonical version)
3. Handle `ChecksumMismatchError` with user-friendly output
4. Propagate `installFresh` flag through the flow

### Alternatives Considered
- Add new function instead of modifying existing: Not chosen because design explicitly calls for modifying `installWithDependencies()`
- Keep both Execute() and ExecutePlan() paths: Not chosen - we want a single execution path

## Files to Modify
- `cmd/tsuku/install_deps.go` - Update `installWithDependencies()` to use plan-based flow, remove duplicate `convertExecutorPlan()`

## Files to Create
- None - all test infrastructure exists

## Implementation Steps
- [x] Update `installWithDependencies()` to compute recipe hash before creating executor
- [x] Update `installWithDependencies()` to call `getOrGeneratePlan()` with proper config
- [x] Update `installWithDependencies()` to call `exec.ExecutePlan(ctx, plan)` instead of `exec.Execute(ctx)`
- [x] Handle `ChecksumMismatchError` from ExecutePlan with user-friendly output
- [x] Replace `convertExecutorPlan()` usage with `executor.ToStoragePlan()`
- [x] Remove the now-unused `convertExecutorPlan()` function
- [x] Remove the nolint directive for `getOrGeneratePlan` (it's now used)
- [ ] Update design doc to mark #478 as done and #479 as ready
- [x] Verify all tests pass

## Testing Strategy
- Unit tests: Existing tests in `install_deps_test.go` cover `getOrGeneratePlan()`
- Manual verification:
  1. `tsuku install gh` - fresh install
  2. `tsuku install gh` again - should use cache ("Using cached plan for gh@X.Y.Z")
  3. `tsuku install gh --fresh` - should regenerate ("Generating plan for gh@X.Y.Z")
- The architectural equivalence test (`tsuku eval foo | tsuku install --plan -`) is for future work

## Risks and Mitigations
- Risk: Breaking existing install flow
  - Mitigation: All infrastructure is tested, and the existing Execute() path remains available until cleanup
- Risk: ChecksumMismatchError handling missing
  - Mitigation: The error type already has user-friendly Error() method with recovery instructions

## Success Criteria
- [ ] `installWithDependencies()` uses `getOrGeneratePlan()` + `ExecutePlan()`
- [ ] `executor.ToStoragePlan()` used for plan storage
- [ ] `ChecksumMismatchError` handled with user-friendly output
- [ ] `--fresh` flag propagated through flow
- [ ] `go test ./cmd/tsuku/...` passes
- [ ] Design doc updated to mark #478 as done

## Open Questions
None - design doc provides clear specification.
