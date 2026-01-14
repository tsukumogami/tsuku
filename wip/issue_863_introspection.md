# Issue 863 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-cask-support.md`
- Sibling issues reviewed: #862 (closed), #864, #865, #866 (open)
- PR reviewed: #871 (merged - walking skeleton implementation)

## Prior Patterns Identified

From #862 and PR #871, the walking skeleton established:

1. **File locations**:
   - Provider: `internal/version/provider_cask.go`
   - Tests: `internal/version/provider_cask_test.go`
   - Factory registration: `internal/version/provider_factory.go` (CaskSourceStrategy already registered)

2. **Interface implementation**:
   - `CaskProvider` implements `VersionResolver` interface (ResolveLatest, ResolveVersion, SourceDescription)
   - Uses `VersionInfo` struct with `Metadata` map for `url` and `checksum` fields
   - Does NOT implement `VersionLister` (ListVersions) - matches Homebrew pattern where only one version is exposed

3. **API pattern from Homebrew formula provider**:
   - Uses `Resolver` methods for actual API calls (e.g., `ResolveHomebrew`, `ListHomebrewVersions`)
   - Provider wraps resolver methods
   - Input validation via `isValidHomebrewFormula` pattern
   - Response size limiting via `io.LimitReader`
   - Structured error handling via `ResolverError`

4. **Template integration**:
   - Metadata fields (`url`, `checksum`) are populated in `VersionInfo.Metadata` map
   - Plan generator expands `{version.url}` and `{version.checksum}` templates

5. **Recipe configuration**:
   - Recipe `VersionSection` already has `Cask` field (`internal/recipe/types.go`)
   - `CaskSourceStrategy.CanHandle()` checks `source == "cask"` AND `cask != ""`

## Gap Analysis

### Minor Gaps

1. **Architecture handling not explicit in issue**: The issue mentions "Architecture selection works correctly (arm64 selects Apple Silicon URL, amd64 selects Intel URL)" but doesn't specify implementation approach. Per the design doc, this should use `runtime.GOARCH` to select the correct URL from the API response.

2. **Resolver method naming**: The Homebrew provider pattern uses `resolver.ResolveHomebrew()` and `resolver.ListHomebrewVersions()`. The cask provider should add `resolver.ResolveCask()` method (not yet implemented). Currently the stub directly returns hardcoded values. The full implementation should delegate to a Resolver method.

3. **Cask name validation**: Issue mentions "Invalid cask names rejected with clear error message (validation matches existing patterns)" but doesn't reference the specific function. Should follow `isValidHomebrewFormula` pattern - create `isValidCaskName()` with similar validation rules (lowercase, numbers, hyphens, no path traversal).

4. **Response size limit**: Not mentioned in issue, but required per Homebrew pattern. Should use `maxCaskResponseSize` constant with `io.LimitReader`.

### Moderate Gaps

None identified. The issue acceptance criteria align well with the design document and established patterns.

### Major Gaps

None identified. The issue spec is complete given the walking skeleton infrastructure.

## Recommendation

**Proceed**

The issue specification is complete and implementable. The walking skeleton (#862) established all necessary interfaces and patterns. Implementation should:

1. Add `ResolveCask()` method to `Resolver` in `internal/version/homebrew.go` (or new `cask.go` file in resolver) following the `ResolveHomebrew()` pattern
2. Add `isValidCaskName()` validation function
3. Update `CaskProvider.ResolveLatest()` to delegate to `resolver.ResolveCask()`
4. Handle architecture-specific URL selection using `runtime.GOARCH`
5. Handle `:no_check` checksum gracefully with warning

## Implementation Notes

### API Response Structure

The Homebrew Cask API at `https://formulae.brew.sh/api/cask/{name}.json` returns:

```json
{
  "token": "visual-studio-code",
  "name": ["Visual Studio Code"],
  "version": "1.96.4",
  "sha256": "abc123...",
  "url": "https://update.code.visualstudio.com/1.96.4/darwin/stable",
  "url_specs": {
    "verified": "update.code.visualstudio.com"
  },
  "artifacts": [
    {"app": ["Visual Studio Code.app"]},
    {"binary": ["{{appdir}}/Visual Studio Code.app/Contents/Resources/app/bin/code", {"target": "code"}]}
  ]
}
```

For architecture-specific URLs, some casks have:
- `url`: default/Universal URL
- ARM64-specific URL in `url_specs` or different endpoint

### Test Coverage Required

Per acceptance criteria:
1. Successful resolution with real API response structure
2. 404 handling (cask not found)
3. Architecture selection (arm64 vs amd64)
4. Missing checksum (`:no_check`) handling
5. Invalid cask name validation

### Files to Modify/Create

| File | Action |
|------|--------|
| `internal/version/provider_cask.go` | Replace stub with API integration |
| `internal/version/cask.go` (new) | Add `ResolveCask()` method to Resolver, or add to `homebrew.go` |
| `internal/version/provider_cask_test.go` | Expand tests per acceptance criteria |
