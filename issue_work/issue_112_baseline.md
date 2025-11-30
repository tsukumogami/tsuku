# Issue #112 Baseline

## Issue
**Title:** refactor(errors): define specific error types in internal/version/errors.go
**URL:** https://github.com/tsuku-dev/tsuku/issues/112

## Summary
Add granular network error types to the existing error type system in `internal/version/errors.go`. This is the foundation for issues #113 and #114, which will update providers to detect and return these specific errors.

## Current State

### Existing Error Types in `internal/version/errors.go`
```go
const (
    ErrTypeNetwork ErrorType = iota      // Generic network error
    ErrTypeNotFound                      // HTTP 404, resource not found
    ErrTypeParsing                       // JSON/TOML parsing failures
    ErrTypeValidation                    // Invalid version format
    ErrTypeUnknownSource                 // Unknown version source
    ErrTypeNotSupported                  // Operation not supported
)
```

### Files to Modify
- `internal/version/errors.go` - Add new specific error types

## Planned Changes

### New Error Types to Add
1. `ErrTypeRateLimit` - GitHub API rate limit exceeded (HTTP 403 with rate limit headers)
2. `ErrTypeTimeout` - Request timeout
3. `ErrTypeDNS` - DNS resolution failure
4. `ErrTypeConnection` - Connection refused/reset
5. `ErrTypeTLS` - TLS/SSL certificate errors

### Helper Function
Add `Suggestion()` method to `ResolverError` that returns actionable suggestions based on error type.

## Test Status
All tests passing at baseline.

## Branch
`refactor/112-specific-error-types` created from main at `97d8bc69c06d9724c8f508d77750af2ea4e747dd`
