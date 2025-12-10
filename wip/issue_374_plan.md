# Issue 374 Implementation Plan

## Summary

Add `--yes` flag to `tsuku create` command that skips the recipe preview confirmation (to be implemented in #375) and shows a warning message about skipping review.

## Approach

Add the flag and variable now so the infrastructure is in place. When the preview flow is added in #375, it will check for `createAutoApprove` to skip the confirmation prompt. For now, the `--yes` flag will simply print a warning message to inform users that review is being skipped.

### Alternatives Considered

- Wait until #375 is implemented: This would delay the flag addition and create a larger PR. Adding the flag now keeps the change atomic and allows parallel work.
- Skip the warning when preview flow doesn't exist: This would be inconsistent - users would see different behavior depending on whether they use `--yes` before or after #375 lands.

## Files to Modify

- `cmd/tsuku/create.go` - Add `--yes` flag and warning message

## Implementation Steps

- [ ] Add `createAutoApprove` variable and `--yes` flag in `init()`
- [ ] Add warning message when `--yes` flag is used in `runCreate()`
- [ ] Update help text to document the flag

## Testing Strategy

- Manual verification: Run `tsuku create --help` to confirm flag appears
- Manual verification: Run `tsuku create foo --from crates.io --yes` and confirm warning is shown
- Unit tests: Not needed for flag parsing (cobra handles this automatically)

## Risks and Mitigations

- Risk: Flag name conflicts with future flags - Mitigation: `--yes` is a standard CLI convention (yum, apt, etc.)
- Risk: Warning message style inconsistent - Mitigation: Follow existing warning pattern in the codebase

## Success Criteria

- [ ] `tsuku create --help` shows `--yes` flag with description
- [ ] Running with `--yes` prints warning to stderr
- [ ] All existing tests pass
- [ ] Build succeeds
