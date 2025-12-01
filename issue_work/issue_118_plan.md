# Issue 118 Implementation Plan

## Summary

Implement a Go module version provider that queries proxy.golang.org to resolve versions for Go modules (e.g., "v1.64.8" with "v" prefix per go.mod convention).

## Approach

Follow the established pattern from GoToolchainProvider:
1. Create `provider_goproxy.go` with the provider struct and methods
2. Add resolver methods to `resolver.go` for the actual API calls
3. Register a new strategy in `provider_factory.go` for `source = "goproxy:<module>"`

Key API endpoints:
- `https://proxy.golang.org/{module}/@latest` - Returns JSON with latest version
- `https://proxy.golang.org/{module}/@v/list` - Returns newline-separated version list

### Module Path Encoding

Go proxy requires special encoding for uppercase characters in module paths:
- Uppercase letters are replaced with `!` followed by the lowercase letter
- Example: `github.com/User/Repo` -> `github.com/!user/!repo`

## Files to Modify
- `internal/version/resolver.go` - Add `ResolveGoProxy` and `ListGoProxyVersions` methods, add `goProxyURL` field
- `internal/version/provider_factory.go` - Register `GoProxySourceStrategy`

## Files to Create
- `internal/version/provider_goproxy.go` - GoProxyProvider implementing VersionResolver and VersionLister
- `internal/version/provider_goproxy_test.go` - Unit tests

## Implementation Steps
- [ ] Add goProxyURL field to Resolver struct and constructors
- [ ] Implement `encodeModulePath` for uppercase character escaping
- [ ] Add `ResolveGoProxy` method to resolver (queries `@latest`)
- [ ] Add `ListGoProxyVersions` method to resolver (queries `@v/list`)
- [ ] Create `provider_goproxy.go` with provider struct
- [ ] Register `GoProxySourceStrategy` in factory (handles `goproxy:<module>`)
- [ ] Add unit tests for path encoding
- [ ] Add unit tests for version resolution
- [ ] Verify all tests pass

## Testing Strategy
- Unit tests: Mock HTTP server to test API parsing, module path encoding, version parsing
- Path encoding tests: Verify uppercase escaping works correctly

## Risks and Mitigations
- **API changes**: Low risk - proxy.golang.org API is stable. Mitigation: Parse only required fields.
- **Module path injection**: Validate module paths before use.

## Success Criteria
- [ ] `provider_goproxy.go` implemented with VersionResolver and VersionLister interfaces
- [ ] Provider queries `proxy.golang.org/{module}/@latest` for latest version
- [ ] Provider queries `proxy.golang.org/{module}/@v/list` for version list
- [ ] Module path encoding works (uppercase -> `!lowercase`)
- [ ] Factory integration: `source = "goproxy:<module>"` works in recipes
- [ ] Unit tests pass for path encoding and version resolution

## Open Questions
None
