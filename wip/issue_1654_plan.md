# Issue 1654 Implementation Plan

## Summary

Add `DisambiguationRecord` type to the batch package and integrate tracking into the orchestrator to record disambiguation decisions when processing tools with multiple ecosystem matches. The orchestrator will parse disambiguation info from `tsuku create --json` output.

## Approach

Follow the existing pattern established for `FailureRecord` in the batch package: define the record type in `results.go`, collect records during orchestration, and include them in `BatchResult`. The orchestrator currently invokes `tsuku create` as an external process, so disambiguation tracking data must flow through the CLI's JSON output.

The implementation has two parts:
1. **Batch package**: Add `DisambiguationRecord` type and extend `BatchResult` to hold these records
2. **CLI integration**: Extend `tsuku create --json` output to include disambiguation info when a selection occurs, which the orchestrator parses

### Alternatives Considered

- **Track disambiguation in discover package directly**: Would require significant refactoring to pass tracking data back through the Resolve chain. The introspection already noted that the orchestrator operates at CLI level, not discover level. Rejected because it doesn't match the existing architecture where CLI results are parsed.

- **Add a separate disambiguation output file**: Write disambiguation records to a separate file during batch runs. Rejected because the existing pattern (FailureRecord) embeds tracking data in BatchResult and writes files via SaveResults. Following established patterns is simpler.

## Files to Modify

- `internal/batch/results.go` - Add `DisambiguationRecord` type and extend `BatchResult` with `Disambiguations []DisambiguationRecord` field
- `internal/batch/orchestrator.go` - Parse disambiguation info from `tsuku create --json` output and collect records during batch processing
- `cmd/tsuku/create.go` - Extend JSON output to include disambiguation decision when selection occurs (selection_reason, alternatives, downloads_ratio)

## Files to Create

- `internal/batch/disambiguation_test.go` - Unit tests for disambiguation record parsing and tracking

## Implementation Steps

- [ ] Add `DisambiguationRecord` type to `internal/batch/results.go` with fields: Tool, Selected, Alternatives, SelectionReason, DownloadsRatio, HighRisk
- [ ] Extend `BatchResult` struct to include `Disambiguations []DisambiguationRecord` field
- [ ] Add `WriteDisambiguations` function following the pattern of `WriteFailures` for persistence
- [ ] Extend `Summary()` method to include disambiguation statistics in batch run summary
- [ ] Extend `disambiguate()` function in `internal/discover/disambiguate.go` to return selection reason alongside result
- [ ] Update `tsuku create --json` output to include disambiguation info (selection_reason, alternatives, downloads_ratio, high_risk) when ecosystem probe selects from multiple matches
- [ ] Define `createDisambiguationResult` struct in orchestrator for parsing JSON output
- [ ] Add `parseDisambiguationJSON` function to orchestrator following the pattern of `parseInstallJSON`
- [ ] Update `generate()` in orchestrator to capture and return disambiguation records when present
- [ ] Update `Run()` in orchestrator to collect disambiguation records into BatchResult
- [ ] Update `SaveResults()` to write disambiguation records alongside failures
- [ ] Add unit tests for each SelectionReason value: single_match, 10x_popularity_gap, priority_fallback
- [ ] Add unit test verifying HighRisk is true only for priority_fallback
- [ ] Verify E2E flow still works with existing integration tests

## Testing Strategy

- Unit tests: Test `DisambiguationRecord` JSON marshaling, `parseDisambiguationJSON` parsing for each selection reason, `HighRisk` flag logic
- Integration tests: Test the full orchestrator flow with a fake tsuku binary that returns disambiguation JSON output
- Manual verification: Run `go test ./internal/batch/...` to confirm all tests pass

## Risks and Mitigations

- **Breaking existing batch workflow**: The changes add new fields without modifying existing behavior. New fields use `json:"-"` tag like existing `Failures` field to prevent JSON serialization issues. Run existing tests to verify.
- **CLI output format changes**: The `--json` flag output is an internal interface between CLI and orchestrator, not a public API. Document the new fields in code comments.
- **Disambiguation info availability**: The discover package's `disambiguate()` function currently returns `*DiscoveryResult` without selection reason. Need to extend return value or add a wrapper type to capture this metadata.

## Success Criteria

- [ ] `DisambiguationRecord` type exists in batch package with all required fields
- [ ] Orchestrator tracks disambiguation decisions when processing tools with multiple ecosystem matches
- [ ] `SelectionReason` values include: `single_match`, `10x_popularity_gap`, `priority_fallback`
- [ ] `HighRisk` is `true` when selection used `priority_fallback`
- [ ] Disambiguation records are accessible from `BatchResult` (for issue #1655)
- [ ] Unit tests verify correct tracking for each selection reason
- [ ] Existing tests continue to pass (E2E flow not broken)

## Open Questions

None - the implementation approach is clear from the existing patterns and the introspection findings.
