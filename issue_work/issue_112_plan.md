# Implementation Plan: Issue #112

## Objective
Add granular network error types to `internal/version/errors.go` to enable specific error detection at the source.

## Analysis

### Current State
The `errors.go` file has 6 error types, with `ErrTypeNetwork` being a catch-all for all network-related failures. Providers like `provider_crates_io.go` use `ErrTypeNetwork` even when they can detect specific errors (e.g., rate limiting at line 135-141).

### Problem
When a rate limit is hit, the error is still typed as `ErrTypeNetwork`, making it impossible to give users specific guidance like "wait 60 seconds" vs "check your internet connection".

### Solution
Add specific network subtypes and a `Suggestion()` method to provide actionable guidance.

## Implementation Steps

### Step 1: Add New Error Types
Add 5 new error types to `errors.go`:

```go
const (
    ErrTypeNetwork ErrorType = iota      // Generic network error (fallback)
    ErrTypeNotFound                      // HTTP 404, resource not found
    ErrTypeParsing                       // JSON/TOML parsing failures
    ErrTypeValidation                    // Invalid version format
    ErrTypeUnknownSource                 // Unknown version source
    ErrTypeNotSupported                  // Operation not supported
    // New specific network error types
    ErrTypeRateLimit                     // API rate limit exceeded (HTTP 429 or 403 with headers)
    ErrTypeTimeout                       // Request timeout
    ErrTypeDNS                           // DNS resolution failure
    ErrTypeConnection                    // Connection refused/reset
    ErrTypeTLS                           // TLS/SSL certificate errors
)
```

### Step 2: Add Suggestion() Method
Add a method to `ResolverError` that returns actionable suggestions:

```go
func (e *ResolverError) Suggestion() string {
    switch e.Type {
    case ErrTypeRateLimit:
        return "Wait a few minutes before trying again, or check if you need to authenticate"
    case ErrTypeTimeout:
        return "Check your internet connection and try again"
    case ErrTypeDNS:
        return "Check your DNS settings and internet connection"
    case ErrTypeConnection:
        return "The service may be down or blocked. Check if you can access it in a browser"
    case ErrTypeTLS:
        return "There may be a certificate issue. Check your system time is correct"
    case ErrTypeNotFound:
        return "Verify the tool/package name is correct"
    case ErrTypeNetwork:
        return "Check your internet connection and try again"
    default:
        return ""
    }
}
```

### Step 3: Update Tests
Add tests for:
- New error type constants are distinct
- `Suggestion()` method returns expected strings

## Files to Modify
1. `internal/version/errors.go` - Add error types and Suggestion() method
2. `internal/version/errors_test.go` - Add tests for new types and Suggestion()

## Non-Goals (Deferred to Issues #113, #114)
- Updating providers to detect and use new error types
- Updating registry client to use new error types
- CLI integration

## Success Criteria
- New error types are defined
- Suggestion() method works correctly
- All existing tests still pass
- New tests cover the additions
