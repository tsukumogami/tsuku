# Issue 1018 Introspection

## Context Reviewed
- Design doc: docs/designs/DESIGN-library-verify-dlopen.md
- Sibling issues reviewed: #1014, #1015, #1016, #1017, #1020
- Prior patterns identified: EnsureDltest, InvokeDltest, BatchError, sanitizeEnvForHelper, validateLibraryPaths

## Gap Analysis

### Minor Gaps

1. **Flag already exists**: The `--skip-dlopen` flag is already defined in `verify.go` (line 21, 31). The `LibraryVerifyOptions.SkipDlopen` field is also defined. The integration point is in `verifyLibrary()` at line 472-474.

2. **Tier 3 stub location**: The current code at lines 472-474 is a stub:
   ```go
   if !opts.SkipDlopen {
       printInfo("  Tier 3 (dlopen): not yet implemented\n")
   }
   ```
   This needs to be replaced with actual implementation.

3. **EnsureDltest signature**: `EnsureDltest(cfg *config.Config)` returns `(string, error)`. Errors from installation (network failure, etc.) need to be handled gracefully except for checksum failures which should remain errors.

4. **InvokeDltest signature**: `InvokeDltest(ctx, helperPath, paths, tsukuHome)` - requires paths and tsukuHome, which are available in `verifyLibrary()`.

### Moderate Gaps

None - the issue spec is comprehensive and aligns with prior work.

### Major Gaps

None - all dependencies (#1014, #1016, #1017) have been completed and their patterns are established.

## Recommendation

Proceed with implementation. The main work is:
1. Replace the Tier 3 stub with actual dlopen verification
2. Add fallback logic to EnsureDltest or create a wrapper that handles unavailability gracefully
3. When --skip-dlopen is passed, skip silently (no output)
4. When helper unavailable (non-checksum reasons), print warning and skip

## Implementation Approach

1. Create `EnsureDltestWithFallback()` or modify error handling in `verifyLibrary()`:
   - Network failure → skip with warning
   - Helper not installed and download fails → skip with warning
   - Checksum mismatch → return error (security-critical)

2. In `verifyLibrary()`, replace Tier 3 stub:
   - If `opts.SkipDlopen`, skip silently (no output at all)
   - Otherwise, try to ensure helper and invoke dltest
   - Handle errors with appropriate warning/error behavior

3. Warning message format (from design):
   ```
   Warning: tsuku-dltest helper not available, skipping load test
     Run 'tsuku install tsuku-dltest' to enable full verification
   ```
