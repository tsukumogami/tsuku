# Test Plan: Secrets Manager

Generated from: docs/designs/DESIGN-secrets-manager.md
Issues covered: 5
Total scenarios: 16

---

## Scenario 1: Core secrets package builds and tests pass
**ID**: scenario-1
**Testable after**: #1733
**Category**: infrastructure
**Commands**:
- `go build ./internal/secrets`
- `go test -v ./internal/secrets`
**Expected**: Package compiles without errors and all unit tests pass.
**Status**: passed
**Validated**: 2026-02-16 via Docker (golang:1.25). Build exited 0, all 16 unit tests passed (0.003s).

---

## Scenario 2: Get() resolves secret from environment variable
**ID**: scenario-2
**Testable after**: #1733
**Category**: infrastructure
**Commands**:
- `ANTHROPIC_API_KEY="test-key-abc123" go test -v ./internal/secrets -run TestGet`
**Expected**: Test passes confirming that `Get("anthropic_api_key")` returns `"test-key-abc123"` when the environment variable is set.
**Status**: passed
**Validated**: 2026-02-16 via Docker (golang:1.25). TestGetResolvesFromEnvVar passed: sets ANTHROPIC_API_KEY via t.Setenv and confirms Get() returns the value. Also validated via TestGetAllKnownKeysFromEnv for all 5 keys.

---

## Scenario 3: Get() resolves multi-alias keys in priority order
**ID**: scenario-3
**Testable after**: #1733
**Category**: infrastructure
**Commands**:
- `GOOGLE_API_KEY="primary" GEMINI_API_KEY="fallback" go test -v ./internal/secrets -run TestGet`
- `GEMINI_API_KEY="fallback-only" go test -v ./internal/secrets -run TestGet`
**Expected**: When both `GOOGLE_API_KEY` and `GEMINI_API_KEY` are set, `Get("google_api_key")` returns the value from `GOOGLE_API_KEY` (the first alias). When only `GEMINI_API_KEY` is set, `Get("google_api_key")` returns its value as a fallback.
**Status**: passed
**Validated**: 2026-02-16 via Docker (golang:1.25). TestGetResolvesMultiAliasInPriorityOrder: both aliases set, first wins ("google-key"). TestGetResolvesSecondAlias: only GEMINI_API_KEY set, returns fallback ("gemini-fallback"). Both passed.

---

## Scenario 4: Get() rejects unknown keys with error
**ID**: scenario-4
**Testable after**: #1733
**Category**: infrastructure
**Commands**:
- `go test -v ./internal/secrets -run TestGetUnknownKey`
**Expected**: `Get("nonexistent_key")` returns an error containing "unknown secret" (or similar rejection message). The function does not panic or return an empty string silently.
**Status**: passed
**Validated**: 2026-02-16 via Docker (golang:1.25). TestGetRejectsUnknownKey passed: Get("nonexistent_key") returns error containing "unknown secret key" and the key name "nonexistent_key". No panic, no silent empty string.

---

## Scenario 5: Get() returns guidance error when key is not set anywhere
**ID**: scenario-5
**Testable after**: #1733
**Category**: infrastructure
**Commands**:
- `go test -v ./internal/secrets -run TestGet`
**Expected**: When no environment variable or config file value is set for a known key, `Get()` returns an error whose message includes the environment variable name(s) and mentions how to set the key (initially env var only; after #1734 the message also mentions config file).
**Status**: passed
**Validated**: 2026-02-16 via Docker (golang:1.25). TestGetReturnsGuidanceWhenNotSet: error message includes "ANTHROPIC_API_KEY", "config.toml", and "anthropic_api_key". TestGetGuidanceListsAllAliases: error for google_api_key mentions both GOOGLE_API_KEY and GEMINI_API_KEY. Both passed.

---

## Scenario 6: IsSet() and KnownKeys() return correct values
**ID**: scenario-6
**Testable after**: #1733
**Category**: infrastructure
**Commands**:
- `GITHUB_TOKEN="gh-token" go test -v ./internal/secrets -run TestIsSet`
- `go test -v ./internal/secrets -run TestKnownKeys`
**Expected**: `IsSet("github_token")` returns true when `GITHUB_TOKEN` is set and false when unset. `KnownKeys()` returns a slice containing all five registered secrets (anthropic_api_key, google_api_key, github_token, tavily_api_key, brave_api_key) with correct `Name`, `EnvVars`, and `Desc` fields.
**Status**: passed
**Validated**: 2026-02-16 via Docker (golang:1.25). IsSet tests (4 passed): true when set, false when empty, false for unknown, true via second alias. KnownKeys tests (4 passed): 5 entries sorted, all expected names present, all fields populated, google_api_key has 2 env vars in correct order.

---

## Scenario 7: Userconfig stores and retrieves secrets via [secrets] section
**ID**: scenario-7
**Testable after**: #1734
**Category**: infrastructure
**Commands**:
- `go test -v ./internal/userconfig -run TestSecret`
- `go test -v ./internal/userconfig`
**Expected**: All userconfig tests pass. Tests verify that: (a) `Set("secrets.foo", "bar")` stores the value in the `Secrets` map, (b) `Get("secrets.foo")` retrieves it, (c) the `Secrets` map serializes to a `[secrets]` TOML section, and (d) existing non-secrets tests continue to pass.
**Status**: passed
**Validated**: 2026-02-16 via Docker (golang:1.25). All 52 userconfig tests passed (0.021s). Secrets-specific tests verified Set/Get round-trip, TOML [secrets] section serialization, case insensitivity, nil map initialization, AvailableKeys exclusion, and non-interference with existing config.

---

## Scenario 8: Config file atomic writes with 0600 permissions
**ID**: scenario-8
**Testable after**: #1734
**Category**: infrastructure
**Commands**:
- `go test -v ./internal/userconfig -run TestAtomicWrite`
- `go test -v ./internal/userconfig -run TestPermission`
**Expected**: Tests verify that `saveToPath()` writes to a temporary file first and then renames atomically. The resulting file has exactly 0600 permissions. A file with wider permissions (e.g., 0644) triggers a warning on read but does not prevent loading.
**Status**: passed
**Validated**: 2026-02-16 via Docker (golang:1.25). 7 tests passed (0.013s). Atomic write produces 0600, preserves 0600 on overwrite (restores from 0644), leaves no temp files, maintains content integrity. Permission warning fires on 0644 file but load succeeds; no warning for 0600.

---

## Scenario 9: secrets.Get() falls through to config file on env var miss
**ID**: scenario-9
**Testable after**: #1734
**Category**: infrastructure
**Commands**:
- `go test -v ./internal/secrets -run TestConfigFallback`
**Expected**: When an environment variable is not set but the key exists in `config.Secrets["anthropic_api_key"]`, `Get("anthropic_api_key")` returns the config file value. Environment variables still take priority when both are set.
**Status**: passed
**Validated**: 2026-02-16 via Docker (golang:1.25). 4 tests passed (0.006s). Config fallback resolves from file when env var empty, env var takes priority when both set, error returned when neither source has value, missing config file handled gracefully with guidance message.

---

## Scenario 10: LLM providers use secrets package after migration
**ID**: scenario-10
**Testable after**: #1735
**Category**: infrastructure
**Commands**:
- `go test -v ./internal/llm/...`
**Expected**: All existing LLM tests pass. The `claude.go`, `gemini.go`, and `factory.go` files no longer contain direct `os.Getenv("ANTHROPIC_API_KEY")`, `os.Getenv("GOOGLE_API_KEY")`, or `os.Getenv("GEMINI_API_KEY")` calls in non-test code. Provider detection in `factory.go` uses `secrets.IsSet()`.
**Status**: passed
**Validated**: 2026-02-16 via Docker (golang:1.25). All 142 LLM tests passed (0 failed, 10 skipped integration tests). Source verification confirmed: claude.go uses secrets.Get("anthropic_api_key"), gemini.go uses secrets.Get("google_api_key"), factory.go uses secrets.IsSet() for provider detection. No os.Getenv calls for API keys remain in non-test code.

---

## Scenario 11: Platform tokens migrated to secrets package
**ID**: scenario-11
**Testable after**: #1736
**Category**: infrastructure
**Commands**:
- `go test -v ./internal/discover/... ./internal/version/... ./internal/builders/... ./internal/search/...`
**Expected**: All package tests pass. Non-test source files in discover, version, builders, and search packages no longer contain `os.Getenv("GITHUB_TOKEN")`, `os.Getenv("TAVILY_API_KEY")`, or `os.Getenv("BRAVE_API_KEY")`. Error messages reference both env var and config file options.
**Status**: passed
**Validated**: 2026-02-16 via Docker (golang:1.25). All 760 tests passed (0 failed, 4 skipped integration tests). Source verification confirmed: no os.Getenv calls for GITHUB_TOKEN/TAVILY_API_KEY/BRAVE_API_KEY in non-test code. All access uses secrets.Get() and secrets.IsSet(). Error messages mention both env var and config.toml.

---

## Scenario 12: CLI sets secret via stdin (pipe)
**ID**: scenario-12
**Testable after**: #1737
**Category**: infrastructure
**Commands**:
- `export TSUKU_HOME=$(mktemp -d) && export TSUKU_TELEMETRY=0 && make build-test && echo "test-secret-value" | ./tsuku-test config set secrets.anthropic_api_key && ./tsuku-test config get secrets.anthropic_api_key`
**Expected**: The `config set` command reads the value from stdin (no prompt when piped). The `config get` command prints `(set)` without revealing the actual value. The value never appears in command output.
**Status**: passed
**Validated**: 2026-02-16 via Docker (golang:1.25). Piped "test-secret-value" into config set; exit code 0, output confirmed `(set)` masking. config get returned `(set)` without revealing actual value. Config.toml [secrets] section stored value correctly.

---

## Scenario 13: CLI displays known secrets with status
**ID**: scenario-13
**Testable after**: #1737
**Category**: infrastructure
**Commands**:
- `export TSUKU_HOME=$(mktemp -d) && export TSUKU_TELEMETRY=0 && make build-test && echo "test-key" | ./tsuku-test config set secrets.github_token && ./tsuku-test config`
**Expected**: The `tsuku config` output includes a "Secrets:" section. The `github_token` line shows `(set)`. The other four known secrets (anthropic_api_key, google_api_key, tavily_api_key, brave_api_key) show `(not set)`. The actual secret value does not appear in the output.
**Status**: passed
**Validated**: 2026-02-16 via Docker (golang:1.25). Set github_token via pipe, then ran `tsuku config`. Output shows "Secrets:" section with github_token: (set) and all four other keys as (not set). Secret value "test-key" never appears in output. All 5 known keys listed alphabetically.

---

## Scenario 14: End-to-end secret resolution from config file (no env var)
**ID**: scenario-14
**Testable after**: #1735, #1737
**Category**: use-case
**Environment**: manual (requires a valid Anthropic API key)
**Commands**:
- `export TSUKU_HOME=$(mktemp -d) && export TSUKU_TELEMETRY=0 && make build-test`
- `unset ANTHROPIC_API_KEY`
- `echo "<real-anthropic-key>" | ./tsuku-test config set secrets.anthropic_api_key`
- `./tsuku-test discover jq`
**Expected**: With `ANTHROPIC_API_KEY` unset and the key stored only in the config file, the `discover` command successfully uses the API key from `[secrets]` in `config.toml`. The LLM provider initializes and produces discovery results for `jq`. This validates the full resolution chain: CLI writes secret to config, secrets package reads it as fallback, LLM provider uses it.
**Status**: pending

---

## Scenario 15: End-to-end platform token resolution from config file
**ID**: scenario-15
**Testable after**: #1736, #1737
**Category**: use-case
**Environment**: manual (requires a valid GitHub token)
**Commands**:
- `export TSUKU_HOME=$(mktemp -d) && export TSUKU_TELEMETRY=0 && make build-test`
- `unset GITHUB_TOKEN`
- `echo "<real-github-token>" | ./tsuku-test config set secrets.github_token`
- `./tsuku-test versions gh`
**Expected**: With `GITHUB_TOKEN` unset and the token stored only in the config file, the `versions` command successfully resolves versions for `gh` using the GitHub API with the config-file token. This validates that platform token migration works end-to-end with the config file fallback.
**Status**: pending

---

## Scenario 16: Config file permission enforcement on secret write
**ID**: scenario-16
**Testable after**: #1734, #1737
**Category**: use-case
**Commands**:
- `export TSUKU_HOME=$(mktemp -d) && export TSUKU_TELEMETRY=0 && make build-test && echo "test-key" | ./tsuku-test config set secrets.anthropic_api_key && stat -c "%a" "$TSUKU_HOME/config.toml"`
**Expected**: After writing a secret via `tsuku config set secrets.*`, the config file has exactly `600` permissions. The file is owned by the current user and is not readable by group or others.
**Status**: pending

---
