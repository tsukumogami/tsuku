# Issue #113 Baseline

## Issue
**Title:** refactor(version): detect specific errors in version providers
**URL:** https://github.com/tsuku-dev/tsuku/issues/113

## Summary
Update version providers to detect specific error conditions (rate limits, timeouts, DNS failures, connection errors, TLS errors) and return appropriate error types instead of generic `ErrTypeNetwork`.

## Current State

### Error Types Available (from #112)
```go
const (
    ErrTypeNetwork ErrorType = iota
    ErrTypeNotFound
    ErrTypeParsing
    ErrTypeValidation
    ErrTypeUnknownSource
    ErrTypeNotSupported
    ErrTypeRateLimit     // API rate limit exceeded
    ErrTypeTimeout       // Request timeout
    ErrTypeDNS           // DNS resolution failure
    ErrTypeConnection    // Connection refused/reset
    ErrTypeTLS           // TLS/SSL certificate errors
)
```

### Files to Modify
Provider files that make HTTP requests:
- `resolver.go` - Core HTTP request logic
- `provider_crates_io.go` - crates.io (already detects 429)
- `rubygems.go` - RubyGems (already detects 429)
- `pypi.go` - PyPI
- `provider_github.go` - GitHub releases/tags
- `provider_npm.go` - npm registry
- `provider_nixpkgs.go` - Nixpkgs

## Test Status
All tests passing at baseline.

## Branch
`refactor/113-specific-provider-errors` created from main at `d80f166c40dcbfdb6fe10a939d4b7ed0648d25c6`
