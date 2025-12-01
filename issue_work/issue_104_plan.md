# Issue 104 Implementation Plan

## Summary

Replace the hardcoded 35-second sleep with a configurable timeout approach using the existing `TSUKU_API_TIMEOUT` environment variable, reducing test duration from 35 seconds to under 1 second.

## Approach

The project already has `config.GetAPITimeout()` which reads from `TSUKU_API_TIMEOUT` environment variable. However, `assets.go` uses a hardcoded `APITimeout` constant instead. By switching to the configurable function, the test can set a very short timeout (100ms) and have the mock server sleep just long enough to trigger it (200ms).

### Alternatives Considered
- **Mock HTTP transport**: More invasive, requires replacing the GitHub client's transport layer
- **Use context.WithTimeout in test**: Would work but requires modifying how the test calls FetchReleaseAssets, and the underlying code would still use the 30-second timeout
- **Make APITimeout a package variable**: Less clean than using existing config infrastructure

## Files to Modify
- `internal/version/assets.go` - Replace `APITimeout` constant with `config.GetAPITimeout()` call
- `internal/version/fetch_test.go` - Update test to set `TSUKU_API_TIMEOUT` env var and use shorter sleep

## Files to Create
None

## Implementation Steps
- [ ] Update assets.go to use config.GetAPITimeout() instead of hardcoded constant
- [ ] Update TestFetchReleaseAssets_Timeout to set TSUKU_API_TIMEOUT=100ms
- [ ] Update mock server to sleep 200ms instead of 35 seconds
- [ ] Verify test completes in under 5 seconds
- [ ] Run full test suite to ensure no regressions

## Testing Strategy
- Unit tests: Run `TestFetchReleaseAssets_Timeout` to verify it still detects timeout correctly
- Timing: Verify test completes in under 5 seconds (acceptance criteria)
- Full suite: Run all tests to ensure no regressions

## Risks and Mitigations
- **Risk**: Other tests might depend on the 30-second constant
  - **Mitigation**: Search for usages of `APITimeout` - it's only used in one place (assets.go:125)
- **Risk**: Test flakiness if sleep is too close to timeout
  - **Mitigation**: Use 100ms timeout with 200ms sleep - 2x margin is sufficient

## Success Criteria
- [ ] TestFetchReleaseAssets_Timeout completes in under 5 seconds
- [ ] Test still validates timeout behavior correctly (returns timeout error)
- [ ] All existing tests pass
- [ ] No new test flakiness introduced

## Open Questions
None - straightforward implementation using existing infrastructure.
