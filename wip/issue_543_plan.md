# Issue 543 Implementation Plan

## Summary

Create three validation scripts for build essentials that work on both Linux and macOS: verify-relocation.sh, verify-tool.sh, and verify-no-system-deps.sh.

## Approach

Create portable shell scripts that use platform-specific tools (readelf/ldd on Linux, otool on macOS) to validate binary relocatability and dependency isolation. The scripts will be standalone utilities that can be called from CI or locally.

### Alternatives Considered

- Go-based validation: Would be more portable but adds complexity and binary parsing requirements. Shell scripts are simpler for this use case and align with existing scripts/*.sh pattern.
- Combined single script: Rejected because each validation concern is distinct and CI may want to run them independently.

## Files to Create

- `scripts/verify-relocation.sh` - Checks RPATH/install_name for hardcoded paths
- `scripts/verify-tool.sh` - Runs tool-specific functional tests
- `scripts/verify-no-system-deps.sh` - Verifies only tsuku/system libc deps

## Implementation Steps

- [ ] Create verify-relocation.sh with Linux (readelf) and macOS (otool) support
- [ ] Create verify-tool.sh with tool-specific test cases
- [ ] Create verify-no-system-deps.sh to check library dependencies
- [ ] Update build-essentials.yml to use the new scripts
- [ ] Update design doc to mark issues 542, 546, 543 as complete

## Testing Strategy

- Manual: Run each script against installed tools (make, gdbm, zig)
- CI: Scripts will be exercised by build-essentials workflow
- Cross-platform: Test on both Linux and macOS locally if possible

## Risks and Mitigations

- Platform detection may have edge cases: Use robust uname checks
- Tool paths may vary: Use $TSUKU_HOME with sensible defaults

## Success Criteria

- [ ] verify-relocation.sh detects hardcoded paths in binaries
- [ ] verify-tool.sh validates tool functionality
- [ ] verify-no-system-deps.sh confirms dependency isolation
- [ ] Scripts work on Linux (readelf/ldd) and macOS (otool)
- [ ] CI uses scripts for build essential validation

## Open Questions

None - requirements are clear from design doc.
