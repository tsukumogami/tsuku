# Pragmatic Review: #1737 feat(cli): add secrets management to tsuku config

**Review focus**: pragmatic (simplicity, YAGNI, KISS, correctness, edge cases)
**Files changed**: `cmd/tsuku/config.go`, `cmd/tsuku/config_test.go`

## Summary

The implementation is clean, proportional to the requirements, and avoids over-engineering. No blocking issues found.

## Findings

### Advisory 1: Help text uses `~/.tsuku` instead of `$TSUKU_HOME`

**File**: `cmd/tsuku/config.go`, line 25
**What**: The `configCmd.Long` help text says "Configuration is stored in ~/.tsuku/config.toml." but project conventions require using `$TSUKU_HOME` in documentation and user-facing text.
**Why it matters**: Users who customized `TSUKU_HOME` see incorrect path guidance. Low severity since it's a single help string.
**Suggestion**: Change to `$TSUKU_HOME/config.toml`.

### Advisory 2: `isKnownSecret` iterates KnownKeys on every call

**File**: `cmd/tsuku/config.go`, lines 230-237
**What**: `isKnownSecret` calls `secrets.KnownKeys()` which allocates a new sorted slice, then does a linear scan. Called from both `configGetCmd` and `runConfigSet`.
**Why it matters**: With 5 keys this is negligible. Not worth optimizing. Noting it only for completeness.
**Suggestion**: No change needed at current scale.

## Correctness Checks (all pass)

- **EOF handling**: `readSecretFromStdin` correctly handles `io.EOF` from piped input without trailing newline (line 203: `err != io.EOF` guard).
- **Empty value rejection**: Empty values are properly rejected (line 208-209).
- **CRLF handling**: `strings.TrimRight(line, "\r\n")` handles both Unix and Windows line endings (line 207).
- **Known key validation**: Both `get` and `set` paths validate against `isKnownSecret` before proceeding, preventing storage of arbitrary secret names.
- **Shell history protection**: Secret values are rejected as CLI arguments (line 146-149), forcing stdin input.
- **Secret value never printed**: Both `config get secrets.*` (line 69: prints "(set)") and `config set secrets.*` (line 188: prints "(set)") avoid printing the actual value.
- **JSON output**: Secrets section shows status strings, not actual values (line 294: uses `secretsStatus` map).

## Test Coverage Assessment

- `TestIsKnownSecret`: Covers all 5 known keys plus unknown and empty. Good.
- `TestReadSecretFromStdin`: Covers piped with newline, piped without newline (EOF), CRLF, empty input, empty EOF, and terminal input. Good coverage.
- `TestReadSecretFromStdinReadError`: Covers I/O error path with `errorReader`. Good.
- **Gap**: No integration-level test for the full `runConfigSet` flow (validation, loading config, setting, saving). This is acceptable since the individual pieces are tested and the `config set` command is exercised by the test plan's integration scenarios (12-16).

## Overall Assessment

The implementation matches the design doc's Phase 3b requirements exactly. It adds stdin-based secret input, status-only display, and a Secrets section in `tsuku config` output. No gold-plating, no unnecessary abstractions. The code is straightforward and follows existing patterns in the codebase. The test coverage is adequate for the functionality. Both advisories are minor.
