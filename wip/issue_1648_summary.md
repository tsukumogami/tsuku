# Issue 1648 Summary

## What Was Implemented

Core disambiguation logic for the ecosystem probe that ranks multiple matches by popularity and auto-selects clear winners based on a 10x popularity threshold with secondary signals (version count >= 3, has repository link).

## Changes Made

- `internal/discover/disambiguate.go`: New file implementing ranking algorithm and clear-winner detection
  - `rankProbeResults()`: Sorts matches by downloads DESC, version count DESC, priority ASC
  - `isClearWinner()`: Checks 10x threshold + version count >= 3 + has repository
  - `disambiguate()`: Entry point integrating single-match, clear-winner, and ambiguous scenarios
  - `toDiscoveryResult()` and `toDiscoveryMatches()`: Conversion helpers

- `internal/discover/disambiguate_test.go`: Comprehensive unit tests
  - Tests for ranking algorithm correctness
  - Tests for clear-winner detection edge cases (exactly 10x, missing data, missing signals)
  - Tests for disambiguate() entry point (single match, clear winner, close matches, missing data)
  - Test for AmbiguousMatchError formatting

- `internal/discover/resolver.go`: Extended with disambiguation types
  - Added `AmbiguousMatchError` type with `Tool` and `Matches` fields
  - Added `DiscoveryMatch` type for error display
  - Extended `Metadata` struct with `VersionCount` and `HasRepository` fields

- `internal/discover/ecosystem_probe.go`: Integrated disambiguation
  - Replaced priority-only selection with call to `disambiguate()`
  - Removed now-unused `sort` import

- `internal/discover/ecosystem_probe_test.go`: Updated existing tests
  - Renamed and updated `TestEcosystemProbe_MultipleResults_PriorityRanking` to test clear-winner scenario
  - Added `TestEcosystemProbe_MultipleResults_Ambiguous` for close-match scenario
  - Updated `TestQualityFiltering_PriorityRankingAfterFilter` with realistic download counts

## Key Decisions

- **Used existing ProbeResult fields**: The `builders.ProbeResult` already had `VersionCount` and `HasRepository` fields, so no struct extension was needed
- **Extended Metadata**: Added `VersionCount` and `HasRepository` to `Metadata` proactively for downstream issues (#1651 prompt, #1652 error formatting)
- **Separate file for disambiguation**: Kept disambiguation logic in a dedicated file for testability, following the design doc recommendation

## Trade-offs Accepted

- **Missing download data triggers AmbiguousMatchError**: When either match lacks download data, we return an error rather than falling back to priority. This is a security-conscious design choice to prevent ecosystem-squatting attacks.

## Test Coverage

- New tests added: 17 test cases across 4 test functions
- Coverage: All disambiguation scenarios covered (single match, clear winner, close matches, missing data, edge cases)

## Known Limitations

- Download count comparability across ecosystems: npm weekly downloads differ from crates.io recent downloads. The 10x threshold provides margin, but cross-ecosystem comparisons may not be perfectly accurate.

## Future Improvements

- Downstream issues will add:
  - Interactive prompts for ambiguous cases (#1650, #1651)
  - Formatted `--from` suggestions in error messages (#1652, #1653)
  - Batch pipeline tracking (#1654, #1655)
