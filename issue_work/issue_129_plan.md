# Issue 129 Implementation Plan

## Summary

Implement a MetaCPAN version provider following the established patterns (RubyGems, PyPI, npm) to resolve versions for CPAN distributions from the MetaCPAN API.

## Approach

Follow the existing provider patterns established by `provider_rubygems.go` and `rubygems.go`. Create a thin provider struct that delegates to resolver methods, with the API logic in a separate file for maintainability. Register strategies in the factory for both explicit source (`source="metacpan"`) and inferred (`cpan_install` action) cases.

### Alternatives Considered

- **Single file implementation**: Combine provider and API logic in one file. Rejected because it doesn't match existing patterns (rubygems.go + provider_rubygems.go are separate).
- **Using MetaCPAN's /download_url endpoint**: This endpoint is designed for cpanm, but doesn't provide version history. Rejected because we need ListVersions capability.

## Files to Create

- `internal/version/metacpan.go` - API interaction logic (validation, ListMetaCPANVersions, ResolveMetaCPAN)
- `internal/version/provider_metacpan.go` - Provider struct implementing VersionLister interface
- `internal/version/metacpan_test.go` - Unit tests for API logic and provider

## Files to Modify

- `internal/version/provider_factory.go` - Register MetaCPANSourceStrategy and InferredMetaCPANStrategy
- `internal/version/resolver.go` - Add metacpanRegistryURL field and NewWithMetaCPANRegistry constructor

## Implementation Steps

- [x] Add metacpanRegistryURL field to Resolver struct in resolver.go
- [x] Add NewWithMetaCPANRegistry constructor for testing
- [x] Create metacpan.go with distribution name validation
- [x] Implement ListMetaCPANVersions using POST /_search endpoint
- [x] Implement ResolveMetaCPAN using GET /release/{distribution} endpoint
- [x] Create provider_metacpan.go with MetaCPANProvider struct
- [x] Implement VersionLister interface (ListVersions, ResolveLatest, ResolveVersion, SourceDescription)
- [x] Register MetaCPANSourceStrategy in provider_factory.go
- [x] Register InferredMetaCPANStrategy in provider_factory.go
- [x] Write unit tests for distribution name validation
- [x] Write unit tests for ListMetaCPANVersions with mock server
- [x] Write unit tests for ResolveMetaCPAN with mock server
- [x] Write unit tests for error handling (404, rate limit, invalid content-type)
- [x] Write unit tests for provider strategy matching
- [x] Run full test suite to verify no regressions

## Testing Strategy

- **Unit tests**: Use httptest.NewTLSServer for mock MetaCPAN API responses
- **Validation tests**: Test distribution name validation (reject module names with `::`, accept hyphenated names)
- **Error handling tests**: 404, 429, invalid content-type, network errors
- **Integration test hint**: `tsuku versions App-Ack` returns version list (deferred to issue #144)

## Risks and Mitigations

- **MetaCPAN API format changes**: Use defensive parsing, validate content-type. Low risk as API is stable.
- **Distribution vs module name confusion**: Validate and reject names containing `::` with clear error message suggesting conversion.

## Success Criteria

- [x] `internal/version/provider_metacpan.go` created
- [x] Queries `GET /release/{distribution}` for latest version
- [x] Queries `POST /release/_search` for version history
- [x] Distribution name validation (reject names with `::`, normalize to `-` format)
- [x] Factory strategies registered (MetaCPANSourceStrategy, InferredMetaCPANStrategy)
- [x] Response size limits (10MB) and content-type validation
- [x] Unit tests pass

## Open Questions

None - design document provides clear API specifications.
