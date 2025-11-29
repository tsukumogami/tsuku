# Issue 83 Implementation Plan

## Summary

Add a first-run notice system to the telemetry package that displays a one-time informational message about telemetry collection, using a marker file to track whether the notice has been shown.

## Approach

Add notice functionality to the existing `internal/telemetry` package. The `Client` struct will gain a method to show the notice if needed, checking a marker file for state persistence. This follows the existing package structure from #82 rather than creating a new package.

### Alternatives Considered
- Separate `notice` package: Would add unnecessary package proliferation for a simple feature
- Store notice state in config file: Config system doesn't exist yet (#85), and mixing telemetry state into config creates coupling

## Files to Modify
- `internal/telemetry/client.go` - Add notice method and marker file logic

## Files to Create
- `internal/telemetry/notice.go` - Dedicated file for notice functionality (better organization)
- `internal/telemetry/notice_test.go` - Unit tests for notice

## Implementation Steps
- [ ] Create `notice.go` with notice text constant and ShowNoticeIfNeeded function
- [ ] Add unit tests in `notice_test.go`
- [ ] Verify all tests pass and linting succeeds

## Testing Strategy
- Unit tests:
  - Notice shown when marker file doesn't exist
  - Notice not shown when marker file exists
  - Notice not shown when telemetry is disabled
  - Marker file created after notice shown
  - TSUKU_HOME respected for marker file location
  - Handles directory creation if ~/.tsuku doesn't exist

## Risks and Mitigations
- **Risk**: Directory permissions - user might not have write access
  - **Mitigation**: Silent failure if marker file can't be created (follow telemetry's silent failure pattern)

## Success Criteria
- [ ] Notice displays correct text to stderr
- [ ] Marker file created at `~/.tsuku/telemetry_notice_shown`
- [ ] TSUKU_HOME environment variable respected
- [ ] Notice skipped when TSUKU_NO_TELEMETRY=1
- [ ] All existing tests pass
- [ ] New tests achieve high coverage

## Open Questions
None - requirements are clear from issue and design doc.
