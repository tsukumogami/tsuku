# Issue 471 Implementation Plan

## Summary

Add a `GetCachedPlan(tool, version string) (*Plan, error)` method to StateManager that retrieves stored installation plans from state.json for cache lookup during the orchestration phase.

## Approach

Follow the existing pattern used by `GetToolState` - load state, navigate to the appropriate nested field, return pointer or nil with appropriate error handling.

### Alternatives Considered

- **Reuse GetToolState and let caller extract plan**: Rejected because it exposes internal state structure and requires callers to handle version lookup logic.

## Files to Modify

- `internal/install/state_tool.go` - Add GetCachedPlan method (alongside GetToolState)

## Files to Create

None

## Implementation Steps

- [x] Add GetCachedPlan method to StateManager in state_tool.go
- [x] Add unit tests for cache hit, cache miss, and error scenarios

## Testing Strategy

- Unit tests: Add tests in state_test.go covering:
  - GetCachedPlan returns plan when tool/version exists with plan
  - GetCachedPlan returns nil when tool not installed
  - GetCachedPlan returns nil when version not installed
  - GetCachedPlan returns nil when version exists but no plan cached

## Risks and Mitigations

None - this is a simple read-only accessor following established patterns.

## Success Criteria

- [x] `GetCachedPlan(tool, version string) (*Plan, error)` exists on StateManager
- [x] Returns stored plan from `state.Installed[tool].Versions[version].Plan`
- [x] Returns nil, nil when tool not installed
- [x] Returns nil, nil when version not installed
- [x] Returns nil, nil when no plan cached for version
- [x] Unit tests pass
- [x] `go test ./internal/install/...` passes

## Open Questions

None
