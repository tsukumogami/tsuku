# Issue 1965 Summary

## What Was Implemented

Extended binary wrapper scripts to set `DYLD_LIBRARY_PATH` (macOS) or `LD_LIBRARY_PATH` (Linux) so dynamically linked runtime dependency libraries are found at execution time. Fixed the runtime dependency resolver to look up library state, and corrected recipe metadata so the dependency chain is properly declared.

## Changes Made

- `internal/install/manager.go`: Extended `generateWrapperScript` with `libPathAdditions` parameter that emits `DYLD_LIBRARY_PATH`/`LD_LIBRARY_PATH` export lines. Added `collectLibraryPaths()` that scans all `$TSUKU_HOME/libs/*/lib/` directories. Updated `createBinaryWrapper` to separate tool PATH from library path additions.
- `internal/install/manager_test.go`: Added 5 new test functions covering library path emission, combined PATH + lib path, empty lib path, library-only deps, and mixed deps. Updated 6 existing tests for new function signature.
- `cmd/tsuku/install_deps.go`: Fixed `resolveRuntimeDeps` to check `GetInstalledLibraryVersion` before `GetToolState`, since library dependencies install to `$TSUKU_HOME/libs/` not `$TSUKU_HOME/tools/`.
- `recipes/f/fontconfig.toml`: Added `runtime_dependencies = ["freetype", "gettext"]` to metadata.
- `recipes/f/freetype.toml`: Converted from tool type to `type = "library"` with `install_mode = "directory"` and added `runtime_dependencies = ["libpng"]`.

## Key Decisions

- **Wrapper-only approach**: Chose wrapper script `DYLD_LIBRARY_PATH` over RPATH modification via `install_name_tool`. RPATH modification is fragile on macOS with signed/notarized binaries and requires re-signing on Apple Silicon. The wrapper approach is reliable regardless of binary signing status.
- **Scan all library dirs**: `collectLibraryPaths()` scans all installed library lib/ directories rather than just direct dependencies. This handles transitive dependencies (fontconfig -> freetype -> libpng) without needing to resolve the full dependency graph at wrapper creation time.
- **freetype as library**: Converted freetype from tool type to library type since it primarily provides shared libraries (`libfreetype.dylib`), not user-facing binaries.

## Trade-offs Accepted

- **Broad library path inclusion**: `collectLibraryPaths()` includes all installed library paths, not just the specific transitive closure of dependencies. This is simpler and correct (extra paths don't cause issues), though slightly less precise than computing the exact transitive dependency set.

## Test Coverage

- New tests added: 5 test functions
- All new and existing wrapper tests pass
- Pre-existing failures unchanged: `TestFindLibraryFiles` (unrelated macOS symlink issue)

## Known Limitations

- RPATH defense-in-depth (modifying binary RPATHs via `install_name_tool`) was deferred. Binaries invoked directly from `$TSUKU_HOME/tools/` without the wrapper won't find library dependencies.
- The fix requires library dependencies to be installed before the dependent tool. This is already handled by the installation order logic.

## Requirements Mapping

| AC | Status | Evidence / Reason |
|----|--------|-------------------|
| Wrapper sets DYLD_LIBRARY_PATH for runtime deps | Implemented | `internal/install/manager.go:generateWrapperScript` + `collectLibraryPaths` |
| fc-list works without dyld errors | Verified | Manual test: `fc-list --version` returns `fontconfig version 2.17.1` |
| Affects any homebrew recipe with library deps | Implemented | Generic solution in `createBinaryWrapper`, not fontconfig-specific |
| Linux LD_LIBRARY_PATH also handled | Implemented | `generateWrapperScript` emits `LD_LIBRARY_PATH` on non-darwin |
