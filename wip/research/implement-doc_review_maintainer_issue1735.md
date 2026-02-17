# Maintainer Review: #1735 refactor(llm): migrate API key resolution to secrets package

## Summary

This issue replaces direct `os.Getenv()` calls in `claude.go`, `gemini.go`, and `factory.go` with `secrets.Get()` and `secrets.IsSet()`. The Gemini provider's manual two-variable fallback logic is removed since the alias table in `secrets/specs.go` handles it. The factory's provider detection now uses `secrets.IsSet()` instead of raw env var checks.

## Findings

### Finding 1: Stale godoc on `NewClient` (advisory)

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/client.go`, line 25
**What**: The comment says "NewClient creates a new LLM client using ANTHROPIC_API_KEY from environment" but `NewClient` now delegates to `NewClaudeProvider()` which resolves the key via `secrets.Get("anthropic_api_key")`. The comment describes a resolution path that no longer exists.
**Why it matters**: A developer reading this godoc would believe the key must be an environment variable, missing the config file fallback. This is the kind of outdated comment that erodes trust in documentation.
**Suggestion**: Update to: `// NewClient creates a new LLM client using the anthropic_api_key secret.` and `// Returns an error if the API key is not configured.`

### Finding 2: Duplicated env-var save/restore boilerplate in factory_test.go (advisory)

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/factory_test.go`, lines 25-35, 47-57, 453-461
**What**: The same 3-key save-and-restore pattern (`ANTHROPIC_API_KEY`, `GOOGLE_API_KEY`, `GEMINI_API_KEY`) is copy-pasted across 3 tests. A 1-key variant appears 3 more times (lines 473, 512, 526). Meanwhile, `client_test.go` already has a `setTestAPIKey` helper that handles save/restore for `ANTHROPIC_API_KEY`.
**Why it matters**: When a new env var alias is added to the secrets system, every copy of this boilerplate would need updating. The existing helper in `client_test.go` shows the codebase already has a pattern for this.
**Suggestion**: Extract a `clearAllLLMKeys(t) func()` helper in `factory_test.go` (or expand `setTestAPIKey` in a shared test helper file) that saves, clears, and returns a restore function for all three keys. Each test that needs a clean environment would call `defer clearAllLLMKeys(t)()`.

### Finding 3: NewFactory error message hardcodes env var names (advisory)

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/factory.go`, line 176
**What**: The error message says `"set ANTHROPIC_API_KEY or GOOGLE_API_KEY environment variable, or add keys to [secrets] in $TSUKU_HOME/config.toml, or enable local LLM"`. This hardcodes the env var names rather than deriving them from the secrets package's `KnownKeys()` or `KeySpec` table.
**Why it matters**: If a new LLM provider is added or an env var alias changes, this message won't automatically reflect the change. However, adding a new provider would require code changes in the factory anyway (registering the provider), so the developer would naturally see this error message during that work. This is a minor coupling concern, not a correctness issue.
**Suggestion**: Acceptable as-is for now. If the number of supported providers grows beyond 2-3, consider building the env var list dynamically. The error message is already good quality -- it tells the user both resolution methods.

### Finding 4: NewFactory and NewClaudeProvider godoc comments are well updated (positive)

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/factory.go`, lines 118-125; `claude.go`, lines 19-21; `gemini.go`, lines 21-24
**What**: The godoc on `NewFactory`, `NewClaudeProvider`, and `NewGeminiProvider` have been updated to reference "configured secrets" and the resolution chain (env var then config file). These accurately describe the new behavior.
**Why it matters**: Good documentation makes the migration transparent to consumers of these APIs.

### Finding 5: Integration test skip guards still use raw os.Getenv (acceptable)

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/claude_test.go`, line 167; `gemini_test.go`, lines 13, 632, 667; `integration_test.go`, lines 582, 625, 668-669, 760
**What**: Integration tests use `os.Getenv("ANTHROPIC_API_KEY") == ""` in skip guards. These check whether the test should run, not whether the application should resolve a key.
**Why it matters**: This is intentional and correct. Skip guards check whether the CI/developer environment has the key available for integration testing. They aren't part of the application's key resolution path and should not go through `secrets.IsSet()`, which would also check config files and add unnecessary coupling between test infrastructure and the secrets package.

## Overall Assessment

The migration is clean and well-executed. Production code in `claude.go`, `gemini.go`, and `factory.go` has been fully migrated from `os.Getenv()` to `secrets.Get()`/`secrets.IsSet()`. The Gemini provider's manual two-variable fallback has been correctly removed. Godoc comments on the migrated functions are accurate.

The one stale comment on `client.go:25` is the most actionable finding -- it's a quick fix and prevents misleading the next developer who reads it. The test boilerplate duplication is a minor quality-of-life issue that could be addressed in this PR or a follow-up.

No blocking findings.
