# Pragmatic Review: Issue #1734

**Issue**: feat(userconfig): add secrets section with atomic writes and 0600 permissions
**Reviewer focus**: pragmatic (simplicity, YAGNI, KISS)
**Commit**: 2d6b4b9172dc6434c68bd20cf7569d1853120701

## Files Changed

- `internal/userconfig/userconfig.go` -- Added `Secrets` map field, atomic write in `saveToPath()`, permission warning in `loadFromPath()`, secrets prefix handling in `Get()`/`Set()`
- `internal/userconfig/userconfig_test.go` -- Added tests for secrets CRUD, atomic writes, permissions, TOML serialization
- `internal/secrets/secrets.go` -- Wired config file fallback into `Get()` and `IsSet()`
- `internal/secrets/secrets_test.go` -- Added config fallback tests (`TestConfigFallback*`)

## Findings

### Finding 1: No validation that secret keys are known in userconfig.Set()

**File**: `internal/userconfig/userconfig.go`, lines 306-313
**Severity**: advisory
**What**: `Set("secrets.anything_at_all", value)` accepts any key. There is no cross-check against the `knownKeys` table in the secrets package.
**Why it matters**: A user could typo `tsuku config set secrets.antrhopic_api_key mykey` and it would silently store under the wrong key name. The secrets package would never find it because `Get("anthropic_api_key")` looks up by canonical name.
**Suggestion**: This is defensible as-is since userconfig shouldn't depend on secrets (that would create a circular dependency), and the CLI layer (#1737) is the right place to validate against known keys. No action needed now, but worth noting that #1737 must add validation. If #1737 doesn't validate, this becomes blocking there.

### Finding 2: Config file fallback swallows config load errors silently

**File**: `internal/secrets/secrets.go`, lines 80-85
**Severity**: advisory
**What**: In `Get()`, when `getConfig()` returns an error (e.g., corrupted TOML), the code silently falls through to the "not configured" error. The user gets "anthropic_api_key not configured" instead of learning that their config file is broken.
**Why it matters**: A user who has correctly set a secret in their config file but later corrupts the TOML (e.g., bad edit) would get a misleading error saying the key isn't configured. They'd re-set it, thinking it was lost, when the real problem is a parse error.
**Suggestion**: Consider logging a warning when `configError != nil`, similar to how `loadFromPath` logs a warning for permissive permissions. The fallthrough to env-var-only resolution is still correct (fail open), but visibility into the config load failure would help debugging. Not blocking because the current behavior is fail-safe and the user would eventually notice via `tsuku config` output.

### Finding 3: Implementation is well-scoped -- no gold plating detected

**What**: The changes are tightly focused on what the issue requires:
- `Secrets map[string]string` field with `omitempty` tag (no empty sections in TOML output)
- Atomic write via temp file + rename (straightforward pattern, not over-abstracted)
- Permission check is a simple bitmask test with a log warning (no auto-remediation on read)
- `Get()`/`Set()` prefix routing for `secrets.*` is clean and minimal
- Config fallback in secrets.go is 5 lines of nil-safe checks

There are no unnecessary abstractions, no config options, no feature flags, and no helper functions called once. The atomic write pattern is inline in `saveToPath()` rather than extracted into a generic utility, which is appropriate since there's only one caller.

### Finding 4: Test coverage is thorough without being excessive

**What**: The test suite covers:
- Secrets CRUD (set, get, missing, empty, case insensitivity, nil map init)
- TOML round-trip (save/load with secrets section)
- Atomic write properties (0600 permissions, no temp file leaks, parent dir creation, overwrite restores permissions)
- Config file fallback (env var priority, config-only resolution, both-empty error, missing config file)
- Pre-existing tests preserved (secrets don't affect other config values)

No over-testing detected. Each test validates one concrete property.

## Summary

The implementation matches the issue requirements cleanly. The atomic write pattern is standard and not over-engineered. The `Secrets` map addition to `Config` is minimal. The config fallback wiring in `secrets.Go` is appropriately thin.

Two advisory notes: (1) typos in secret key names via `Set()` won't be caught until the CLI layer validates against known keys in #1737, and (2) config load errors are silently swallowed during secret resolution, which could produce misleading "not configured" errors when the real problem is a corrupted config file. Neither is blocking.

No blocking findings. Ready to proceed.
