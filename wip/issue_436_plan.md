# Issue 436 Implementation Plan

## Summary

Define the `Decomposable` interface, `Step` struct, `EvalContext` struct, and primitive registry in `internal/actions/` to establish the foundation for decomposable actions.

## Approach

Create a new file `decomposable.go` in `internal/actions/` following the design document specification. The primitives registry will be a simple set of action names that are considered primitives (cannot be decomposed further).

### Alternatives Considered
- **Inline in action.go**: Would clutter the existing file with unrelated concepts. Better separation of concerns to have a dedicated file.
- **Separate package**: Unnecessary complexity; the types need access to `Action` interface and registry.

## Files to Create
- `internal/actions/decomposable.go` - Decomposable interface, Step, EvalContext, and primitive registry
- `internal/actions/decomposable_test.go` - Unit tests for IsPrimitive function

## Files to Modify
None - this is additive infrastructure

## Implementation Steps
- [x] Create `decomposable.go` with `Decomposable` interface definition
- [x] Add `Step` struct with Action, Params, Checksum, Size fields
- [x] Add `EvalContext` struct with Version, VersionTag, OS, Arch, Recipe, Resolver fields
- [x] Add primitive registry (set of Tier 1 primitive names)
- [x] Implement `IsPrimitive(action string) bool` function
- [x] Create unit tests for `IsPrimitive` function
- [x] Run tests and verify build

## Testing Strategy
- Unit tests: Test `IsPrimitive()` returns true for all 8 Tier 1 primitives and false for composite actions
- Build verification: Ensure all code compiles and integrates with existing action system

## Risks and Mitigations
- **Risk**: EvalContext depends on validate.PreDownloader which may create import cycles
  - **Mitigation**: The design shows PreDownloader as optional pointer; omit for now as issue 437 (recursive decomposition) will use it. For this issue, leave the field but don't import validate package.

## Success Criteria
- [x] `Decomposable` interface defined with `Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error)` method
- [x] `Step` struct has Action, Params, Checksum, Size fields
- [x] `EvalContext` struct has Version, VersionTag, OS, Arch, Recipe, Resolver fields
- [x] `IsPrimitive("download")` returns true
- [x] `IsPrimitive("github_archive")` returns false
- [x] All 8 Tier 1 primitives registered: download, extract, chmod, install_binaries, set_env, set_rpath, link_dependencies, install_libraries
- [x] Unit tests pass
- [x] Build succeeds

## Open Questions
None - the design document provides clear specifications.
