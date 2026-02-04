# Issue 1448 Implementation Plan

## Summary

Refactor verification functions in `cmd/tsuku/verify.go` to return errors instead of calling `exitWithCode()`, then call these functions from both `installWithDependencies()` and `installLibrary()` before printing success messages.

## Approach

The current verification code in `verify.go` directly calls `exitWithCode()` when failures occur, making it unsuitable for reuse in the install path. The cleanest approach is to extract the verification logic into error-returning functions that both the `verify` command and install functions can use. This maintains a single source of truth for verification behavior.

### Alternatives Considered

- **Create new verification functions in install path**: Would duplicate logic and risk divergence between `tsuku verify` and post-install verification. Rejected to avoid code duplication.
- **Have install call `tsuku verify` as subprocess**: Adds process overhead and makes error handling difficult. Rejected for poor UX.
- **Add a "silent mode" flag to verify command**: Complicates the API and still doesn't allow programmatic access to results. Rejected.

## Files to Modify

- `cmd/tsuku/verify.go` - Refactor verification functions to return errors instead of calling `exitWithCode()`:
  - `verifyWithAbsolutePath()` -> `verifyToolWithAbsolutePath()` returning `error`
  - `verifyVisibleTool()` -> `verifyToolVisible()` returning `error`
  - Extract display/output code to remain in command handler

- `cmd/tsuku/install_deps.go` - Add post-install verification call:
  - After `mgr.InstallWithOptions()` succeeds at line ~482
  - Before printing "Installation successful!" at line ~543
  - Call refactored verification function and return error if it fails

- `cmd/tsuku/install_lib.go` - Add post-install verification call:
  - After `mgr.InstallLibrary()` succeeds at line ~143
  - Before printing success message at line ~165
  - Call library verification (Tiers 1-3, skip Tier 4 integrity)

## Files to Create

None required. All changes fit within existing files.

## Implementation Steps

- [ ] Step 1: Create error-returning tool verification functions
  - Create `runToolVerification(r *recipe.Recipe, toolName, version, installDir string, versionState *install.VersionState, toolState *install.ToolState, cfg *config.Config, state *install.State, isHidden bool) error`
  - This function performs verification and returns nil on success, error on failure
  - For hidden tools: runs command verification, integrity check, and dependency validation
  - For visible tools: runs full 5-step verification (command, PATH check, resolution, integrity, deps)
  - Move output/display logic to caller

- [ ] Step 2: Create error-returning library verification function
  - Create `runLibraryVerification(name string, state *install.State, cfg *config.Config, opts LibraryVerifyOptions) error`
  - Extracts core logic from `verifyLibrary()`, returns error instead of printing and exiting
  - For post-install, call with `CheckIntegrity: false` (Tier 4 is opt-in)

- [ ] Step 3: Update verifyCmd to use new functions
  - Modify `verifyCmd.Run()` to call the new error-returning functions
  - Handle errors by calling `exitWithCode()` in the command handler
  - Preserve existing output format and behavior

- [ ] Step 4: Add verification to tool installation
  - In `installWithDependencies()`, after `mgr.InstallWithOptions()` succeeds
  - Load recipe if needed for verify command
  - Call `runToolVerification()` with appropriate arguments
  - On error, return immediately (don't print success message)
  - Add "Verifying installation..." progress output

- [ ] Step 5: Add verification to library installation
  - In `installLibrary()`, after `mgr.InstallLibrary()` succeeds
  - Call `runLibraryVerification()` with Tiers 1-3 (skip integrity)
  - On error, return immediately (don't print success message)
  - Add "Verifying library..." progress output

- [ ] Step 6: Handle edge cases
  - Skip verification for system dependency recipes (require_system only)
  - Skip verification if recipe has no verify.command defined (with warning)
  - For post-install, skip PATH checks (tool just installed, user may not have added to PATH yet)

- [ ] Step 7: Write unit tests
  - Test error-returning verification functions with mock state
  - Test that verification failures propagate correctly
  - Test that verification successes allow installation to complete

## Testing Strategy

### Unit Tests
- Test `runToolVerification()` returns error when verify command fails
- Test `runToolVerification()` returns error when integrity check fails
- Test `runToolVerification()` returns error when dependency validation fails
- Test `runLibraryVerification()` returns error for each tier failure
- Test successful verification returns nil

### Integration Tests
- `tsuku install jq` should show verification output and succeed
- `tsuku install` with broken verify command should fail with clear error
- `tsuku install` a library should verify tiers 1-3 before success

### Manual Verification
The validation script from IMPLEMENTATION_CONTEXT.md:
```bash
# Test: install a tool and verify it shows verification output
tsuku install jq --force 2>&1 | grep -q "Verif" || echo "Should show verification output"

# Test: verify command failure causes install failure
# (create recipe with broken verify command and verify install fails)
```

## Risks and Mitigations

- **Risk**: Breaking existing `tsuku verify` behavior
  - **Mitigation**: Keep command handler logic unchanged, only refactor internal functions
  - **Mitigation**: Run full test suite before committing

- **Risk**: Performance impact from running verification after every install
  - **Mitigation**: Verification is already required for reliable installs; this just moves it from manual to automatic
  - **Mitigation**: Most verification is fast (command execution, file checks)

- **Risk**: Post-install PATH check fails because user hasn't added tsuku to PATH yet
  - **Mitigation**: For post-install verification, skip PATH resolution checks (Step 3 of visible tool verification)
  - **Mitigation**: Only verify that the binary exists and runs, not that it's accessible from PATH

## Success Criteria

- [ ] `tsuku install <tool>` runs verification before printing "Installation successful!"
- [ ] `tsuku install <library>` runs Tiers 1-3 verification before printing success
- [ ] Installation fails with clear error message if verification fails
- [ ] `tsuku verify` and post-install verification use the same verification functions
- [ ] All existing tests pass
- [ ] No duplicate verification logic between verify.go and install paths

## Open Questions

None - the implementation approach is clear from the issue description and code analysis.
