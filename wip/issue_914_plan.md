# Issue 914 Implementation Plan

## Summary

Implement consistent version sorting across all version providers per DESIGN-version-sorting.md.

## Current State Analysis

### Providers that already sort (using CompareVersions):
- `npm.go:150` - ListNpmVersions
- `pypi.go:201` - ListPyPIVersions
- `provider_crates_io.go:178` - ListCratesIOVersions
- `rubygems.go:187` - ListRubyGemsVersions
- `metacpan.go:357` - ListMetaCPANVersions
- `homebrew.go:229` - ListHomebrewVersions

### Providers that do NOT sort:
- `resolver.go:600` - ListGoProxyVersions (returns API order)
- `resolver.go:288` - ListGitHubVersions (returns API order)
- `resolver.go:429` - ListGoToolchainVersions (needs verification)

### CompareVersions gaps:
Current implementation at version_utils.go:51-84:
- Does NOT handle prereleases (1.0.0-alpha vs 1.0.0)
- Does NOT normalize versions before comparison (v1.0.0 vs 1.0.0)
- Basic numeric comparison only

## Implementation Steps

### Step 1: Enhance CompareVersions with prerelease handling
File: `internal/version/version_utils.go`

Changes:
1. Normalize versions before comparison (strip v prefix, handle go prefix, etc.)
2. Split version into core and prerelease parts
3. Compare core parts first
4. If core is equal, compare prerelease (stable > prerelease)
5. Order prereleases: alpha < beta < rc

### Step 2: Add SortVersionsDescending function
File: `internal/version/version_sort.go` (new file)

```go
// SortVersionsDescending sorts versions in descending order (latest first).
func SortVersionsDescending(versions []string) []string
```

### Step 3: Add sorting to GoProxy provider
File: `internal/version/resolver.go`

Change ListGoProxyVersions to call SortVersionsDescending before returning.

### Step 4: Add sorting to GitHub provider
File: `internal/version/resolver.go`

Change ListGitHubVersions to call SortVersionsDescending before returning.

### Step 5: Verify GoToolchain provider
File: `internal/version/resolver.go`

Check if ListGoToolchainVersions needs sorting.

### Step 6: Add comprehensive tests
Files:
- `internal/version/version_sort_test.go` (new)
- Update existing provider tests with sorted assertions

Tests to add:
- TestCompareVersions_Prereleases
- TestCompareVersions_Normalization
- TestSortVersionsDescending_Semver
- TestSortVersionsDescending_Mixed
- TestAssertVersionsSorted helper

## Files to Modify

| File | Changes |
|------|---------|
| `internal/version/version_utils.go` | Enhance CompareVersions |
| `internal/version/version_sort.go` | New file: SortVersionsDescending |
| `internal/version/version_sort_test.go` | New file: sorting tests |
| `internal/version/resolver.go` | Add sort calls to ListGoProxyVersions, ListGitHubVersions |
| `internal/version/version_utils_test.go` | Add prerelease comparison tests |

## Testing Strategy

1. Unit tests for CompareVersions enhancements
2. Unit tests for SortVersionsDescending
3. Integration tests verifying provider output is sorted
4. Run full test suite to catch regressions

## Risk Assessment

- **Low risk**: Changes are additive and localized to internal/version package
- **Backward compatible**: Existing behavior for already-sorted providers unchanged
- **Performance**: O(n log n) sorting is negligible for typical version counts (<100)
