# Issue 1649 Summary

## What Was Implemented

Typosquatting detection using Levenshtein edit distance. The `CheckTyposquat` function compares a requested tool name against all discovery registry entries and returns a warning when the name is suspiciously similar (distance 1 or 2). Exact matches do not trigger warnings.

## Changes Made

- `internal/discover/typosquat.go`: New file with `TyposquatWarning` struct, `levenshtein()` distance function, and `CheckTyposquat()` main function
- `internal/discover/typosquat_test.go`: Comprehensive table-driven tests for both Levenshtein algorithm and typosquat checking
- `internal/discover/chain.go`: Added `registry` field, `WithRegistry()` builder method, and typosquat check call in `Resolve()` after normalization

## Key Decisions

- **Pure Go Levenshtein implementation**: No external dependencies; the algorithm is ~30 lines and well-understood
- **Case-insensitive comparison**: Both tool name and registry entries are lowercased before comparison
- **Logging warning (not blocking)**: The check logs warnings rather than blocking or prompting; interactive behavior is deferred to future CLI work
- **Space-optimized DP**: Used O(min(m,n)) space algorithm instead of O(m*n) matrix

## Trade-offs Accepted

- **Short name false positives**: Names like "go", "fd" have many neighbors at distance 2. The design doc acknowledges this as acceptable given the security benefit
- **Linear scan of registry**: O(n) iteration over registry entries. Acceptable for ~500 tools; would need indexing for 10K+

## Test Coverage

- New tests added: 4 test functions (TestLevenshtein, TestCheckTyposquat, TestCheckTyposquatNilRegistry, TestCheckTyposquatEmptyRegistry)
- Test cases: 25+ for Levenshtein, 11 for CheckTyposquat
- Edge cases covered: empty strings, Unicode, case sensitivity, nil/empty registry

## Known Limitations

- Warning is logged but not surfaced to CLI user yet (follow-up issue)
- No Damerau-Levenshtein (transpositions counted as 2 edits, not 1)
- First match returned; if multiple entries are within threshold, only one warning is shown

## Future Improvements

- CLI integration to display typosquat warnings interactively
- Consider BK-tree indexing if registry grows beyond 10K entries
