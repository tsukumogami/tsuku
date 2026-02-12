# Issue 1648 Implementation Plan

## Summary

Create `disambiguate.go` with ranking algorithm and clear-winner detection, add `AmbiguousMatchError` to `resolver.go`, and integrate disambiguation into `ecosystem_probe.go` to replace priority-only selection with popularity-based auto-selection.

## Approach

The implementation uses the existing `probeOutcome` struct which already wraps `builders.ProbeResult` containing `VersionCount` and `HasRepository` fields. The disambiguation logic is isolated in a new file for testability, with minimal changes to the existing ecosystem probe flow. The entry point replaces the current priority-only sorting with a call to `disambiguate()` which handles all selection scenarios.

### Alternatives Considered

- **Inline all logic in ecosystem_probe.go**: Would make the file too large and mix concerns. The design doc explicitly recommends a separate file for testability.
- **Extend probeOutcome with redundant fields**: The introspection confirmed `builders.ProbeResult` already has `VersionCount` and `HasRepository`. Adding duplicate fields to `probeOutcome` would create unnecessary indirection.

## Files to Modify

- `internal/discover/resolver.go` - Add `AmbiguousMatchError` type with `Tool` and `Matches` fields
- `internal/discover/ecosystem_probe.go` - Replace priority-only selection (lines 112-124) with `disambiguate()` call

## Files to Create

- `internal/discover/disambiguate.go` - Ranking algorithm, clear-winner detection, and entry point
- `internal/discover/disambiguate_test.go` - Unit tests for all disambiguation scenarios

## Implementation Steps

- [ ] Add `AmbiguousMatchError` type to `resolver.go` with `Tool string` and `Matches []probeOutcome` fields, plus minimal `Error()` method returning a generic message
- [ ] Create `disambiguate.go` with `rankProbeResults()` function that sorts by downloads DESC, version count DESC, priority ASC
- [ ] Add `isClearWinner()` function checking 10x downloads gap + version count >= 3 + has repository
- [ ] Add `disambiguate()` entry point function that handles single-match, clear-winner, and close-match scenarios
- [ ] Update `ecosystem_probe.go` to call `disambiguate()` after collecting matches, replacing current priority-only sort
- [ ] Extend `Metadata` struct in `resolver.go` to include `VersionCount` and `HasRepository` for downstream issues
- [ ] Create `disambiguate_test.go` with tests for: single match auto-select, clear winner auto-select, close matches returning error, missing download data returning error
- [ ] Run `go test ./internal/discover/...` to verify all tests pass

## Testing Strategy

- **Unit tests**: Cover four main scenarios in `disambiguate_test.go`:
  1. Single match - auto-selects without checking thresholds
  2. Clear winner - 10x gap with version >= 3 and has repository auto-selects
  3. Close matches - returns `AmbiguousMatchError` with both matches
  4. Missing download data - returns `AmbiguousMatchError` (never auto-select on priority alone)
- **Edge cases**: Test boundary conditions (exactly 10x gap, version count = 3, missing repository)
- **Integration**: Existing `ecosystem_probe_test.go` patterns with `mockProber` extend to disambiguation scenarios
- **Manual verification**: `go build ./cmd/tsuku` confirms no compile errors

## Risks and Mitigations

- **Breaking existing priority behavior**: The current priority-based selection will change. Tests like `TestEcosystemProbe_MultipleResults_PriorityRanking` expect priority to win. Mitigation: Update test to use realistic download counts that trigger clear-winner or close-match behavior.
- **Download count comparability across ecosystems**: npm weekly downloads differ from crates.io recent downloads. Mitigation: This is a known limitation noted in the design doc. The 10x threshold provides margin for cross-ecosystem comparisons.

## Success Criteria

- [ ] `disambiguate.go` implements `rankProbeResults()` sorting by downloads DESC, version count DESC, priority ASC
- [ ] `disambiguate.go` implements `isClearWinner()` checking 10x downloads gap + version count >= 3 + has repository
- [ ] `disambiguate.go` implements `disambiguate()` as entry point called from ecosystem probe
- [ ] `AmbiguousMatchError` type added to `resolver.go` with `Tool` and `Matches` fields
- [ ] `ecosystem_probe.go` integrates disambiguation and returns single best result
- [ ] When exactly one match exists, it's auto-selected
- [ ] When clear winner exists (>10x gap + secondary signals), it's auto-selected
- [ ] When no popularity data available, returns `AmbiguousMatchError`
- [ ] Unit tests cover: single match, clear winner, close matches, missing download data
- [ ] Tests pass: `go test ./internal/discover/...`

## Open Questions

None - the introspection confirmed `builders.ProbeResult` has the required fields, and the design doc provides clear implementation guidance.
