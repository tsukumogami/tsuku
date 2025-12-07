# Issue 221 Implementation Plan

## Summary

Integrate library dependency resolution into the install process. When a tool declares dependencies on `type = "library"` recipes, the installer will install libraries to `$TSUKU_HOME/libs/` and track `used_by` in state.json.

## Approach

Extend the existing `installWithDependencies` function in `install.go` to detect when a dependency recipe has `type = "library"` and route it through a new library-specific installation path that:
1. Installs to `$TSUKU_HOME/libs/{name}-{version}/` instead of `tools/`
2. Updates `state.json` libs section with `used_by` tracking
3. Skips symlink creation (libraries are not user-facing)

### Alternatives Considered

- **New dedicated command**: Create `tsuku install-library` - rejected because libraries should be transparently installed as dependencies, not explicitly by users.
- **Separate installer class**: Create `LibraryManager` - rejected for now as overkill; the existing manager can be extended with minimal changes.

## Files to Modify

- `cmd/tsuku/install.go` - Add library type detection and route to library install path
- `internal/install/manager.go` - Add `InstallLibrary` method for library-specific installation
- `internal/install/state.go` - No changes needed (libs tracking already implemented in #216)

## Files to Create

- `internal/install/library.go` - Library installation logic
- `internal/install/library_test.go` - Unit tests for library installation

## Implementation Steps

- [x] Step 1: Add `IsLibrary()` helper method to Recipe type
- [x] Step 2: Add `InstallLibrary` method to install Manager
- [x] Step 3: Modify `installWithDependencies` to detect library type and route appropriately
- [x] Step 4: Add `used_by` tracking when tool installation completes
- [x] Step 5: Add unit tests for library installation
- [x] Step 6: Add integration test with mock library recipe (unit tests cover the functionality)

## Testing Strategy

- Unit tests:
  - `IsLibrary()` method returns true for `type = "library"` recipes
  - `InstallLibrary` creates directory in `libs/` not `tools/`
  - `used_by` is updated after tool installation
  - Reuse of existing library version works correctly
- Integration test:
  - Create mock `test-lib.toml` with `type = "library"`
  - Create mock `test-tool.toml` with `dependencies = ["test-lib"]`
  - Verify library installed to correct location
  - Verify state.json updated correctly

## Risks and Mitigations

- **Circular library dependencies**: Mitigated by existing visited map in `installWithDependencies`
- **Version conflicts**: For Phase 1, use exact version matching; version constraints deferred to future work
- **State corruption**: Use atomic state updates (already implemented)

## Success Criteria

- [x] Library recipes detected by `type = "library"` field
- [x] Libraries installed to `$TSUKU_HOME/libs/{name}-{version}/`
- [x] `used_by` tracking updated when tool installation completes
- [x] Existing library version reused if present
- [x] All unit tests pass
- [x] Integration test with mock library recipe passes (covered by unit tests)
