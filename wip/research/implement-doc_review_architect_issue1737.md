# Architectural Review: #1737 feat(cli): add secrets management to tsuku config

**Reviewer focus**: Architecture (design patterns, separation of concerns)
**Files in scope**: `cmd/tsuku/config.go`, `cmd/tsuku/config_test.go`
**Dependencies already landed**: `internal/secrets` (#1733), `internal/userconfig` secrets section (#1734)

---

## Finding 1: Two different terminal-detection patterns coexist

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/cmd/tsuku/config.go` lines 122-125
**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/cmd/tsuku/install.go` line 231-236
**Severity**: advisory

The new code introduces `stdinIsTerminal` using `golang.org/x/term.IsTerminal()` at package level, while the existing codebase already has `isInteractive()` in `install.go` using `os.Stdin.Stat()` with `ModeCharDevice` checks. These are two different patterns for detecting terminal input.

The new approach (package-level `var stdinIsTerminal = func() bool`) is more testable than `isInteractive()`, which directly calls `os.Stdin.Stat()` and can't be swapped for testing. This is actually a better pattern, but having two coexisting approaches for the same concern is an inconsistency.

**Impact**: Minimal -- both approaches work. Over time, this creates confusion about which to use for new commands.

**Suggestion**: This is fine for now. A follow-up could consolidate `isInteractive()` to use the same replaceable-function pattern, but that's out of scope for this issue.

---

## Finding 2: Secret validation duplicated between CLI and secrets package

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/cmd/tsuku/config.go` lines 230-237 (`isKnownSecret`)
**Severity**: advisory

The `isKnownSecret()` function in `config.go` iterates `secrets.KnownKeys()` to check if a name is valid. The `secrets` package itself validates key names in `Get()` and `IsSet()` -- they return an error or `false` for unknown keys. The CLI pre-validates before calling into `secrets` or `userconfig`, which is a reasonable defensive pattern for producing better error messages (listing known secrets). However, there's a subtle mismatch: `userconfig.Set("secrets.unknown_key", value)` succeeds silently because `Set()` doesn't validate against the known keys registry -- it just stores any string in the `Secrets` map.

This means the validation gate only exists in the CLI layer. If any other code path calls `cfg.Set("secrets.foo", val)` it will happily store an unknown key. The `secrets.Get()` call will later reject it, but the config file now contains a stale entry.

**Impact**: Low -- there are no other code paths today that call `cfg.Set("secrets.*", ...)`. But if a future programmatic caller (e.g., `tsuku init` or plugin system) writes secrets, they'd bypass validation.

**Suggestion**: This is acceptable as-is. The `userconfig` package is a general key-value store; it shouldn't know about the `secrets` package's registry (that would create a circular dependency or coupling). The CLI is the right layer for this validation. If programmatic callers appear later, they should also validate through `secrets.KnownKeys()`.

---

## Finding 3: `configOutput` JSON struct is defined inline and missing LLM fields

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/cmd/tsuku/config.go` lines 281-287
**Severity**: advisory

The `configOutput` struct is defined inline within `runConfig()`. It includes `Secrets` (new) but omits LLM settings that are available in `userconfig.Config` (e.g., `llm.enabled`, `llm.providers`, `llm.daily_budget`). The human-readable output also omits LLM settings.

This is a pre-existing gap -- the missing LLM fields in the output predate this issue. The new code correctly adds `Secrets` to both the JSON and human-readable outputs, which is consistent with the pattern (show what's there).

**Impact**: Not caused by this issue. The JSON output doesn't fully represent config state, but secrets are correctly integrated into both output modes.

**Suggestion**: Out of scope for this issue. A follow-up could add LLM settings to the display output.

---

## Finding 4: Architecture alignment -- design doc vs. implementation

The design doc specifies:
- `tsuku config set secrets.<name>` reads value from stdin (not CLI args) -- **implemented correctly** (lines 145-149 reject CLI args, lines 153-158 read from stdin)
- `tsuku config get secrets.<name>` shows `(set)` / `(not set)` -- **implemented correctly** (lines 61-73)
- `tsuku config` display shows all known secrets with status -- **implemented correctly** (lines 269-308)
- Piped input supported (`echo key | tsuku config set secrets.*`) -- **implemented correctly** via `readSecretFromStdin` which reads from `stdinReader`

The implementation matches the design doc's Phase 3b specification precisely. The separation of concerns is clean:
- CLI layer (`config.go`): input handling, validation against known keys, user-facing output
- `secrets` package: key registry, resolution logic
- `userconfig` package: persistence, atomic writes, permissions

The dependency direction is correct: `config.go` -> `secrets` and `config.go` -> `userconfig`. The `secrets` package depends on `userconfig` for config file fallback. No circular dependencies.

---

## Finding 5: stdin reader and stdinIsTerminal as package-level mutable state

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/cmd/tsuku/config.go` lines 119-125
**Severity**: advisory

The `stdinReader` and `stdinIsTerminal` are package-level vars used for testing seams. This is a common Go pattern for CLI commands that need testability, and the test file properly saves/restores them. Since this is in the `main` package (not a library), the risk of concurrent test interference is low -- Go test functions in the same package run sequentially by default.

**Impact**: None currently. If tests ever use `t.Parallel()`, the shared mutable state could cause races. But this is standard practice for `main` package tests in Go CLIs.

**Suggestion**: Acceptable as-is. Matches the pragmatic approach in this codebase.

---

## Summary Assessment

The implementation cleanly follows the design doc's Phase 3b specification. The separation of concerns between the CLI layer (`config.go`), secrets resolution (`internal/secrets`), and persistence (`internal/userconfig`) is well-maintained. Dependencies flow in the correct direction with no circular imports. The `secrets.*` key prefix routing in both `Get()` and `Set()` on `userconfig.Config` is consistent and the CLI layer adds appropriate validation before delegating.

No blocking findings. The advisory items are minor inconsistencies and improvement opportunities that don't affect correctness or architecture integrity.
