# Issue 490 Implementation Plan

## Summary

Add cross-platform bottle availability checking to HomebrewBuilder and create a GitHub Actions workflow to test Homebrew recipes on macOS.

## Approach

Extend the existing HomebrewBuilder to check bottle availability for all target platforms (macOS ARM64/x86_64, Linux ARM64/x86_64) during recipe generation. The validation infrastructure already exists - this issue focuses on:
1. Adding platform-specific bottle availability checks
2. Warning when bottles are missing for certain platforms
3. Adding macOS CI for Homebrew bottle validation

### Alternatives Considered
- **Full bottle pre-download before validation**: Rejected - the `homebrew_bottle` action already downloads at runtime. Pre-downloading would duplicate code and slow generation.
- **Require all platforms or fail**: Rejected - graceful degradation with warnings is more user-friendly. Many tools only support certain platforms.

## Files to Modify
- `internal/builders/homebrew.go` - Add `checkBottleAvailability` method and platform warnings
- `internal/builders/homebrew_test.go` - Add tests for bottle availability checking

## Files to Create
- `.github/workflows/homebrew-builder-tests.yml` - CI workflow for macOS Homebrew testing

## Implementation Steps

- [x] Add `checkBottleAvailability` method to query GHCR for all platforms
- [x] Call availability check in `Build()` and add warnings for missing platforms
- [x] Add unit tests for bottle availability checking
- [x] Create macOS CI workflow for Homebrew builder testing
- [ ] Run tests and verify CI passes

## Testing Strategy
- Unit tests: Mock GHCR API responses to test availability checking
- Integration tests: CI workflow validates actual Homebrew bottles on macOS
- Manual verification: Run builder on a formula and check warnings appear

## Risks and Mitigations
- **GHCR API rate limits**: Mitigated by anonymous token which has reasonable limits; parallel queries kept minimal
- **CI macOS runner costs**: Mitigated by only running on Homebrew-related changes

## Success Criteria
- [ ] `checkBottleAvailability` returns availability map for all 4 platforms
- [ ] Missing platform bottles generate warnings in BuildResult
- [ ] Unit tests cover availability checking with mocked API
- [ ] macOS CI workflow validates Homebrew bottles work on macOS
- [ ] All CI checks pass

## Open Questions
None - design doc is clear on requirements.
