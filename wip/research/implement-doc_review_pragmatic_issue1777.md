# Pragmatic Review: Issue #1777 (feat(llm): add llm.backend config key)

## Summary

The implementation is correct, minimal, and follows existing patterns exactly. No blocking issues.

## Files Reviewed

- `internal/userconfig/userconfig.go`
- `internal/userconfig/userconfig_test.go`
- `internal/llm/factory.go`
- `internal/llm/factory_test.go`

## Findings

### Advisory: `o.config` stored but never read in `factoryOptions`

**File**: `internal/llm/factory.go:41` (field declaration), `:61` (assignment)

`WithConfig()` at line 61 stores `cfg` into `o.config`, but `o.config` is never accessed after that -- only the individual fields (`o.enabled`, `o.localEnabled`, etc.) extracted from it are used. This field predates this PR (it existed before `LLMBackend()` was added to the interface), so it's pre-existing dead weight, not scope creep from this issue. Noting it only because adding `LLMBackend()` to the `LLMConfig` interface means the stored config *could* be used to read the backend later, but currently isn't.

**Severity**: Advisory (pre-existing, not introduced by this PR)

### Advisory: `LLMBackend()` on `LLMConfig` interface has no consumer yet

**File**: `internal/llm/factory.go:27`

`LLMBackend() string` was added to the `LLMConfig` interface, but nothing in `factory.go` or `NewFactory()` reads it. The design doc says #1778 (migrate addon to recipe system) will consume it. This is the expected sequence per the design doc -- #1777 is explicitly listed as a dependency of #1778 and is described as "Independent of GPU detection, can start in parallel."

The `mockLLMConfig` in `factory_test.go` has a `backend` field and implements `LLMBackend()`, which confirms the interface is satisfiable. The `userconfig.Config` type also satisfies the expanded interface (verified: `LLMBackend()` method exists at `userconfig.go:268`).

**Severity**: Advisory (deliberate staging per design doc, not speculative)

## Correctness Assessment

1. **`Backend *string` field**: Matches the `*bool` pattern used by `Enabled`, `LocalEnabled`, `LocalPreemptive`. Nil means unset (auto-detect), non-nil means explicit override. Correct.

2. **`validLLMBackends` slice**: Contains only `"cpu"`, matching the design doc's spec ("initially only `cpu` override"). The validation loop in `Set()` at line 404 correctly rejects anything not in the slice and allows empty string to clear (sets pointer to nil).

3. **`Set("llm.backend", "")` clears to nil**: Line 400-402. This means after clearing, `LLMBackend()` returns `""` and the TOML serialization omits the field (`omitempty`). Correct behavior for "return to auto-detect."

4. **Case-sensitive value validation**: `Set()` lowercases the *key* but not the *value*. So `Set("LLM.BACKEND", "cpu")` works but `Set("llm.backend", "CPU")` is rejected. This matches the test at line 1218 (`"CPU"` is in the invalid values list). The design doc says the valid value is `"cpu"` (lowercase), so rejecting `"CPU"` is correct.

5. **TOML round-trip**: Tested at lines 1140-1202. Save with `backend = "cpu"`, load it back, confirm. Save with nil backend, load it back, confirm empty. Both pass.

6. **`AvailableKeys()`**: Includes `"llm.backend"` with a description mentioning `cpu`. Tested at line 1204.

## Test Coverage

Tests cover: default (nil), set to "cpu", clear with empty string, invalid values (7 different strings including "CPU"), case-insensitive key, TOML round-trip (set and unset), load from file, and AvailableKeys inclusion. This is thorough for a config key addition.

## Verdict

Clean implementation. Follows existing patterns without deviation. No correctness issues. The two advisory notes are informational -- one is pre-existing and one is deliberate staging.
