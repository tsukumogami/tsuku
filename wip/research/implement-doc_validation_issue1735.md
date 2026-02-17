# Validation Report: Issue #1735 - Scenario 10

**Date**: 2026-02-16
**Environment**: Docker (golang:1.25)
**Scenario**: LLM providers use secrets package after migration

---

## Scenario 10: LLM providers use secrets package after migration

**ID**: scenario-10
**Category**: infrastructure
**Status**: PASSED

### Test Execution

**Command**: `go test -v ./internal/llm/...`
**Exit code**: 0
**Duration**: ~0.7s (llm), ~0.02s (addon)

**Results**:
- 142 tests passed
- 0 tests failed
- 10 tests skipped (integration tests requiring API keys or running services)

All skipped tests are gated by `LLM_INTEGRATION_TEST=true` or API key presence checks,
which is expected behavior for environment-dependent integration tests.

### Source Code Verification

**Check 1**: `claude.go` does not contain direct `os.Getenv` calls for API keys.
- Result: PASSED. No `os.Getenv` calls found in `claude.go`.
- Uses `secrets.Get("anthropic_api_key")` at line 23.

**Check 2**: `gemini.go` does not contain direct `os.Getenv` calls for API keys.
- Result: PASSED. No `os.Getenv` calls found in `gemini.go`.
- Uses `secrets.Get("google_api_key")` at line 26.

**Check 3**: `factory.go` does not contain direct `os.Getenv` calls for API keys.
- Result: PASSED. No `os.Getenv` calls found in `factory.go`.
- Uses `secrets.IsSet("anthropic_api_key")` at line 150.
- Uses `secrets.IsSet("google_api_key")` at line 159.

**Check 4**: Provider detection in `factory.go` uses `secrets.IsSet()`.
- Result: PASSED. Both Claude and Gemini provider detection use `secrets.IsSet()`.

**Note on remaining `os.Getenv` calls in `internal/llm/`**:
- `lifecycle.go:45` - Uses `os.Getenv(IdleTimeoutEnvVar)` for idle timeout config. Not an API key, not in scope.
- `local.go:133` - Uses `os.Getenv("TSUKU_HOME")` for home directory. Not an API key, not in scope.
- `addon/manager.go:36,233` - Uses `os.Getenv("TSUKU_HOME")` for home directory. Not an API key, not in scope.
- All `_test.go` files - Test code is explicitly excluded from the migration requirement.

### Conclusion

Scenario 10 fully passes. The LLM provider migration to the secrets package is complete:
- All existing LLM tests pass without modification.
- `claude.go`, `gemini.go`, and `factory.go` no longer use `os.Getenv` for API key resolution.
- Provider detection in `factory.go` correctly uses `secrets.IsSet()` for both Claude and Gemini.
- The `secrets.Get()` function is used in both provider constructors, enabling the env-var-first-then-config-file resolution chain.
