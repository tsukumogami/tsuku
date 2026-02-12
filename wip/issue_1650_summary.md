# Issue 1650 Summary

## Implementation Complete

Added `ConfirmDisambiguationFunc` callback type for interactive disambiguation prompts.

## Changes Made

### resolver.go
- Added `ProbeMatch` struct for callback input with fields: Builder, Source, Downloads, VersionCount, HasRepository
- Added `ConfirmDisambiguationFunc` type: `func(matches []ProbeMatch) (int, error)`

### ecosystem_probe.go
- Added `confirmDisambiguation` field to `EcosystemProbe` struct
- Added `EcosystemProbeOption` type for functional options
- Added `WithConfirmDisambiguation()` option function
- Updated `NewEcosystemProbe()` to accept variadic options
- Pass callback to `disambiguate()` call

### disambiguate.go
- Added `toProbeMatches()` helper function
- Updated `disambiguate()` signature to accept `ConfirmDisambiguationFunc`
- Added callback invocation logic: when matches are close and callback is provided, invoke it before returning AmbiguousMatchError
- Added bounds checking for returned index

### disambiguate_test.go
- Updated existing tests to pass `nil` for callback parameter
- Added `TestConfirmDisambiguationCallback` with subtests:
  - callback invoked with correct data
  - callback selection honored (select first)
  - callback selection honored (select second)
  - callback error propagates
  - out of range index returns AmbiguousMatchError
  - negative index returns AmbiguousMatchError
  - callback not invoked for clear winner
- Added `TestToProbeMatches` for helper function

## Acceptance Criteria Status

- [x] `ConfirmDisambiguationFunc` type defined: `func(matches []ProbeMatch) (int, error)`
- [x] `ProbeMatch` struct defined with fields for builder name, source, downloads, version count, and repository presence
- [x] `EcosystemProbe` struct has optional `ConfirmDisambiguation` field of this callback type
- [x] Helper function `toProbeMatches([]probeOutcome) []ProbeMatch` converts internal results to callback format
- [x] Unit tests verify callback invocation with correct match data
- [x] E2E flow still works (all existing tests pass)

## Test Results

All discover package tests pass:
```
go test ./internal/discover/...
ok      github.com/tsukumogami/tsuku/internal/discover   0.463s
```

## Files Modified

1. `internal/discover/resolver.go` - 2 new types
2. `internal/discover/ecosystem_probe.go` - callback field and option function
3. `internal/discover/disambiguate.go` - helper function and callback invocation
4. `internal/discover/disambiguate_test.go` - comprehensive callback tests
