# Issue 922 Implementation Plan

## Summary

Extend the constrained evaluation infrastructure to support go_build by extracting go_sum from golden files and reusing it during constrained evaluation instead of live dependency resolution via `go get`.

## Approach

Follow the same pattern established in #921 for pip:
1. Update ExtractConstraints() to parse go_sum from go_build steps
2. Modify GoInstallAction.Decompose() to check for GoSum constraints and use them when available
3. Add tests for go constraint extraction

The go_build action already receives and uses the go_sum parameter during execution. The change is in the Decompose() phase where go_sum is generated via `go get` - we skip that and use the constraint instead.

### Alternatives Considered

- **Modify go_build instead of go_install**: Rejected because go_build is the primitive and doesn't have Decompose(). go_install is the composite action that decomposes to go_build.

## Files to Modify

- `internal/executor/constraints.go` - Add extractGoConstraintsFromSteps() and call it from ExtractConstraintsFromPlan()
- `internal/actions/go_install.go` - Modify Decompose() to use constraints when available
- `internal/executor/constraints_test.go` - Add tests for go constraint extraction

## Implementation Steps

- [ ] Add extractGoConstraintsFromSteps() to extract go_sum from go_build steps
- [ ] Update ExtractConstraintsFromPlan() to call the new function
- [ ] Update extractConstraintsFromDependency() to extract go constraints
- [ ] Modify GoInstallAction.Decompose() to check ctx.Constraints.GoSum
- [ ] If GoSum is populated, skip `go get` and use the constraint directly
- [ ] Add tests for go constraint extraction
- [ ] Add helper function HasGoSumConstraint() for consistency with pip pattern

## Testing Strategy

### Unit Tests

1. `TestExtractConstraints_GoBuild`: Verify extraction of go_sum from go_build steps
2. `TestExtractConstraints_GoBuildInDependency`: Verify go_sum extraction from nested dependencies
3. `TestHasGoSumConstraint`: Test helper function

### Manual Verification

- Run `go test ./...` and verify all tests pass
- Run `go vet ./...` and verify no issues

## Success Criteria

- [ ] go_sum is extracted from golden files containing go_build steps
- [ ] When constraints are provided with GoSum populated, go_install.Decompose() reuses it
- [ ] Tests cover constraint extraction for go
- [ ] Existing unconstrained go_install evaluation continues to work
- [ ] All tests pass
