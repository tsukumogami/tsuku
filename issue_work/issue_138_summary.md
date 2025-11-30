# Issue 138 Summary

## What Was Implemented

Added decompression bomb protection to all HTTP clients in the codebase by setting `DisableCompression: true` in their Transport configuration and adding defense-in-depth measures.

## Changes Made

- `internal/version/resolver.go`: Exported `NewHTTPClient()` with enhanced documentation for reuse by other packages
- `internal/version/provider_nixpkgs.go`: Replaced hardcoded HTTP clients with calls to `NewHTTPClient()`, added `Accept-Encoding: identity` headers
- `internal/actions/download.go`: Added `newDownloadHTTPClient()` with full security hardening (DisableCompression, SSRF protection via IP validation and DNS rebinding prevention), added `Accept-Encoding: identity` header and Content-Encoding validation
- `internal/registry/registry.go`: Added `newRegistryHTTPClient()` with DisableCompression enabled
- `internal/version/security_test.go`: Added `TestHTTPClientDisableCompression` test
- `internal/actions/download_test.go`: Added `TestDownloadHTTPClient_DisableCompression`, `TestDownloadAction_RejectsCompressedResponse`, and `TestDownloadAction_ValidateIP` tests
- `internal/registry/registry_test.go`: Added `TestRegistryHTTPClient_DisableCompression` test

## Key Decisions

- **Inline client factories vs shared package**: Chose to create inline HTTP client factory functions in each package (download.go, registry.go) rather than importing from version package to avoid potential import cycles. The version package's `NewHTTPClient()` is exported for packages that can safely import it (like nixpkgs provider).
- **Full SSRF protection for downloads**: Added complete IP validation (private, loopback, link-local, multicast, unspecified) and DNS rebinding protection to download action since it handles arbitrary URLs from recipes.
- **Simpler protection for registry**: Registry only fetches from known GitHub URLs, so full SSRF protection was not added - just DisableCompression.

## Trade-offs Accepted

- **Code duplication**: IP validation logic is duplicated between resolver.go and download.go. This is acceptable because the download action is in a different package and the validation is small, well-tested code.
- **No content limit for downloads**: Unlike API responses, downloads may be legitimately large (binaries), so no size limit was added. Checksum verification provides integrity protection instead.

## Test Coverage

- New tests added: 4 tests
  - `TestHTTPClientDisableCompression` (version package)
  - `TestDownloadHTTPClient_DisableCompression` (actions package)
  - `TestDownloadAction_ValidateIP` (actions package) - 12 test cases
  - `TestRegistryHTTPClient_DisableCompression` (registry package)

## Known Limitations

- The decompression bomb protection relies on servers honoring `Accept-Encoding: identity`. Malicious servers could still send compressed responses, which are now rejected but after headers are received.
- The registry client doesn't have SSRF protection since it only fetches from the configured registry URL (default: GitHub raw content).

## Future Improvements

- Consider extracting HTTP security utilities to a shared internal package if more HTTP clients are added.
- Add rate limiting to HTTP clients as suggested in the security review.
