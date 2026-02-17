# Pragmatic Review: Issue #1735 -- refactor(llm): migrate API key resolution to secrets package

**Reviewer focus**: pragmatic (simplicity, YAGNI, KISS, correctness)
**Files changed**: `internal/llm/claude.go`, `internal/llm/gemini.go`, `internal/llm/factory.go`

## Summary

This is a clean, well-scoped refactoring. The changes are minimal and do exactly what the issue asks: replace direct `os.Getenv()` calls with `secrets.Get()` and `secrets.IsSet()` in the three LLM files. No gold-plating, no unnecessary abstractions, no over-engineering.

## Verification

### Correctness

- `claude.go:23`: `secrets.Get("anthropic_api_key")` replaces `os.Getenv("ANTHROPIC_API_KEY")`. Error is wrapped with provider context (`"claude provider: %w"`). Correct.
- `gemini.go:26`: `secrets.Get("google_api_key")` replaces the previous dual-variable check (`GOOGLE_API_KEY` / `GEMINI_API_KEY`). The alias handling is now done by the `KeySpec` in `specs.go`. Error is wrapped. Correct.
- `factory.go:150,159`: `secrets.IsSet("anthropic_api_key")` and `secrets.IsSet("google_api_key")` replace direct env var checks. Correct.

### Error handling

- Both `claude.go` and `gemini.go` properly wrap the `secrets.Get()` error with `fmt.Errorf("... provider: %w", err)`, preserving the error chain. Good.
- `factory.go:176`: Error message when no providers are available mentions both env vars and config file: "set ANTHROPIC_API_KEY or GOOGLE_API_KEY environment variable, or add keys to [secrets] in $TSUKU_HOME/config.toml, or enable local LLM". Correct and helpful.
- `IsSet()` returning false on config load error is correct behavior for auto-detection -- it just means the provider won't be registered, rather than crashing.

### Dead code / unused imports

- `"os"` import was correctly removed from all three non-test source files since they no longer call `os.Getenv()` directly.
- `secrets` import was correctly added.
- No dead code introduced.

### Test coverage

- `os.Getenv()` calls in test files (`*_test.go`) remain, which is correct -- these are used for:
  - Skip-guards in integration tests (checking if real API keys are available)
  - Save/restore of env vars during unit tests
- The test files don't need to go through `secrets.Get()` for skip guards because they're checking whether to run the test, not resolving secrets for use.
- Existing tests (`claude_test.go`, `gemini_test.go`, `factory_test.go`) properly exercise the provider constructors that now use `secrets.Get()`/`secrets.IsSet()`.

### Gemini dual-variable simplification

- Previously, `gemini.go` had manual fallback logic checking both `GOOGLE_API_KEY` and `GEMINI_API_KEY`. This is now handled by the `KeySpec` alias table in `specs.go`. The simplification is real and meaningful -- one line (`secrets.Get("google_api_key")`) replaces what was previously multi-line env var juggling. Good.

## Findings

### Advisory findings

**1. Integration test skip guards still use `os.Getenv()` directly**
- Files: `integration_test.go:582,625,668-669,760`, `claude_test.go:167`, `gemini_test.go:13,632,667`
- These use `os.Getenv("ANTHROPIC_API_KEY") == ""` as skip-guard conditions.
- This is fine -- skip guards need to check the raw env var to decide whether to run, and `secrets.IsSet()` would introduce an unnecessary dependency on config loading in test setup. This is the correct pattern.
- **No action needed.**

**2. Factory error message references specific env var names**
- File: `factory.go:176`
- The error message says "set ANTHROPIC_API_KEY or GOOGLE_API_KEY" -- if new providers are added later, this message would need updating. But this is fine for now; the message matches the current provider set and is more helpful than a generic message would be.
- **No action needed now.** Could consider generating this from `KnownKeys()` in the future, but YAGNI applies.

## Blocking findings

None.

## Overall assessment

This is a textbook-clean refactoring. Three files changed, each in the most minimal way possible. The Gemini dual-variable fallback was correctly simplified by delegating to the alias table. No unnecessary abstractions were introduced. The error messages are helpful and specific. The `"os"` import was correctly removed where no longer needed. Tests remain valid with their existing `os.Getenv()` usage for skip guards and env management.

No blocking issues. No advisory issues that need action.
