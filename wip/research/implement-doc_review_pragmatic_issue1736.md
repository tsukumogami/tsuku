# Pragmatic Review: #1736 - refactor: migrate platform tokens to secrets package

## Review Scope

Reviewed the commit `a031c91f7e7328ab93e5eeedf9e0e8f3be1ccfdb` which migrates `GITHUB_TOKEN`, `TAVILY_API_KEY`, and `BRAVE_API_KEY` access to use the `secrets` package across discover, version, builders, and search packages.

## Files Changed

- `internal/discover/llm_discovery.go` - `defaultHTTPGet` and `Suggestion()` on `RateLimitError`
- `internal/discover/validate.go` - `NewGitHubValidator`
- `internal/version/resolver.go` - `New()`
- `internal/version/provider_tap.go` - `fetchFormulaFile`
- `internal/builders/github_release.go` - `setGitHubHeaders`, `fetchReleases`, `fetchRepoMeta`
- `internal/search/factory.go` - `NewSearchProvider`
- `internal/builders/errors.go` - `GitHubRateLimitError.Suggestion()`
- `internal/version/errors.go` - `GitHubRateLimitError.Suggestion()`
- `internal/version/assets.go` - various error messages

## Findings

### Finding 1: No blocking issues

**Severity**: n/a

The migration is clean and mechanical. Every call site that previously used `os.Getenv("GITHUB_TOKEN")` etc. now uses `secrets.Get("github_token")` or `secrets.IsSet("github_token")`. The pattern is consistent across all packages.

### Finding 2 (Advisory): Error messages in search/factory.go duplicate guidance that secrets.Get() already provides

**File**: `internal/search/factory.go`, lines 20-21, 27-28
**Severity**: advisory

When `secrets.Get("tavily_api_key")` returns an error, it already contains guidance text:
```
tavily_api_key not configured. Set the TAVILY_API_KEY environment variable, or add tavily_api_key to [secrets] in $TSUKU_HOME/config.toml
```

But the code wraps it with its own message:
```go
return nil, fmt.Errorf("--search-provider=tavily requires tavily_api_key: set TAVILY_API_KEY environment variable or add tavily_api_key to [secrets] in $TSUKU_HOME/config.toml")
```

This is a minor duplication. The custom error message adds context about `--search-provider=tavily` which the secrets package can't know, so the override is justified. However, the env var and config file guidance is duplicated in the format string rather than using the error from `secrets.Get()`. If the canonical error message in secrets.go changes, these would go stale.

**Suggestion**: Consider using `%w` wrapping: `fmt.Errorf("--search-provider=tavily: %w", err)` to inherit the guidance from secrets.Get(). Or keep as-is since the extra context is valuable. This is cosmetic.

### Finding 3 (Advisory): `secrets.Get()` called on every HTTP request in `setGitHubHeaders`

**File**: `internal/builders/github_release.go`, line 830
**Severity**: advisory

`setGitHubHeaders` calls `secrets.Get("github_token")` on every GitHub API request. The secrets package caches config via `sync.Once`, and env var lookups are cheap (kernel-level), so this has no real performance impact. But contrast with `internal/version/resolver.go` line 80 which resolves the token once in `New()` and stores it for the lifetime of the resolver.

Both patterns work correctly. The resolver approach is slightly better since it resolves the token once and makes auth state explicit (the `authenticated` field). The builders approach is simpler since it doesn't need to store state.

**Suggestion**: No change needed. Both patterns are valid for a CLI tool. Noting the inconsistency for awareness.

### Finding 4 (Advisory): `defaultHTTPGet` in discover/llm_discovery.go silently swallows secrets.Get error

**File**: `internal/discover/llm_discovery.go`, line 798
**Severity**: advisory

```go
token, _ := secrets.Get("github_token")
```

The error is intentionally ignored because not having a GITHUB_TOKEN is a valid state (unauthenticated requests work, just with lower rate limits). This pattern is used consistently across all files (`validate.go:77`, `provider_tap.go:169`). The behavior is correct -- if the secret isn't set, we proceed without authentication.

**Suggestion**: No change. The pattern is correct and consistent.

### Finding 5: All error messages correctly updated

All error messages that previously said "Set GITHUB_TOKEN environment variable" now say "Set GITHUB_TOKEN environment variable or add github_token to [secrets] in $TSUKU_HOME/config.toml". This includes:

- `internal/discover/llm_discovery.go:111` (RateLimitError.Suggestion)
- `internal/version/errors.go:236` (GitHubRateLimitError.Suggestion)
- `internal/version/assets.go:135,229,242` (various error strings)
- `internal/builders/errors.go:84` (GitHubRateLimitError.Suggestion)
- `internal/search/factory.go:20,27` (missing key errors)

This is the core requirement of the issue and it's been met across all packages.

### Finding 6: Tests are adequate

The existing test suites in `search/factory_test.go` exercise the key resolution paths via `t.Setenv`. These tests work because `secrets.Get()` checks `os.Getenv()` first, so setting env vars in tests exercises the correct path. The tests verify both the happy path (key present) and error path (key missing) for tavily and brave.

The `discover/validate_test.go` uses test doubles (`testGitHubValidator`), so the `NewGitHubValidator` -> `secrets.Get` path isn't directly tested, but the function is straightforward enough that this is acceptable.

No new tests were needed since this is a mechanical migration and existing tests already covered the behavior.

## Overall Assessment

This is a clean, focused migration that does exactly what the issue asks for. No over-engineering, no unnecessary abstractions, no gold-plating. The changes are mechanical replacements of `os.Getenv()` with `secrets.Get()` or `secrets.IsSet()`, plus updating error messages to mention both resolution methods. The two advisory findings are minor inconsistencies, not defects.
