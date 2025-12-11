# Issue 401 Implementation Plan

## Summary

Create new data types for installation plans in the executor package, following the design specification exactly. The types will be standalone Go structs with JSON serialization, placed in a new `internal/executor/plan.go` file.

## Approach

Create a minimal, focused implementation that defines only the data types and their JSON serialization. The types follow the design document exactly, with the action evaluability classification as a separate constant map for easy reference by future issues.

### Alternatives Considered

- **Embedding in state.go**: Rejected because these types belong to the executor package (plan generation logic) rather than install package (state management). Issue #404 will add a reference to these types in VersionState.
- **Creating a new `internal/plan` package**: Rejected as premature - the design places PlanGenerator in executor, so the types should live there too. Can be extracted later if needed.

## Files to Create

- `internal/executor/plan.go` - Installation plan data types and evaluability classification
- `internal/executor/plan_test.go` - Unit tests for JSON round-trip serialization

## Files to Modify

None - this issue only adds new types.

## Implementation Steps

- [ ] Create `internal/executor/plan.go` with InstallationPlan, Platform, and ResolvedStep structs
- [ ] Add ActionEvaluability constant map classifying all actions
- [ ] Add FormatVersion constant (set to 1)
- [ ] Create `internal/executor/plan_test.go` with JSON serialization tests
- [ ] Run tests and verify build

## Testing Strategy

- **Unit tests**:
  - JSON round-trip serialization for InstallationPlan (marshal, unmarshal, verify fields match)
  - JSON round-trip for ResolvedStep with and without optional fields (URL, Checksum, Size)
  - Verify JSON field names match design spec (snake_case)
  - Test Platform struct serialization
  - Test time.Time serialization in GeneratedAt field

## Risks and Mitigations

- **Risk**: Design may evolve as later issues are implemented
  - **Mitigation**: Types follow design exactly; later issues can add fields without breaking compatibility

## Success Criteria

- [ ] `InstallationPlan` struct defined with all required fields and JSON tags
- [ ] `Platform` struct defined with OS and Arch fields
- [ ] `ResolvedStep` struct defined with Action, Params, Evaluable, URL, Checksum, Size fields
- [ ] Action evaluability classification documented via ActionEvaluability map
- [ ] FormatVersion constant set to 1
- [ ] JSON serialization tests pass
- [ ] `go build ./...` succeeds
- [ ] `go test ./internal/executor/...` passes

## Open Questions

None - the design is clear for this foundational issue.
