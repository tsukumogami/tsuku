# Issue 708 Implementation Plan

## Summary

Add a new validation function `DetectDownloadFileVersionMismatch` that warns when a recipe has a dynamic version source (homebrew, github, npm, etc.) but uses `download_file` action with version-like patterns in URLs, suggesting conversion to `download` action with `{version}` placeholders.

## Approach

Extend the existing pattern from `DetectHardcodedVersions` (which skips `download_file` intentionally) by adding a companion function that specifically detects the inconsistent case: dynamic version source + hardcoded `download_file` URLs. This follows the same architectural pattern as `DetectRedundantVersion` in the version package.

The function will:
1. Check if the recipe has a dynamic version source (not pin, not empty)
2. Scan `download_file` steps for version-like patterns in URLs
3. Return warnings suggesting use of `download` action instead

### Alternatives Considered

- **Add to existing DetectHardcodedVersions**: Rejected because `DetectHardcodedVersions` intentionally skips `download_file` (static URLs are expected for static assets). Modifying it would conflate two different concerns and could break existing behavior for legitimate static assets.

- **Add to ValidateSemantic**: Rejected because this is a warning, not an error, and follows the same pattern as `DetectRedundantVersion` (called from CLI validate command, returns warnings).

## Files to Modify

- `internal/recipe/hardcoded.go` - Add `DetectDownloadFileVersionMismatch` function and `DownloadFileVersionMismatch` struct (same file as related `DetectHardcodedVersions`)
- `internal/recipe/hardcoded_test.go` - Add unit tests for the new detection function
- `cmd/tsuku/validate.go` - Add call to new detection function and append warnings to result

## Files to Create

None - all changes fit within existing files.

## Implementation Steps

- [ ] Define `DownloadFileVersionMismatch` struct in `hardcoded.go` with fields: Step, URL, DetectedVersion, SuggestedURL
- [ ] Add `hasDynamicVersionSource` helper function to check if recipe has a non-pin version source (checks for Source != "", or GitHubRepo != "", or FossilRepo != "")
- [ ] Implement `DetectDownloadFileVersionMismatch` function that scans `download_file` steps when recipe has dynamic version source
- [ ] Add unit tests covering:
  - Dynamic source (homebrew) + download_file with version in URL (should warn)
  - Pin version + download_file with version in URL (should NOT warn)
  - No version source + download_file with version in URL (should NOT warn)
  - Dynamic source + download_file with no version pattern (should NOT warn)
  - Dynamic source + download_file with {version} placeholder (should NOT warn)
- [ ] Integrate into `cmd/tsuku/validate.go` after existing hardcoded version check
- [ ] Run tests: `go test ./internal/recipe/... ./cmd/tsuku/...`
- [ ] Run linter: `golangci-lint run --timeout=5m ./...`

## Testing Strategy

- Unit tests: Test the detection function with various recipe configurations (dynamic sources, pin, empty, with/without version patterns)
- Integration: Validate against `testdata/recipes/gdbm-source.toml` which exemplifies the problem case (homebrew source + hardcoded download_file URL)
- Manual verification: Run `go build -o tsuku ./cmd/tsuku && ./tsuku validate testdata/recipes/gdbm-source.toml` to confirm warning is produced

## Risks and Mitigations

- **False positives**: The existing `findVersionPattern` function has exclusion patterns (api versions, architecture strings). Reuse this function to minimize false positives.
- **Pin detection reliability**: The `pin` field is directly on `VersionSection` struct, making it easy to detect. No risk of missing pin configurations.

## Success Criteria

- [ ] `tsuku validate testdata/recipes/gdbm-source.toml` produces a warning about `download_file` with hardcoded version
- [ ] `tsuku validate testdata/recipes/bash-source.toml` does NOT produce a warning (uses `pin = "5.3"`)
- [ ] All existing tests pass
- [ ] No linter warnings introduced

## Open Questions

None - the introspection clarified that `pin` is the static version source to skip.
