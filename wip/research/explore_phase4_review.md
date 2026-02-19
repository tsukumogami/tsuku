# Architect Review: DESIGN-llm-testing-strategy.md

## Review Scope

Evaluating problem statement specificity, alternative completeness, rejection fairness, unstated assumptions, and strawman risk for the three decisions in the LLM testing strategy design.

---

## 1. Problem Statement Assessment

**Verdict: Specific enough, with one gap.**

The problem statement is grounded in concrete evidence: two failure categories, specific test case names (fly, trivy, ast-grep), measured pass rate (80% on 10 cases), and traceable root causes. It identifies two infrastructure gaps (no local quality testing, no stability testing under realistic workloads) and links them to existing code (`TestLLMGroundTruth`, `lifecycle_integration_test.go`). This is evaluable -- each proposed solution can be checked against whether it catches these specific failures.

**Gap:** The problem statement says the test suite "only works with Claude via `ANTHROPIC_API_KEY`" and attributes this to "the builder initialization is hardcoded to use `NewGitHubReleaseBuilder()` and `NewHomebrewBuilder()` which pick the provider through the factory." I verified this against the codebase. The test at `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/builders/llm_integration_test.go:58-59` skips if `ANTHROPIC_API_KEY` is not set, and the builders are constructed without a `WithFactory` option. But the real coupling point isn't the builders -- it's the factory auto-detection inside `NewSession()`. The builders call the factory, which checks env vars. The design should clarify that the fix is at the factory/session level (passing an explicit provider), not at the builder constructor level. This matters because implementers might try to change builder construction when the actual change needs to happen in how the factory is parameterized for tests.

---

## 2. Missing Alternatives

### Decision 1 (Quality Test Architecture)

No significant missing alternatives. The four options span the realistic design space: shared parameterized suite, per-provider suites, aggregate threshold, snapshot comparison. The chosen approach (parameterized suite with per-provider baselines) is the standard pattern for multi-backend test matrices.

**Minor gap:** The design doesn't consider a "baseline-free" variant where tests are purely structural (does the recipe have the right action type, correct keys in mappings) without tracking pass/fail history. The existing `validateGitHubRecipe` in `llm_integration_test.go` already does structural validation -- it checks action type, archive format, and mapping keys. The design could acknowledge that the structural checks already exist and that baselines add regression detection on top of them, rather than framing this as entirely new infrastructure.

### Decision 2 (Server Stability Testing)

**Missing alternative: Timeout-based crash detection in CI.** The design frames the choice as "full-length reproduction (20+ minutes)" vs "short sequential tests (5 minutes)." There's a middle option: run the known crash-prone test cases (fly, trivy) with a shorter timeout (e.g., 3 minutes each) that catches early-stage failures without waiting for the full 589-990 second crash point. If the server dies at 589 seconds, a 180-second test won't catch it -- but it would catch regressions that move the crash point earlier. This is worth mentioning even if rejected.

**Missing alternative: Watchdog-based memory monitoring.** The design proposes monitoring memory via `/proc/<pid>/status` in the manual runbook. An automated variant could run a goroutine that checks VmRSS every N seconds during CI stability tests and fails if growth exceeds a threshold. This sits between "full manual monitoring" and "cgroups enforcement" in complexity. Worth mentioning in the runbook even if not implemented immediately.

### Decision 3 (CI Integration)

No significant missing alternatives. The tiered approach with path-based triggering is the standard CI pattern for expensive test suites.

---

## 3. Rejection Rationale Fairness

### Decision 1: Rejections are fair

- **Separate suites per provider**: Correctly identifies duplication and three-place maintenance. Fair rejection.
- **Single pass/fail threshold**: The example ("5% drop could mean one hard case or five easy cases") is concrete and accurate. Fair rejection.
- **Snapshot-based comparison**: The non-determinism argument is accurate for LLM output. Even temperature 0 with llama.cpp produces platform-dependent differences due to floating-point ordering. Fair rejection.

### Decision 2: One rejection is slightly unfair

- **Full-length crash reproduction in CI**: Fair. 20+ minutes per test is impractical.
- **Rust-side stress tests**: The rejection says "there are currently zero Rust integration tests, and the infrastructure cost of adding them is high." This is accurate per codebase verification. But the argument "crashes happen through the full stack" is slightly unfair -- the design itself acknowledges (in Uncertainties) that the root cause may be in llama.cpp, the Rust wrapper, or gRPC. If the root cause is in llama.cpp's KV cache management, a Rust-side test that exercises llama.cpp directly would be the most targeted approach. The rejection is still reasonable (high infrastructure cost is real), but the reasoning should acknowledge that Rust tests could be valuable for isolating root causes even though they don't test the full stack.
- **Memory limit enforcement**: Fair. Environment-specific and doesn't diagnose root cause.

### Decision 3: Rejections are fair

- **Run quality in every LLM CI job**: Fair. Time cost is real.
- **Nightly only**: Fair. The delay-to-detection argument is sound.
- **Mock-based quality approximation**: Fair. Defeats the purpose of testing the actual model.

---

## 4. Unstated Assumptions

### A. CI runner has enough RAM for inference (Critical)

The design proposes CI quality tests using the 0.5B model. CPU-only inference on a 0.5B Q4 model needs roughly 500MB for model weights plus context. GitHub Actions runners have 7GB RAM. This should work, but the design doesn't state the RAM requirement explicitly. If the runner is shared with the Rust addon build (which can consume 2-4GB during compilation), peak memory could be an issue. The CI job should build the addon first, then run tests sequentially, not in parallel.

### B. 0.5B model catches prompt-level regressions (Significant)

The design says "CI quality tests use the smallest model (0.5B)" and "CI catches prompt-level regressions but not model-size-specific quality issues." This assumes that prompt changes that break 3B inference also break 0.5B inference. That's not always true. A prompt that adds archive listing context (the ast-grep regression) might overwhelm 0.5B's context handling entirely (producing garbage) while causing a subtler quality regression on 3B (timing out on complex patterns). The 0.5B test might show a different failure mode than what ships in production. The design should be explicit that CI tests with 0.5B are a "canary" -- they catch gross regressions but not all regressions, and the manual runbook with 3B is the authoritative quality check.

### C. The connection recovery fix is small (Moderate)

The design says "the fix is small (invalidate `p.client` on error)." I verified this against `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/llm/local.go:74-77`. The `Complete` method returns the error but doesn't invalidate `p.client`. The fix is indeed small -- add `p.client = nil; p.conn.Close(); p.conn = nil` in the error path. But "small" still means a code change in a production path that needs its own test. The design should either scope this as a prerequisite issue (separate PR, merged before test infrastructure) or explicitly include it in the implementation plan.

### D. Build tag isolation works as expected (Minor)

The design proposes `//go:build llmquality` for quality tests. The existing integration tests use `//go:build integration`. Having two different build tags for LLM tests means running "all LLM tests" requires `-tags=integration,llmquality`. This is workable but should be documented. An alternative is a single `llm` build tag with test file naming conventions to separate fast vs slow tests.

### E. 21 test cases vs 10-case benchmark (Clarification needed)

The problem statement references a "10-case benchmark" with 80% pass rate. The ground truth suite has 21 test cases. These appear to be different things -- the benchmark was likely a manual QA run with a subset. The design should clarify this to avoid confusion about which 10 cases were tested and how they map to the 21-case matrix.

---

## 5. Strawman Analysis

**No options appear designed to fail.** Each alternative is a legitimate approach used in other projects:

- Separate per-provider suites: Used by projects with fundamentally different backend capabilities (e.g., different test vectors per GPU vendor).
- Single threshold: Used by ML evaluation frameworks where per-test tracking is impractical.
- Snapshot comparison: Used extensively for deterministic code generation tools.
- Full-length CI reproduction: Used by projects with dedicated CI hardware budgets.
- Nightly quality runs: Common in ML pipelines where inference cost is high.

The rejections are based on specific properties of this codebase (non-deterministic output, CI time limits, small API surface), not generic dismissals. The alternatives are presented fairly.

---

## 6. Architectural Fit Assessment

From an architecture perspective, the design fits the existing patterns well:

**Extends rather than duplicates.** The provider-parameterized approach reuses `TestLLMGroundTruth`, `llm-test-matrix.json`, and the existing builder infrastructure. No parallel test framework.

**Provider interface respected.** The design routes through the `Provider` interface and `Factory`, not bypassing them with direct LocalProvider calls in tests. The `WithFactory` option already exists on `NewGitHubReleaseBuilder` (used in `repair_loop_test.go`), so parameterizing the provider in tests is a natural extension.

**CI pattern consistent.** The `dorny/paths-filter` change detection in `/home/dangazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/.github/workflows/test.yml` already has an `llm` filter for `tsuku-llm/**` and `internal/llm/**`. Adding a quality job with a different filter set (prompts, matrix, baselines) follows the same pattern.

**One concern: baseline file location.** The design puts baselines in `testdata/llm-quality-baselines/`. The existing test matrix is in `internal/builders/llm-test-matrix.json` (embedded via `//go:embed`). Putting baselines in a different directory (`testdata/`) creates a split where related test configuration lives in two places. Consider putting baselines alongside the test matrix in `internal/builders/` or co-locating them in `testdata/builders/`.

---

## Summary of Findings

| Finding | Severity | Category |
|---------|----------|----------|
| Problem statement should clarify that the fix is at factory/session level, not builder constructor | Advisory | Problem specificity |
| Missing alternative: timeout-bounded crash detection in CI | Advisory | Completeness |
| Missing alternative: automated memory monitoring during stability tests | Advisory | Completeness |
| Unstated: CI runner RAM requirements during combined addon build + inference | Advisory | Assumption |
| Unstated: 0.5B as CI model is a canary, not a complete quality check | Advisory | Assumption |
| Connection recovery fix should be scoped as prerequisite or explicitly planned | Advisory | Assumption |
| 21 test cases vs 10-case benchmark distinction needs clarification | Advisory | Problem specificity |
| Baseline file location splits related config across directories | Advisory | Architectural fit |
| Rust-side stress test rejection reasoning slightly undersells targeted value | Minor | Rejection fairness |
| Build tag proliferation (integration vs llmquality) should be documented | Minor | Assumption |

**No blocking findings.** The design is structurally sound, extends existing infrastructure correctly, and the alternatives analysis is fair. The findings are refinements, not redirections.
