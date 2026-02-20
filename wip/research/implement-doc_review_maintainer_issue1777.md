# Maintainer Review: Issue #1777 (feat(llm): add llm.backend config key)

## Review Scope

Files changed:
- `internal/userconfig/userconfig.go`
- `internal/userconfig/userconfig_test.go`
- `internal/llm/factory.go`
- `internal/llm/factory_test.go`

## Findings

### 1. Backend comment says "Empty or nil means auto-detect" but the valid values list will grow

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/userconfig/userconfig.go:69`
**Severity**: Advisory

The `Backend` field comment says:
```go
// Valid values: "cpu" (force CPU variant). Empty or nil means auto-detect.
```

The `validLLMBackends` slice at line 88 currently has only `"cpu"`. The design doc (section "Decision 4: Runtime Failure Handling" and the override table) confirms that only `cpu` is needed initially. The comment is accurate today.

The `validLLMBackends` slice pattern makes it easy to add values later without changing the validation logic -- good for extension. No issue here, just noting the pattern is well-chosen for future work.

### 2. Value case sensitivity is inconsistent with key case insensitivity

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/userconfig/userconfig.go:399-410`
**Severity**: Advisory

The `Set()` method lowercases the *key* (line 279: `lowerKey := strings.ToLower(key)`), so `LLM.BACKEND` and `llm.backend` both work. But the *value* comparison at line 405 is exact-match: `if value == v`. This means `tsuku config set llm.backend CPU` fails with "invalid value for llm.backend: must be one of: cpu".

The test at line 1215-1225 (`TestSetLLMBackendRejectsMultipleInvalidValues`) explicitly includes `"CPU"` in the rejected values list, confirming this is intentional. The error message tells the user what the valid values are, so debugging is straightforward.

However, the next developer might wonder: is the value case-sensitive by design or by accident? Other config keys like `telemetry` parse values through `strconv.ParseBool`, which handles mixed-case (`"True"`, `"TRUE"` are valid). A one-line comment above the validation loop -- something like `// Values are case-sensitive (lowercase only)` -- would prevent the question.

### 3. `LLMBackend()` added to `LLMConfig` interface but not consumed by `WithConfig`

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/llm/factory.go:27` and `:59-72`
**Severity**: Advisory

`LLMBackend() string` is added to the `LLMConfig` interface at line 27, but `WithConfig()` at line 59-72 does not read `cfg.LLMBackend()`. The backend value is not stored in `factoryOptions` or used anywhere in the factory.

This is consistent with the design doc, which says issue #1778 (the addon-to-recipe migration) will wire `llm.backend` into plan generation. The interface change ships now so that #1778 can use it without a second interface-breaking change.

The next developer seeing `LLMBackend()` on the interface but nothing calling it might wonder if it's dead code. The `config` field at line 41 (`config LLMConfig`) does store the full config object, so downstream code (e.g., a future `AddonManager`) can access it through `o.config.LLMBackend()`. This makes the interface addition forward-looking rather than dead.

This is fine as-is. The risk is low because the interface, its only implementation (`userconfig.Config`), and the mock (`mockLLMConfig` in tests) are all internal.

### 4. Three `LLMConfig` interfaces exist across packages with different method sets

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/llm/factory.go:22-28`, `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/builders/builder.go:30-39`, `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/discover/llm_discovery.go:53-55`
**Severity**: Advisory (pre-existing, not introduced by this change)

There are three separate `LLMConfig` interfaces in the codebase:
- `internal/llm/factory.go`: `LLMEnabled`, `LLMLocalEnabled`, `LLMIdleTimeout`, `LLMProviders`, `LLMBackend`
- `internal/builders/builder.go`: `LLMEnabled`, `LLMDailyBudget`, `LLMHourlyRateLimit`
- `internal/discover/llm_discovery.go`: `LLMDailyBudget`

All three are satisfied by `userconfig.Config`, which implements every method. This is idiomatic Go (consumer-side interface segregation). But the name collision (`LLMConfig` in three packages) means the next developer searching for "which LLMConfig interface does this code use?" needs to check import paths carefully. This is pre-existing and not worsened by this change; just noting for context.

### 5. Test coverage is thorough and well-organized

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/userconfig/userconfig_test.go:1050-1225`
**Severity**: (Positive observation)

The test suite covers:
- Default value (nil pointer returns "")
- Set valid value ("cpu")
- Clear with empty string (resets to nil)
- Invalid values (including case variants like "CPU", "GPU")
- Case-insensitive key handling
- TOML round-trip (save and load)
- TOML round-trip for unset value
- Load from file
- AvailableKeys includes the new key

Test names accurately describe what they test. The `mockLLMConfig` in `factory_test.go` was updated to include the `backend` field and `LLMBackend()` method, keeping the mock in sync with the interface.

### 6. Error message is actionable

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/userconfig/userconfig.go:410`
**Severity**: (Positive observation)

The error for invalid backend values -- `"invalid value for llm.backend: must be one of: cpu"` -- dynamically lists valid values from `validLLMBackends`. When new backends are added to the slice, the error message updates automatically. This avoids stale error messages.

## Overall Assessment

This is a clean, well-scoped change. The implementation follows the established patterns exactly: `*string` for optional fields, `Get()`/`Set()` dispatch in the switch statement, `AvailableKeys()` registration, and matching test coverage. The `validLLMBackends` slice is a good design choice for extensibility.

No blocking findings. The two advisory items worth considering are: (1) adding a one-line comment that backend values are case-sensitive, since the key handling is case-insensitive and the asymmetry could surprise someone; and (2) the `LLMBackend()` method on the interface is unused today, which is expected per the design doc but could benefit from a brief comment referencing issue #1778.
