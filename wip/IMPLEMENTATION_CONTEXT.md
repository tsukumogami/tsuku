# Implementation Context: Issue #943

**Source**: docs/designs/DESIGN-library-verification.md

## Summary

This is a **testable tier** issue from the Library Verification Infrastructure milestone.

**Goal**: Add library type detection to the `verify` command and implement flag routing for library-specific verification options.

## Key Requirements

1. Add `--integrity` flag (enable checksum verification for libraries)
2. Add `--skip-dlopen` flag (skip load testing for libraries)
3. Detect library recipes (via `Recipe.IsLibrary()`) and route to library verification
4. Implement stub `verifyLibrary()` function that checks library directory exists
5. Keep existing tool verification path unchanged

## Design Reference

From DESIGN-library-verification.md:

**New Flags:**
- `--integrity`: Enables Level 4 (checksum verification)
- `--skip-dlopen`: Disables Level 3 (load testing)

**State Lookup:**
Libraries use different state storage than tools - look up in `state.Libs` instead of `state.Installed`.

**Output Format:**
```
Verifying <library> (version X.Y.Z)...
  Library directory exists
  (Full verification not yet implemented)
<library> is working correctly
```

## Dependencies

- None (this issue can be implemented independently)

## Downstream Dependencies

- #947 (Tier 1 header validation design) depends on this routing being in place

## Files to Modify

1. `cmd/tsuku/verify.go` - Add flags and library detection/routing
2. `cmd/tsuku/verify_test.go` - Add tests for flag parsing and library detection

## Exit Criteria

- [ ] `--integrity` flag is added to verify command
- [ ] `--skip-dlopen` flag is added to verify command
- [ ] Library recipes (type = "library") trigger library verification path
- [ ] Tool recipes continue to use existing verification path
- [ ] Stub verification succeeds if library directory exists
- [ ] Code compiles and existing tests pass
