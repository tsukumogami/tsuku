# Issue 1168 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-library-verify-integrity.md` (status: Planned)
- Sibling issues reviewed: #1169, #1170 (both OPEN, none closed)
- Prior patterns identified:
  - `internal/verify/dltest.go` - Tier 3 verification module with `DlopenResult`, `DlopenVerificationResult`, `RunDlopenVerification()`
  - `internal/verify/types.go` - Shared types (`HeaderInfo`, `ValidationError`, `DepResult`)
  - `internal/install/checksum.go` - Existing `ChecksumMismatch` type and `VerifyLibraryChecksums()`, `ComputeFileChecksum()`
  - `cmd/tsuku/verify.go:707-741` - Existing `verifyLibraryIntegrity()` inline function

## Gap Analysis

### Minor Gaps

1. **Module file naming pattern**: The design doc specifies `internal/verify/integrity.go`, which aligns with existing modules (`dltest.go`, `header.go`, `deps.go`, `external.go`). No gap.

2. **Result type pattern**: The design proposes `IntegrityResult` with `Skipped` and `Reason` fields, consistent with `DlopenVerificationResult.Skipped` and `DlopenVerificationResult.Warning`. Good alignment.

3. **ComputeFileChecksum reuse**: The issue mentions `computeFileChecksum(path string)` can "delegate to `install.ComputeFileChecksum`". The existing function in `internal/install/checksum.go` at line 22-35 is suitable for delegation. This is consistent with the design doc which shows an internal helper.

4. **symlink handling**: The design doc specifies symlink resolution via `filepath.EvalSymlinks()`. The existing `ComputeBinaryChecksums()` function (line 41-78 in checksum.go) follows this exact pattern. Minor gap: ensure the new implementation follows the same security pattern (checking resolved path is within allowed directory).

### Moderate Gaps

None identified. The issue spec is detailed and includes:
- Clear acceptance criteria with checkboxes
- Specific file locations for new/modified code
- Reference to existing code to refactor (with line numbers)
- Validation script
- Dependencies and downstream dependencies documented

### Major Gaps

None identified. The issue was just created today along with the design doc. No sibling issues have been implemented yet, so there are no established patterns to follow or conflicts to resolve.

## Recommendation

**Proceed**

The issue specification is complete and well-aligned with existing patterns:
1. The design doc provides comprehensive implementation guidance
2. The issue references specific existing code locations for refactoring
3. Type patterns match existing Tier 3 verification (`DlopenResult`, `DlopenVerificationResult`)
4. The existing `install.ComputeFileChecksum()` function can be reused as documented
5. No sibling issues have been completed that would establish patterns to follow

## Implementation Notes

The following existing code patterns should be followed:

1. **Result struct pattern** (from `dltest.go`):
   ```go
   type IntegrityResult struct {
       Verified   int
       Mismatches []IntegrityMismatch
       Missing    []string
       Skipped    bool
       Reason     string  // Similar to Warning in DlopenVerificationResult
   }
   ```

2. **Checksum computation**: Delegate to `install.ComputeFileChecksum()` rather than duplicating the SHA256 logic.

3. **Symlink security**: Follow the pattern from `ComputeBinaryChecksums()` which validates resolved paths stay within the allowed directory using `isWithinDir()`.

4. **Error handling for missing files**: The existing `VerifyLibraryChecksums()` uses `ChecksumMismatch.Error` for file access errors. The new `IntegrityResult.Missing` separates this more cleanly per the design doc.

## Proposed Amendments

None required. The issue spec is sufficient for implementation.
