# Issue 978 Implementation Plan

## Summary

Add a `Sonames []string` field to `LibraryVersionState` struct with JSON `omitempty` tag, plus a `SetLibrarySonames()` helper method following the established `SetLibraryChecksums()` pattern.

## Approach

This is a straightforward addition that mirrors the existing `Checksums` field and helper pattern. The `omitempty` tag ensures backward compatibility with existing state files that don't have the field.

### Alternatives Considered

- **Store sonames in separate state file**: Rejected; inconsistent with existing library state pattern and adds complexity.
- **Store as map[string]string (filename -> soname)**: Rejected; the design doc specifies `[]string` since the mapping can be reconstructed from binaries. A simple list suffices for the SonameIndex downstream.

## Files to Modify

- `internal/install/state.go` - Add `Sonames []string` field to `LibraryVersionState` struct (line 96-99)
- `internal/install/state_lib.go` - Add `SetLibrarySonames()` helper method (after line 56)
- `internal/install/state_test.go` - Add tests in library state section (after line 1239)

## Files to Create

None.

## Implementation Steps

- [x] Add `Sonames []string` field with `json:"sonames,omitempty"` tag to `LibraryVersionState` struct in `state.go`
- [x] Add `SetLibrarySonames(libName, libVersion string, sonames []string) error` helper to `state_lib.go`
- [x] Add `TestLibraryVersionState_Sonames_SaveAndLoad` test for serialization round-trip
- [x] Add `TestLibraryVersionState_Sonames_BackwardCompatibility` test for loading old state format
- [x] Add `TestLibraryVersionState_Sonames_OmitsEmpty` test for nil slice omission in JSON
- [x] Add `TestStateManager_SetLibrarySonames` test for helper method
- [x] Add `TestStateManager_SetLibrarySonames_UpdatesExisting` test for preserving other fields
- [x] Run `go vet ./...` and `go test -short ./...`
- [x] Run full `go test ./...` to verify no regressions

## Testing Strategy

- **Unit tests**: Table-driven tests following existing patterns in `state_test.go`:
  - Serialization round-trip (save state with Sonames, load, verify)
  - Backward compatibility (load JSON without sonames field, verify Sonames is nil)
  - `omitempty` behavior (verify nil slice is omitted from JSON output)
  - Helper method (SetLibrarySonames creates new entry with sonames)
  - Helper preserves existing fields (SetLibrarySonames on library with UsedBy preserves UsedBy)

- **Integration tests**: None required; this is pure state management.

- **Manual verification**: None required; tests provide full coverage.

## Risks and Mitigations

- **JSON key collision**: Mitigated by using `sonames` which is not used elsewhere in state.
- **nil vs empty slice behavior**: Mitigated by using `omitempty` which omits both nil and empty slices.

## Success Criteria

- [x] `LibraryVersionState` struct has `Sonames []string` field with `json:"sonames,omitempty"` tag
- [x] `SetLibrarySonames()` helper method exists and follows `SetLibraryChecksums()` pattern
- [x] All new tests pass
- [x] Existing tests pass (no regression)
- [x] `go vet ./...` produces no warnings
- [x] `go build ./...` succeeds

## Open Questions

None.
