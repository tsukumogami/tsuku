# Issue 1965 Implementation Plan

## Summary

Add dependency library RPATH entries to Mach-O tool binaries during `homebrew_relocate`, mirroring what `fixLibraryDylibRpaths` already does for library dylibs. The fix threads dependency info into `fixMachoRpath` so it can add absolute RPATHs pointing to `$TSUKU_HOME/libs/<dep>-<version>/lib` for each install-time dependency.

## Approach

The `fixMachoRpath` function currently only adds a self-relative `@loader_path/../lib` RPATH, which lets binaries find their own sibling libraries but not libraries from dependencies installed elsewhere. The `fixLibraryDylibRpaths` function already solves this for library dylibs by iterating over `ctx.Dependencies.InstallTime` and adding absolute RPATHs to each dependency's lib dir. The fix applies the same pattern to tool binaries processed by `fixMachoRpath`.

This approach was chosen because:
- It mirrors existing, working code (`fixLibraryDylibRpaths`)
- It keeps the fix inside `homebrew_relocate.go` where all RPATH logic lives
- It works at install time so no runtime overhead (unlike wrapper scripts)
- RPATHs baked into the binary are the correct macOS mechanism for dylib resolution

### Alternatives Considered

- **Wrapper scripts with DYLD_LIBRARY_PATH**: The existing wrapper mechanism in `manager.go` only sets `PATH` for runtime deps, not `DYLD_LIBRARY_PATH`. Adding library path support to wrappers would work but has downsides: SIP strips `DYLD_LIBRARY_PATH` from child processes on macOS, making it unreliable. Also, `DYLD_LIBRARY_PATH` affects all child processes, not just the target binary, which can cause unexpected side effects.
- **`link_dependencies` action to copy/symlink dylibs into tool's own lib/**: This would place dependency dylibs alongside the tool's own libs so the existing `@loader_path/../lib` RPATH finds them. However, this creates duplicate copies of shared libraries and complicates upgrades (changing a library version requires updating all tools that use it). The RPATH approach is cleaner.
- **Fix only in `fixElfRpath` (Linux) too**: The issue specifically targets macOS, but the Linux ELF path has the same gap. However, Linux uses `LD_LIBRARY_PATH` which is less restrictive, and patchelf supports colon-separated multi-path RPATHs natively. Linux could be fixed in a follow-up. This plan focuses on macOS per the issue scope.

## Files to Modify

- `internal/actions/homebrew_relocate.go` - Main changes:
  - Change `fixMachoRpath` signature to accept `*ExecutionContext` (like `fixElfRpath` already does) so it can access dependency info
  - Add dependency lib RPATHs inside `fixMachoRpath` after the self-relative RPATH
  - Update `fixBinaryRpath` call site to pass `ctx` to `fixMachoRpath`
  - Remove debug print statements that are no longer needed (cleanup)
- `internal/actions/homebrew_test.go` - Add tests for the new RPATH behavior

## Files to Create

None.

## Implementation Steps

- [ ] Change `fixMachoRpath` signature from `(binaryPath, installPath string)` to `(ctx *ExecutionContext, binaryPath, installPath string)` to match `fixElfRpath` and give it access to dependency information
- [ ] Update the call site in `fixBinaryRpath` (line 303) to pass `ctx` to `fixMachoRpath`
- [ ] After the existing self-relative RPATH addition in `fixMachoRpath` (around line 508), add a new block that iterates over `ctx.Dependencies.InstallTime` and adds an absolute RPATH to each dependency's lib directory (`ctx.LibsDir/<dep>-<version>/lib`), using the same `install_name_tool -add_rpath` pattern and duplicate-error suppression already used in `fixLibraryDylibRpaths`
- [ ] Add `@rpath`-based library reference rewriting for dependency libraries (not just HOMEBREW-placeholder references): use `otool -L` to find references to absolute paths under `ctx.LibsDir` and rewrite them to `@rpath/<basename>` so the RPATH can resolve them
- [ ] Re-sign modified binaries on arm64 (existing code already does this at end of `fixMachoRpath`, just verify it runs after the new RPATH additions)
- [ ] Add unit test: `TestHomebrewRelocateAction_FixMachoRpath_IncludesDependencyPaths` -- verify that when `ExecutionContext.Dependencies.InstallTime` and `LibsDir` are populated, `fixMachoRpath` attempts to add RPATHs for each dependency (can mock the tool calls since we're on Linux CI)
- [ ] Add unit test: `TestHomebrewRelocateAction_FixMachoRpath_NoDepsNoop` -- verify that when there are no dependencies, behavior is unchanged (regression guard)
- [ ] Clean up debug print statements in `fixLibraryDylibRpaths` and `relocatePlaceholders` that were added during development
- [ ] Run `go test ./internal/actions/...` and `go vet ./...` to verify

## Testing Strategy

- **Unit tests**: Test that `fixMachoRpath` with populated dependencies calls `install_name_tool -add_rpath` for each dependency lib path. Since CI runs on Linux (no `install_name_tool`), tests verify the logic path (function doesn't error when tool is missing, prints warning). On macOS, tests would exercise the full path.
- **Unit tests (regression)**: Test that `fixMachoRpath` without dependencies produces the same output as before the change.
- **Manual verification on macOS**: Install fontconfig with tsuku on macOS, then run `otool -l $(which fc-list) | grep -A2 LC_RPATH` to verify RPATHs include both `@loader_path/../lib` and the absolute gettext/freetype lib paths. Run `fc-list --version` to verify the binary loads correctly.
- **Manual verification (Linux)**: Verify existing Linux homebrew recipe installs still work (this change doesn't touch `fixElfRpath`, so it should be a no-op on Linux).

## Risks and Mitigations

- **Absolute RPATHs break if library is upgraded**: If gettext is upgraded to a new version, the absolute path baked into fontconfig's binaries becomes stale. Mitigation: This is already the case for `fixLibraryDylibRpaths` on library dylibs and is accepted behavior -- upgrading a library requires reinstalling dependent tools. This is tracked in state.json's `used_by` field.
- **install_name_tool not available**: The function already handles this gracefully (prints warning, returns nil). No change needed.
- **Binary re-signing fails**: On Apple Silicon, modified binaries must be re-signed. The existing code already handles this at the end of `fixMachoRpath`. The new RPATH additions happen before the re-signing step, so no additional handling needed.
- **Large number of dependencies**: Each dependency adds one RPATH entry. For tools with many transitive library deps, the binary could accumulate many RPATHs. In practice, Homebrew recipes rarely have more than 5-10 library deps, so this is not a concern.

## Success Criteria

- [ ] `fixMachoRpath` adds dependency lib RPATHs when `ctx.Dependencies.InstallTime` is populated
- [ ] Existing behavior preserved when no dependencies are present
- [ ] Unit tests pass on Linux CI
- [ ] `go vet` and `go test ./...` pass clean
- [ ] On macOS: `fc-list --version` succeeds after installing fontconfig (manual verification)

## Open Questions

None -- the approach directly mirrors existing working code in `fixLibraryDylibRpaths`.
