# Maintainability Review: Issue #1734

## Review Scope

Issue: #1734 (feat(userconfig): add secrets section with atomic writes and 0600 permissions)
Focus: Maintainability (clarity, readability, duplication, naming, test quality)

Files reviewed:
- `internal/userconfig/userconfig.go`
- `internal/userconfig/userconfig_test.go`
- `internal/secrets/secrets.go`
- `internal/secrets/specs.go`
- `internal/secrets/secrets_test.go`

---

## Finding 1: Package doc comment uses `~/.tsuku` instead of `$TSUKU_HOME`

**File and line:** `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/userconfig/userconfig.go`, lines 1-3
**Severity:** advisory
**What:** The package doc comment says `~/.tsuku/config.toml`, but the project convention (documented in CLAUDE.local.md under "Conventions > Use `$TSUKU_HOME` in documentation") requires using `$TSUKU_HOME` in documentation and comments.
**Why it matters:** A contributor reading this doc comment would assume the path is hardcoded to `~/.tsuku`, when in fact the path is configurable via `TSUKU_HOME`. This could lead to incorrect assumptions when debugging config loading issues or writing new features.
**Suggestion:** Change to:
```go
// Package userconfig provides user configuration management for tsuku.
// Configuration is stored in $TSUKU_HOME/config.toml and can be modified
// via the `tsuku config` command.
```

---

## Finding 2: Duplicated resolution logic in `Get()` and `IsSet()`

**File and line:** `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/secrets/secrets.go`, lines 66-118
**Severity:** advisory
**What:** `Get()` (lines 66-93) and `IsSet()` (lines 97-118) duplicate the entire resolution chain: look up spec, iterate env vars, fall through to config file. The only difference is the return type (value vs. bool).
**Why it matters:** If the resolution logic changes (e.g., adding a third resolution source, changing the empty-check behavior), a developer must remember to update both functions identically. With only two functions this is manageable, but it's one line away from a clean extraction.
**Suggestion:** Extract a shared `resolve(name string) (string, KeySpec, error)` function that returns the resolved value (or empty string + not-found error), then have both `Get()` and `IsSet()` delegate to it:
```go
func resolve(name string) (value string, spec KeySpec, err error) {
    spec, ok := knownKeys[name]
    if !ok {
        return "", KeySpec{}, fmt.Errorf("unknown secret key: %q", name)
    }
    for _, env := range spec.EnvVars {
        if val := os.Getenv(env); val != "" {
            return val, spec, nil
        }
    }
    cfg, err := getConfig()
    if err == nil && cfg != nil && cfg.Secrets != nil {
        if val, ok := cfg.Secrets[name]; ok && val != "" {
            return val, spec, nil
        }
    }
    return "", spec, nil // not found, but not an error
}
```

---

## Finding 3: Permission warning message lacks actionability

**File and line:** `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/userconfig/userconfig.go`, lines 118-122
**Severity:** advisory
**What:** The warning message `"config file has permissive permissions"` tells the user something is wrong but doesn't tell them what will happen or what to do. The design doc specifies the message should mention `"will be tightened to 0600 on next write"`.
**Why it matters:** A user seeing this warning in `--verbose` output won't know whether they need to act. Adding "will be tightened to 0600 on next write" communicates that no manual intervention is needed, which matches the design's intent of a graceful transition.
**Suggestion:** Update the message to:
```go
log.Default().Warn("config file has permissive permissions, will be tightened to 0600 on next write",
    "path", path,
    "mode", fmt.Sprintf("%04o", mode),
)
```

---

## Finding 4: Permission warning tests don't verify the warning is actually logged

**File and line:** `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/userconfig/userconfig_test.go`, lines 988-1026
**Severity:** advisory
**What:** `TestPermissionWarningOnPermissiveFile` claims to test that a warning is emitted for permissive files, but the test body only verifies that loading succeeds and returns correct data. It never checks that a warning was logged. Similarly, `TestPermissionWarningNotTriggeredFor0600` doesn't verify the absence of a warning.
**Why it matters:** These test names promise behavior they don't validate. If the warning logic is accidentally removed, these tests will still pass. A future developer may trust the test names and skip verifying that warnings work correctly.
**Suggestion:** Either:
1. Rename the tests to `TestLoadSucceedsWithPermissiveFile` and `TestLoadSucceedsWith0600File` to reflect what they actually test, or
2. Inject a test logger and assert on the warning output (preferred, since the warning behavior is a key design requirement of this issue).

---

## Finding 5: `TestKnownKeysReturnsAllSecrets` uses a magic number

**File and line:** `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/secrets/secrets_test.go`, line 142
**Severity:** advisory
**What:** The test asserts `len(keys) != 5` without linking that number to the actual `knownKeys` map. When a developer adds a 6th secret to `specs.go`, they'll get a test failure in a seemingly unrelated test with no indication of why 5 was expected.
**Why it matters:** A developer adding a new KeySpec has to discover this hard-coded count by reading the failure. The test `TestKnownKeysContainsExpectedEntries` already validates the exact set, making this count check redundant.
**Suggestion:** Either remove the count check entirely (since `TestKnownKeysContainsExpectedEntries` covers the same ground more precisely), or derive the count from the expected set:
```go
expected := []string{"anthropic_api_key", "google_api_key", ...}
if len(keys) != len(expected) {
    t.Fatalf("expected %d known keys, got %d", len(expected), len(keys))
}
```

---

## Overall Assessment

The implementation is clean, well-organized, and closely follows the design doc. The `internal/secrets` package has clear separation from `userconfig`, the atomic write logic in `saveToPath` is straightforward with good error handling, and the test coverage is thorough for the happy path and config fallback scenarios.

The most actionable finding is the duplication between `Get()` and `IsSet()` (Finding 2), which is a moderate maintenance risk as the resolution logic grows. The permission warning tests (Finding 4) are the most misleading -- their names imply behavior validation that isn't present -- but this is a polish issue rather than a correctness one.

No blocking issues found.
