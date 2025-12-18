# Issue 613 Implementation Plan

## Summary
Implement nix_install decomposition to capture flake.lock and derivation information at eval time, enabling deterministic execution via the nix_realize primitive. The code structure already exists; we need to test and fix the implementation.

## Approach
The nix_install already has a Decompose() method that calls helper functions to capture:
1. Flake metadata (via `nix flake metadata --json`)
2. Derivation and output paths (via `nix derivation show`)
3. Nix version and system type

The nix_realize primitive action already exists to execute with locked parameters.

Our approach:
1. Test the existing decomposition with hello-nix
2. Fix any issues found in the decomposition or realize steps
3. Ensure the sandbox test T40 (now nix_hello-nix_simple) passes
4. Add hello-nix to CI test matrix if not already present

### Alternatives Considered
- **Alternative 1: Implement from scratch** - Not needed, the infrastructure exists
- **Alternative 2: Skip flake metadata capture** - Would not meet acceptance criteria for deterministic lock file capture

## Files to Modify
- `internal/actions/nix_install.go` - Fix any issues in Decompose() method
- `internal/actions/nix_realize.go` - Fix any issues in Execute() method
- `internal/actions/nix_portable.go` - Fix helper functions if needed
- `test-matrix.json` - Add nix test to scheduled or linux CI (if not already)

## Files to Create
None - all necessary files exist

## Implementation Steps
- [ ] Test current nix_install decomposition with hello-nix locally
- [ ] Debug and fix any issues in the eval phase (Decompose)
- [ ] Debug and fix any issues in the exec phase (nix_realize Execute)
- [ ] Verify flake.lock content is properly captured in plan
- [ ] Verify derivation paths are captured
- [ ] Test sandbox execution with generated plan
- [ ] Ensure nix_hello-nix_simple test passes in CI
- [ ] Run full test suite to ensure no regressions

## Testing Strategy
- Unit tests: Verify Decompose() returns correct structure with mock data
- Integration tests: Run `tsuku eval hello-nix` and verify plan structure
- Sandbox tests: Run nix_hello-nix_simple test to validate end-to-end
- Manual verification:
  - `./tsuku eval hello-nix --yes > plan.json`
  - Inspect plan.json for locks, derivation_path, output_path
  - `./tsuku install --plan plan.json --sandbox --force`
  - Verify hello binary works

## Risks and Mitigations
- **Risk**: nix-portable bootstrapping during eval may be slow
  - **Mitigation**: Already implemented - nix-portable is bootstrapped once and cached

- **Risk**: Flake metadata fetch might fail or timeout
  - **Mitigation**: Add proper error handling and timeouts

- **Risk**: CI environment may not support nix-portable
  - **Mitigation**: Test is Linux-only and uses tier3, indicating it's expected to work

- **Risk**: Derivation paths may not be consistent across eval/exec
  - **Mitigation**: Use `--no-update-lock-file` flag during realize to ensure consistency

## Success Criteria
- [x] nix_install.Decompose() captures flake.lock content at eval time
- [ ] Derivation and output paths captured via `nix derivation show`
- [ ] Exec phase uses locked parameters (already implemented in nix_realize)
- [ ] Locked flake reference stored in plan
- [ ] nix-portable version and system type captured
- [ ] nix_hello-nix_simple (T40) passes sandbox tests (Linux only)

## Open Questions
None - implementation path is clear from existing code structure.
