# Issue 982 Summary

## What Changed

Implemented RPATH extraction and path variable expansion for library dependency validation (Tier 2).

### Files Created
- `internal/verify/rpath.go` - Main implementation (447 lines)
- `internal/verify/rpath_test.go` - Comprehensive test coverage (600+ lines)

### Files Modified
- `internal/verify/types.go` - Added 4 new error categories

## Implementation Details

### ExtractRpaths(path string) ([]string, error)

Extracts RPATH entries from ELF and Mach-O binaries:
- **ELF**: Uses `DT_RUNPATH` (preferred) with `DT_RPATH` fallback
- **Mach-O**: Parses `LC_RPATH` load commands
- **Fat binaries**: Extracts from first architecture slice
- Returns empty slice for non-binary files (not an error)
- Includes panic recovery for robustness

### ExpandPathVariables(dep, binaryPath string, rpaths []string, allowedPrefix string) (string, error)

Expands runtime path variables in dependency paths:
- `$ORIGIN`, `${ORIGIN}` - Directory containing the binary (ELF)
- `@loader_path` - Directory containing the binary (Mach-O)
- `@executable_path` - Main executable directory (Mach-O)
- `@rpath` - Tries each RPATH entry in order, returns first match

Security features:
- Path normalization via `filepath.Clean()`
- Symlink resolution via `filepath.EvalSymlinks()`
- Canonical path validation via `allowedPrefix` parameter
- Unexpanded variable detection after expansion

### Error Categories Added (types.go)

| Value | Category | Description |
|-------|----------|-------------|
| 11 | ErrRpathLimitExceeded | Binary has >100 RPATH entries |
| 12 | ErrPathLengthExceeded | Path exceeds 4096 characters |
| 13 | ErrUnexpandedVariable | Path contains unexpanded $ or @ variable |
| 14 | ErrPathOutsideAllowed | Expanded path outside allowed directories |

### Security Limits

- `MaxRpathEntries = 100` - Maximum RPATH entries per binary
- `MaxPathLength = 4096` - Maximum path length (matches Linux PATH_MAX)

## Testing

All tests pass:
```
ok      github.com/tsukumogami/tsuku/internal/verify    0.214s
```

Test coverage includes:
- ELF RPATH extraction with system binaries
- Non-binary file handling (returns empty, no error)
- All path variable types ($ORIGIN, @rpath, @loader_path, @executable_path)
- @rpath with multiple RPATH candidates (selects existing file)
- RPATH count limit enforcement
- Path length limit enforcement
- Allowed prefix validation
- Unexpanded variable detection
- containsPathVariable() conservative detection

## Design Patterns Used

- **Panic recovery**: All binary parsing functions use defer/recover
- **Local Mach-O constants**: `lcRpath = 0x8000001c` (LC_RPATH not exported by Go)
- **Format detection helpers**: Local copies to avoid coupling (same pattern as soname.go)
- **ValidationError**: Consistent error categorization with existing verify package

## Downstream Dependencies

This implementation enables:
- Issue #989: Dependency resolution verification (tier:critical)
