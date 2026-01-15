# Issue 914 Summary

## What Was Implemented

Consistent version sorting across all version providers to ensure versions are always returned in descending order (latest first). This fixes the non-deterministic behavior where scripts using `tsuku versions <tool> | head -1` could get arbitrary versions instead of the actual latest.

## Changes Made

- `internal/version/version_utils.go`: Enhanced CompareVersions with prerelease handling, version normalization, and build metadata support
- `internal/version/version_sort.go`: New file with SortVersionsDescending function and IsSortedDescending helper
- `internal/version/version_sort_test.go`: Comprehensive tests for sorting and comparison
- `internal/version/resolver.go`: Added sorting to ListGoProxyVersions, ListGitHubVersions, and ListGoToolchainVersions

## Key Decisions

- **Centralized sort function**: Chose Option A from design doc - providers explicitly call SortVersionsDescending rather than automatic wrapper, making behavior transparent and debuggable
- **Normalize before compare**: Use existing normalizeVersion to strip prefixes (v, go, Release_) before comparison, ensuring v1.0.0 equals 1.0.0
- **Stable > prerelease**: Stable versions always sort higher than prereleases of same core version (1.0.0 > 1.0.0-rc.1)
- **Prerelease ordering**: Common identifiers have defined order (alpha < beta < rc), others sort lexicographically

## Trade-offs Accepted

- **Minor performance overhead**: Sorting adds O(n log n) but version lists are typically <100 items, making this negligible
- **Not removing duplicate sort calls**: Some providers (npm, pypi, etc.) already sorted - we leave their sorts in place since sorting is idempotent

## Test Coverage

- New tests added: 31 test cases across 6 test functions
- Coverage for: prerelease comparison, version normalization, build metadata, sorting correctness
- All existing tests continue to pass

## Known Limitations

- Cross-format comparison (e.g., calver vs semver within same tool) produces reasonable but not necessarily semantically correct ordering
- Tools that drastically change versioning schemes may require manual intervention
- Build metadata is ignored per semver spec, not used for ordering

## Future Improvements

- Could add test assertions to existing provider tests to verify sorted output
- Could add IsSortedDescending checks in debug builds to catch regressions
