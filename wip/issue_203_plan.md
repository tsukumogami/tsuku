# Issue 203 Implementation Plan

## Summary
Implement post-install checksum pinning by adding SHA256 computation for installed binaries, storing checksums in state.json, and extending `tsuku verify` to detect tampering.

## Approach
Follow the design document `docs/DESIGN-checksum-pinning.md`. The implementation reuses existing SHA256 infrastructure from `internal/actions/util.go` and extends the state schema with a new `BinaryChecksums` field. The verify command gains a third verification step for binary integrity.

### Alternatives Considered
- **Checksum all files**: Rejected - too slow, binaries are primary attack surface
- **Store in Plan**: Rejected - Plan is for download checksums; binary checksums are post-install artifacts
- **External checksum file**: Rejected - state.json already handles per-version metadata

## Files to Modify
- `internal/install/state.go` - Add `BinaryChecksums` field to `VersionState`
- `internal/install/manager.go` - Compute checksums after install, before state save
- `cmd/tsuku/verify.go` - Add integrity verification step

## Files to Create
- `internal/install/checksum.go` - `ComputeBinaryChecksums()` and `VerifyBinaryChecksums()` functions
- `internal/install/checksum_test.go` - Unit tests for checksum functions

## Implementation Steps
- [ ] Add `BinaryChecksums` field to `VersionState` in `state.go`
- [ ] Create `internal/install/checksum.go` with `ComputeFileChecksum()` helper
- [ ] Implement `ComputeBinaryChecksums(toolDir, binaries)` to compute SHA256 for all binaries
- [ ] Implement `VerifyBinaryChecksums(toolDir, stored)` to compare computed vs stored
- [ ] Integrate checksum computation into `Manager.InstallWithOptions()` after install
- [ ] Update `InstallOptions` to expose checksums or compute inline
- [ ] Add integrity verification step to `verifyVisibleTool()` in verify.go
- [ ] Add integrity verification step to `verifyWithAbsolutePath()` in verify.go
- [ ] Add unit tests for checksum.go functions
- [ ] Add backward compatibility test (old state without checksums)
- [ ] Update documentation in `tsuku verify --help`

## Testing Strategy
- Unit tests:
  - `ComputeFileChecksum()` - normal file, missing file, permission denied
  - `ComputeBinaryChecksums()` - multiple binaries, symlinks, empty list
  - `VerifyBinaryChecksums()` - all match, mismatch, missing file
  - State JSON serialization with `BinaryChecksums` field

- Integration tests:
  - Install tool, verify checksums stored in state.json
  - Verify command reports integrity status

- Backward compatibility:
  - Load old state.json without `binary_checksums` field

## Risks and Mitigations
- **Performance on large binaries**: SHA256 is fast (~500MB/s); typical binaries are <100MB. Acceptable overhead.
- **Symlink resolution**: Binary paths may include symlinks; resolve to real files before hashing.
- **State migration**: New field is optional (`omitempty`); old state files work without checksums.

## Success Criteria
- [ ] `tsuku install <tool>` stores binary checksums in state.json
- [ ] `tsuku verify <tool>` reports "Integrity: OK" for unmodified binaries
- [ ] `tsuku verify <tool>` reports "Integrity: MODIFIED" when binary is changed
- [ ] `tsuku verify <tool>` reports "Integrity: SKIPPED" for pre-feature installations
- [ ] All tests pass including backward compatibility
- [ ] No coverage regression

## Open Questions
None - design document approved.
