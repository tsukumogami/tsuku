# Issue 473 Implementation Plan

## Summary

Add `ExecutePlan(ctx, plan)` method to the Executor that executes installation plans with checksum verification for download steps, using the `ChecksumMismatchError` type from #470.

## Approach

The implementation adds a new method to the Executor struct that:
1. Creates an execution context from the plan metadata
2. Iterates through plan steps sequentially
3. For download steps with checksums, verifies the computed checksum matches the plan
4. Returns `ChecksumMismatchError` on mismatch with tool/version context for recovery

The existing `Execute()` method remains functional for now (cleanup in #479).

### Alternatives Considered

- **Inline checksum in download action params**: Would require modifying the download action interface. Rejected because the plan-level checksum is conceptually separate from action parameters.
- **Post-download validation pass**: Would require tracking which files were downloaded. Rejected because per-step verification is cleaner and fails faster.

## Files to Modify

- `internal/executor/executor.go` - Add `ExecutePlan()` method and helper

## Implementation Steps

- [ ] Add `computeFileChecksum()` helper function
- [ ] Add `ExecutePlan(ctx, plan)` method to Executor
- [ ] Add `executeDownloadWithVerification()` helper method
- [ ] Add unit tests for ExecutePlan with mock plans
- [ ] Add tests for checksum mismatch scenarios
- [ ] Verify all tests pass

## Testing Strategy

- Unit tests with mock InstallationPlans
- Test scenarios:
  - Simple plan with no download steps (extract, chmod, etc.)
  - Plan with download step and matching checksum
  - Plan with download step and mismatching checksum (expect ChecksumMismatchError)
  - Plan with unknown action (expect error)
  - Context cancellation during execution

## Risks and Mitigations

- **Download action may modify params**: The existing download action reads `dest` from params. We need to ensure the destination path is correctly resolved from params or step.URL.
  - Mitigation: Follow the same logic as existing download action for determining destination

- **Checksum algorithm mismatch**: Plans store SHA256 checksums, but need to ensure the computation matches.
  - Mitigation: Use sha256 consistently (matching plan_generator.go)

## Success Criteria

- [ ] `ExecutePlan()` method added to Executor
- [ ] Download steps verify checksums against plan
- [ ] `ChecksumMismatchError` returned on mismatch with tool/version info
- [ ] Existing `Execute()` method still works
- [ ] Unit tests cover pass/fail scenarios
- [ ] `go test ./internal/executor/...` passes
