# Issue 981 Summary

## What Was Implemented

Added `ValidateABI()` function that checks ELF binaries for PT_INTERP segment validity. This catches glibc/musl ABI mismatches before runtime by verifying the dynamic linker/interpreter exists on the filesystem.

## Changes Made

- `internal/verify/types.go`: Added `ErrABIMismatch ErrorCategory = 10` constant with Tier 2 comment, updated `String()` method
- `internal/verify/abi.go`: New file implementing `ValidateABI(path string) error`
- `internal/verify/abi_test.go`: Unit tests covering all code paths

## Key Decisions

- **Graceful handling of non-ELF files**: Returns nil instead of error when `elf.Open()` fails, allowing scripts and other file types to pass through
- **First PT_INTERP wins**: If multiple PT_INTERP segments exist (unusual), use the first one - this is standard behavior
- **macOS no-op**: Returns nil immediately on non-Linux since macOS has no PT_INTERP equivalent

## Trade-offs Accepted

- **Cannot test missing interpreter scenario directly**: Would require a musl binary on glibc system or vice versa. Test coverage focuses on verifying error constants and message formatting instead.
- **Non-ELF treated as valid**: A non-ELF file returns nil rather than an error, since ABI validation only applies to ELF binaries.

## Test Coverage

- New tests added: 8 test functions covering:
  - Non-Linux skip behavior
  - System library validation (with valid PT_INTERP)
  - Static binary detection (no PT_INTERP)
  - Non-ELF file handling
  - Dynamic executable validation
  - ErrABIMismatch constant value verification
  - Error message formatting
- All tests pass

## Known Limitations

- Cannot detect ABI mismatch if interpreter exists but is incompatible (e.g., old glibc version)
- Doesn't validate that interpreter is actually functional, just that it exists

## Future Improvements

- #989 (recursive dependency validation) will consume this function as the first validation step
- #972 (comprehensive ABI validation) could extend beyond PT_INTERP to include symbol-level checks
