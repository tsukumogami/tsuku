# Issue 983 Implementation Plan

## Summary

Implement soname extraction functions for ELF and Mach-O binaries using Go's standard library (`debug/elf` and `debug/macho`). The implementation follows existing `internal/verify/header.go` patterns including panic recovery and format auto-detection.

## Approach

Use the existing header validation patterns in `internal/verify/header.go` as a template. The soname extraction functions reuse the format detection logic (`detectFormat`) and error handling patterns already established. For ELF, use `elf.DynString(elf.DT_SONAME)`. For Mach-O, iterate `f.Loads` looking for `*macho.Dylib` entries where `Cmd == macho.LoadCmdIdDylib` (LC_ID_DYLIB).

### Alternatives Considered

- **Option A: Separate file for format detection**: Move `readMagic` and `detectFormat` to a shared file. Rejected because these are small functions and duplication is acceptable; refactoring can happen later if needed.
- **Option B: External tools (readelf, otool)**: Rejected because the issue explicitly requires Go standard library only for portability.
- **Option C: Return error for missing soname**: Rejected because the issue specifies returning empty string (not error) when soname is absent - many libraries don't have a soname set.

## Files to Modify

None. All implementation is in new files.

## Files to Create

- `internal/verify/soname.go` - Core soname extraction functions
- `internal/verify/soname_test.go` - Unit tests for extraction functions

## Implementation Steps

- [ ] Create `internal/verify/soname.go` with package declaration and imports
- [ ] Implement `ExtractELFSoname(path string) (string, error)` using `elf.Open()` and `DynString(DT_SONAME)`
- [ ] Implement `ExtractMachOInstallName(path string) (string, error)` by iterating `f.Loads` for LC_ID_DYLIB
- [ ] Implement `extractMachOInstallNameFromFile(f *macho.File) string` helper for fat binary reuse
- [ ] Implement `extractFatInstallName(path string) (string, error)` for universal binaries
- [ ] Implement `ExtractSoname(path string) (string, error)` using `readMagic` and `detectFormat` for auto-detection
- [ ] Implement `ExtractSonames(libDir string) ([]string, error)` to scan a directory for all library sonames
- [ ] Add panic recovery to all format-specific functions (follow `header.go` pattern)
- [ ] Create `internal/verify/soname_test.go` with test structure
- [ ] Add tests using system libraries (Linux: libc.so.6, macOS: libSystem.B.dylib)
- [ ] Add tests for missing soname case (should return empty string)
- [ ] Add tests for invalid/non-binary files
- [ ] Add tests for `ExtractSonames` directory scanning
- [ ] Run `go vet ./...`, `go test ./internal/verify/...`, and `go build ./...`

## Testing Strategy

### Unit Tests

**System library tests (platform-conditional):**
- Linux: Extract soname from `/lib/x86_64-linux-gnu/libc.so.6` or similar paths (verify returns "libc.so.6")
- macOS: Extract install name from system dylibs (verify returns expected path like "/usr/lib/libSystem.B.dylib")

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
| Mach-O LC_ID_DYLIB iteration complexity | Study `debug/macho` source; the `Dylib` type includes `Cmd` field indicating load command type. Check for `LoadCmdIdDylib` (0x0D). |
| Fat binary architecture selection | Reuse `macho.OpenFat()` pattern from `header.go`; extract from any slice since install name should be same across architectures. |
| Test fixtures for binaries | Use system libraries (platform-conditional skips) as in `header_test.go`; avoid creating binary fixtures which are hard to maintain. |
| Parser panics on malformed binaries | Add defer/recover blocks following `header.go` pattern. |

## Success Criteria

- [ ] `ExtractELFSoname` correctly extracts DT_SONAME from ELF shared objects
- [ ] `ExtractMachOInstallName` correctly extracts LC_ID_DYLIB from Mach-O dylibs
- [ ] `ExtractSoname` auto-detects format and delegates appropriately
- [ ] `ExtractSonames` scans a directory and returns all library sonames
- [ ] Missing soname returns empty string, not error
- [ ] Fat/universal binaries handled correctly
- [ ] All tests pass on Linux (ELF tests) and macOS (Mach-O tests)
- [ ] `go vet ./...` reports no issues
- [ ] `go build ./...` succeeds

## Open Questions

None. The introspection confirmed all prerequisites are in place and the implementation approach is clear.
