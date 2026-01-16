# Issue 924 Implementation Plan

## Summary

Extend the constrained evaluation infrastructure to support npm_install by extracting package_lock from golden files and reusing it during constrained evaluation instead of live lockfile generation.

## Approach

Follow the same pattern established in #921 for pip, #922 for go, and #923 for cargo:
1. Update ExtractConstraints() to parse package_lock from npm_exec steps
2. Modify NpmInstallAction.Decompose() to check for NpmLock constraints and use them when available
3. Add tests for npm constraint extraction

The npm_install action already receives and uses the package_lock parameter during execution via npm_exec. The change is in the Decompose() phase where package_lock is generated via `npm install --package-lock-only` - we skip that and use the constraint instead.

## Files to Modify

- `internal/executor/constraints.go` - Add extractNpmConstraintsFromSteps() and HasNpmLockConstraint()
- `internal/actions/npm_install.go` - Modify Decompose() to use constraints when available
- `internal/executor/constraints_test.go` - Add tests for npm constraint extraction

## Implementation Steps

- [ ] Add extractNpmConstraintsFromSteps() to extract package_lock from npm_exec steps
- [ ] Update ExtractConstraintsFromPlan() to call the new function
- [ ] Update extractConstraintsFromDependency() to extract npm constraints
- [ ] Add HasNpmLockConstraint() helper function
- [ ] Modify NpmInstallAction.Decompose() to check ctx.Constraints.NpmLock
- [ ] Add decomposeWithConstraints() method that skips npm install --package-lock-only
- [ ] Add tests for npm constraint extraction

## Testing Strategy

### Unit Tests

1. `TestExtractConstraints_NpmExec`: Verify extraction of package_lock from npm_exec steps
2. `TestExtractConstraints_NpmExecInDependency`: Verify package_lock extraction from nested dependencies
3. `TestExtractConstraints_NpmExecFirstWins`: Verify first-wins semantics
4. `TestExtractConstraints_NpmExecEmptyPackageLock`: Verify empty package_lock handling
5. `TestHasNpmLockConstraint`: Test helper function

### Manual Verification

- Run `go test ./...` and verify all tests pass
- Run `go vet ./...` and verify no issues

## Success Criteria

- [ ] package_lock is extracted from golden files containing npm_exec steps
- [ ] When constraints are provided with NpmLock populated, npm_install.Decompose() reuses it
- [ ] Tests cover constraint extraction for npm
- [ ] Existing unconstrained npm_install evaluation continues to work
- [ ] All tests pass
