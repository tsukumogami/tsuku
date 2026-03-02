## Context Summary

**Issue**: #1965 - macOS homebrew recipes missing rpath for runtime dependency libraries
**Type**: Bug fix
**Scope**: macOS-only, affects homebrew-based recipes with library dependencies

## Root Cause Analysis

The `fixMachoRpath` function in `homebrew_relocate.go` (lines 425-564) adds
`@loader_path/../lib` as RPATH for tool binaries, allowing them to find their
own sibling libraries. However, it does NOT add RPATHs pointing to dependency
libraries installed under `$TSUKU_HOME/libs/<dep>-<version>/lib`.

There's a separate `fixLibraryDylibRpaths` function (lines 566-689) that adds
dependency RPATHs to **library** dylibs (recipe type "library"). But tool
binaries don't go through this path -- they only get the self-relative RPATH.

So when fontconfig's `fc-list` binary needs `libintl.8.dylib` from gettext,
it can't find it because:
1. `@loader_path/../lib` only searches fontconfig's own lib dir
2. No RPATH points to `$TSUKU_HOME/libs/gettext-<ver>/lib`
3. No wrapper sets `DYLD_LIBRARY_PATH`

## Fix Approach

The fix should add dependency library RPATHs to tool binaries during the
`homebrew_relocate` step, similar to what `fixLibraryDylibRpaths` already does
for library dylibs. The `fixMachoRpath` function receives the install path but
not the dependency paths -- those need to be threaded through from the
execution context.

## Key Files

- `internal/actions/homebrew_relocate.go` - RPATH fixup (fixMachoRpath, fixLibraryDylibRpaths)
- `internal/install/manager.go` - wrapper/symlink creation (createBinaryWrapper)
- `internal/actions/set_rpath.go` - fallback wrapper with DYLD_LIBRARY_PATH
- `internal/verify/rpath.go` - RPATH inspection utilities

## Dependencies

None -- standalone bug fix.

## Integration Points

- Must not break Linux ELF handling (patchelf path)
- Must re-sign modified binaries on arm64 (codesign)
- Should work for any homebrew recipe with library deps, not just fontconfig
