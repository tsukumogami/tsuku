# Issue 313 Summary

## What Was Implemented

Extracted shared HTTP client and SSRF protection code from three locations into a new `internal/httputil/` package, eliminating code duplication and centralizing security-critical code.

## Changes Made

- `internal/httputil/ssrf.go`: New file with `ValidateIP()` function for SSRF protection
- `internal/httputil/client.go`: New file with `NewSecureClient()` and `ClientOptions` struct
- `internal/httputil/ssrf_test.go`: Comprehensive tests for IP validation
- `internal/httputil/client_test.go`: Tests for HTTP client creation and redirect validation
- `internal/actions/download.go`: Removed duplicate `validateDownloadIP()` and `newDownloadHTTPClient()`, now uses httputil
- `internal/validate/predownload.go`: Removed duplicate `validatePreDownloadIP()` and `newPreDownloadHTTPClient()`, now uses httputil
- `internal/version/resolver.go`: Removed duplicate `validateIP()` and simplified `NewHTTPClient()` to wrap httputil

## Key Decisions

- **EnableCompression field (inverted default)**: Used `EnableCompression bool` instead of `DisableCompression` so that the zero value (false) means compression is disabled (the secure default). This prevents accidental security regression if a caller forgets to set the option.
- **Kept NewHTTPClient exported in version package**: Maintains backward compatibility for any external code using `version.NewHTTPClient()`.
- **Kept validateIP wrapper in version package**: Existing tests in version/security_test.go use the package-local function; wrapper avoids unnecessary test changes.

## Trade-offs Accepted

- **Slight API difference in redirect limits**: version package uses 5 redirects, others use 10. Preserved via ClientOptions.MaxRedirects.
- **Timeout configurability**: Each call site specifies its own timeout (actions uses config.GetAPITimeout(), predownload uses 10 minutes). This is necessary for their different use cases.

## Test Coverage

- New tests added: 15 tests across ssrf_test.go and client_test.go
- All existing tests continue to pass
- Coverage: No decrease (existing tests now exercise httputil instead of local functions)

## Known Limitations

- None

## Future Improvements

- Could add rate limiting support to ClientOptions
- Could add custom DNS resolver option for testing
