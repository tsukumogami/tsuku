# Issue 239 Implementation Plan

## Summary

Add `InstallDependencies` and `RuntimeDependencies` fields to `ToolState` struct and record them during installation, enabling dependency tree display and uninstall warnings.

## Approach

The implementation extends the existing state tracking infrastructure with two new fields per tool. Dependencies are resolved using the existing `actions.ResolveDependencies()` function and recorded during installation. Backward compatibility is maintained through proper JSON handling of optional fields.

### Alternatives Considered
- **Compute dependencies on-demand from recipes**: Would require loading recipes every time, slower and may fail for removed recipes
- **Store only runtime deps (already partially implemented)**: Incomplete - install deps needed for `tsuku info` tree display

## Files to Modify
- `internal/install/state.go` - Add `InstallDependencies` and `RuntimeDependencies` fields to `ToolState`
- `cmd/tsuku/install.go` - Record dependencies during installation using `ResolveDependencies()`
- `internal/install/state_test.go` - Add tests for new fields

## Implementation Steps
- [ ] Add InstallDependencies and RuntimeDependencies fields to ToolState struct
- [ ] Update installation flow to record both dependency types
- [ ] Add unit tests for state structure with dependencies
- [ ] Test backward compatibility with existing state.json files

## Testing Strategy
- Unit tests: Verify ToolState serialization/deserialization with new fields
- Unit tests: Verify backward compatibility (old state.json without dep fields loads correctly)
- Manual verification: Install a tool with dependencies and check state.json

## Risks and Mitigations
- **Backward compatibility**: JSON `omitempty` ensures old state files load without errors
- **Circular state updates**: Dependencies already resolved before state update, no risk

## Success Criteria
- [ ] state.json includes `install_dependencies` array per tool
- [ ] state.json includes `runtime_dependencies` array per tool
- [ ] Dependencies recorded at install time
- [ ] Existing state.json files load without errors (backward compat)
- [ ] Unit tests verify state structure

## Open Questions
None - straightforward extension of existing infrastructure.
