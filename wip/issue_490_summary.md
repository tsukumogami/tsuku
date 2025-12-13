# Issue 490 Summary

## What Was Implemented

Added cross-platform bottle availability checking to HomebrewBuilder. The builder now queries GHCR to verify bottles exist for all target platforms (macOS ARM64, macOS x86_64, Linux x86_64, Linux ARM64) and adds warnings when bottles are missing for certain platforms. Also added a macOS CI workflow to test Homebrew bottle functionality.

## Changes Made

- `internal/builders/homebrew.go`:
  - Added `BottleAvailability` struct and `checkBottleAvailability` method
  - Added `getGHCRToken` and `fetchGHCRManifest` helper methods
  - Added `targetPlatforms` and `platformDisplayNames` variables
  - Updated `Build()` to check bottle availability and add warnings

- `internal/builders/homebrew_test.go`:
  - Added tests for bottle availability checking with mocked GHCR API
  - Added `mockGHCRTransport` for redirecting GHCR requests

- `.github/workflows/homebrew-builder-tests.yml`:
  - New workflow for macOS unit and integration testing
  - Integration test validates homebrew_bottle action on macOS

## Key Decisions

- **Graceful degradation**: Missing platform bottles generate warnings rather than errors, allowing recipe generation to continue for platforms that have bottles available
- **GHCR manifest parsing**: Use `strings.HasSuffix` to extract platform tags from ref.name annotations, handling arbitrary version formats (e.g., "1.0.0.arm64_sonoma")

## Trade-offs Accepted

- **Single GHCR request per check**: One manifest fetch checks all platforms; could cache across builds but current approach is simple and stateless

## Test Coverage

- New tests added: 5 (bottle availability tests)
- Coverage: Maintained above 70%

## Known Limitations

- Bottle availability check requires network access to GHCR
- Check is non-fatal if it fails (continues with generation)

## Future Improvements

- Cache GHCR tokens across multiple availability checks
- Add bottle availability info to BuildResult for programmatic access
