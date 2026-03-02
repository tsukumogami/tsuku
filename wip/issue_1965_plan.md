# Issue 1965 Implementation Plan

## Summary

Add `DYLD_LIBRARY_PATH` (macOS) and `LD_LIBRARY_PATH` (Linux) to the binary wrapper script so dynamically linked runtime dependency libraries are found at execution time, and also fix the `homebrew_relocate` action to add dependency library RPATHs to tool binaries (not just library dylibs).

## Approach

The wrapper-based approach is chosen as the primary fix because it's reliable, works regardless of binary signing status (Apple Silicon binaries can reject RPATH modifications), and follows the existing wrapper generation pattern. The RPATH fix in `homebrew_relocate` is added as defense-in-depth so binaries also work when invoked directly (not through the wrapper).

The fix targets two layers:
1. **Wrapper scripts** (reliable, always works): extend `generateWrapperScript` to include `DYLD_LIBRARY_PATH`/`LD_LIBRARY_PATH` entries pointing at `$TSUKU_HOME/libs/<dep>-<version>/lib` for each runtime dependency that is a library.
2. **RPATH in binaries** (defense-in-depth): extend `homebrew_relocate`'s `fixMachoRpath` to add dependency library paths as RPATHs on tool binaries, not just on `.dylib` files within library recipes.

### Alternatives Considered

- **RPATH-only approach (via install_name_tool)**: Modify `homebrew_relocate` to add dependency lib paths to each binary's RPATH via `install_name_tool -add_rpath`. This is elegant but fragile on macOS: Apple Silicon requires re-signing after RPATH modification, some binaries are sealed/notarized and reject modification, and `install_name_tool` silently fails on some binary types. Rejected as sole approach because reliability is lower.

- **Recipe-level set_rpath step**: Add an explicit `set_rpath` step to every affected recipe (fontconfig, etc.) with the appropriate dependency paths via `{deps.gettext.version}` variable expansion. This works but doesn't scale: every homebrew recipe with library deps would need manual set_rpath entries with hardcoded dependency names. Rejected because it pushes systemic infrastructure problems onto recipe authors.

- **DYLD_LIBRARY_PATH-only wrapper approach (no RPATH fix)**: Only modify the wrapper script. This works for all binaries invoked through `$TSUKU_HOME/bin/` but doesn't help if someone invokes the binary directly from the tool directory. Rejected in favor of doing both wrapper + RPATH for defense-in-depth.

## Files to Modify

- `internal/install/manager.go` - Extend `generateWrapperScript` to accept library path additions and emit `DYLD_LIBRARY_PATH`/`LD_LIBRARY_PATH` export lines. Extend `createBinaryWrapper` to compute library paths from runtime dependencies.
- `internal/install/manager_test.go` - Add tests for library path generation in wrapper scripts.
- `internal/actions/homebrew_relocate.go` - Extend `fixMachoRpath` (and `fixElfRpath`) to add dependency lib directory RPATHs to tool binaries when the execution context has install-time dependencies that are libraries. Clean up debug `fmt.Printf` statements.
- `internal/actions/homebrew_test.go` - Add tests for the dependency RPATH addition logic.
- `recipes/f/fontconfig.toml` - Add `runtime_dependencies = ["freetype", "gettext"]` at the metadata level (currently dependencies are only declared at step level as install-time deps, but they're needed at runtime too for dynamic linking).

## Files to Create

None.

## Implementation Steps

- [x] 1. **Add runtime_dependencies to fontconfig recipe**: Added `runtime_dependencies = ["freetype", "gettext"]` to fontconfig's `[metadata]` section.

- [x] 2. **Convert freetype recipe to library type**: Changed freetype from tool type to `type = "library"` with `install_mode = "directory"` so shared libraries install to `$TSUKU_HOME/libs/`. Added `runtime_dependencies = ["libpng"]`.

- [x] 3. **Extend wrapper script to set library paths**: Modified `generateWrapperScript` in `internal/install/manager.go` to accept `libPathAdditions []string` parameter. Emits `DYLD_LIBRARY_PATH` (darwin) or `LD_LIBRARY_PATH` (linux) with export before `exec`.

- [x] 4. **Compute library paths in createBinaryWrapper**: Separated tool PATH additions from library path additions. Added `collectLibraryPaths()` method that scans all `$TSUKU_HOME/libs/*/lib/` directories to cover transitive dependencies.

- [x] 5. **Fix resolveRuntimeDeps for libraries**: Fixed `resolveRuntimeDeps` in `cmd/tsuku/install_deps.go` to check library state (`GetInstalledLibraryVersion`) before tool state (`GetToolState`), since library dependencies install to `$TSUKU_HOME/libs/` not `$TSUKU_HOME/tools/`.

- [x] 6. **Add unit tests for wrapper library paths**: Added tests for `DYLD_LIBRARY_PATH`/`LD_LIBRARY_PATH` emission, combined PATH + lib path, empty lib path, library-only dependency, and mixed dependency scenarios.

- [ ] ~7. **Extend homebrew_relocate for RPATH defense-in-depth**: Deferred — the wrapper approach proved fully sufficient in manual testing. RPATH modification on macOS is fragile with signed/notarized binaries.~

- [ ] ~8. **Clean up debug printf statements**: Out of scope for this bug fix.~

## Testing Strategy

- **Unit tests**:
  - Test `generateWrapperScript` with library path additions (new test cases in `manager_test.go`)
  - Test wrapper content includes correct `DYLD_LIBRARY_PATH` on darwin
  - Test wrapper content includes correct `LD_LIBRARY_PATH` on linux
  - Test empty library paths produces no lib path line
  - Test dependency RPATH logic in homebrew_relocate (mock execution context)

- **Integration tests**:
  - On macOS: `tsuku install fontconfig` followed by `fc-list --version` should succeed without `dyld: Library not loaded` errors
  - Verify the generated wrapper script for fontconfig contains `DYLD_LIBRARY_PATH` pointing to gettext and freetype lib dirs

- **Manual verification**:
  - On macOS, after installing fontconfig: `cat ~/.tsuku/bin/fc-list` to inspect wrapper content
  - Run `otool -L ~/.tsuku/tools/fontconfig-*/bin/fc-list` to verify RPATHs include dependency lib dirs
  - Run `fc-list` and verify it works without errors

## Risks and Mitigations

- **SIP and DYLD_LIBRARY_PATH**: macOS System Integrity Protection restricts `DYLD_LIBRARY_PATH` for system binaries. This doesn't affect tsuku-installed binaries since they're user-space, but worth noting. **Mitigation**: The RPATH layer provides a fallback.

- **Library type detection**: We need to distinguish library dependencies from tool dependencies to know which get lib path entries. **Mitigation**: Libraries install to `$TSUKU_HOME/libs/`, tools to `$TSUKU_HOME/tools/`. Check whether the dep directory exists under libs.

- **Performance**: Adding RPATH to every binary during homebrew_relocate adds `install_name_tool` calls. **Mitigation**: Only done when the recipe has library dependencies and only on macOS. Typically affects a small number of binaries.

- **install_name_tool failure on sealed binaries**: Some macOS binaries resist modification. **Mitigation**: The wrapper script is the primary mechanism; RPATH is defense-in-depth with best-effort error handling.

## Success Criteria

- [x] `tsuku install fontconfig` on macOS completes successfully
- [x] `fc-list --version` works without `dyld: Library not loaded` errors
- [x] Wrapper script for fontconfig binaries includes `DYLD_LIBRARY_PATH` with gettext and freetype lib paths
- [x] Existing tests pass (no regressions from baseline)
- [x] New unit tests cover wrapper library path generation
- [x] Other recipes with `runtime_dependencies` pointing to libraries (e.g., par2 -> libomp, qrencode -> libpng) would also benefit from this fix

## Open Questions

None blocking. The approach is straightforward and follows existing patterns in the codebase.
