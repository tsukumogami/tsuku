# Issue 1648 Introspection

## Context Reviewed
- Design doc: `docs/designs/DESIGN-disambiguation.md`
- Sibling issues reviewed: None closed (all 8 issues in milestone are open)
- Prior patterns identified:
  - `ConfirmFunc` callback pattern in `internal/discover/llm_discovery.go`
  - `builders.ProbeResult` already has `VersionCount` and `HasRepository` fields
  - Current `probeOutcome` in `ecosystem_probe.go` only wraps `builderName`, `result`, `err`
  - Priority-based selection currently implemented in `ecosystem_probe.go` (lines 112-124)

## Gap Analysis

### Minor Gaps

1. **`builders.ProbeResult` already has the required fields**: The acceptance criteria says "extend `probeOutcome` with `VersionCount int` and `HasRepository bool` fields (or verify existing in `builders.ProbeResult`)". The verification path is correct - `builders.ProbeResult` already has these fields:
   ```go
   type ProbeResult struct {
       Source        string
       Downloads     int
       VersionCount  int    // Already exists
       HasRepository bool   // Already exists
   }
   ```
   The `probeOutcome` struct doesn't need modification because it wraps `*builders.ProbeResult` which carries these fields.

2. **Metadata passthrough incomplete**: Current `ecosystem_probe.go` only passes `Downloads` to `Metadata`:
   ```go
   Metadata: Metadata{
       Downloads: best.result.Downloads,
   },
   ```
   The `Metadata` struct in `resolver.go` doesn't have `VersionCount` or `HasRepository` fields. This will need to be addressed either in this issue (add to Metadata) or deferred to a downstream issue.

3. **Secondary signals for `isClearWinner()`**: The design doc specifies that `isClearWinner()` should check:
   - 10x downloads gap
   - version count >= 3
   - has repository

   The issue acceptance criteria correctly captures this, but the design doc code snippet shows a simpler version without secondary signals. The acceptance criteria should take precedence.

### Moderate Gaps

None identified. The issue spec is well-aligned with the design document and current codebase state.

### Major Gaps

None identified.

## Recommendation

**Proceed**

The issue specification is complete and implementable. The `builders.ProbeResult` already has the required fields, which the acceptance criteria anticipated with the "or verify existing" clause.

## Notes for Implementation

1. **Don't modify `probeOutcome`**: It already wraps `*builders.ProbeResult` which has the needed fields. The ranking functions should operate on `probeOutcome` slices and access `outcome.result.VersionCount` and `outcome.result.HasRepository`.

2. **Consider extending `Metadata`**: For downstream issues (#1651 prompt, #1652 error formatting) to display version count and repository status, the `Metadata` struct may need extension. This can be done in this issue proactively or deferred to those issues.

3. **Test patterns exist**: `ecosystem_probe_test.go` has a comprehensive test structure including `mockProber` and integration tests. New disambiguation tests should follow these patterns.

4. **AmbiguousMatchError location**: The acceptance criteria specifies `resolver.go`, which aligns with the existing error types (`NotFoundError` is already there).
