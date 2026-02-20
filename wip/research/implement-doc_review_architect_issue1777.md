# Architect Review: Issue #1777 (feat(llm): add llm.backend config key)

## Review Scope

Files changed:
- `internal/userconfig/userconfig.go`
- `internal/userconfig/userconfig_test.go`
- `internal/llm/factory.go`
- `internal/llm/factory_test.go`

## Summary

The change fits the existing architecture cleanly. No blocking findings.

## Architectural Analysis

### Pattern Compliance

**Config key registration**: The `llm.backend` key follows the exact pattern established by every other LLM config key in `userconfig.go`. The field uses `*string` with `toml:"backend,omitempty"` (matching `*bool` for `Enabled`, `*float64` for `DailyBudget`, etc.), has a corresponding `LLMBackend()` accessor, and is wired into `Get()`, `Set()`, and `AvailableKeys()`. No parallel pattern introduced.

**Validation approach**: The `validLLMBackends` slice at `userconfig.go:88` provides a single source of truth for valid values, with `Set()` iterating over it at `userconfig.go:404`. This is a good extension point -- adding future backend overrides means appending to one slice.

**Interface extension**: `LLMBackend() string` was added to the `LLMConfig` interface in `internal/llm/factory.go:27`. This is the right interface to extend because the design doc specifies that `LLMBackend()` will be consumed by the addon lifecycle code (issue #1778), which lives in `internal/llm/`. The `internal/llm` package's `LLMConfig` is the interface that the factory and addon code depend on.

### Multiple LLMConfig Interfaces

The codebase has three separate `LLMConfig` interfaces:

1. `internal/llm/factory.go:22` -- now includes `LLMBackend() string`
2. `internal/builders/builder.go:30` -- does NOT include `LLMBackend()`
3. `internal/discover/llm_discovery.go:53` -- does NOT include `LLMBackend()`

This is not a problem. Each interface declares only the methods its package needs. `userconfig.Config` satisfies all three via Go's structural typing because it has all the methods. The builders package doesn't need backend info (it handles recipe creation, not addon binary selection). The discover package doesn't need it either (LLM discovery is about tool name resolution). Only the `internal/llm` package needs `LLMBackend()` for the upcoming addon-to-recipe migration (#1778). This respects the interface segregation pattern already established in the codebase.

### Dependency Direction

The dependency flow is correct:
- `internal/userconfig` (lower level) owns the config struct and `LLMBackend()` method
- `internal/llm` (higher level) defines the `LLMConfig` interface that declares `LLMBackend()`
- `cmd/tsuku/create.go` (top level) wires `*userconfig.Config` into `llm.WithConfig()`

No circular dependencies. No lower-level package importing a higher-level one.

### Design Doc Alignment

The design doc (#1777 description in DESIGN-gpu-backend-selection.md, line 57-58) specifies:
> Registers `llm.backend` in userconfig with `cpu` as the only valid override value. Adds `LLMBackend()` to the `LLMConfig` interface. Independent of GPU detection, can start in parallel.

The implementation matches exactly:
- `validLLMBackends = []string{"cpu"}` -- only `cpu` is valid
- `LLMBackend() string` added to `internal/llm/factory.go`'s `LLMConfig` interface
- Empty string (nil pointer) means auto-detect; `"cpu"` forces CPU variant
- No dependency on GPU detection code (issues #1773-#1775)

### Consumer Readiness

The `LLMBackend()` method is defined but not yet consumed by any production code path (the factory's `WithConfig` stores the config but doesn't read `LLMBackend()`). This is expected and correct. Issue #1778 (migrate addon to recipe system) is the consumer, and it depends on this issue. The field isn't dead weight; it has a planned consumer in the next issue of the dependency chain.

### Test Coverage

The `mockLLMConfig` in `factory_test.go:425` was updated with a `backend` field and `LLMBackend()` method, keeping the mock in sync with the interface. The userconfig tests cover: default (nil/empty), set to "cpu", clear with empty string, invalid values, case-insensitive key, TOML round-trip for both set and unset states, and loading from file. The test suite is structurally complete for the config layer.

## Findings

No blocking findings. No advisory findings.

The implementation is a clean, minimal addition that follows every existing pattern in the `userconfig` and `llm` packages. The `*string` type for distinguishing unset from empty, the validation slice for extensibility, the interface-segregated `LLMConfig`, and the dependency direction are all consistent with the established architecture.
