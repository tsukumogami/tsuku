# Issue 863 Implementation Plan

## Summary
Replace the hardcoded CaskProvider stub with real Homebrew Cask API integration, following the established patterns from HomebrewProvider (homebrew.go) for HTTP calls, error handling, and URL injection for testing.

## Approach
Follow the existing HomebrewProvider pattern exactly: add `caskRegistryURL` to Resolver, implement API calls with proper security measures (response size limits, URL validation), and return `VersionInfo` with metadata containing `url` and `checksum`. Architecture selection uses `runtime.GOARCH` to pick the appropriate download URL.

### Alternatives Considered
- **Reuse HomebrewProvider code via composition**: Rejected because the Cask API has different response structure and requires architecture-aware URL selection, making direct reuse more complex than beneficial.
- **Share HTTP client logic in a helper function**: Considered but rejected for this issue; the existing homebrew.go pattern duplicates similar code and keeps each provider self-contained. Future refactoring could consolidate.

## Files to Modify
- `internal/version/provider_cask.go` - Replace hardcoded stub with full API implementation
- `internal/version/provider_cask_test.go` - Replace stub tests with mock server tests for all cases
- `internal/version/resolver.go` - Add `caskRegistryURL` field to Resolver struct
- `internal/version/options.go` - Add `WithCaskRegistry` option function

## Files to Create
None - all changes are to existing files.

## Implementation Steps
- [ ] Add `caskRegistryURL` field to Resolver struct in resolver.go (line 39 area)
- [ ] Add `WithCaskRegistry` option function in options.go
- [ ] Define Cask API response struct in provider_cask.go
- [ ] Implement `ResolveCask` method on Resolver (following homebrew.go pattern)
- [ ] Update `CaskProvider.ResolveLatest` to call `resolver.ResolveCask`
- [ ] Update `CaskProvider.ResolveVersion` to validate version against API result
- [ ] Implement architecture selection logic (arm64 vs amd64) for URL
- [ ] Handle missing checksum (`:no_check` case) gracefully with warning
- [ ] Add cask name validation function (similar to `isValidHomebrewFormula`)
- [ ] Write tests for successful resolution
- [ ] Write tests for 404 not found
- [ ] Write tests for architecture selection (arm64/amd64)
- [ ] Write tests for missing checksum case
- [ ] Write tests for invalid cask name validation
- [ ] Write tests for network/parsing errors
- [ ] Run `go test ./internal/version/...` to verify
- [ ] Run `go vet ./...` and `go build` to verify

## Testing Strategy
- Unit tests: Mock HTTP server tests following homebrew_test.go patterns
  - Success case with full metadata
  - 404 not found error
  - Architecture selection (arm64 URL vs x86_64 URL)
  - Missing checksum (sha256 = "" or `:no_check` equivalent)
  - Invalid cask name validation
  - Network errors (connection refused)
  - Parsing errors (invalid JSON)
  - Unexpected status codes
- Manual verification: N/A (API integration tested via mocks)

## Risks and Mitigations
- **API response structure differs from expected**: Use real API response from formulae.brew.sh for reference when implementing struct. Mitigation: document the actual response structure in code comments.
- **Architecture mapping incorrect**: GOARCH values are `arm64` and `amd64`, need to map to Cask's `arm64` and `x86_64`/Intel URL. Mitigation: Test with real VS Code cask which has both architectures.
- **Casks without checksums**: Some casks use `:no_check`. Mitigation: Return empty checksum in metadata with debug-level log; let downstream handle verification skip.

## Success Criteria
- [ ] `go test ./internal/version/...` passes with all new tests
- [ ] `go vet ./...` passes
- [ ] `go build ./cmd/tsuku` succeeds
- [ ] CaskProvider can resolve any valid cask name (not just hardcoded ones)
- [ ] Metadata includes `url` and `checksum` fields
- [ ] Architecture-appropriate URL is selected based on `runtime.GOARCH`
- [ ] Missing checksum handled gracefully (empty string, no error)
- [ ] Test coverage includes: success, not found, arch selection, no checksum, invalid name, network error, parse error

## Open Questions
None - the design document and implementation context provide clear requirements.
