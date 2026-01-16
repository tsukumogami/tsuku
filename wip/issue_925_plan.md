# Issue 925 Implementation Plan

## Summary

Extend the constrained evaluation infrastructure to support gem_install by extracting lock_data from golden files and reusing it during constrained evaluation instead of live lockfile generation.

## Approach

Follow the same pattern established in #921 for pip, #922 for go, #923 for cargo, and #924 for npm:
1. Add extractGemConstraintsFromSteps() function to constraints.go
2. Add HasGemLockConstraint() helper function
3. Update ExtractConstraintsFromPlan() and extractConstraintsFromDependency() to call the new function
4. Add tests for gem constraint extraction

Note: Unlike the other ecosystems, gem_install does NOT have a Decompose() method that generates lockfiles at eval time. The gem_install action is a simple action that wraps gem_exec. The constraint support here is purely for extraction - the decomposeWithConstraints pattern is NOT needed for gem.

## Files to Modify

- `internal/executor/constraints.go` - Add extractGemConstraintsFromSteps() and HasGemLockConstraint()
- `internal/executor/constraints_test.go` - Add tests for gem constraint extraction

## Implementation Steps

- [ ] Add extractGemConstraintsFromSteps() to extract lock_data from gem_exec steps
- [ ] Update ExtractConstraintsFromPlan() to call the new function
- [ ] Update extractConstraintsFromDependency() to extract gem constraints
- [ ] Add HasGemLockConstraint() helper function
- [ ] Add tests for gem constraint extraction

## Testing Strategy

### Unit Tests

1. `TestExtractConstraints_GemExec`: Verify extraction of lock_data from gem_exec steps
2. `TestExtractConstraints_GemExecInDependency`: Verify lock_data extraction from nested dependencies
3. `TestExtractConstraints_GemExecFirstWins`: Verify first-wins semantics
4. `TestExtractConstraints_GemExecEmptyLockData`: Verify empty lock_data handling
5. `TestHasGemLockConstraint`: Test helper function

### Manual Verification

- Run `go test ./...` and verify all tests pass
- Run `go vet ./...` and verify no issues

## Success Criteria

- [ ] lock_data is extracted from golden files containing gem_exec steps
- [ ] Tests cover constraint extraction for gem
- [ ] All tests pass
