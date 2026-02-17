# Maintainability Review: #1737 feat(cli): add secrets management to tsuku config

## Review Scope

Files reviewed (changes attributed to #1737):
- `cmd/tsuku/config.go` -- secrets integration in CLI commands
- `cmd/tsuku/config_test.go` -- unit tests for new helpers

## Findings

### Finding 1: Help text key lists are triplicated across commands

**File**: `cmd/tsuku/config.go`, lines 27-32, 49-55, 102-108
**Severity**: advisory
**What**: The "Available keys/settings" list appears in three separate places -- the parent `configCmd.Long`, `configGetCmd.Long`, and `configSetCmd.Long`. Each has slightly different formatting and content. For example, the parent command omits `secrets.<name>` while the subcommands include it. The parent also omits `llm.daily_budget` and `llm.hourly_rate_limit` that `AvailableKeys()` exposes.

**Why it matters**: When a new config key or secret is added, a developer must remember to update three help strings manually. The parent command already drifts from the subcommands (no secrets mention), which means `tsuku config --help` doesn't tell users about the feature this issue adds.

**Suggestion**: Add `secrets.<name>` to the parent `configCmd.Long` help text at minimum. Long-term, consider generating the key list from `AvailableKeys()` + `KnownKeys()` to keep them in sync, though that's a larger refactor beyond this issue's scope.

---

### Finding 2: `printAvailableKeys()` omits secrets in error hints

**File**: `cmd/tsuku/config.go`, lines 83-87 (configGetCmd) and lines 175-179 (runConfigSet)
**Severity**: advisory
**What**: When a user types an unknown non-secret key like `tsuku config get typo`, the error shows "Unknown config key: typo" followed by "Available keys:" which only prints regular config keys from `userconfig.AvailableKeys()`. Secret keys are valid for both `get` and `set` but aren't listed in the error hint.

**Why it matters**: A user who knows about secrets but makes a typo (e.g., `tsuku config get secret.anthropic_api_key` -- missing the "s") will see an error that doesn't mention secrets at all. They might not realize the correct prefix is `secrets.`.

**Suggestion**: After `printAvailableKeys()`, also call `printKnownSecrets()` (perhaps under a "Secret keys:" subheading) so the error output shows the full set of valid keys.

---

### Finding 3: Inconsistent prefix detection -- `HasPrefix` vs `CutPrefix`

**File**: `cmd/tsuku/config.go`, lines 61, 131, 135
**Severity**: advisory
**What**: `configGetCmd` uses `strings.CutPrefix()` on line 61 to detect and extract the secret name in one step. `runConfigSet` uses `strings.HasPrefix()` on line 131 to detect the prefix, then calls `strings.CutPrefix()` again on line 135 to extract the name. The two code paths do the same thing with different patterns.

**Why it matters**: A reader comparing the get and set paths might wonder if the behavioral difference is intentional. Using the same approach in both places reduces cognitive load.

**Suggestion**: Use the same pattern in both: either `CutPrefix` (preferred, since it avoids the redundant check) or `HasPrefix` + `CutPrefix`. The get handler's approach is cleaner:
```go
if secretName, ok := strings.CutPrefix(strings.ToLower(key), "secrets."); ok {
    // ...
}
```

---

### Finding 4: `isKnownSecret` iterates the full list on every call

**File**: `cmd/tsuku/config.go`, lines 229-237
**Severity**: advisory
**What**: `isKnownSecret` calls `secrets.KnownKeys()` (which builds a new sorted slice each time) and does a linear scan. It's called in both `configGetCmd` and `runConfigSet`.

**Why it matters**: This is functionally fine for 5 keys, but the pattern is slightly wasteful. More importantly, it duplicates knowledge about how to check secret validity. The `secrets` package exposes `IsSet()` which already does a map lookup internally, but there's no `IsKnown()` function. This forces the CLI to re-implement "is this a valid secret name?" logic.

**Suggestion**: Consider adding an `IsKnown(name string) bool` function to the `secrets` package that does a direct map lookup (`_, ok := knownKeys[name]`). This is cleaner and avoids the O(n) construction + scan. This is a minor optimization that also improves API completeness, so it could be deferred.

---

### Finding 5: Test coverage focuses on helpers but not on command behavior

**File**: `cmd/tsuku/config_test.go`
**Severity**: advisory
**What**: Tests cover `isKnownSecret` and `readSecretFromStdin` thoroughly, which is good. However, there are no tests for the higher-level command behavior: that `configGetCmd` returns "(set)"/"(not set)" for secrets, that `runConfigSet` rejects CLI arguments for secrets, or that `runConfig` displays the Secrets section.

**Why it matters**: The tested helpers have simple logic that's unlikely to break. The untested command handlers contain the security-relevant behavior (rejecting CLI args for secrets, never displaying actual values). A future refactor could accidentally expose secret values in `config get` output without any test catching it.

**Suggestion**: This is partly constrained by the `exitWithCode(os.Exit)` pattern making command-level testing harder. The test plan's scenarios 12-13 cover this via integration tests, which is acceptable. If unit-level command testing becomes feasible (e.g., by making `exitWithCode` a replaceable function for testing, which it already wraps), adding a test that verifies `configGetCmd` never outputs the actual secret value would be a valuable safety net.

---

### Finding 6: `configOutput` JSON struct doesn't include LLM settings

**File**: `cmd/tsuku/config.go`, lines 281-287
**Severity**: advisory (pre-existing, not introduced by #1737)
**What**: The JSON output struct includes `Secrets` (added by this issue) but omits LLM settings (`llm.enabled`, `llm.providers`, etc.) that are shown in the help text and managed by `get`/`set`. The human-readable output also omits LLM settings.

**Why it matters**: Noting this only because the new `Secrets` field was added to `configOutput` -- a developer looking at this struct might wonder why LLM settings are missing. This is out of scope for #1737 but worth a follow-up.

**Suggestion**: Out of scope for this issue. No action needed.

---

### Finding 7: The `~/.tsuku/config.toml` path in help text violates the `$TSUKU_HOME` convention

**File**: `cmd/tsuku/config.go`, line 25
**Severity**: advisory (pre-existing)
**What**: The `configCmd.Long` help text says "Configuration is stored in ~/.tsuku/config.toml" but the project convention (per CLAUDE.local.md) is to use `$TSUKU_HOME` in documentation and error messages.

**Why it matters**: Pre-existing issue, not introduced by #1737. The new secrets-related code correctly uses `$TSUKU_HOME` in the `secrets` package error messages, so there's no new violation.

**Suggestion**: Out of scope for this issue. The pre-existing reference should be fixed separately.

---

## Overall Assessment

The implementation is clean and well-organized. The code reads naturally, function names communicate intent (`readSecretFromStdin`, `isKnownSecret`, `printKnownSecrets`), and the security-critical behavior (stdin-only input, status-only display) is correctly implemented with clear comments explaining why.

The test helpers (`stdinReader`, `stdinIsTerminal`) are a practical approach to making I/O-dependent code testable, and the test cases cover meaningful edge cases (CRLF, EOF without newline, empty input, broken reader).

No blocking findings. The advisory items are minor improvements that can be addressed in follow-up work.
