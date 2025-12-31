# Issue 760 Implementation Plan

## Summary

Add `MatchesTarget(target Target) bool` method to the `Constraint` struct and update unit tests. Most of the acceptance criteria from #755 is already implemented.

## Approach

The `Constraint` struct and `ImplicitConstraint()` are already implemented from #755. The only missing piece is `MatchesTarget()` which checks if a constraint applies to a given target.

## Files to Modify

- `internal/actions/system_action.go` - Add MatchesTarget() to Constraint struct
- `internal/actions/system_action_test.go` - Add unit tests

## Files to Create

None - using existing files.

## Implementation Steps

- [ ] Add MatchesTarget(target Target) bool to Constraint struct
- [ ] Import platform package in system_action.go
- [ ] Add unit tests for MatchesTarget
- [ ] Verify all existing ImplicitConstraint tests still pass

## Testing Strategy

- Unit tests:
  - MatchesTarget returns true for matching constraint
  - MatchesTarget returns false for non-matching OS
  - MatchesTarget returns false for non-matching LinuxFamily
  - Darwin constraint matches darwin target
  - Linux constraint without family matches any linux family

## Risks and Mitigations

- **Import cycle**: Need to import platform.Target - verify no cycles exist

## Success Criteria

- [ ] MatchesTarget() implemented on Constraint struct
- [ ] All unit tests pass
- [ ] go vet and go build succeed

## Open Questions

None - straightforward implementation.
