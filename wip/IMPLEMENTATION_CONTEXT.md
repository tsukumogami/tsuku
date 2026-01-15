# Implementation Context: Issue #875

## Goal

Add GitHub token support to the tap version provider to increase API rate limits from 60/hour (unauthenticated) to 5,000/hour (authenticated).

## Key Requirements

1. Read `GITHUB_TOKEN` from environment variable
2. Add `Authorization: Bearer <token>` header to GitHub API requests
3. Include rate limit status in error messages (remaining requests, reset time)
4. **Security**: Never log the token value
5. Work correctly without a token (unauthenticated mode)
6. Invalid tokens should fall back to unauthenticated with a warning

## Rate Limit Headers

GitHub returns rate limit info in response headers:
- `X-RateLimit-Limit`: Max requests per hour
- `X-RateLimit-Remaining`: Requests remaining  
- `X-RateLimit-Reset`: Unix timestamp when limit resets

## Target File

`internal/version/tap.go` - Add token support to the existing tap provider

## Error Message Format

```
GitHub API rate limited (0/60 remaining, resets in 45m).
Set GITHUB_TOKEN environment variable for higher limits (5000/hour).
```

## Dependencies

- #872 (tap provider core) - Already implemented
