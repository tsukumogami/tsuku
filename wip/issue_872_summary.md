# Issue 872 Summary

## What Was Implemented

Implemented a tap version provider that resolves formula metadata from third-party Homebrew taps via GitHub. The provider fetches formula files directly from `raw.githubusercontent.com`, parses Ruby formula files using regex patterns, and returns version information with bottle URLs and checksums.

## Changes Made

- `internal/recipe/types.go`: Added `Tap` field to `VersionSection` struct for tap specification
- `internal/version/provider_tap.go`: Created `TapProvider` implementing `VersionResolver` interface
  - GitHub raw content fetching with formula file discovery
  - Platform tag selection with fallback chain
  - Bottle URL construction
- `internal/version/tap_parser.go`: Ruby formula parsing via targeted regex patterns
  - Version extraction (`version "x.y.z"`)
  - Bottle block parsing (`bottle do...end`)
  - Root URL and SHA256 checksum extraction
  - Platform tag mapping (darwin/linux)
- `internal/version/provider_factory.go`: Added `TapSourceStrategy` for factory integration
- `internal/version/provider_tap_test.go`: Comprehensive test coverage

## Key Decisions

- **VersionInfo.Metadata pattern**: Used existing `VersionInfo.Metadata` map instead of custom return struct, following cask provider pattern for consistency
- **Regex-based parsing**: Targeted stable Homebrew formula patterns rather than full Ruby parsing for simplicity and reliability
- **Platform fallback chain**: macOS platform tags include fallback chain (sonoma -> ventura -> monterey) for compatibility
- **Formula discovery**: Check multiple locations (Formula/, HomebrewFormula/, root) as different taps use different structures

## Trade-offs Accepted

- **macOS version detection deferred**: Using default macOS version (Sonoma) with fallback chain rather than runtime detection. This is acceptable as bottles are generally backwards compatible.
- **Source-only formulas not supported**: Tsuku only supports pre-built bottles. Clear error message directs users to appropriate alternatives.
- **Version pinning limited**: Can only resolve the current formula version since we fetch from HEAD. This is consistent with tap behavior.

## Test Coverage

- New tests added: 14 test cases
- Parser tests: version extraction, alternate sha256 syntax, error cases
- Provider tests: ResolveLatest, ResolveVersion, formula not found, interface compliance
- Strategy tests: CanHandle with various recipe configurations
- Helper function tests: parseTap, getPlatformTags, buildBottleURL

## Known Limitations

- macOS version detection not implemented (defaults to Sonoma with fallback)
- No GitHub token support yet (issue #875 adds this)
- No caching layer (issue #873 adds this)
- Cannot resolve historical versions (only current formula version)

## Future Improvements

Downstream issues will add:
- #873: Local metadata caching to reduce API calls
- #874: Factory integration for production use
- #875: GitHub token support for higher rate limits
