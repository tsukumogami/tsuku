# Issue 242 Implementation Plan

## Summary

Remove legacy `Ensure*` bootstrap functions and refactor `ensurePackageManagersForRecipe()` to use the new dependency resolution system from `actions.ResolveDependencies()`.

## Approach

Replace the hardcoded action-to-function mapping in `ensurePackageManagersForRecipe()` with a loop that:
1. Uses `actions.ResolveDependencies()` to get install-time deps
2. Calls `installWithDependencies()` to install each dep
3. Finds the installed binary paths using the config/state

### Alternatives Considered
- Keep Ensure* as internal helpers: Not chosen - they duplicate logic that `installWithDependencies()` already handles
- Inline everything: Not chosen - keep separation of concerns

## Files to Modify
- `cmd/tsuku/install.go` - Refactor `ensurePackageManagersForRecipe()` to use ResolveDependencies
- `internal/install/bootstrap.go` - Remove `EnsureNpm`, `EnsurePipx`, `EnsureCargo`, `EnsurePython`, `ensurePackageManager`, `getDepExecutableNames`

## Files to Create
None

## Implementation Steps
- [x] Refactor `ensurePackageManagersForRecipe()` to use ResolveDependencies and installWithDependencies
- [x] Remove Ensure* functions from bootstrap.go
- [x] Clean up unused imports
- [x] Verify all tests pass

## Testing Strategy
- All existing tests must pass
- Manual verification: `./tsuku install turbo` should install nodejs as dependency

## Risks and Mitigations
- **Breaking existing installs**: Mitigated by using the same `installWithDependencies()` that already works
- **Path injection**: The refactored code needs to find and inject paths like before

## Success Criteria
- [x] Remove `EnsureNpm()` from npm_install action
- [x] Remove `EnsurePipx()` from pipx_install action
- [x] Remove `EnsureCargo()` from cargo_install action
- [x] Remove other bootstrap helpers from `internal/install/bootstrap.go`
- [x] Actions rely on dependency resolver to ensure deps are installed
- [x] All existing tests pass

## Open Questions
None
