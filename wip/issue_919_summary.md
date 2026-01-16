# Issue 919 Summary

## What Was Implemented

Fixed the MetaCPAN version provider to correctly resolve versions regardless of `v` prefix. The provider now normalizes both API-returned versions and user-provided versions before comparison, allowing users to resolve versions like `1.0.35` when the API returns `v1.0.35`.

## Changes Made

- `internal/version/provider_metacpan.go`: Updated `ResolveVersion` to:
  - Normalize user-provided version using `normalizeVersion()` before comparison
  - Compare normalized versions (stripping `v` prefix) for both exact and fuzzy matching
  - Return original API version as `Tag` and normalized version as `Version`
- `internal/version/metacpan_test.go`: Added `TestMetaCPANProvider_ResolveVersion_PrefixNormalization` test covering:
  - Resolving version without `v` prefix when API returns with prefix
  - Resolving version with `v` prefix (backward compatibility)
  - Fuzzy matching with normalized versions
- `testdata/golden/exclusions.json`: Removed 3 carton exclusions (issue #919 now fixed)
- `testdata/golden/plans/c/carton/`: Added golden files for all platforms

## Key Decisions

- **Normalize both sides of comparison**: Rather than just adding the `v` prefix or checking multiple variants, we use the existing `normalizeVersion()` utility. This is more robust and handles edge cases consistently.
- **Keep Tag as original API value**: The `Tag` field preserves the original version string from the API (e.g., `v1.0.35`) for URL construction, while `Version` contains the normalized form (e.g., `1.0.35`) for display and comparison.

## Trade-offs Accepted

- **Fuzzy matching uses normalized versions**: This means `1.0` will match `v1.0.35` correctly. The previous behavior would not match due to the prefix mismatch.

## Test Coverage

- New tests added: 1 (with 3 sub-cases)
- All existing MetaCPAN tests continue to pass

## Known Limitations

- None identified. The fix is consistent with how other providers (GitHub, npm) handle version normalization.

## Future Improvements

None needed. The implementation follows established patterns in the codebase.
