# Issue 978 Introspection

## Context Reviewed
- Design doc: `docs/designs/DESIGN-library-verify-deps.md`
- Sibling issues reviewed: #979 (closed)
- Prior patterns identified:
  - `SetLibraryChecksums()` helper pattern in `state_lib.go`
  - `UpdateLibrary()` atomic update pattern
  - `omitempty` JSON tag for optional fields
  - Table-driven tests for state operations

## Gap Analysis

### Minor Gaps

1. **Test file location**: Issue specifies tests go in `state_test.go`, but library state tests are already in that file alongside tool state tests. The existing pattern shows library-specific tests grouped together (lines 733-1168 in `state_test.go`). The new `Sonames` tests should follow this pattern.

2. **Backward compatibility test pattern**: Issue proposes a test for old state format, but existing tests (e.g., `TestLibraryVersionState_Checksums_BackwardCompatibility`) show the established pattern - write JSON without the new field, load, verify no errors and field is nil.

3. **Helper method pattern**: The `SetLibrarySonames()` helper should follow the exact pattern of `SetLibraryChecksums()` in `state_lib.go`:
   ```go
   func (sm *StateManager) SetLibrarySonames(libName, libVersion string, sonames []string) error {
       return sm.UpdateLibrary(libName, libVersion, func(ls *LibraryVersionState) {
           ls.Sonames = sonames
       })
   }
   ```

4. **`omitempty` behavior**: The `omitempty` tag is correct for optional fields. Existing tests verify that nil/empty maps are omitted from JSON (see `TestLibraryVersionState_Checksums_OmitsEmpty`). A similar test should be added for Sonames.

### Moderate Gaps

None identified. The issue spec is complete and aligns with established patterns.

### Major Gaps

None identified. The implementation approach matches the codebase conventions.

## Recommendation

**Proceed**

The issue specification is well-aligned with the current codebase. The closed sibling issue #979 (IsExternallyManaged) established patterns in a different package (actions), so there are no conflicting patterns to reconcile. The state management patterns in `state_lib.go` provide clear templates for the new `SetLibrarySonames()` helper.

## Implementation Notes

Follow these patterns from the existing codebase:

1. Add `Sonames` field to `LibraryVersionState` struct (lines 95-99 in `state.go`)
2. Add `SetLibrarySonames()` helper to `state_lib.go` following `SetLibraryChecksums()` pattern (lines 52-56)
3. Add tests to `state_test.go` in the "Library state tests" section, following existing patterns:
   - Serialization round-trip test
   - Backward compatibility test (load old JSON without field)
   - `omitempty` test (verify nil slice is omitted)
   - Helper method test
