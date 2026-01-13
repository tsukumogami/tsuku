# Issue 708 Summary

## What Was Implemented

Added validation to detect when recipes have a dynamic version source (homebrew, github_repo, fossil_repo) but use `download_file` action with hardcoded version URLs. This inconsistency suggests the recipe should use `download` action with `{version}` placeholders instead.

## Changes Made

- `internal/recipe/hardcoded.go`: Added `DownloadFileVersionMismatch` struct, `hasDynamicVersionSource` helper, and `DetectDownloadFileVersionMismatch` function
- `internal/recipe/hardcoded_test.go`: Added comprehensive tests for the new detection logic
- `cmd/tsuku/validate.go`: Integrated the new detection into the validate command

## Key Decisions

- **Only warn for dynamic sources**: Recipes with no version source (using `pin = "X.Y.Z"`) are intentionally static and should not trigger warnings
- **Separate from existing hardcoded detection**: The new function complements `DetectHardcodedVersions` which intentionally skips `download_file` for static assets
- **Reuse existing version pattern detection**: Leverages `findVersionPattern` for consistency with existing hardcoded version detection

## Trade-offs Accepted

- **Silent TOML field ignored**: The `pin` field in TOML files is not parsed into the Go struct (BurntSushi/toml ignores unknown fields). Detection relies on absence of dynamic sources rather than presence of `pin`. This works correctly but is implicit.

## Test Coverage

- New tests added: 8 test cases (2 for hasDynamicVersionSource, 6 for DetectDownloadFileVersionMismatch) plus 1 for String() output
- All existing tests continue to pass

## Known Limitations

- The `pin` field is not actually parsed by the Go code - detection works by checking that no dynamic source is configured
- False positives possible if a URL contains version-like strings that aren't actually versions (mitigated by existing exclusion patterns)

## Future Improvements

- Consider parsing the `pin` field explicitly in VersionSection to make pinned version handling more explicit
- Could add auto-fix capability to convert download_file to download with {version} placeholder
