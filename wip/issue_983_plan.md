# Issue 983 Implementation Plan

## Summary

Implement soname extraction functions for ELF and Mach-O binaries using Go's standard library (`debug/elf` and `debug/macho`). The implementation follows existing `internal/verify/header.go` patterns including panic recovery and format auto-detection.

## Approach

Use the existing header validation patterns in `internal/verify/header.go` as a template. The soname extraction functions reuse the format detection logic and error handling patterns already established. For ELF, use `elf.DynString(elf.DT_SONAME)`. For Mach-O, parse raw load command bytes to find LC_ID_DYLIB (0xd) since Go's standard library doesn't expose this constant.

### Alternatives Considered

- **Option A: Separate file for format detection**: Move `readMagic` and `detectFormat` to a shared file. Rejected because these are small functions and duplication is acceptable; refactoring can happen later if needed.
- **Option B: External tools (readelf, otool)**: Rejected because the issue explicitly requires Go standard library only for portability.
- **Option C: Return error for missing soname**: Rejected because the issue specifies returning empty string (not error) when soname is absent - many libraries don't have a soname set.
- **Option D: Use third-party macho library**: Rejected because the issue requires Go standard library only.

## Files to Modify

None. All implementation is in new files.

## Files to Create

- `internal/verify/soname.go` - Core soname extraction functions
- `internal/verify/soname_test.go` - Unit tests for extraction functions

## Implementation Steps

- [x] Create `internal/verify/soname.go` with package declaration and imports
- [x] Implement `ExtractELFSoname(path string) (string, error)` using `elf.Open()` and `DynString(DT_SONAME)`
- [x] Implement `ExtractMachOInstallName(path string) (string, error)` by parsing raw load commands for LC_ID_DYLIB
- [x] Implement `extractMachOInstallNameFromFile(f *macho.File) string` helper for fat binary reuse
- [x] Implement `extractFatInstallName(path string) (string, error)` for universal binaries
- [x] Implement `ExtractSoname(path string) (string, error)` using `readMagic` and `detectFormat` for auto-detection
- [x] Implement `ExtractSonames(libDir string) ([]string, error)` to scan a directory for all library sonames
- [x] Add panic recovery to all format-specific functions (follow `header.go` pattern)
- [x] Create `internal/verify/soname_test.go` with test structure
- [x] Add tests using system libraries (Linux: libc.so.6)
- [x] Add tests for missing soname case (should return empty string)
- [x] Add tests for invalid/non-binary files
- [x] Add tests for `ExtractSonames` directory scanning
- [x] Run `go vet ./...`, `go test ./internal/verify/...`, and `go build ./...`

## Testing Strategy

### Unit Tests

**System library tests (platform-conditional):**
- Linux: Extract soname from `/lib/x86_64-linux-gnu/libc.so.6` or similar paths (verify returns "libc.so.6")
- macOS: Extract install name from system dylibs (skipped if files are in dyld cache)

**Edge case tests:**
- File with valid format but no soname/install name -> return empty string, no error
- Non-binary file -> return appropriate error
- Empty file -> return appropriate error
- Non-existent file -> return error
- Directory instead of file -> return error

**Directory scanning tests:**
- Create temp directory with minimal test binaries or use system lib directories
- Verify `ExtractSonames` returns expected list of sonames

### Manual Verification

```bash
# Build and run tests
go test -v ./internal/verify/...

# Verify no regressions
go test ./...

# Check for vet issues
go vet ./...
```

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Mach-O LC_ID_DYLIB iteration complexity | Parse raw load command bytes since Go stdlib doesn't expose LC_ID_DYLIB (0xd) constant or parsing. |
| Fat binary architecture selection | Reuse `macho.OpenFat()` pattern from `header.go`; extract from any slice since install name should be same across architectures. |
| Test fixtures for binaries | Use system libraries (platform-conditional skips) as in `header_test.go`; avoid creating binary fixtures which are hard to maintain. |
| Parser panics on malformed binaries | Add defer/recover blocks following `header.go` pattern. |

## Success Criteria

- [x] `ExtractELFSoname` correctly extracts DT_SONAME from ELF shared objects
- [x] `ExtractMachOInstallName` correctly extracts LC_ID_DYLIB from Mach-O dylibs
- [x] `ExtractSoname` auto-detects format and delegates appropriately
- [x] `ExtractSonames` scans a directory and returns all library sonames
- [x] Missing soname returns empty string, not error
- [x] Fat/universal binaries handled correctly
- [x] All tests pass on Linux (ELF tests) and macOS (Mach-O tests skipped if in dyld cache)
- [x] `go vet ./...` reports no issues
- [x] `go build ./...` succeeds

## Open Questions

None. The introspection confirmed all prerequisites are in place and the implementation approach is clear.
