# Maintainer Review: Issue #1736 - refactor: migrate platform tokens to secrets package

## Summary

Issue #1736 migrates `GITHUB_TOKEN`, `TAVILY_API_KEY`, and `BRAVE_API_KEY` access across discover, version, builders, and search packages to use `secrets.Get()` / `secrets.IsSet()`. The migration is clean and consistent. No blocking issues found. Several stale comments reference the old env-var-only pattern and should be updated.

## Findings

### Finding 1 (advisory): Stale godoc on fetchFormulaFile

**File:** `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/version/provider_tap.go`, line 159
**What:** The comment says "If GITHUB_TOKEN is set, it will be used for authentication to increase rate limits." This describes the old direct env-var access pattern. The function body now calls `secrets.Get("github_token")`, which resolves from env vars *and* config file.
**Why it matters:** A developer reading this godoc would think only the environment variable works, missing the config file fallback.
**Suggestion:** Update to: "If github_token is available (via env var or config file), it will be used for authentication to increase rate limits."

### Finding 2 (advisory): Stale godoc on defaultHTTPGet

**File:** `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/discover/llm_discovery.go`, line 788
**What:** The comment says "performs an HTTP GET request with GITHUB_TOKEN if available." The function body now uses `secrets.Get("github_token")`.
**Why it matters:** Same as Finding 1 -- comment describes the old resolution path, not the current one.
**Suggestion:** Update to: "performs an HTTP GET request with github_token if available (resolved via env var or config file)."

### Finding 3 (advisory): RateLimitError.Authenticated field comment references raw env var name

**File:** `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/discover/llm_discovery.go`, line 90
**What:** The struct field comment says `// Whether request used GITHUB_TOKEN`. Now that token resolution goes through `secrets.Get("github_token")`, the token could come from either env var or config file.
**Why it matters:** Minor, but a developer reading this might wonder if it only tracks env var auth, not config file auth.
**Suggestion:** Update to: `// Whether request was authenticated with a GitHub token`

### Finding 4 (advisory): Duplicated rate-limit error construction in builders/github_release.go

**File:** `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/github_release.go`, lines 680-697 and 752-767
**What:** The `fetchReleases` and `fetchRepoMeta` methods both contain nearly identical blocks for parsing rate limit responses and constructing `GitHubRateLimitError`. The only difference is the function name. Both call `secrets.IsSet("github_token")` inline.
**Why it matters:** If the rate-limit parsing logic changes (e.g., adding a new header), two locations must be updated. This pre-existed the issue but the migration to `secrets.IsSet()` is a good moment to notice it.
**Suggestion:** Extract a shared method like `(b *GitHubReleaseBuilder) handleRateLimitResponse(resp *http.Response) error` that handles the 403/429 check, header parsing, and error construction. This isn't introduced by #1736 so it's purely advisory.

### Finding 5 (advisory): Error message in search/factory.go hardcodes env var name and config path separately

**File:** `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/search/factory.go`, lines 20-21 and 27-28
**What:** The error messages for missing tavily and brave keys hardcode both the env var name and the config file path: `"set TAVILY_API_KEY environment variable or add tavily_api_key to [secrets] in $TSUKU_HOME/config.toml"`. This duplicates the guidance that `secrets.Get()` already includes in its error message.
**Why it matters:** If the error format from `secrets.Get()` changes (e.g., mentioning a `tsuku config set` command), these custom messages will be out of date. However, the custom messages here add context about *why* the key is needed (`--search-provider=tavily requires...`), so they aren't pure duplication -- they wrap the guidance with a usage context.
**Suggestion:** Consider using the error from `secrets.Get()` directly and wrapping it: `return nil, fmt.Errorf("--search-provider=tavily requires tavily_api_key: %w", err)`. This would let the secrets package own the guidance format. Alternatively, keep as-is since the current messages are clear and actionable.

### Finding 6 (advisory): Comment in setGitHubHeaders still uses lower-level description

**File:** `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/github_release.go`, line 829
**What:** The inline comment says `// Use github_token if available for higher rate limits`. This is accurate but doesn't mention the dual resolution path (env var + config file) that `secrets.Get()` provides.
**Why it matters:** Minor. A developer might wonder how the token is resolved.
**Suggestion:** Update to: `// Use github_token if available (env var or config file) for higher rate limits`

### Finding 7 (advisory): Suggestion messages in error types now mention both env var and config file consistently

**File:** Multiple files
**What:** The `Suggestion()` methods on `builders.GitHubRateLimitError` (line 84), `version.GitHubRateLimitError` (line 236), and `discover.RateLimitError` (line 111) all consistently mention `"Set GITHUB_TOKEN environment variable or add github_token to [secrets] in $TSUKU_HOME/config.toml"`. This is good -- the messaging is uniform.
**Why it matters:** Positive observation. The migration delivers on the design doc's goal of "consistent error messages across all providers."

## Overall Assessment

The migration is well-executed. All `os.Getenv("GITHUB_TOKEN")` calls in the target packages have been replaced with `secrets.Get("github_token")` or `secrets.IsSet("github_token")`. Error messages consistently mention both env var and config file options. The search factory properly uses `secrets.Get()` for Tavily and Brave keys.

The main maintainability concern is a handful of stale comments (Findings 1-3, 6) that still describe the old direct-env-var pattern. These should be updated to reflect that token resolution now goes through the secrets package, which checks both env vars and the config file. None of these are blocking -- the code behavior is correct, but the comments create a disconnect between documentation and implementation that could confuse the next developer.
