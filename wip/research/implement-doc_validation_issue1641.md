# Validation Report: Issue #1641

## Environment

- Platform: Linux 6.17.0-14-generic, amd64
- Go version: as configured in go.mod
- No LLM provider available (TSUKU_LLM_BINARY, ANTHROPIC_API_KEY, GOOGLE_API_KEY all unset)
- No GPU-accessible inference stack in this environment

## Scenario 1: Benchmark suite runs against local provider

**ID**: scenario-1
**Status**: skipped
**Reason**: environment: tsuku-llm addon binary not available; TSUKU_LLM_BINARY not set

**What was validated instead:**
- Package `internal/builders` compiles without errors: `go build ./internal/builders/...` (PASS)
- `go vet ./internal/builders/` reports no issues (PASS)
- Test matrix JSON (`llm-test-matrix.json`) is valid and contains exactly 18 test cases as expected (PASS)
- Test matrix is correctly embedded via `//go:embed` directive (PASS -- compilation succeeds)
- `TestLLMGroundTruth` correctly skips with `-short` flag (PASS)
- `TestLLMGroundTruth` correctly skips when no provider is configured, with clear message: "Skipping LLM integration test: no provider configured (set TSUKU_LLM_BINARY, ANTHROPIC_API_KEY, or GOOGLE_API_KEY)" (PASS)
- Baseline files exist at `testdata/llm-quality-baselines/local.json` and `testdata/llm-quality-baselines/claude.json` (PASS)
- Both baseline files contain results for all 18 test matrix cases (PASS)
- All unit tests for baseline infrastructure pass (PASS -- 15/15 subtests)

**Cannot validate**: Actual LLM inference against local provider, generation of all 18 test matrix results, serialization of results to baseline format from a live run.

---

## Scenario 2: Benchmark detects regressions against saved baseline

**ID**: scenario-2
**Status**: skipped
**Reason**: environment: requires LLM provider to produce current results for regression comparison

**What was validated instead:**
- `TestReportRegressions_NoRegression` -- no regressions when results match baseline (PASS)
- `TestReportRegressions_ImprovementOnly` -- improvements logged but no failure (PASS)
- `TestReportRegressions_OrphanedDetected` -- orphaned entries (test removed/renamed) detected correctly (PASS)
- `TestCompareBaseline_Regressions` -- regression detection when pass->fail (PASS)
- `TestCompareBaseline_Mixed` -- mixed regression + improvement correctly classified (PASS)
- `reportRegressions()` outputs "Quality regressions detected" when regressions exist (verified in code at `llm_integration_test.go:294`)
- `reportRegressions()` outputs "Improvements" when improvements exist (verified in code at `llm_integration_test.go:278`)
- `-update-baseline` flag declared and wired into `TestLLMGroundTruth` (verified in code at `llm_integration_test.go:26,566`)
- `writeBaseline()` enforces 50% minimum pass rate (verified by `TestWriteBaseline_MinimumPassRate` -- PASS, 5/5 subtests)
- `loadBaseline()` handles missing file (nil, no error), valid file (parsed), and invalid JSON (error) (PASS -- 3/3 subtests)

**Cannot validate**: End-to-end regression detection with live inference results against saved baseline.

---

## Scenario 3: Benchmark compares local vs cloud provider quality

**ID**: scenario-3
**Status**: skipped
**Reason**: environment: requires both tsuku-llm addon (GPU) and cloud API keys (ANTHROPIC_API_KEY or GOOGLE_API_KEY)

**What was validated instead:**
- Both `testdata/llm-quality-baselines/local.json` and `testdata/llm-quality-baselines/claude.json` exist and are valid JSON (PASS)
- Both files contain results for all 18 test cases (PASS)
- Local baseline: 18/18 pass. Claude baseline: 17/18 pass (minikube_file_no_mapping is "fail")
- `detectProvider()` correctly prioritizes: TSUKU_LLM_BINARY > ANTHROPIC_API_KEY > GOOGLE_API_KEY (verified in code)
- `providerModel()` returns correct identifiers for all provider types (PASS -- 4/4 subtests)

**Cannot validate**: Running both providers and comparing quality side-by-side.

---

## Scenario 4: Benchmark records latency metrics per hardware profile

**ID**: scenario-4
**Status**: skipped
**Reason**: environment: requires running inference on hardware with known GPU profile

**What was validated instead:**
- `TestPercentile` -- all 7 subtests pass:
  - single value (PASS)
  - p50 of two values (PASS)
  - p0 returns minimum (PASS)
  - p100 returns maximum (PASS)
  - p50 of three values (PASS)
  - p99 skews toward max with interpolation (PASS)
  - empty returns zero (PASS)
- `TestParseBenchmarkTimeout` -- all 6 subtests pass:
  - unset returns 10min default (PASS)
  - valid "300" returns 5min (PASS)
  - invalid "not-a-number" returns default (PASS)
  - "0" returns default (PASS)
  - "-5" returns default (PASS)
  - "60" returns 1min (PASS)
- `logBenchmarkSummary()` logs p50, p99, min, max latencies plus repair turn statistics (verified in code at `llm_integration_test.go:319-369`)
- `testCaseMetrics` struct tracks Latency and RepairAttempts per case (verified in code at `llm_integration_test.go:181-185`)
- Per-case metrics are collected during Generate() and logged in summary (verified in code at `llm_integration_test.go:496-517,563`)

**Cannot validate**: Actual latency measurements against real hardware, GPU detection logging, latency vs hardware profile expectations.

---

## Summary

| Scenario | Status | Unit Tests | Code Structure |
|----------|--------|------------|----------------|
| scenario-1 | skipped (env) | 15/15 pass | valid |
| scenario-2 | skipped (env) | 10/10 pass | valid |
| scenario-3 | skipped (env) | 4/4 pass | valid |
| scenario-4 | skipped (env) | 13/13 pass | valid |

Total unit tests executed: 42 subtests across 13 test functions, all passing.
Code compiles and passes `go vet` without issues.

All four scenarios require an LLM provider (tsuku-llm binary, ANTHROPIC_API_KEY, or GOOGLE_API_KEY) that is not available in this environment. The unit test infrastructure underlying all four scenarios is solid -- every testable component works correctly in isolation. Full end-to-end validation requires a manual run with the appropriate LLM provider configured.
