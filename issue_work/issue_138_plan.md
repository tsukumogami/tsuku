# Issue 138 Implementation Plan

## Summary

Apply decompression bomb protection to all HTTP clients in the codebase by refactoring them to use the already-hardened `newHTTPClient()` function from resolver.go or by applying equivalent Transport configuration.

## Approach

The resolver.go already has a properly configured `newHTTPClient()` function with:
- `DisableCompression: true` - prevents auto-decompression bombs
- `Accept-Encoding: identity` header support
- SSRF protection via redirect validation
- Proper timeouts

The strategy is to:
1. Export a shared HTTP client factory from internal/version for reuse
2. Update all other HTTP clients to use this shared factory or apply equivalent hardening

### Alternatives Considered
- **Create a shared package for HTTP client**: Would require more refactoring and package restructuring. The version package already has the correct implementation, so exporting from there is simpler.
- **Duplicate the Transport config in each package**: Would lead to code duplication and potential for drift. Rejected in favor of reuse.

## Files to Modify

- `internal/version/resolver.go` - Export `NewHTTPClient()` (capitalize) for use by other packages
- `internal/version/provider_nixpkgs.go` - Replace hardcoded HTTP clients with imported factory
- `internal/actions/download.go` - Add Transport with `DisableCompression: true` and SSRF protection
- `internal/registry/registry.go` - Add Transport with `DisableCompression: true`

## Files to Create

None - reusing existing pattern

## Implementation Steps

- [ ] Export `NewHTTPClient()` from resolver.go (rename from unexported `newHTTPClient`)
- [ ] Update nixpkgs provider to use `NewHTTPClient()`
- [ ] Harden download action HTTP client with decompression protection and SSRF checks
- [ ] Harden registry client HTTP client with decompression protection
- [ ] Add security tests for the hardened clients
- [ ] Run all tests and verify

## Testing Strategy

- **Unit tests**: Add tests in the respective packages to verify:
  - HTTP client has `DisableCompression: true`
  - Requests send `Accept-Encoding: identity` header where appropriate
  - Compressed responses are rejected
- **Existing tests**: All existing tests must pass (regression testing)
- **Manual verification**: Build tsuku and verify basic install operations work

## Risks and Mitigations

- **Risk**: Breaking existing functionality by changing HTTP client behavior
  - **Mitigation**: Existing tests should catch regressions; run full test suite
- **Risk**: Import cycle when version package is imported by other packages
  - **Mitigation**: download.go and registry.go already don't import version, so they'll get inline Transport config instead of the shared factory

## Success Criteria

- [ ] All HTTP clients in the codebase have `DisableCompression: true`
- [ ] All tests pass
- [ ] No regressions in basic tool installation functionality
- [ ] Build succeeds

## Open Questions

None - the fix is well-defined from the security review.
