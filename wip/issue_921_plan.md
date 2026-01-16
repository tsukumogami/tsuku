# Issue 921 Implementation Plan

## Summary

Implement the foundational constrained evaluation infrastructure by adding `EvalConstraints` struct, `ExtractConstraints` function, extending `EvalContext` with a `Constraints` field, adding `--pin-from` CLI flag to `tsuku eval`, and modifying `pipx_install.Decompose()` to use pip constraints when available.

## Approach

The implementation follows the design document closely, using pip as the first ecosystem to support constrained evaluation. The approach leverages pip's native `--constraint` mechanism to pin dependency versions during evaluation, ensuring deterministic plan output that matches golden files exactly.

Key design decisions:
1. Place `EvalConstraints` in `internal/actions/decomposable.go` alongside `EvalContext` for co-location
2. Create a new `internal/executor/constraints.go` for the `ExtractConstraints` function since it operates on plans (executor domain)
3. Modify `pipx_install.Decompose()` to check for constraints and use them when available
4. The constraint mechanism reuses the existing `locked_requirements` format from golden files

### Alternatives Considered

- **Separate constraints package**: Rejected because constraints are tightly coupled to eval context and plan structures
- **Pass constraints as action params**: Rejected because it pollutes the action interface; context is the right place for cross-cutting concerns
- **Parse constraints at action level**: Rejected because extraction should happen once at CLI level, not per-action

## Files to Modify

- `internal/actions/decomposable.go` - Add `EvalConstraints` struct and `Constraints` field to `EvalContext`
- `cmd/tsuku/eval.go` - Add `--pin-from` flag and constraint loading logic
- `internal/actions/pipx_install.go` - Modify `Decompose()` to use constraints when available

## Files to Create

- `internal/executor/constraints.go` - Implement `ExtractConstraints` function to parse golden files
- `internal/executor/constraints_test.go` - Unit tests for constraint extraction

## Implementation Steps

- [x] Add `EvalConstraints` struct to `internal/actions/decomposable.go` with `PipConstraints map[string]string` field (other ecosystems will be empty for now)
- [x] Add `Constraints *EvalConstraints` field to `EvalContext` struct
- [x] Create `internal/executor/constraints.go` with `ExtractConstraints(planPath string) (*actions.EvalConstraints, error)` function
- [x] Implement pip constraint parsing in `ExtractConstraints` - parse `locked_requirements` from `pip_exec` steps to extract `package==version` pairs
- [x] Add `--pin-from` flag to `cmd/tsuku/eval.go` with validation (file must exist, be valid JSON)
- [x] Load and parse constraints in `runEval()` when `--pin-from` is provided
- [x] Pass constraints to `EvalContext` in plan generation via `PlanConfig`
- [x] Extend `PlanConfig` in `internal/executor/plan_generator.go` to include `Constraints *actions.EvalConstraints`
- [x] Modify `pipx_install.Decompose()` to check `ctx.Constraints` and use `PipConstraints` when available
- [x] Implement `generateLockedRequirementsFromConstraints()` that reuses the pinned versions instead of live resolution
- [x] Write unit tests for constraint extraction
- [ ] Write integration test: eval with `--pin-from` produces output matching golden file (deferred to follow-up issue)

## Testing Strategy

### Unit Tests

1. `TestExtractConstraints_PipExec`: Verify extraction of pip constraints from a plan with `pip_exec` step containing `locked_requirements`
2. `TestExtractConstraints_EmptyPlan`: Verify handling of plans without pip_exec steps
3. `TestExtractConstraints_InvalidFile`: Verify error handling for non-existent or malformed files
4. `TestParsePipRequirements`: Verify parsing of various pip requirements formats (with hashes, without, continuation lines)

### Integration Tests

1. Test that `tsuku eval httpie@X.Y.Z --pin-from golden.json` produces output matching the golden file
2. Test that existing unconstrained evaluation continues to work (no regressions)

### Manual Verification

- Run `go vet ./...` and `go test -v -test.short ./...` during development
- Full test suite `go test ./...` before committing
- Build verification: `go build -o tsuku ./cmd/tsuku`

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Pip constraint format varies | Use robust regex parsing that handles common formats (package==version with optional hash comments) |
| Performance overhead from loading golden files | Constraint loading is a single JSON parse; negligible for CLI usage |
| Breaking existing eval behavior | All new code paths are opt-in via `--pin-from` flag; default behavior unchanged |
| pip `--constraint` flag limitations | For initial implementation, generate a constraint file; if pip limitations emerge, can fall back to rewriting requirements directly |

## Success Criteria

- [x] `EvalConstraints` struct exists with `PipConstraints map[string]string` field
- [x] `EvalContext` has `Constraints *EvalConstraints` field
- [x] `ExtractConstraints` correctly parses pip constraints from golden files
- [x] `--pin-from` flag is available on `tsuku eval` command
- [x] When `--pin-from` is provided, constraints are extracted and passed to `EvalContext`
- [x] `pipx_install.Decompose()` uses constraints when `ctx.Constraints.PipConstraints` is populated
- [ ] Constrained evaluation produces deterministic output matching the golden file (requires integration test)
- [x] All existing tests pass (no regressions)
- [x] `go vet ./...` passes
- [x] `go build -o tsuku ./cmd/tsuku` succeeds

## Open Questions

None - the design document provides clear guidance on all implementation details.
