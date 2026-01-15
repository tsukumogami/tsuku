# Issue #875 Introspection

**Issue:** feat(version): add GitHub token support for tap provider
**Date:** 2026-01-14

## Staleness Signals

| Signal | Value |
|--------|-------|
| Issue age (days) | 0 |
| Sibling issues closed since creation | 4 |
| Milestone position | middle |

## Dependency Status

| Dependency | Status | Notes |
|------------|--------|-------|
| #872 (tap provider core) | **CLOSED (Merged)** | Merged via PR #883 on 2026-01-14 |

## Current State Analysis

### Tap Provider Implementation Review

The tap provider has been fully implemented in:
- `/internal/version/provider_tap.go` - Core provider implementation
- `/internal/version/tap_parser.go` - Ruby formula parsing

**Key observations from code review:**

1. **HTTP fetching is centralized**: The `fetchFormulaFile` method (lines 163-193 in `provider_tap.go`) is the single point for GitHub API requests. This is the ideal location to add authentication headers.

2. **HTTP client is injectable**: The `TapProvider` struct uses `httpClient *http.Client` from the resolver, which allows for easy extension with custom transport or headers.

3. **No token handling currently exists**: The current implementation makes unauthenticated requests to `raw.githubusercontent.com`.

4. **Rate limit handling not implemented**: The code returns generic errors on non-200 responses without parsing rate limit headers.

### Implementation Approach Validation

The issue specifies the following implementation:

| Requirement | Still Valid | Notes |
|-------------|-------------|-------|
| Read `GITHUB_TOKEN` from env | **Yes** | Standard GitHub convention |
| Add `Authorization: Bearer <token>` header | **Yes** | Aligns with current HTTP request structure |
| Rate limit status in error messages | **Yes** | Need to parse `X-RateLimit-*` headers |
| Secure token handling (never log) | **Yes** | Standard security practice |
| Work without token | **Yes** | Current implementation already works unauthenticated |

### Code Change Points

Based on the current implementation, the changes should be made to:

1. **`internal/version/provider_tap.go`**:
   - Add `getGitHubToken()` helper function
   - Modify `fetchFormulaFile()` to add Authorization header when token is present
   - Add rate limit header parsing on 403 responses
   - Generate helpful error messages with rate limit info

2. **Suggested additions**:
   - Helper to parse `X-RateLimit-Remaining` and `X-RateLimit-Reset` headers
   - Rate limit error type with remaining/reset fields
   - Graceful degradation on invalid token (warn and continue unauthenticated)

### Test Coverage Gaps

The existing tests in `provider_tap_test.go` use a test server with `testTransport` that rewrites URLs. New tests should:
- Test authenticated requests include the Authorization header
- Test rate limit error parsing and message formatting
- Test invalid token fallback behavior
- Verify token is never exposed in logs/errors

## Gap Analysis

| Acceptance Criterion | Gap | Risk |
|---------------------|-----|------|
| Read GITHUB_TOKEN from env | None | Low |
| Add Authorization header | None | Low |
| Rate limit info in errors | None | Low |
| Secure token handling | None | Low |
| Work without token | Already works | None |
| Documentation | Not specified where | Low |

## Recommendation

**Proceed** - The implementation approach is sound and aligns with the current codebase structure.

### Rationale

1. **Dependency is complete**: Issue #872 (tap provider core) has been merged, unblocking this work.

2. **Implementation approach is validated**: The issue's suggested approach (modify `fetchFormulaFile`, add rate limit parsing) aligns perfectly with the actual code structure.

3. **No specification gaps**: All acceptance criteria are clear and achievable with the current architecture.

4. **Test infrastructure exists**: The existing test patterns with `testTransport` can be extended for authentication testing.

### Minor Suggestions

1. **Error message wording**: The issue shows "GitHub API rate limited" but the code fetches from `raw.githubusercontent.com` which has different rate limits than the GitHub API. Consider clarifying this is raw content rate limiting.

2. **Test token handling**: Add a test using environment variable isolation to verify token reading behavior.

## Blocking Concerns

None identified. The issue is ready for implementation.
