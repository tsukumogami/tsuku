# Issue 946 Implementation Plan

## Summary

Implement library checksum computation at install time by creating a `ComputeLibraryChecksums()` function that walks the library directory, skips symlinks, and computes SHA256 checksums for all regular files. Add a `SetLibraryChecksums()` method to StateManager and integrate checksum computation into `InstallLibrary()`.

## Approach

Follow the established pattern from tool binary checksum implementation but adapt it for library-specific requirements:

1. Libraries need to walk the entire directory (tools use an explicit binary list)
2. Libraries skip symlinks entirely (tools follow symlinks to checksum the real file)
3. Error handling should warn but not fail installation (matching tool behavior)

The approach adds minimal code while reusing `ComputeFileChecksum()` and following the existing `UpdateLibrary()` pattern for state updates.

### Alternatives Considered

- **Reuse ComputeBinaryChecksums() with file list**: Not chosen because it would require first listing all files, then calling the function. The existing function also follows symlinks, which is wrong for libraries (we want to skip symlinks entirely). A dedicated function is cleaner.

- **Add checksums as part of copyDir()**: Not chosen because it couples file copying with checksum computation. Keeping them separate allows for better error isolation and follows the existing pattern where checksums are computed after file operations complete.

## Files to Modify

- `internal/install/checksum.go` - Add `ComputeLibraryChecksums()` function that walks directory, skips symlinks, and computes checksums using existing `ComputeFileChecksum()`
- `internal/install/state_lib.go` - Add `SetLibraryChecksums()` method following the `AddLibraryUsedBy()` pattern
- `internal/install/library.go` - Call `ComputeLibraryChecksums()` after `copyDir()` and store via `SetLibraryChecksums()`

## Files to Create

- None required (all changes fit naturally into existing files)

## Implementation Steps

- [x] Add `ComputeLibraryChecksums(libDir string) (map[string]string, error)` to `checksum.go`
  - Use `filepath.Walk()` to iterate all files in libDir
  - Use `os.Lstat()` to detect symlinks (check `Mode()&os.ModeSymlink != 0`)
  - Skip symlinks, only checksum regular files
  - Compute relative paths from libDir root as map keys
  - Call existing `ComputeFileChecksum()` for each regular file
  - Return map[relativePath]hexChecksum

- [x] Add `SetLibraryChecksums(libName, libVersion string, checksums map[string]string) error` to `state_lib.go`
  - Implement using `UpdateLibrary()` pattern (like `AddLibraryUsedBy`)
  - Set `ls.Checksums = checksums` in the update callback

- [x] Modify `InstallLibrary()` in `library.go` to compute and store checksums
  - After `copyDir()` succeeds, call `ComputeLibraryChecksums(libDir)`
  - Log warning on error but don't fail (matching tool checksum behavior)
  - Call `m.state.SetLibraryChecksums()` to store in state

- [x] Add unit tests for `ComputeLibraryChecksums()` in `checksum_test.go`
  - Test basic directory with regular files
  - Test directory with symlinks (verify symlinks are skipped)
  - Test empty directory (should return empty map)
  - Test nested directory structure (verify relative paths)
  - Test nonexistent directory handling (should return error)

- [x] Add unit test for `SetLibraryChecksums()` in `state_test.go`
  - Verify checksums are stored correctly in state
  - Verify existing library state (UsedBy) is preserved when adding checksums

- [x] Existing InstallLibrary tests verify integration works (checksums are computed and stored)

## Testing Strategy

**Unit tests:**
- `TestComputeLibraryChecksums` - basic functionality with regular files
- `TestComputeLibraryChecksums_WithSymlinks` - verify symlinks are skipped
- `TestComputeLibraryChecksums_EmptyDirectory` - edge case handling
- `TestComputeLibraryChecksums_NestedDirectories` - verify relative path computation
- `TestSetLibraryChecksums` - state management method

**Integration tests:**
- Extend `TestManager_InstallLibrary` to verify checksums are stored in state
- Verify end-to-end that installing a library results in checksums in state.json

**Manual verification:**
- Install a library (e.g., `gcc-libs`) and verify `state.json` contains checksums under `libs.gcc-libs.<version>.checksums`
- Verify symlinked files (common in library packages) are not included in checksums

## Risks and Mitigations

- **Large libraries with many files could slow installation**: Mitigated by logging a warning on checksum errors without failing installation. SHA256 computation is fast (~100-300MB/s), so even large libraries (Qt at ~200MB) complete in <1 second.

- **Path handling inconsistencies across platforms**: Mitigated by using `filepath.Rel()` for computing relative paths, which handles OS-specific path separators correctly.

- **Existing tests may need state structure updates**: Unlikely since `Checksums` field was already added by #942 with `omitempty`, so existing tests should pass unchanged.

## Success Criteria

- [ ] `go test ./internal/install/...` passes with new tests
- [ ] `go build ./cmd/tsuku` succeeds
- [ ] Installing a library stores checksums in `state.json` under `libs.<name>.<version>.checksums`
- [ ] Checksum keys are relative paths (e.g., `lib/libstdc++.so.6.0.33`)
- [ ] Symlinks are not included in checksums (only real files)
- [ ] Checksum computation errors log warnings but do not fail installation

## Open Questions

None. The issue specification is complete and aligns with prior work from #942 and #943.
