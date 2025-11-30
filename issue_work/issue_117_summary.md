# Issue 117 Summary

## What Was Implemented

Go toolchain version provider that queries go.dev/dl JSON API to resolve Go versions. This enables recipes to use `source = "go_toolchain"` for version resolution.

## Changes Made
- `internal/version/provider_go_toolchain.go`: New provider implementing VersionResolver and VersionLister interfaces
- `internal/version/resolver.go`: Added `ResolveGoToolchain` and `ListGoToolchainVersions` methods
- `internal/version/provider_factory.go`: Registered `GoToolchainSourceStrategy` for source = "go_toolchain"
- `internal/version/provider_go_toolchain_test.go`: Comprehensive unit tests

## Key Decisions
- **Version format**: Returns versions without "v" prefix (e.g., "1.23.4") per Go toolchain convention (distinct from Go modules which use "v" prefix)
- **Only stable releases**: Filters out beta/RC releases from the API response
- **Test isolation**: Created wrapper type for injecting mock HTTP server, following existing test patterns

## Trade-offs Accepted
- **No injectable URL in production Resolver**: The go.dev/dl URL is hardcoded. This is consistent with other providers and acceptable since go.dev/dl is the only official source.

## Test Coverage
- New tests added: 12 test functions covering normalization, resolution, listing, error handling, and factory integration
- All tests use mock HTTP servers for isolation

## Known Limitations
- Go toolchain URL is hardcoded (no custom proxy support)
- No caching of API responses (relies on Go's internal caching and HTTP client behavior)

## Future Improvements
- Could add support for custom Go download proxies if needed for enterprise environments
