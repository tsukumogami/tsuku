# Architect Review: #1736 refactor: migrate platform tokens to secrets package

**Review focus**: architecture (design patterns, separation of concerns)
**Commit**: a031c91f7e7328ab93e5eeedf9e0e8f3be1ccfdb

## Summary

This issue migrates `GITHUB_TOKEN`, `TAVILY_API_KEY`, and `BRAVE_API_KEY` access across the discover, version, builders, and search packages from direct `os.Getenv()` calls to the centralized `secrets.Get()` / `secrets.IsSet()` API introduced in #1733.

## Design Alignment Assessment

The implementation correctly follows the design doc's Phase 3a specification. All targeted packages now use `secrets.Get("github_token")`, `secrets.Get("tavily_api_key")`, and `secrets.Get("brave_api_key")` instead of direct environment variable access. The migration is consistent with the pattern established in #1735 (LLM provider migration).

## Files Reviewed

- `internal/discover/llm_discovery.go` -- migrated `defaultHTTPGet()` to `secrets.Get("github_token")`
- `internal/discover/validate.go` -- migrated `NewGitHubValidator()` to `secrets.Get("github_token")`
- `internal/version/resolver.go` -- migrated `New()` to `secrets.Get("github_token")`
- `internal/version/provider_tap.go` -- migrated `fetchFormulaFile()` to `secrets.Get("github_token")`
- `internal/builders/github_release.go` -- migrated `setGitHubHeaders()` to `secrets.Get("github_token")` and `fetchReleases()`/`fetchRepoMeta()` to `secrets.IsSet("github_token")`
- `internal/search/factory.go` -- migrated `NewSearchProvider()` to `secrets.Get("tavily_api_key")` and `secrets.Get("brave_api_key")`
- `internal/builders/errors.go` -- updated `GitHubRateLimitError.Suggestion()` to mention both env var and config file
- `internal/version/errors.go` -- updated `GitHubRateLimitError.Suggestion()` to mention both env var and config file
- `internal/discover/llm_discovery.go` -- updated `RateLimitError.Suggestion()` to mention both env var and config file

## Findings

### Finding 1: Pattern consistency achieved across all packages

**File**: All migrated files
**Severity**: POSITIVE (not a finding -- confirming alignment)

The migration follows the exact same pattern used in #1735 for LLM providers:
- `secrets.Get("key_name")` for value retrieval with error propagation
- `secrets.IsSet("key_name")` for boolean checks (e.g., authenticated rate limit detection)
- Ignoring the error when the token is optional: `token, _ := secrets.Get("github_token")`

This is consistent across discover, version, builders, and search packages. No parallel patterns introduced.

### Finding 2: Dependency direction is correct

**File**: All migrated files
**Severity**: POSITIVE

All consuming packages (`discover`, `version`, `builders`, `search`) import `internal/secrets`. The `secrets` package itself imports only `internal/userconfig` and standard library. No circular dependencies. The dependency graph flows correctly:

```
discover, version, builders, search
    |
    v
  secrets
    |
    v
  userconfig
```

### Finding 3: Error messages consistently mention both resolution methods

**File**: Multiple error/suggestion paths
**Severity**: POSITIVE

Error messages across all packages now follow the format: "Set GITHUB_TOKEN environment variable or add github_token to [secrets] in $TSUKU_HOME/config.toml". This matches the design doc's requirement that error messages reference both env var and config file options. Checked in:
- `internal/builders/errors.go:84`
- `internal/version/errors.go:236`
- `internal/version/assets.go:135,229,242`
- `internal/discover/llm_discovery.go:111`
- `internal/search/factory.go:20,27`

### Finding 4 (Advisory): Stale godoc comment in provider_tap.go

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/version/provider_tap.go`, line 159
**What**: The godoc for `fetchFormulaFile` says "If GITHUB_TOKEN is set, it will be used for authentication" which references the env var directly rather than mentioning the secrets package resolution.
**Impact**: Minor documentation inconsistency. Developers reading this comment might think only the env var works, not the config file fallback.
**Suggestion**: Update comment to "If github_token is available (via env var or config file), it will be used for authentication to increase rate limits." This matches the comment style already used at `resolver.go:73`.

### Finding 5 (Advisory): Stale comment in llm_discovery.go

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/discover/llm_discovery.go`, line 788
**What**: The comment `// defaultHTTPGet performs an HTTP GET request with GITHUB_TOKEN if available.` references the raw env var name rather than the secrets resolution path.
**Impact**: Minor documentation inconsistency.
**Suggestion**: Update to `// defaultHTTPGet performs an HTTP GET request with github_token if available.` (matching the secrets key name convention).

### Finding 6 (Advisory): RateLimitError struct comment references GITHUB_TOKEN

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/discover/llm_discovery.go`, line 90
**What**: The `RateLimitError` struct has `Authenticated bool // Whether request used GITHUB_TOKEN`. This references the raw env var name.
**Impact**: Minor. The struct field comment is internal and accurate enough.
**Suggestion**: Could update to `// Whether request was authenticated (github_token was available)` for consistency, but this is very low priority.

### Finding 7: No os.Getenv calls remain for migrated tokens in non-test code

**Verified**: Grep across `internal/` confirms the only remaining `os.Getenv("GITHUB_TOKEN")` is in `internal/discover/llm_discovery_test.go:153`, which is test code checking if the integration test should run with authentication. This is correct -- tests may need to check the raw env var for setup purposes.

### Finding 8: version/assets.go error messages updated without importing secrets

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/version/assets.go`
**What**: This file has error messages that mention both env var and config file for github_token, but doesn't import or use `secrets` directly. It doesn't need to -- the token is resolved once in `resolver.go:New()` and injected into the GitHub client. The error messages are static guidance text.
**Impact**: None. This is architecturally correct -- the token resolution happens at construction time, and error messages just tell users how to fix the problem.

## Verdict

No blocking findings. The migration is clean, follows established patterns, and aligns with the design doc's intent. All findings are advisory documentation nits (stale comments referencing env var names directly instead of the secrets key name).
