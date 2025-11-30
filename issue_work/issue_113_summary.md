# Issue #113: Detect Specific Errors in Version Providers

## Summary

Updated version providers to detect specific error conditions and return appropriate error types instead of generic `ErrTypeNetwork`.

## Changes

### New Functions in `internal/version/errors.go`

1. **`ClassifyError(err error) ErrorType`** - Examines an error and returns the most specific ErrorType using Go's type system:
   - `context.DeadlineExceeded` → `ErrTypeTimeout`
   - `*net.DNSError` → `ErrTypeDNS` (or `ErrTypeTimeout` if DNS timeout)
   - `*tls.CertificateVerificationError` → `ErrTypeTLS`
   - `*net.OpError` → `ErrTypeTimeout` (if timeout) or `ErrTypeConnection`
   - `*url.Error` → Delegates to inner error classification, detects TLS errors via message
   - Default → `ErrTypeNetwork`

2. **`WrapNetworkError(err, source, message) *ResolverError`** - Helper that wraps network errors with automatic classification

### Provider Updates

| Provider | File | Change |
|----------|------|--------|
| crates.io | `provider_crates_io.go` | HTTP 429 → `ErrTypeRateLimit`; `httpClient.Do` → `WrapNetworkError()` |
| RubyGems | `rubygems.go` | HTTP 429 → `ErrTypeRateLimit`; `httpClient.Do` → `WrapNetworkError()` |
| PyPI | `pypi.go` | `httpClient.Do` → `WrapNetworkError()` |

### Test Updates

- Added `TestClassifyError` with 10 test cases covering all error types
- Added `TestWrapNetworkError` with 3 test cases
- Updated `TestListRubyGemsVersions_RateLimit` to expect `ErrTypeRateLimit`

## Benefits

1. **Type-safe error detection** - Uses `errors.As()` and `errors.Is()` instead of fragile string matching
2. **Centralized logic** - All error classification in one place
3. **Better user experience** - Users get specific suggestions based on error type (DNS, timeout, TLS, etc.)
4. **Foundation for #20** - Enables improved error messages with actionable suggestions

## Testing

```bash
go test ./internal/version/... -v
go vet ./internal/version/...
```

All tests pass.

## Blocking

This unblocks issue #20 (improve error messages).
