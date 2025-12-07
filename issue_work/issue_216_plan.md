# Issue 216 Implementation Plan

## Summary

Add a `Libs` section to state.json for tracking installed libraries and which tools depend on them via `used_by` arrays.

## Approach

Extend the existing State struct with a `Libs` field containing a nested map structure: `map[libName]map[version]LibraryState`. Add helper methods for managing library state similar to the existing tool state methods.

### Alternatives Considered

- **Separate libs.json file**: Rejected - splitting state increases complexity and doesn't align with how tools are tracked
- **Flat library-version key**: Rejected - nested structure matches design doc and enables efficient queries by library name

## Files to Modify

- `internal/install/state.go` - Add LibraryState struct, Libs field to State, and helper methods
- `internal/install/state_test.go` - Add tests for new library state functionality

## Files to Create

None

## Implementation Steps

- [ ] Add LibraryState struct with UsedBy field
- [ ] Add Libs field to State struct
- [ ] Initialize Libs map in Load() for backward compatibility
- [ ] Add UpdateLibrary method for updating library state
- [ ] Add AddLibraryUsedBy method for adding a dependent tool
- [ ] Add RemoveLibraryUsedBy method for removing a dependent tool
- [ ] Add RemoveLibrary method for removing a library version
- [ ] Add unit tests for all new methods
- [ ] Add backward compatibility test for state.json without libs section

## Testing Strategy

- **Unit tests**: Verify LibraryState CRUD operations, UsedBy tracking, backward compatibility with existing state.json

## Risks and Mitigations

- **Backward compatibility**: Mitigated by initializing Libs map to empty in Load() when not present
- **JSON omitempty**: Need to ensure empty libs section doesn't clutter state.json

## Success Criteria

- [ ] `libs` section added to state.json schema
- [ ] `used_by` array tracks dependent tools per library version
- [ ] State persistence and loading works correctly
- [ ] Backward compatible: existing state.json without `libs` section loads correctly
- [ ] Unit tests for state operations

## Open Questions

None
