# Issue 117 Implementation Plan

## Summary

Implement a Go toolchain version provider that queries go.dev/dl JSON API to resolve Go versions (e.g., "1.23.4" without "v" prefix).

## Approach

Follow the established pattern from existing providers (PyPI, npm, crates.io):
1. Create `provider_go_toolchain.go` with the provider struct and methods
2. Add resolver methods to `resolver.go` for the actual API calls
3. Register a new strategy in `provider_factory.go` for `source = "go_toolchain"`

This approach was chosen because it matches existing patterns (single file for provider, resolver methods in resolver.go, factory registration), maintaining codebase consistency.

### Alternatives Considered
- **Inline API calls in provider**: Rejected because other providers delegate to resolver methods, which enables better testability and consistent HTTP client handling.

## Files to Modify
- `internal/version/resolver.go` - Add `ResolveGoToolchain` and `ListGoToolchainVersions` methods
- `internal/version/provider_factory.go` - Register `GoToolchainSourceStrategy`

## Files to Create
- `internal/version/provider_go_toolchain.go` - GoToolchainProvider struct implementing VersionResolver and VersionLister
- `internal/version/provider_go_toolchain_test.go` - Unit tests

## Implementation Steps
- [x] Create `provider_go_toolchain.go` with provider struct
- [x] Add `ResolveGoToolchain` method to resolver
- [x] Add `ListGoToolchainVersions` method to resolver
- [x] Register `GoToolchainSourceStrategy` in factory
- [x] Add unit tests for version parsing and resolution
- [x] Add unit tests for fuzzy version matching
- [x] Verify all tests pass

Mark each step [x] after it is implemented and committed. This enables clear resume detection.

## Testing Strategy
- Unit tests: Mock HTTP server to test API parsing, version sorting, error handling
- Manual verification: Build and test against real go.dev/dl API

## Risks and Mitigations
- **API changes**: Low risk - go.dev/dl API is stable. Mitigation: Parse only required fields.
- **Network issues**: Handled by existing HTTP client with timeouts and error handling.

## Success Criteria
- [ ] `provider_go_toolchain.go` implemented with VersionResolver and VersionLister interfaces
- [ ] Provider queries `https://go.dev/dl/?mode=json` for stable releases
- [ ] Returns versions without "v" prefix (e.g., "1.23.4")
- [ ] Factory integration: `source = "go_toolchain"` works in recipes
- [ ] Unit tests pass for version parsing and latest resolution

## Open Questions
None
