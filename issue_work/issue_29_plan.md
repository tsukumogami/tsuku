# Issue 29 Implementation Plan

## Summary

Implement atomic installation by using a staging directory pattern: install to a temporary location first, then atomically rename to the final location. On failure, clean up the staging directory and leave any previous installation intact.

## Approach

Use the existing temp work directory for recipe execution, then copy to a staging directory (`~/.tsuku/tools/.{name}-{version}.staging/`) before atomically renaming to the final location. This ensures the final tool directory is either fully installed or not present at all.

### Alternatives Considered

- **Manifest-based rollback**: Track all created files and remove on failure. More complex, requires additional state tracking, and still leaves a window where partial files exist.
- **Copy directly with cleanup on failure**: Current approach - has a race window where partial directory exists. Not chosen because it doesn't provide true atomicity.
- **Soft links to versioned directories**: Would require restructuring the entire tools directory layout. Too invasive for this change.

## Files to Modify

- `internal/install/manager.go` - Change `InstallWithOptions()` to use staging directory pattern with atomic rename
- `internal/install/manager_test.go` - Add tests for atomic installation behavior

## Files to Create

None - changes are contained within existing manager.go

## Implementation Steps

- [ ] Add staging directory support to `InstallWithOptions()` - copy to `.staging` suffix, then atomic rename
- [ ] Handle existing staging directories (cleanup stale staging dirs from previous failed installs)
- [ ] Add rollback on symlink creation failure - remove newly installed directory if symlinks fail
- [ ] Handle update scenario - preserve old version until new version fully installed
- [ ] Add tests for atomic installation scenarios (success, failure mid-copy, failure at symlink)

## Testing Strategy

- Unit tests:
  - Successful installation creates final directory atomically
  - Failed copy leaves no partial directory
  - Stale staging directories are cleaned up
  - Update preserves old version until new is ready
- Manual verification:
  - Install a tool, verify directory structure
  - Simulate failure (e.g., permissions), verify cleanup

## Risks and Mitigations

- **Cross-filesystem rename fails**: `os.Rename()` fails across filesystems. Mitigated by staging directory being in same parent as final directory.
- **Stale staging directories accumulate**: Add cleanup of old staging directories at start of install.
- **Symlink failure after directory rename**: Directory exists but not usable. Mitigate by removing directory if symlink creation fails.

## Success Criteria

- [ ] Installation to final directory is atomic (rename, not copy)
- [ ] Failed installations leave no partial directories
- [ ] Existing installations preserved until new version fully ready
- [ ] All tests pass
- [ ] Manual test: interrupt installation, verify no orphaned directories

## Open Questions

None - the staging directory pattern is well-established and fits the existing architecture.
