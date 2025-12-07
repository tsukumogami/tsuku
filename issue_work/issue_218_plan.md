# Issue 218 Implementation Plan

## Summary

Implement a Homebrew version provider that queries the Homebrew API to resolve library versions at runtime, following the existing provider pattern.

## Approach

Follow the established provider pattern (PyPI, crates.io, etc.): create a `HomebrewProvider` struct, add API methods to the Resolver, and register a strategy in the factory. The Homebrew API returns formula info including `versions.stable` for the latest version.

### Alternatives Considered

- **Scrape Homebrew GitHub releases**: Rejected - Homebrew bottles are in GHCR, not GitHub releases, and the official API provides version info directly
- **Use GHCR manifest for versions**: Rejected - GHCR requires auth and the formulae API is simpler for version resolution

## Files to Modify

- `internal/version/resolver.go` - Add `homebrewRegistryURL` field to Resolver struct
- `internal/version/provider_factory.go` - Register HomebrewSourceStrategy

## Files to Create

- `internal/version/homebrew.go` - Homebrew API resolution methods (ResolveHomebrew, ListHomebrewVersions)
- `internal/version/provider_homebrew.go` - HomebrewProvider implementation
- `internal/version/homebrew_test.go` - Unit tests with mocked HTTP responses

## Implementation Steps

- [ ] Add `homebrewRegistryURL` field to Resolver struct
- [ ] Create `homebrew.go` with API types and resolution methods
- [ ] Create `provider_homebrew.go` with HomebrewProvider
- [ ] Add HomebrewSourceStrategy to provider factory
- [ ] Add unit tests with mocked HTTP responses

## Testing Strategy

- **Unit tests**: Mock HTTP responses for formulae.brew.sh API
- **Test cases**:
  - ResolveHomebrew returns latest stable version
  - ListHomebrewVersions returns available versions
  - Invalid formula name handling
  - Network error handling
  - 404 handling for unknown formulae
  - Integration with provider factory for `source = "homebrew"`

## Risks and Mitigations

- **Homebrew API changes**: Mitigated by using stable public API endpoint (formulae.brew.sh); API is well-documented
- **Network failures**: Mitigated by proper error types (ResolverError) matching existing patterns

## Success Criteria

- [ ] Provider queries `https://formulae.brew.sh/api/formula/{formula}.json`
- [ ] Extracts `versions.stable` for latest version
- [ ] Integrated into version provider factory for `source = "homebrew"`
- [ ] Unit tests with mocked HTTP responses

## Open Questions

None
