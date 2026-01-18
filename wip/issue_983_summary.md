# Issue 983 Summary

## What Was Implemented

Soname extraction functions for ELF and Mach-O binary formats that enable tsuku to auto-discover which sonames a library provides at install time. This is foundational work for the Tier 2 dependency validation system.

## Changes Made

- `internal/verify/soname.go`: New file with four extraction functions:
  - `ExtractELFSoname()` - extracts DT_SONAME using `elf.DynString()`
  - `ExtractMachOInstallName()` - extracts LC_ID_DYLIB by parsing raw load commands
  - `ExtractSoname()` - auto-detects format and delegates
  - `ExtractSonames()` - scans directory for all library sonames

- `internal/verify/soname_test.go`: Comprehensive test suite using platform-conditional system library tests

## Key Decisions

- **Parse raw load commands for Mach-O**: Go's standard library doesn't expose LC_ID_DYLIB (0xd) as a named constant, so we parse raw load command bytes to identify and extract the install name.
- **Return empty string for missing soname**: Many libraries don't have explicit sonames set - returning empty string (not error) allows callers to handle this gracefully.
- **Local format detection functions**: Created `readMagicForSoname` and `detectFormatForSoname` rather than exporting the ones in header.go, to avoid coupling.

## Trade-offs Accepted

- **Code duplication**: Format detection code is duplicated from header.go rather than refactoring to share. Acceptable for now; refactoring can happen later if needed.
- **Platform-conditional tests**: Mach-O tests may be skipped on modern macOS where system libraries are in dyld cache.

## Test Coverage

- 11 new tests covering ELF extraction, Mach-O extraction, format detection, edge cases (invalid files, empty files, directories), and directory scanning
- All tests pass on Linux; Mach-O tests skip gracefully if system dylibs are in dyld cache

## Known Limitations

- Mach-O testing on macOS is limited due to system libraries being in the dyld shared cache since Big Sur
- Does not extract sonames from static archives (returns error as expected)

## Future Improvements

None required for this issue. Downstream issues (#985, #986) will consume these functions.
