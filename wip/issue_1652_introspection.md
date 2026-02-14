# Issue 1652 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-disambiguation.md`
- Sibling issues reviewed: #1648, #1650, #1651, #1654, #1655
- Prior patterns identified:
  - `AmbiguousMatchError` type already exists in `resolver.go` with basic `Error()` method
  - `DiscoveryMatch` struct holds builder/source/downloads/versionCount/hasRepository fields
  - `ProbeMatch` struct exists for callback display with same fields
  - Matches are already ranked by `disambiguate()` before populating the error
  - PR #1657 established the pattern of returning `AmbiguousMatchError` from `disambiguate()`
  - PR #1661 established the callback pattern for interactive disambiguation

## Gap Analysis

### Minor Gaps

1. **Current `Error()` method is minimal**: The existing `AmbiguousMatchError.Error()` returns `"multiple sources found for '<tool>': use --from to specify"` but the issue acceptance criteria specifies a more detailed format:
   ```
   Error: Multiple sources found for "bat". Use --from to specify:
   tsuku install bat --from crates.io:sharkdp/bat
   tsuku install bat --from npm:bat-cli
   tsuku install bat --from rubygems:bat
   ```

2. **Sort by popularity already implemented**: The issue AC says "Matches are sorted by popularity before formatting" - this is already done by `rankProbeResults()` before the error is created. No additional work needed.

3. **Location clear**: The type already exists in `resolver.go`. Enhancement goes in the same file.

4. **CLI error handling deferred to #1653**: Issue #1652 focuses only on the error type and formatting. Displaying this in create/install is #1653's scope.

### Moderate Gaps

None. The scope is well-defined: enhance the existing `AmbiguousMatchError.Error()` method to produce the formatted `--from` suggestions output.

### Major Gaps

None. The foundation work from #1648, #1650, #1651, and #1654 is complete and this issue slots cleanly into the existing architecture.

## Recommendation

**Proceed**

## Implementation Notes

The work is straightforward:
1. Enhance `AmbiguousMatchError.Error()` in `resolver.go` to produce multi-line output with `--from` suggestions
2. Matches are already ranked (caller responsibility via `disambiguate()`)
3. Add unit tests for various match counts (2, 3, 5 matches)
4. The `DiscoveryMatch` struct already has all fields needed for formatting

Key pattern from #1661: The `formatDownloadCount()` helper in `create.go` formats download counts (45K, 1.2M). Consider whether to duplicate or share for error formatting, but the simple format in the error message (just the source specifier) may not need download counts.

Design doc reference for exact format (line 277-282):
```
Error: Multiple sources found for "bat". Use --from to specify:
  tsuku install bat --from crates.io:sharkdp/bat
  tsuku install bat --from npm:bat-cli
  tsuku install bat --from rubygems:bat
```

Note the two-space indent for the suggestions.
