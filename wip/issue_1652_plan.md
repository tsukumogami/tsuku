# Issue 1652 Implementation Plan

## Summary

Enhance `AmbiguousMatchError.Error()` to format matches as actionable `--from` suggestions. The type and fields already exist; only the `Error()` method needs updating.

## Approach

Modify the existing `Error()` method in `resolver.go` to produce multi-line output with copy-paste ready `--from` commands. The matches are already sorted by popularity before the error is created (via `disambiguate()`), so no additional sorting is needed in `Error()`.

### Alternatives Considered

- **Implement Suggester interface separately**: Could add a `Suggestion()` method with the `--from` commands. Not chosen because the design spec shows the suggestions as part of the error message itself, not as a separate "Suggestion:" block.
- **Add sorting in Error()**: Could re-sort matches in `Error()` method. Not chosen because `disambiguate()` already ranks matches before creating the error, and sorting is a concern of the caller not the error type.

## Files to Modify

- `internal/discover/resolver.go` - Enhance `Error()` method to format multi-line suggestions
- `internal/discover/disambiguate_test.go` - Update existing test and add more test cases for error formatting

## Files to Create

None

## Implementation Steps

- [ ] Update `AmbiguousMatchError.Error()` to return multi-line formatted output per design spec
- [ ] Update `TestAmbiguousMatchError` to verify new format
- [ ] Add test case for 2 matches
- [ ] Add test case for 3+ matches
- [ ] Add test case for matches with varying source formats (simple name vs owner/repo)
- [ ] Run `go test ./internal/discover/...` to verify all tests pass
- [ ] Run `go vet ./...` and `go build ./...` to verify no issues

## Testing Strategy

- Unit tests: Test `Error()` output format with various match counts (2, 3, 5 matches)
- Unit tests: Test that output matches the design spec format exactly
- Manual verification: Run existing E2E flow to ensure skeleton still works

## Risks and Mitigations

- **Breaking existing callers**: The error message format changes from single-line to multi-line. Mitigation: The `AmbiguousMatchError` type itself and fields don't change, only the string representation. Callers doing string matching on the error message may break, but this is acceptable for display-oriented changes.
- **Format mismatch with CLI handling (#1653)**: The CLI error handler in `errmsg.Fprint` adds an "Error: " prefix. Mitigation: Design spec shows "Error: ..." as the first line, which matches the CLI behavior.

## Success Criteria

- [ ] `AmbiguousMatchError.Error()` returns format matching design spec:
  ```
  Multiple sources found for "bat". Use --from to specify:
    tsuku install bat --from crates.io:sharkdp/bat
    tsuku install bat --from npm:bat-cli
  ```
- [ ] Unit tests cover 2, 3, and 5 match cases
- [ ] All existing tests in `internal/discover/...` continue to pass
- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` reports no issues

## Open Questions

None
