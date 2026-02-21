# Architecture Review: Issue #1641 (test(llm): add quality benchmark suite)

**Review Focus**: Architecture (design patterns, separation of concerns)
**Commit**: 23b96e651d2d48c250482ef12d4b271d15d846ca
**Files Changed**:
- `internal/builders/llm_integration_test.go`
- `internal/builders/baseline_test.go`

## Review Scope

This issue extends the existing `TestLLMGroundTruth` test in `internal/builders/` with three new capabilities: latency metrics collection, repair turn tracking, and a configurable per-test-case timeout. All changes are within test files in the `builders` package.

## Design Alignment

The implementation follows the design doc's Phase 5 ("Testing and Quality Validation") intent. The design doc calls for comparing recipe quality across providers and measuring inference latency. This issue adds the measurement infrastructure (latency percentiles, repair turn statistics) to the existing test framework rather than creating a separate benchmark suite.

The decision to extend `TestLLMGroundTruth` rather than introducing a new test function or package is architecturally sound. The existing test already has the provider detection, factory injection, test matrix, and baseline comparison infrastructure. Adding metrics collection alongside the existing pass/fail tracking avoids duplicating the test harness.

## Pattern Consistency

### Provider interface usage: Correct

The test uses the established provider detection pattern (`detectProvider`) and factory injection (`NewFactoryWithProviders`). Providers are accessed through the `Provider` interface, not instantiated inline. The `BuildSession` interface is used correctly -- `session.Generate(ctx)` returns a `BuildResult` from which `RepairAttempts` is read. No interface bypass.

### Test file organization: Follows existing pattern

New helper functions (`testCaseMetrics`, `percentile`, `parseBenchmarkTimeout`, `logBenchmarkSummary`) are defined in `llm_integration_test.go` alongside the test they serve. Unit tests for the helpers are in `baseline_test.go`, which already tests other helper functions (`writeBaselineToDir`, `compareBaseline`, `loadBaselineFromDir`, `providerModel`). This matches the existing split.

### Dependency direction: Correct

`internal/builders` test files import `internal/llm` and `internal/recipe`. This is the expected direction: builders is a higher-level package that depends on the llm provider interface. No reverse dependencies introduced.

## Findings

### Finding 1: No parallel patterns introduced

**Severity**: Not a finding (positive observation)

The metrics collection reuses the existing `BuildResult.RepairAttempts` field (defined in `internal/builders/builder.go:133-135`) rather than introducing a separate repair tracking mechanism. Latency is measured by wrapping the existing `session.Generate(ctx)` call with `time.Now()/time.Since()` rather than adding timing hooks inside the builder.

### Finding 2: Benchmark utilities stay in test code

**Severity**: Not a finding (positive observation)

`testCaseMetrics`, `percentile()`, `parseBenchmarkTimeout()`, and `logBenchmarkSummary()` are all defined in `_test.go` files and are not exported. This keeps benchmark-specific utilities out of the production code path. If these ever need to be shared with other test packages, they could move to a testutil package, but for now the containment is appropriate.

### Finding 3: Environment variable naming convention

**Severity**: Advisory

The new `LLM_BENCHMARK_TIMEOUT` environment variable uses a different naming convention than the existing `TSUKU_LLM_IDLE_TIMEOUT` (from the design doc). The existing convention prefixes with `TSUKU_` for tsuku-specific env vars. However, `LLM_BENCHMARK_TIMEOUT` is test-only infrastructure (not user-facing), and test-specific env vars in the codebase don't consistently use the `TSUKU_` prefix. This is a minor inconsistency that doesn't compound -- no other code will import or reference this env var name.

## Overall Assessment

The changes fit cleanly within the existing architecture. The implementation extends the established `TestLLMGroundTruth` framework rather than introducing parallel test infrastructure. Dependencies flow in the correct direction. The `BuildResult` and `BuildSession` interfaces are consumed correctly. Helper functions are contained in test files. No structural violations found.
