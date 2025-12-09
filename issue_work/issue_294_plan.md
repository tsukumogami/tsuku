# Issue 294 Implementation Plan

## Summary

Add multi-version support to state.json by introducing a `VersionState` struct and migrating `ToolState` from single `Version` field to `ActiveVersion` + `Versions` map, with automatic migration of old state files.

## Approach

Extend the existing state management patterns:
- Add `VersionState` struct for per-version metadata
- Modify `ToolState` to use `active_version` and `versions` map
- Detect old format in `Load()` and migrate automatically (like existing backward-compat patterns)
- Add version string validation to prevent path traversal attacks

### Alternatives Considered
- **Separate migration command**: Rejected - automatic migration is simpler and follows existing pattern (see `BackwardCompatibility_NoLibsSection` test)
- **Keep old field names**: Rejected - `version` vs `active_version` clearly indicates semantic change

## Files to Modify
- `internal/install/state.go` - Add VersionState, modify ToolState, add migration logic, add validation
- `internal/install/state_test.go` - Add tests for migration, validation, new fields

## Files to Create
None - all changes fit in existing files

## Transition Strategy

Since other code (hidden.go, manager.go, remove.go) still uses `ToolState.Version`, we need a careful transition:

1. **Add new fields alongside old** - `ActiveVersion` and `Versions` added to `ToolState`
2. **Keep old `Version` field** - Mark as deprecated, use `omitempty` so it's not written
3. **Migration in Load()** - If `Version` is set but `ActiveVersion` is not, migrate
4. **Later issues update callers** - #298, #299, etc. will update code to use `ActiveVersion`
5. **Final cleanup** - Remove deprecated `Version` field once all code migrated

## Implementation Steps
- [ ] Add `VersionState` struct with `Requested`, `Binaries`, `InstalledAt` fields
- [ ] Modify `ToolState` to add `ActiveVersion` and `Versions` fields (keep `Version` as deprecated)
- [ ] Add `ValidateVersionString()` function to reject path traversal characters
- [ ] Modify `Load()` to detect old format and migrate (set `ActiveVersion` from `Version`)
- [ ] Add unit tests for migration (old format → new format)
- [ ] Add unit tests for version string validation
- [ ] Update existing tests to use new fields where appropriate

Mark each step [x] after it is implemented and committed. This enables clear resume detection.

## Testing Strategy
- Unit tests: Migration from old to new format, validation of version strings
- Backward compatibility: Ensure old state.json files are migrated correctly
- Edge cases: Empty state, corrupted state, version strings with special characters

## Risks and Mitigations
- **Breaking existing code**: Other files use `ToolState.Version` - need to check all usages and update
- **Data loss on migration**: Migration preserves all data, just restructures it

## Success Criteria
- [ ] `VersionState` struct exists with correct JSON tags
- [ ] `ToolState` has `ActiveVersion` and `Versions` fields
- [ ] Old state.json files migrate automatically on Load()
- [ ] Version strings with `..`, `/`, `\` are rejected
- [ ] All existing tests pass
- [ ] New tests cover migration and validation
