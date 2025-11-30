# Issue #114: Detect Specific Errors in Registry Client

## Summary

Added structured error types to the registry client for better error detection and user-friendly suggestions.

## Changes

### New File: `internal/registry/errors.go`

1. **`ErrorType` constants** - Registry-specific error types:
   - `ErrTypeNetwork` - Generic network errors
   - `ErrTypeNotFound` - Recipe not found (HTTP 404)
   - `ErrTypeParsing` - Response parsing errors
   - `ErrTypeValidation` - Invalid input
   - `ErrTypeRateLimit` - Rate limit exceeded (HTTP 429)
   - `ErrTypeTimeout` - Request timeout
   - `ErrTypeDNS` - DNS resolution failure
   - `ErrTypeConnection` - Connection refused/reset
   - `ErrTypeTLS` - TLS/SSL certificate errors

2. **`RegistryError` struct** - Structured error with Type, Recipe, Message, Err fields

3. **`Suggestion()` method** - Returns actionable suggestions based on error type

4. **`classifyError()` function** - Uses Go's type system to detect specific error types

5. **`WrapNetworkError()` helper** - Wraps network errors with automatic classification

### Updated: `internal/registry/registry.go`

Updated `FetchRecipe` to return `*RegistryError`:
- Invalid name → `ErrTypeValidation`
- Network error → `classifyError()` via `WrapNetworkError()`
- HTTP 404 → `ErrTypeNotFound`
- HTTP 429 → `ErrTypeRateLimit`
- Other HTTP errors → `ErrTypeNetwork`
- Read error → `ErrTypeParsing`

### New File: `internal/registry/errors_test.go`

- `TestRegistryError_Error` - Error message formatting
- `TestRegistryError_Unwrap` - Error unwrapping
- `TestRegistryError_Suggestion` - Suggestions for each error type
- `TestWrapNetworkError` - Network error wrapping
- `TestClassifyError` - Error classification logic

## Design Decision

Created registry-local error types instead of importing from `version` package to avoid import cycles. The error classification logic is duplicated but keeps packages independent.

## Testing

```bash
go test ./internal/registry/... -v
go vet ./...
```

All tests pass.

## Blocking

This unblocks issue #20 (improve error messages with actionable suggestions).
