# Issue 982 Implementation Plan

## Summary

Implement `ExtractRpaths()` and `ExpandPathVariables()` functions in a new `internal/verify/rpath.go` file, following established patterns from sibling implementations (panic recovery, error categorization, local constant definitions for Mach-O load commands).

## Approach

Create two functions that work together to enable path resolution for dependency validation. `ExtractRpaths()` reads RPATH entries from ELF (DT_RUNPATH with DT_RPATH fallback) and Mach-O (LC_RPATH) binaries using Go's standard library. `ExpandPathVariables()` expands runtime path variables ($ORIGIN, @rpath, @loader_path, @executable_path) with security-focused path normalization.

This approach was chosen because:
1. It aligns with the established code patterns in the verify package (soname.go, abi.go)
2. It uses Go stdlib only (no external tools), matching design requirement
3. It provides clear separation between extraction and expansion concerns
4. It integrates with existing `IsPathVariable()` helper in system_libs.go

### Alternatives Considered

- **Single combined function**: Rejected because extraction and expansion are logically separate operations with different inputs. The downstream consumer (#989) needs RPATH lists independently.
- **External tool approach (readelf/otool)**: Rejected per design requirement to use only Go stdlib for cross-platform consistency and reduced dependencies.
- **Lazy extraction in ExpandPathVariables**: Rejected because callers often need the RPATH list for multiple dependencies, making upfront extraction more efficient.

## Files to Modify

- `internal/verify/types.go` - Add new error categories: `ErrRpathLimitExceeded`, `ErrPathLengthExceeded`, `ErrUnexpandedVariable`, `ErrPathOutsideAllowed` (explicit values 11-14, following design decision #2)

## Files to Create

- `internal/verify/rpath.go` - Main implementation with:
  - `ExtractRpaths(path string) ([]string, error)` - extracts RPATH/RUNPATH entries
  - `ExpandPathVariables(dep, binaryPath string, rpaths []string) (string, error)` - expands path variables
  - Local constant `lcRpath macho.LoadCmd = 0x8000001c` for Mach-O LC_RPATH
  - Helper functions for format detection and path validation

- `internal/verify/rpath_test.go` - Test coverage including:
  - ELF RPATH extraction with system binaries
  - Mach-O RPATH extraction (macOS only)
  - Path variable expansion cases ($ORIGIN, @rpath, @loader_path, @executable_path)
  - Security limit enforcement (100 RPATHs, 4096 char paths)
  - Symlink resolution and canonical path validation
  - Error conditions (malformed binaries, invalid paths, unexpanded variables)

## Implementation Steps

- [x] Add error categories to `types.go` with explicit values (11-14)
- [x] Create `rpath.go` with constants and RPATH limit definitions
- [x] Implement `extractELFRpaths()` using `debug/elf.DynString(DT_RUNPATH)` with DT_RPATH fallback
- [x] Implement `extractMachORpaths()` using `debug/macho.File.Loads` parsing LC_RPATH
- [x] Implement `extractFatRpaths()` for fat/universal binaries
- [x] Implement `ExtractRpaths()` as format-detection wrapper with panic recovery
- [x] Implement `ExpandPathVariables()` with:
  - $ORIGIN expansion (dirname of binaryPath)
  - @rpath expansion (try each rpath in order)
  - @loader_path expansion (same as $ORIGIN)
  - @executable_path expansion (dirname of binaryPath for binaries, or main executable for dylibs)
  - Path normalization via filepath.Clean()
  - Symlink resolution via filepath.EvalSymlinks()
  - Canonical path validation (must resolve to $TSUKU_HOME/tools/)
  - Unexpanded variable detection
- [x] Create `rpath_test.go` with comprehensive test coverage
- [x] Verify all tests pass with `go test ./internal/verify/...`

## Testing Strategy

### Unit tests

- **ExtractRpaths**: Test with system binaries on Linux/macOS that have known RPATH entries. Test edge cases: no RPATH, multiple entries, empty binary, non-binary files.
- **ExpandPathVariables**: Test each path variable type ($ORIGIN, @rpath, @loader_path, @executable_path) with known inputs. Test path traversal prevention, symlink resolution, canonical path validation.
- **Error conditions**: Test RPATH count limit (>100), path length limit (>4096), unexpanded variables after expansion, paths outside allowed directories.

### Integration tests

- Not required for this issue; the downstream #989 will provide integration coverage.

### Manual verification

- Build tsuku and run `ExtractRpaths()` on installed tools with RPATH entries (e.g., ninja, cmake if installed via homebrew relocation)

## Risks and Mitigations

- **Path traversal via symlinks**: Mitigated by using `filepath.EvalSymlinks()` on all resolved paths before validation, consistent with PR #963.
- **Path normalization tricks**: Mitigated by applying `filepath.Clean()` to all paths before processing.
- **Parser panics on malformed input**: Mitigated by panic recovery in all binary parsing functions (established pattern from soname.go, abi.go).
- **DoS via large RPATH lists**: Mitigated by enforcing 100 RPATH limit per binary and 4096 character path length limit.
- **Information leakage in error messages**: Mitigated by not including full internal paths in error messages returned to users.
- **Mach-O LC_RPATH constant not exported**: Mitigated by defining local constant `lcRpath = 0x8000001c` following pattern from #983 (lcIDDylib).

## Success Criteria

- [x] `go test ./internal/verify/...` passes
- [x] `ExtractRpaths()` correctly extracts RPATH from ELF binaries (DT_RUNPATH preferred, DT_RPATH fallback)
- [x] `ExtractRpaths()` correctly extracts RPATH from Mach-O binaries (LC_RPATH)
- [x] `ExpandPathVariables()` correctly expands $ORIGIN, @rpath, @loader_path, @executable_path
- [x] RPATH count limit (100) is enforced
- [x] Path length limit (4096) is enforced
- [x] Unexpanded variables ($, @) after expansion cause errors
- [x] Resolved paths are validated against canonical path requirements
- [x] Error categories added to types.go with explicit values
- [x] All functions include panic recovery

## Open Questions

None. The issue specification is complete and aligned with the design document. Minor gaps identified in introspection are resolvable from existing code patterns.
