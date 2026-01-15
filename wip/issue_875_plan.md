# Issue 875 Implementation Plan

## Summary

Add GitHub token support to the tap version provider's `fetchFormulaFile` method.

## Current State

- The tap provider (`internal/version/provider_tap.go`) uses `p.httpClient.Do(req)` directly
- The main resolver (`internal/version/resolver.go`) already reads `GITHUB_TOKEN` from env and creates an authenticated GitHub client
- The tap provider doesn't leverage the resolver's authentication

## Approach

Modify `fetchFormulaFile` to:
1. Read `GITHUB_TOKEN` from environment
2. Add `Authorization: Bearer <token>` header when token is present
3. Parse rate limit response headers on 403 errors
4. Return helpful error messages with rate limit info

## Changes

### File: `internal/version/provider_tap.go`

1. Add helper function `getGitHubToken()` to read from env
2. Modify `fetchFormulaFile` to:
   - Add Authorization header when token is present
   - Handle 403 responses with rate limit info from headers
   - Return `GitHubRateLimitError` with proper context

### Rate Limit Headers to Parse

- `X-RateLimit-Limit`: Max requests/hour (60 unauth, 5000 auth)
- `X-RateLimit-Remaining`: Requests left
- `X-RateLimit-Reset`: Unix timestamp for reset

### Error Format

```
GitHub API rate limited (0/60 remaining, resets in 45m).
Set GITHUB_TOKEN environment variable for higher limits (5000/hour).
```

## Security Requirements

- Never log the token value
- Never include token in error messages
- Only read from environment (not config files)

## Tests

Add tests for:
1. Token added to request header when present
2. Rate limit error parsing from 403 response
3. Graceful fallback when no token
4. Token not exposed in errors
