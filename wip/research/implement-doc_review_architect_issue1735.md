# Architect Review: #1735 refactor(llm): migrate API key resolution to secrets package

## Summary

This issue migrates the LLM provider files (`claude.go`, `gemini.go`, `factory.go`) from direct `os.Getenv()` calls to the centralized `secrets.Get()` / `secrets.IsSet()` API introduced in #1733/#1734. The migration is clean and aligns well with the design document's Phase 3a specification.

## Findings

### Advisory: Stale godoc on NewClient (client.go:25)

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/client.go`, line 25
**What**: The `NewClient` godoc says "using ANTHROPIC_API_KEY from environment" but it now resolves via `secrets.Get("anthropic_api_key")` through `NewClaudeProvider`, which checks env vars and config file.
**Impact**: Misleading documentation. Users or contributors reading the godoc would not know config file resolution is available. Minor since `NewClient` delegates to `NewClaudeProvider` which has correct docs, but it's a public API surface worth fixing.
**Suggestion**: Update to something like "using the anthropic_api_key secret (resolved from env var or config file)."

### Advisory: Error message in factory.go:176 partially duplicates secrets guidance

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/factory.go`, line 176
**What**: The "no LLM providers available" error message hardcodes `"set ANTHROPIC_API_KEY or GOOGLE_API_KEY environment variable, or add keys to [secrets] in $TSUKU_HOME/config.toml"`. The `secrets.Get()` already produces guidance messages for each individual key. Having guidance in two places means if the resolution sources change (e.g., adding keychain support in the future), this message must also be updated.
**Impact**: Low. The factory-level message serves a different purpose (aggregate "no providers" vs. individual key missing), so some duplication is acceptable. The message is factually correct today. If a new secret source is added, this message would go stale.
**Suggestion**: Consider referencing `secrets.KnownKeys()` to dynamically build the env var list, or accept the duplication since this message is unlikely to change frequently. This is not blocking.

## Design Alignment Assessment

The implementation matches the design document's intent precisely:

1. **`claude.go`**: Replaced `os.Getenv("ANTHROPIC_API_KEY")` with `secrets.Get("anthropic_api_key")` -- correct per design.
2. **`gemini.go`**: Replaced dual `os.Getenv("GOOGLE_API_KEY")` / `os.Getenv("GEMINI_API_KEY")` fallback with single `secrets.Get("google_api_key")` -- eliminates the manual two-variable fallback logic since the alias table handles it, exactly as specified.
3. **`factory.go`**: Replaced env var checks with `secrets.IsSet()` for provider auto-detection -- correct per design.

**Dependency direction** is clean: `internal/llm` -> `internal/secrets` -> `internal/userconfig`. No circular dependencies. The `secrets` package remains a thin read-only resolution layer as designed.

**Separation of concerns** is preserved:
- The `secrets` package handles resolution (env var -> config file -> error).
- The `llm` package handles provider construction and API interaction.
- No business logic leaked into the secrets layer; no I/O logic in the provider constructors beyond calling `secrets.Get()`.

**Pattern consistency**: The migration follows the exact pattern established by the secrets package API. All three files use the same approach (`secrets.Get()` for values, `secrets.IsSet()` for availability checks), which is consistent and predictable.

**Test code** appropriately uses `os.Getenv` / `os.Setenv` / `os.Unsetenv` for setup/teardown since tests need to control the environment. Integration tests use `os.Getenv` for skip conditions, which is the correct pattern (checking if a real key is available to run integration tests).

## Verdict

No blocking findings. Two advisory items (stale godoc, minor guidance duplication). The architecture is sound and the migration is exactly what the design document prescribed. Ready to proceed.
