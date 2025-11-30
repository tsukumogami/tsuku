# Issue 137 Implementation Plan

## Summary

Add `IsMulticast()` check to complete SSRF protection per security audit requirements.

## Context

The security audit (SECURITY-REVIEW-version-provider-refactoring.md) requires blocking ALL multicast addresses:
- IPv4: 224.0.0.0/4 (all class D addresses)
- IPv6: ff00::/8 (all multicast)

Currently only `IsLinkLocalMulticast()` is checked, which covers a subset (224.0.0.0/24, ff02::/16).

## Implementation Steps

- [x] Add `IsMulticast()` check to `validateIP()` function in resolver.go
- [x] Add test case for multicast addresses in security_test.go
- [x] Run tests to verify no regressions

## Files to Modify

- `internal/version/resolver.go` - Add IsMulticast check (~4 lines)
- `internal/version/security_test.go` - Add test case for multicast addresses

## Testing

Test IPv4 multicast: 224.0.0.1, 239.255.255.250 (SSDP)
Test IPv6 multicast: ff00::1, ff05::1 (site-local multicast)
