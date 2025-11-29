# Issue 93 Implementation Plan

## Summary

Implement a Recipe Writer component (`internal/recipe/writer.go`) that serializes Recipe structs to TOML format with atomic file operations using write-temp-rename pattern.

## Approach

Use `github.com/BurntSushi/toml` for encoding (already a dependency). Implement atomic writes by:
1. Writing to a temp file in the same directory (ensures same filesystem for rename)
2. Syncing the file to disk
3. Atomically renaming to the final path

The Step struct has custom UnmarshalTOML but no MarshalTOML - I'll need to add MarshalTOML to ensure proper TOML encoding (reconstructing the flat map structure from Action/When/Params fields).

### Alternatives Considered
- **Direct file write without atomic pattern**: Rejected due to risk of partial writes corrupting recipes
- **Using a different TOML library**: Rejected since BurntSushi/toml is already used and supports both encoding/decoding

## Files to Create
- `internal/recipe/writer.go` - Writer implementation with atomic file operations
- `internal/recipe/writer_test.go` - Unit tests for writer

## Files to Modify
- `internal/recipe/types.go` - Add `MarshalTOML` method to Step struct for proper TOML encoding

## Implementation Steps
- [ ] Add MarshalTOML method to Step struct in types.go
- [ ] Create writer.go with Writer struct and Write function
- [ ] Implement atomic write pattern (temp file, sync, rename)
- [ ] Add unit tests for successful writes
- [ ] Add unit tests for round-trip (write then read)
- [ ] Add unit tests for atomic behavior (no partial files on error)
- [ ] Run full test suite and verify build

## Testing Strategy
- **Unit tests**:
  - Test Write function succeeds with valid recipe
  - Test round-trip: write recipe, read it back, verify equivalence
  - Test atomic behavior: simulate failure mid-write, verify no partial file exists
  - Test error handling: permission denied, directory doesn't exist
- **Manual verification**: Create a recipe file and verify it can be loaded by existing Loader

## Risks and Mitigations
- **TOML encoding may differ from hand-written format**: Mitigation: round-trip test ensures output is parseable. Exact formatting is less important than correctness.
- **Step.Params uses interface{} which may not serialize correctly**: Mitigation: Test with various action types to verify complex param structures serialize properly.

## Success Criteria
- [ ] `WriteRecipe(recipe *Recipe, path string) error` function implemented
- [ ] Atomic write pattern used (write-tmp-rename)
- [ ] TOML output is valid and parseable by existing recipe loader
- [ ] Unit tests for successful writes
- [ ] Unit tests for atomic behavior (no partial files on error)
- [ ] All existing tests still pass
- [ ] Build succeeds

## Open Questions
None - requirements are clear from issue and design document.
