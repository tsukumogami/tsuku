# Issue 946 Introspection

## Context Reviewed

- Design doc: `/home/dgazineu/dev/workspace/tsuku/tsuku-6/public/tsuku/docs/designs/DESIGN-library-verification.md`
- Sibling issues reviewed: #942, #943
- Prior patterns identified:
  - `LibraryVersionState.Checksums` field added in #942 (PR #958)
  - Library type detection and flag routing added in #943 (PR #959)
  - `StateManager.UpdateLibrary()` pattern for atomic state updates
  - `ComputeFileChecksum()` and `ComputeBinaryChecksums()` exist in `checksum.go`

## Gap Analysis

### Minor Gaps

1. **SetLibraryChecksums method location**: The issue spec asks for `SetLibraryChecksums(name, version string, checksums map[string]string)` in `state_lib.go`. The existing `UpdateLibrary()` function provides a generic callback pattern that can be used instead:
   ```go
   sm.UpdateLibrary(name, version, func(ls *LibraryVersionState) {
       ls.Checksums = checksums
   })
   ```
   However, adding a dedicated `SetLibraryChecksums` method is cleaner and matches the acceptance criteria. This is a minor implementation choice - follow the spec.

2. **Checksum computation function naming**: The spec asks for `ComputeLibraryChecksums(libDir string)`. The existing `ComputeBinaryChecksums(toolDir, binaries)` takes explicit binary paths, while library checksums need to walk the entire directory. This is a design difference, not a gap.

3. **Symlink skipping**: The spec explicitly requires skipping symlinks during checksum computation. The existing `ComputeBinaryChecksums()` follows symlinks to the real file. For libraries, we should skip symlinks entirely (only checksum real files). This is correctly specified in the issue.

### Moderate Gaps

None identified. The issue spec is well-defined and aligns with prior work.

### Major Gaps

None identified.

## Recommendation

**Proceed**

The issue specification is complete and accurate. The two closed sibling issues (#942, #943) created exactly the foundation this issue expects:

1. **#942** added `Checksums map[string]string` to `LibraryVersionState` with correct JSON tag `json:"checksums,omitempty"` - exactly as issue #946 requires
2. **#943** added library verification routing with stub implementation - the stub explicitly notes full verification comes later and stores `libState` for future integrity use

The implementation notes in issue #946 align with actual code structure:
- `state.go` has the schema (from #942)
- `state_lib.go` has `UpdateLibrary()` pattern for atomic updates
- `checksum.go` has `ComputeFileChecksum()` for reuse
- `library.go` has `InstallLibrary()` where checksum computation should be added

## Proposed Amendments

None required. The issue specification accurately reflects the current state of the codebase after sibling issue completion.

## Implementation Notes from Prior Work

For implementation, follow these patterns established by sibling issues:

1. **State update pattern** (from `state_lib.go`):
   ```go
   func (sm *StateManager) SetLibraryChecksums(libName, libVersion string, checksums map[string]string) error {
       return sm.UpdateLibrary(libName, libVersion, func(ls *LibraryVersionState) {
           ls.Checksums = checksums
       })
   }
   ```

2. **Checksum computation** (adapt from `checksum.go:ComputeBinaryChecksums`):
   - Use `ComputeFileChecksum()` for individual files
   - Walk directory recursively with `filepath.Walk`
   - Skip symlinks using `os.Lstat` and checking `Mode()&os.ModeSymlink`
   - Use relative paths as map keys (from library directory root)

3. **Error handling** (per issue spec): Log warnings on checksum errors but don't fail installation, matching tool behavior.
