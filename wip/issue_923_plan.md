# Issue 923 Implementation Plan

## Summary

Extend the constrained evaluation infrastructure to support cargo_install by extracting lock_data from golden files and reusing it during constrained evaluation instead of live lockfile generation.

## Approach

Follow the same pattern established in #921 for pip and #922 for go:
1. Update ExtractConstraints() to parse lock_data from cargo_build steps
2. Modify CargoInstallAction.Decompose() to check for CargoLock constraints and use them when available
3. Add tests for cargo constraint extraction

The cargo_install action already receives and uses the lock_data parameter during execution via cargo_build. The change is in the Decompose() phase where lock_data is generated via `cargo generate-lockfile` - we skip that and use the constraint instead.

## Files to Modify

- `internal/executor/constraints.go` - Add extractCargoConstraintsFromSteps() and HasCargoLockConstraint()
- `internal/actions/cargo_install.go` - Modify Decompose() to use constraints when available
- `internal/executor/constraints_test.go` - Add tests for cargo constraint extraction

## Implementation Steps

- [ ] Add extractCargoConstraintsFromSteps() to extract lock_data from cargo_build steps
- [ ] Update ExtractConstraintsFromPlan() to call the new function
- [ ] Update extractConstraintsFromDependency() to extract cargo constraints
- [ ] Add HasCargoLockConstraint() helper function
- [ ] Modify CargoInstallAction.Decompose() to check ctx.Constraints.CargoLock
- [ ] Add decomposeWithConstraints() method that skips generateCargoLock()
- [ ] Add tests for cargo constraint extraction

## Testing Strategy

### Unit Tests

1. `TestExtractConstraints_CargoBuild`: Verify extraction of lock_data from cargo_build steps
2. `TestExtractConstraints_CargoBuildInDependency`: Verify lock_data extraction from nested dependencies
3. `TestExtractConstraints_CargoBuildFirstWins`: Verify first-wins semantics
4. `TestHasCargoLockConstraint`: Test helper function

### Manual Verification

- Run `go test ./...` and verify all tests pass
- Run `go vet ./...` and verify no issues

## Success Criteria

- [ ] lock_data is extracted from golden files containing cargo_build steps
- [ ] When constraints are provided with CargoLock populated, cargo_install.Decompose() reuses it
- [ ] Tests cover constraint extraction for cargo
- [ ] Existing unconstrained cargo_install evaluation continues to work
- [ ] All tests pass
