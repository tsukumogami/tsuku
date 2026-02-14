# Issue 1652 Summary

## What Was Implemented

Enhanced `AmbiguousMatchError.Error()` to format matches as actionable `--from` suggestions for non-interactive mode. When the disambiguation system can't prompt interactively and multiple ecosystem matches exist, the error now outputs copy-paste ready commands.

## Changes Made

- `internal/discover/resolver.go`: Enhanced `Error()` method to produce multi-line output with `--from` suggestions. Added `strings` import for builder.
- `internal/discover/disambiguate_test.go`: Expanded `TestAmbiguousMatchError` from single test case to table-driven tests covering 2, 3, and 5 match scenarios plus owner/repo source formats.

## Key Decisions

- **Format matches in-line rather than via separate Suggester**: The design spec shows suggestions as part of the error message itself, not a separate "Suggestion:" block. Kept it simple.
- **No sorting in Error()**: Matches are already sorted by `disambiguate()` before the error is created. The `Error()` method trusts the ordering provided.
- **Two-space indent for suggestions**: Matches the design doc format exactly.

## Trade-offs Accepted

- **Error message format change**: The error message changed from single-line to multi-line. Any code doing string matching on the old format would break, but this is acceptable for display-oriented error formatting.

## Test Coverage

- New tests added: 4 table-driven test cases (two_matches, three_matches, five_matches, source_with_owner/repo_format)
- Coverage: All new code paths covered by tests

## Known Limitations

- None. The implementation matches the design spec completely.

## Future Improvements

- Issue #1653 will add CLI error handling to display this error to users
