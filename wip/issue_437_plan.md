# Issue 437 Implementation Plan

## Summary

Implement `DecomposeToPrimitives()` function that recursively decomposes composite actions until only primitives remain, with cycle detection to prevent infinite recursion.

## Approach

Add a new function `DecomposeToPrimitives()` in `internal/actions/decomposable.go` that:
1. Checks if action is already primitive (base case)
2. Gets action from registry and checks if it implements `Decomposable`
3. Calls `Decompose()` and recursively processes each returned step
4. Carries forward checksum/size from decomposed steps
5. Uses a visited set to detect cycles

### Alternatives Considered

- **Option: Return error on non-decomposable composite**: Reject actions that are neither primitive nor decomposable. Chosen because it provides clear error handling.
- **Option: Silently pass through non-decomposable actions**: Could hide bugs where actions should be decomposable but aren't. Not chosen.

## Files to Modify

- `internal/actions/decomposable.go` - Add `DecomposeToPrimitives()` function and cycle detection

## Files to Create

None - all changes are in existing file.

## Implementation Steps

- [ ] Add `PrimitiveStep` struct to match design (same as `Step` but for fully decomposed results)
- [ ] Add `DecomposeToPrimitives()` function with recursive logic
- [ ] Add cycle detection using visited set with action+params hash
- [ ] Add helper function to compute params hash for cycle detection
- [ ] Add unit tests for recursive decomposition
- [ ] Add unit tests for cycle detection
- [ ] Add unit tests for checksum/size propagation

Mark each step [x] after it is implemented and committed. This enables clear resume detection.

## Testing Strategy

- Unit tests:
  - Test decomposition of a single primitive (passthrough)
  - Test decomposition of a composite that returns primitives
  - Test recursive decomposition (composite returns composite returns primitives)
  - Test cycle detection (A -> B -> A)
  - Test checksum/size propagation from decomposed step to primitive
  - Test error handling for non-decomposable composites
- No integration tests needed - this is pure logic

## Risks and Mitigations

- **Risk: Cycle detection false positives with same action, different params**: Mitigation - hash both action name and params together
- **Risk: Performance with deep recursion**: Mitigation - unlikely in practice, but could add depth limit if needed

## Success Criteria

- [ ] `DecomposeToPrimitives()` function implemented
- [ ] Recursive decomposition handles nested composites
- [ ] Cycle detection prevents infinite recursion
- [ ] Checksum/size carried forward from decomposed steps
- [ ] Error messages identify which action failed decomposition
- [ ] Unit tests for recursive decomposition and cycle detection
- [ ] All existing tests pass
- [ ] `go vet ./...` passes
- [ ] Build succeeds

## Open Questions

None - design document is clear on the approach.
