# Issue 313 Implementation Plan

## Summary

Extract shared HTTP client and SSRF protection code from `internal/actions/download.go`, `internal/validate/predownload.go`, and `internal/version/resolver.go` into a new `internal/httputil/` package.

## Approach

Create a new `internal/httputil/` package that provides:
1. A `ValidateIP()` function for SSRF protection
2. A `NewSecureClient()` function that creates an HTTP client with configurable options

The existing code in all three locations has nearly identical IP validation and HTTP client setup. By extracting this, we:
- Eliminate ~150 lines of duplicated security-critical code
- Ensure consistent SSRF protection across the codebase
- Make it easier to update security policies in one place

### Alternatives Considered

1. **Just extract to actions package and import**: Would create circular dependency issues since validate already imports actions indirectly.
2. **Extend version.NewHTTPClient() for reuse**: Version package has different concerns (GitHub client, registry URLs) and would create odd dependencies.

## Files to Modify

- `internal/actions/download.go` - Replace `validateDownloadIP()` and `newDownloadHTTPClient()` with httputil imports
- `internal/validate/predownload.go` - Replace `validatePreDownloadIP()` and `newPreDownloadHTTPClient()` with httputil imports
- `internal/version/resolver.go` - Replace `validateIP()` and `NewHTTPClient()` with httputil imports (keep exported wrapper if needed)

## Files to Create

- `internal/httputil/ssrf.go` - `ValidateIP()` function for SSRF protection
- `internal/httputil/client.go` - `NewSecureClient()` with `ClientOptions` struct
- `internal/httputil/ssrf_test.go` - Tests for IP validation
- `internal/httputil/client_test.go` - Tests for HTTP client creation

## Implementation Steps

- [x] Create `internal/httputil/ssrf.go` with `ValidateIP()` extracted from existing code
- [x] Create `internal/httputil/ssrf_test.go` with IP validation tests
- [x] Create `internal/httputil/client.go` with `NewSecureClient()` and `ClientOptions`
- [x] Create `internal/httputil/client_test.go` with client creation tests
- [ ] Update `internal/actions/download.go` to use httputil
- [ ] Update `internal/validate/predownload.go` to use httputil
- [ ] Update `internal/version/resolver.go` to use httputil (keep NewHTTPClient as wrapper)
- [ ] Run full test suite to verify no regressions
- [ ] Run golangci-lint to catch any issues

Mark each step [x] after it is implemented and committed. This enables clear resume detection.

## API Design

```go
// internal/httputil/ssrf.go

// ValidateIP checks if an IP is allowed for HTTP requests.
// Returns error if the IP is private, loopback, link-local, multicast, or unspecified.
func ValidateIP(ip net.IP, host string) error

// internal/httputil/client.go

// ClientOptions configures the secure HTTP client.
type ClientOptions struct {
    Timeout               time.Duration // Overall request timeout (default: 30s)
    DialTimeout           time.Duration // TCP dial timeout (default: 30s)
    TLSHandshakeTimeout   time.Duration // TLS handshake timeout (default: 10s)
    ResponseHeaderTimeout time.Duration // Response header timeout (default: 10s)
    MaxRedirects          int           // Max redirect depth (default: 10)
    DisableCompression    bool          // Disable Accept-Encoding (default: true)
}

// NewSecureClient creates an HTTP client with SSRF protection and security hardening.
func NewSecureClient(opts ClientOptions) *http.Client
```

## Testing Strategy

- **Unit tests**: Test IP validation for all blocked categories (private, loopback, link-local, multicast, unspecified) and allowed public IPs
- **Unit tests**: Test client creation with various options
- **Integration via existing tests**: The existing tests in actions, validate, and version packages will verify the refactored code works correctly

## Risks and Mitigations

- **Behavior changes**: By extracting code carefully and preserving exact logic, behavior should be identical. Existing tests validate this.
- **Timeout differences**: The three call sites have different timeout requirements. Using `ClientOptions` allows each to specify their needs.

## Success Criteria

- [ ] All existing tests pass
- [ ] No new code duplication (3 copies → 1)
- [ ] golangci-lint passes
- [ ] Clear, documented API for secure HTTP clients
- [ ] Design doc updated if any security-related changes affect caching behavior

## Open Questions

None - the extraction is straightforward and maintains existing behavior.
