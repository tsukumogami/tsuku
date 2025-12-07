# Issue 239 Summary

## What Was Implemented

Added `InstallDependencies` and `RuntimeDependencies` fields to `ToolState` struct, enabling dependency tracking in state.json. Dependencies are resolved during installation using the existing `ResolveDependencies` algorithm and recorded per tool.

## Changes Made
- `internal/install/state.go`: Added `InstallDependencies` and `RuntimeDependencies` fields to `ToolState` struct with JSON tags and omitempty
- `cmd/tsuku/install.go`: Updated installation flow to resolve dependencies and record them in state during `UpdateTool` call
- `internal/install/state_test.go`: Added 3 tests for dependency tracking and backward compatibility

## Key Decisions
- **Use dependency names only (not versions)**: State tracks dependency names as strings. Versions are resolved dynamically when needed (e.g., for wrapper generation), keeping state simpler and avoiding version drift
- **Reuse existing ResolveDependencies**: No new resolution code needed - the central algorithm from issue #235 handles all dependency resolution

## Trade-offs Accepted
- **Dependencies stored without versions**: Acceptable because installed versions are looked up from state when needed (e.g., for wrapper scripts)
- **No transitive deps in state**: Direct dependencies only - transitive resolution happens at read time if needed

## Test Coverage
- New tests added: 3
  - `TestStateManager_SaveAndLoad_WithDependencies`
  - `TestStateManager_BackwardCompatibility_NoDependencyFields`
  - `TestStateManager_UpdateTool_WithDependencies`

## Known Limitations
- Existing installations don't have dependency fields until re-installed or updated
- Migration happens automatically on next install/update as documented in acceptance criteria

## Future Improvements
None - this is a foundational change that enables #240 (info display) and #241 (uninstall warnings)
