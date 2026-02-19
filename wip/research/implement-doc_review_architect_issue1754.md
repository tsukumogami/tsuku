# Architect Review: Issue #1754

## Findings

### 1. Factory injection matches existing patterns - NO FINDING

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/llm_integration_test.go:274-279` -- The test creates a factory via `NewFactoryWithProviders` with `WithPrimaryProvider` and injects it into builders using `WithFactory(factory)` and `WithHomebrewFactory(factory)`. This is the same pattern used in `github_release_test.go` (e.g., `createMockFactory` at line 53) and `repair_loop_test.go` (lines 126, 214, 284). No bypass of the factory abstraction.

### 2. Provider detection duplicates factory auto-detection logic - ADVISORY

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/llm_integration_test.go:55-83` -- `detectProvider` manually checks env vars and instantiates providers, which partially overlaps with `NewFactory` in `internal/llm/factory.go:125-179`. The production `NewFactory` checks the same env vars (`ANTHROPIC_API_KEY`, `GOOGLE_API_KEY`) and calls the same constructors (`NewClaudeProvider`, `NewGeminiProvider`, `NewLocalProvider`).

However, this is a deliberate design choice documented in the design doc: the test needs to detect exactly one provider for baseline keying, while `NewFactory` registers all available providers. The test also inverts the priority order -- local first (`TSUKU_LLM_BINARY`) vs production's cloud-first default. The duplication is contained to a single test helper and doesn't create a second provider-registration path that production code could call. Not blocking because the divergence is intentional and the function is test-only.

### 3. Baseline logic is well-layered as pure functions - NO FINDING

`compareBaseline` (line 177) is a pure function: `(baseline, results) -> baselineDiff`. `writeBaselineToDir` and `loadBaselineFromDir` accept directory parameters, making them testable without filesystem tricks. `reportRegressions` is the only function that touches `*testing.T`. The unit tests in `baseline_test.go` exercise the pure functions directly. This decomposition matches Go test conventions (pure logic separate from test harness).

### 4. Dependency direction is correct - NO FINDING

`internal/builders` imports `internal/llm` and `internal/recipe`. Both are lower-level packages. `baseline_test.go` imports only stdlib packages. No circular dependencies introduced. The test file in `internal/builders` uses `llm.Provider`, `llm.NewFactoryWithProviders`, `llm.Model`, and `llm.GeminiModel` -- all public API from the `llm` package. Dependencies flow downward.

### 5. providerModel switch statement couples test code to provider constants - ADVISORY

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/llm_integration_test.go:229-240` -- `providerModel` maps provider names to model strings using a switch statement that references `llm.Model` and `llm.GeminiModel` constants. If a new provider is added to the system, this function needs a manual update. The `Provider` interface has `Name()` but no `Model()` method, so there's no way to get this generically.

This is test-only code with no other callers. The model string is only used for the baseline JSON metadata (not for test logic). If a provider is added but this switch isn't updated, the baseline file just records the provider name as the model string -- an imperfect label but not a correctness issue. Not blocking because it's contained and the consequence is cosmetic.

### 6. No parallel pattern introduction for baseline storage - NO FINDING

The baseline files live in `testdata/llm-quality-baselines/`, which follows Go's `testdata/` convention. The JSON format is simple and purpose-built. No competing config parser, no new serialization format that duplicates an existing one. The `-update-baseline` flag follows Go's golden file convention (see `go help testflag` for precedent with `-update`).

### 7. Test matrix ordering hardcoded separately from matrix definition - ADVISORY

`/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/llm_integration_test.go:298-306` -- `testIDs` is a hardcoded slice of 21 test case IDs that must stay in sync with `llm-test-matrix.json`. If a test case is added to the JSON but not to `testIDs`, it silently won't run. If a test case is removed from JSON but left in `testIDs`, the test reports "not found in matrix" but continues.

This is a deliberate tradeoff: the explicit ordering gives deterministic test execution order (GitHub tests before Homebrew), which matters for LLM cost budgeting and debugging. The "not found" error catches removals. But additions are silent. This is contained to a single test function and doesn't affect production architecture. Not blocking because it's a test maintenance concern, not a structural divergence.

## Summary

Blocking: 0, Advisory: 3

The changes fit the existing architecture well. Provider detection constructs providers through the same `llm.New*Provider` constructors used everywhere else, wraps them in a factory via `NewFactoryWithProviders` (the established test pattern), and injects them through the existing `WithFactory`/`WithHomebrewFactory` options. No factory bypass, no dependency direction violation, no parallel patterns introduced.

The baseline comparison logic is cleanly decomposed into pure functions (`compareBaseline`, `writeBaselineToDir`, `loadBaselineFromDir`) with a thin test-harness wrapper (`reportRegressions`). This layering makes the baseline logic independently testable, as demonstrated by the thorough unit tests in `baseline_test.go`.

The three advisory items are all contained: `detectProvider` intentionally diverges from `NewFactory` for test-specific reasons, `providerModel` has cosmetic-only failure modes, and the hardcoded `testIDs` ordering is a maintenance tradeoff within a single test function.
