---
summary:
  constraints:
    - Verify only files with stored checksums (not all files in directory)
    - Handle nil/empty checksums gracefully (skip with informative reason)
    - Resolve symlinks before checksumming (checksums stored for real files only)
    - Keep existing install.ComputeFileChecksum for reuse
  integration_points:
    - cmd/tsuku/verify.go - Replace verifyLibraryIntegrity() with verify.VerifyIntegrity()
    - internal/verify/ package - New integrity.go file alongside other verification modules
    - internal/install/checksum.go - May deprecate VerifyLibraryChecksums() if no longer needed
    - LibraryVersionState.Checksums map - Already exists in state schema
  risks:
    - Output format changes could break existing scripts parsing verify output
    - Need to match existing output style (use printInfo/printInfof helpers)
    - Tests may need to handle symlinks differently on different platforms
  approach_notes: |
    This is a refactoring of existing functionality. The basic integrity verification
    already works in cmd/tsuku/verify.go (verifyLibraryIntegrity function). The goal
    is to move it to internal/verify/integrity.go with a cleaner IntegrityResult type
    that separates missing files from mismatches and provides a verified count.

    Key changes:
    1. Create internal/verify/integrity.go with VerifyIntegrity() and types
    2. Update cmd/tsuku/verify.go to call verify.VerifyIntegrity()
    3. Remove the old verifyLibraryIntegrity() inline function
    4. Add unit tests for the new module
    5. Clean up "basic implementation" comments
---

# Implementation Context: Issue #1168

**Source**: docs/designs/DESIGN-library-verify-integrity.md

## Summary

This issue refactors existing Tier 4 integrity verification from scattered code into a proper module at `internal/verify/integrity.go`. The functionality already works - this is architectural cleanup with improved result types.

**Key files:**
- NEW: `internal/verify/integrity.go` - Main module with VerifyIntegrity() function
- NEW: `internal/verify/integrity_test.go` - Unit tests
- MODIFY: `cmd/tsuku/verify.go` - Use new module, remove inline function
- DEPRECATE: `internal/install/checksum.go` VerifyLibraryChecksums() if unused after refactor

**Structured result type:**
- `Verified int` - Count of files that passed
- `Mismatches []IntegrityMismatch` - Files with wrong checksums
- `Missing []string` - Files that no longer exist
- `Skipped bool` + `Reason string` - For pre-checksum libraries
