# Issue 1168 Summary

## What Was Implemented

Refactored Tier 4 integrity verification from scattered inline code into a proper module at `internal/verify/integrity.go` with structured result types that match the established patterns from Tier 3 (dltest.go).

## Changes Made

- `internal/verify/integrity.go` (NEW): Added `IntegrityResult` and `IntegrityMismatch` types with `VerifyIntegrity()` function that computes checksums using `install.ComputeFileChecksum()`
- `internal/verify/integrity_test.go` (NEW): Comprehensive unit tests covering success, mismatch, missing file, skipped, symlink, and mixed scenarios
- `cmd/tsuku/verify.go`: Replaced inline `verifyLibraryIntegrity()` with call to `verify.VerifyIntegrity()`, removed the old function
- `internal/install/checksum.go`: Marked `VerifyLibraryChecksums()` as deprecated with pointer to the new function

## Key Decisions

- **Reuse existing checksum helper**: Used `install.ComputeFileChecksum()` rather than duplicating SHA256 logic, maintaining consistency with other checksum operations
- **Separate Missing from Mismatches**: The new `IntegrityResult.Missing` slice separates file-not-found errors from actual checksum mismatches, providing cleaner error reporting
- **Deprecation over removal**: Marked `VerifyLibraryChecksums()` as deprecated rather than removing it, allowing for backward compatibility

## Trade-offs Accepted

- **Kept deprecated function**: Left `VerifyLibraryChecksums()` in place with a deprecation notice rather than removing it immediately, as it may be used elsewhere or by tests. Future cleanup can remove it.

## Test Coverage

- New tests added: 7 tests in `integrity_test.go`
  - TestVerifyIntegrity_AllMatch
  - TestVerifyIntegrity_Mismatch
  - TestVerifyIntegrity_MissingFile
  - TestVerifyIntegrity_EmptyChecksums
  - TestVerifyIntegrity_NilChecksums
  - TestVerifyIntegrity_Symlink
  - TestVerifyIntegrity_Mixed
- All existing tests continue to pass (24 packages)

## Known Limitations

- Symlink modification detection: Only real files are checksummed; changes to symlink targets won't be detected if the symlink itself resolves to the same content
- Error detail: Files that fail to read (permission denied, etc.) are treated as missing rather than reported as separate errors

## Future Improvements

- Could add an `Errors` field to `IntegrityResult` for detailed error reporting separate from missing files
- Downstream issues #1169 and #1170 will add comprehensive tests and documentation
