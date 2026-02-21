# Pragmatic Review: Issue #1641

**Issue**: #1641 - test(llm): add quality benchmark suite
**Review Focus**: pragmatic (simplicity, YAGNI, KISS)
**Files Changed**: `internal/builders/llm_integration_test.go`, `internal/builders/baseline_test.go`

## Findings

### 1. Advisory: `loadBaselineFromDir` / `writeBaselineToDir` are single-caller testability shims

`internal/builders/llm_integration_test.go:113` -- `loadBaselineFromDir` and `writeBaselineToDir` exist only to let `baseline_test.go` inject a temp directory. The public callers (`loadBaseline`, `writeBaseline`) each delegate to the `*FromDir` variant exactly once.

This is a reasonable testing pattern. The functions are small, named clearly, and the alternative (making `baselineDir()` overridable via a global) would be worse. **Advisory** -- no action needed.

### 2. Advisory: `percentile()` linear interpolation for a logging-only metric

`internal/builders/llm_integration_test.go:207-224` -- The percentile function implements nearest-rank with linear interpolation. This is used only for t.Logf output. A simpler index-based approach without interpolation would produce the same practical value for debugging.

Small and well-tested, so not worth changing. **Advisory.**

### 3. Advisory: `baselineDiff.Orphaned` field

`internal/builders/llm_integration_test.go:239` -- The `Orphaned` field in `baselineDiff` catches renamed/removed test cases. This is useful for preventing silent drift between baselines and the test matrix. Tested in `TestReportRegressions_OrphanedDetected`. Not over-engineering. **Advisory** -- this is fine.

## No Blocking Findings

The implementation is the simplest correct approach for the three requirements (latency metrics, repair tracking, configurable timeout):

- Metrics are logged via `t.Logf`, not a separate JSON export system. Correct choice -- the test plan mentions JSON export but the actual AC only asks for metrics.
- Repair tracking reads `BuildResult.RepairAttempts` directly. No abstraction layer.
- Timeout uses `strconv.Atoi` on a single env var. No config struct, no duration parsing library.
- No new files beyond `baseline_test.go` for unit tests of the helper functions. The main logic extends the existing `TestLLMGroundTruth` rather than creating a new test framework.

The `testCaseMetrics` struct and `logBenchmarkSummary` function are the minimum viable instrumentation. Nothing was introduced that serves no current caller.
