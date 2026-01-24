# Issue 1016 Introspection

## Context Reviewed

- **Design doc**: `docs/designs/DESIGN-library-verify-dlopen.md`
- **Sibling issues reviewed**: #1014 (skeleton, closed), #1015 (recipe, closed)
- **Prior PRs reviewed**: #1070 (skeleton impl), #1072 (recipe impl)

## Prior Patterns Identified

### From #1014 (Skeleton - PR #1070)

1. **File location**: `internal/verify/dltest.go` is the home for all dltest code
2. **API pattern**: `InvokeDltest(ctx context.Context, helperPath string, paths []string)` signature
3. **Context handling**: Already uses `exec.CommandContext(ctx, ...)` with ctx.Err() check
4. **Error handling**: Parse JSON even on non-zero exit (exit 1 means some libraries failed)
5. **Test patterns**:
   - JSON parsing tests use mock data, not actual helper
   - Tests avoid subprocess invocation to prevent recursion
   - State tests use temp directories with real StateManager

### From #1015 (Recipe - PR #1072)

1. **Installation pattern**: Uses `installDltest()` subprocess to reuse tsuku infrastructure
2. **State checking**: Uses `stateManager.GetToolState()` and checks `ActiveVersion`
3. **Dev mode**: `pinnedDltestVersion == "dev"` accepts any installed version

## Gap Analysis

### Minor Gaps

1. **Test naming convention**: Existing tests use `TestDlopenResult_*` and `TestEnsureDltest_*` patterns. The issue acceptance criteria mention `TestBatchSplitting`, `TestTimeoutHandling`, `TestBatchRetryOnCrash` - should follow established patterns like `TestInvokeDltest_BatchSplitting`, `TestInvokeDltest_TimeoutHandling`, `TestInvokeDltest_RetryOnCrash`.

2. **Context propagation**: The existing `InvokeDltest` accepts a context but doesn't enforce timeout. The design shows adding a 5-second timeout *inside* `invokeHelper`. Decision: add batch timeout internally, let caller's context still work for overall cancellation.

3. **Function organization**: Issue mentions adding to `invokeHelper` but current code uses `InvokeDltest`. These are the same function - just naming variation between design doc and implementation.

### Moderate Gaps

None identified. The issue spec is well-aligned with current implementation.

### Major Gaps

None identified. All acceptance criteria are achievable with the current codebase structure.

## Recommendation

**Proceed**

The issue specification is complete and aligned with the current codebase. The only adjustments are minor naming conventions that can be incorporated during implementation.

## Implementation Notes

Based on introspection, recommend the following approach:

1. **Batch function**: Add `splitIntoBatches(paths []string, batchSize int) [][]string` internal helper

2. **Timeout wrapper**: Modify `InvokeDltest` to:
   - Split paths into batches of 50
   - For each batch, create child context with 5-second timeout
   - On timeout, include batch info in error message
   - On crash (exit code != 0/1/2 or signal), retry with halved batch size

3. **Result aggregation**: Collect `[]DlopenResult` from each batch into single slice

4. **Tests**: Follow naming pattern `TestInvokeDltest_*` and use mock helper output where possible to avoid subprocess recursion

## Proposed Amendments

None required - the issue spec is complete.
