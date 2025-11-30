# Implementation Plan: Issue #113

## Objective
Update version providers to detect specific error conditions and return appropriate error types instead of generic `ErrTypeNetwork`.

## Analysis

### Current State
Providers currently use string matching like `strings.Contains(err.Error(), "no such host")` which is:
- Fragile (depends on error message text)
- Not type-safe
- Not centralized

### Solution
1. Add a `ClassifyError(err error) ErrorType` function to `errors.go` that:
   - Uses Go's `errors.As()` and type assertions to detect specific error types
   - Returns the most specific error type possible

2. Add a helper function `WrapNetworkError(err error, source, message string) *ResolverError` that:
   - Classifies the error
   - Wraps it in a `ResolverError` with proper type

3. Update providers to use these helpers instead of string matching

## Implementation Steps

### Step 1: Add error classification to errors.go
Add helper functions to detect specific error types:

```go
import (
    "context"
    "crypto/tls"
    "errors"
    "net"
    "net/url"
)

// ClassifyError examines an error and returns the most specific ErrorType.
func ClassifyError(err error) ErrorType {
    if err == nil {
        return ErrTypeNetwork
    }

    // Check for context timeout/deadline
    if errors.Is(err, context.DeadlineExceeded) {
        return ErrTypeTimeout
    }
    if errors.Is(err, context.Canceled) {
        return ErrTypeNetwork
    }

    // Check for DNS errors
    var dnsErr *net.DNSError
    if errors.As(err, &dnsErr) {
        return ErrTypeDNS
    }

    // Check for connection errors
    var opErr *net.OpError
    if errors.As(err, &opErr) {
        if opErr.Timeout() {
            return ErrTypeTimeout
        }
        // Connection refused/reset
        return ErrTypeConnection
    }

    // Check for TLS errors
    var certErr *tls.CertificateVerificationError
    if errors.As(err, &certErr) {
        return ErrTypeTLS
    }

    // Check for URL errors (includes TLS)
    var urlErr *url.Error
    if errors.As(err, &urlErr) {
        if urlErr.Timeout() {
            return ErrTypeTimeout
        }
        // Recurse into the wrapped error
        return ClassifyError(urlErr.Err)
    }

    // Default to generic network error
    return ErrTypeNetwork
}

// WrapNetworkError wraps an error with the appropriate error type.
func WrapNetworkError(err error, source, message string) *ResolverError {
    return &ResolverError{
        Type:    ClassifyError(err),
        Source:  source,
        Message: message,
        Err:     err,
    }
}
```

### Step 2: Update provider_crates_io.go
Change rate limit detection from `ErrTypeNetwork` to `ErrTypeRateLimit`:
- Line 135-141: Change `ErrTypeNetwork` to `ErrTypeRateLimit` for HTTP 429

Update network error handling to use `WrapNetworkError()`.

### Step 3: Update rubygems.go
Already detects 429 but uses `ErrTypeNetwork`. Change to `ErrTypeRateLimit`.

### Step 4: Update pypi.go
Update all network error returns to use `WrapNetworkError()`.

### Step 5: Update resolver.go
Replace string-matching error detection with `WrapNetworkError()` or `ClassifyError()`.

### Step 6: Add tests
Add tests for `ClassifyError()` function.

## Files to Modify
1. `internal/version/errors.go` - Add ClassifyError() and WrapNetworkError()
2. `internal/version/errors_test.go` - Add tests for new functions
3. `internal/version/provider_crates_io.go` - Use ErrTypeRateLimit for 429
4. `internal/version/rubygems.go` - Use ErrTypeRateLimit for 429
5. `internal/version/pypi.go` - Use WrapNetworkError()
6. `internal/version/resolver.go` - Replace string matching

## Success Criteria
- Rate limits detected with ErrTypeRateLimit
- Timeouts detected with ErrTypeTimeout
- DNS failures detected with ErrTypeDNS
- Connection errors detected with ErrTypeConnection
- All existing tests pass
- New tests added for error classification
