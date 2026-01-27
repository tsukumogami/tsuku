# Issue 1168 Implementation Plan

## Summary

Refactor Tier 4 integrity verification from scattered inline code in `cmd/tsuku/verify.go` and `internal/install/checksum.go` into a proper module at `internal/verify/integrity.go` with structured result types that match the established patterns from Tier 3 (dltest.go).

## Approach

The implementation follows the existing verification module patterns established by `dltest.go`, creating a clean separation between the verification logic (`internal/verify/integrity.go`) and the CLI output formatting (`cmd/tsuku/verify.go`). The new module reuses `install.ComputeFileChecksum()` for the actual SHA256 computation.

### Alternatives Considered

- **Inline refactor in verify.go only**: Rejected because it would not follow the established module pattern where verification logic lives in `internal/verify/`. The current inline function is the legacy approach that this issue aims to fix.
- **Move everything to install package**: Rejected because the install package handles installation-time operations while verification is a distinct concern. The verify package already contains all other verification tiers.

## Files to Modify

- `cmd/tsuku/verify.go` - Replace `verifyLibraryIntegrity()` inline function with call to `verify.VerifyIntegrity()`; remove the inline function definition (lines 707-741)
- `internal/install/checksum.go` - Remove "basic implementation" comment from `VerifyLibraryChecksums()` (line 185-186); function remains for backward compatibility

## Files to Create

- `internal/verify/integrity.go` - Main module with `IntegrityResult`, `IntegrityMismatch`, `VerifyIntegrity()`, and `computeFileChecksum()` helper
- `internal/verify/integrity_test.go` - Unit tests covering success, mismatch, missing file, and skipped scenarios

## Implementation Steps

- [ ] Step 1: Create `internal/verify/integrity.go` with types
  - `IntegrityMismatch` struct (Path, Expected, Actual, Error fields)
  - `IntegrityResult` struct (Verified int, Mismatches []IntegrityMismatch, Missing []string, Skipped bool, Reason string)
  - `computeFileChecksum()` helper that delegates to `install.ComputeFileChecksum()`

- [ ] Step 2: Implement `VerifyIntegrity()` function
  - Accept libDir string and checksums map[string]string parameters
  - Handle nil/empty checksums by returning Skipped=true with appropriate Reason
  - Iterate stored checksums, compute actual checksums, track verified count and mismatches
  - Separate missing files into the Missing slice (not Mismatches)
  - Use `filepath.EvalSymlinks()` before checksumming to match stored checksum format

- [ ] Step 3: Create `internal/verify/integrity_test.go` with unit tests
  - `TestVerifyIntegrity_AllMatch` - all files verified successfully
  - `TestVerifyIntegrity_Mismatch` - file content changed
  - `TestVerifyIntegrity_MissingFile` - file deleted after installation
  - `TestVerifyIntegrity_EmptyChecksums` - returns Skipped=true with reason
  - `TestVerifyIntegrity_NilChecksums` - returns Skipped=true with reason

- [ ] Step 4: Update `cmd/tsuku/verify.go` to use new module
  - Replace `verifyLibraryIntegrity()` call with `verify.VerifyIntegrity()`
  - Update result handling to use new structured `IntegrityResult`
  - Keep output format consistent (use same truncateChecksum, printInfo patterns)

- [ ] Step 5: Remove old inline function from `cmd/tsuku/verify.go`
  - Delete `verifyLibraryIntegrity()` function definition (lines 707-741)
  - Remove "basic implementation" comments from code

- [ ] Step 6: Remove "basic implementation" comment from `internal/install/checksum.go`
  - Remove lines 185-186 comment about "basic implementation for CI validation"
  - Keep `VerifyLibraryChecksums()` for backward compatibility (may be used elsewhere)

- [ ] Step 7: Run tests and verify
  - Run `go test ./internal/verify/...` to verify new tests pass
  - Run `go test ./...` to verify no regressions
  - Run `go vet ./...` and `golangci-lint run --timeout=5m ./...`
  - Build CLI with `go build -o tsuku ./cmd/tsuku`
  - Manual e2e verification: `./tsuku verify <library> --integrity`

## Testing Strategy

- **Unit tests** (`internal/verify/integrity_test.go`):
  - Success case: all stored checksums match current files
  - Mismatch case: file modified after installation
  - Missing case: file deleted (should appear in Missing slice, not Mismatches)
  - Skip case: nil or empty checksums map (Skipped=true, Reason populated)
  - Error handling: permission denied, broken symlinks

- **Integration tests**:
  - Not strictly required for this refactor since it's moving existing functionality
  - The existing e2e command `tsuku verify <library> --integrity` exercises the full path

- **Manual verification**:
  - Build tsuku binary
  - Install a library: `./tsuku install openssl`
  - Verify with integrity: `./tsuku verify openssl --integrity`
  - Confirm output format matches existing behavior

## Risks and Mitigations

- **Output format regression**: The CLI output format could change slightly during refactoring, potentially breaking scripts. Mitigation: carefully preserve the exact output format using the same `printInfo`, `printInfof`, and `truncateChecksum` helpers.

- **Missing file classification**: The new `Missing` slice separates file-not-found errors from mismatches. This is a deliberate improvement but could affect downstream code. Mitigation: verify no other code depends on the old `ChecksumMismatch.Error` pattern for missing files.

- **Symlink handling**: Checksums are stored for real files after symlink resolution. If the new code handles symlinks differently, verification could fail spuriously. Mitigation: follow the exact same pattern from `install.ComputeBinaryChecksums()` using `filepath.EvalSymlinks()`.

## Success Criteria

- [ ] `internal/verify/integrity.go` exists with `IntegrityResult`, `IntegrityMismatch`, `VerifyIntegrity()`, `computeFileChecksum()`
- [ ] `internal/verify/integrity_test.go` exists with tests for success, mismatch, missing, and skipped cases
- [ ] `cmd/tsuku/verify.go` uses `verify.VerifyIntegrity()` instead of inline function
- [ ] `verifyLibraryIntegrity()` is removed from `cmd/tsuku/verify.go`
- [ ] "basic implementation" comments removed from affected files
- [ ] All tests pass: `go test ./...`
- [ ] Lint passes: `go vet ./...` and `golangci-lint run --timeout=5m ./...`
- [ ] E2E works: `tsuku verify <library> --integrity` produces expected output

## Open Questions

None - the issue spec and introspection analysis are complete. All implementation details are clear from the existing code patterns.
