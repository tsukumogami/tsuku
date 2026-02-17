# Validation Report: Issue #1737

**Date**: 2026-02-16
**Environment**: Docker (golang:1.25, go1.25.7 linux/amd64)
**Binary**: `tsuku-test` built with `-buildvcs=false -ldflags "-X main.defaultHomeOverride=.tsuku-test"`
**Isolation**: Each scenario used a fresh `TSUKU_HOME=$(mktemp -d)` with `TSUKU_TELEMETRY=0`

---

## Scenario 12: CLI sets secret via stdin (pipe)

**Status**: PASSED

### Execution

```
export TSUKU_HOME=$(mktemp -d)
export TSUKU_TELEMETRY=0

# Step 1: Pipe secret value into config set
echo "test-secret-value" | ./tsuku-test config set secrets.anthropic_api_key
# Output: secrets.anthropic_api_key = (set)
# Exit code: 0

# Step 2: Retrieve the secret
./tsuku-test config get secrets.anthropic_api_key
# Output: (set)
# Exit code: 0
```

### Checks

| Check | Result | Detail |
|-------|--------|--------|
| `config set` reads from stdin without prompt | PASSED | Exit code 0, no interactive prompt |
| `config get` prints `(set)` | PASSED | Output is exactly `(set)` |
| Actual value not in `config get` output | PASSED | `test-secret-value` not found in output |
| Value stored in config.toml correctly | PASSED | `[secrets]` section contains `anthropic_api_key = "test-secret-value"` |

### Config file contents after set

```toml
telemetry = true

[llm]

[secrets]
  anthropic_api_key = "test-secret-value"
```

---

## Scenario 13: CLI displays known secrets with status

**Status**: PASSED

### Execution

```
export TSUKU_HOME=$(mktemp -d)
export TSUKU_TELEMETRY=0

# Step 1: Set one secret
echo "test-key" | ./tsuku-test config set secrets.github_token
# Output: secrets.github_token = (set)
# Exit code: 0

# Step 2: View full config
./tsuku-test config
# Exit code: 0
```

### Full output from `tsuku config`

```
TSUKU_HOME: /tmp/tmp.iBzFdl4wqC
TSUKU_API_TIMEOUT: 30s
TSUKU_VERSION_CACHE_TTL: 1h0m0s
telemetry: true

Secrets:
  anthropic_api_key: (not set)
  brave_api_key: (not set)
  github_token: (set)
  google_api_key: (not set)
  tavily_api_key: (not set)
```

### Checks

| Check | Result | Detail |
|-------|--------|--------|
| Output contains "Secrets:" section | PASSED | Section header present |
| github_token shows `(set)` | PASSED | Line: `github_token: (set)` |
| anthropic_api_key shows `(not set)` | PASSED | Line: `anthropic_api_key: (not set)` |
| google_api_key shows `(not set)` | PASSED | Line: `google_api_key: (not set)` |
| tavily_api_key shows `(not set)` | PASSED | Line: `tavily_api_key: (not set)` |
| brave_api_key shows `(not set)` | PASSED | Line: `brave_api_key: (not set)` |
| Actual secret value not in output | PASSED | `test-key` not found in output |

---

## Summary

Both scenarios passed all validation checks. The CLI correctly:

1. Reads secret values from stdin when piped (no interactive prompt)
2. Confirms the set operation with `(set)` masking
3. Returns `(set)` for `config get` on secret keys, never revealing the value
4. Displays a "Secrets:" section in `config` output listing all 5 known keys
5. Shows `(set)` vs `(not set)` status for each key
6. Never leaks actual secret values in any command output
