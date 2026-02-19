# Test Plan: LLM Testing Strategy

Generated from: docs/designs/DESIGN-llm-testing-strategy.md
Issues covered: 7
Total scenarios: 14

---

## Scenario 1: Dead gRPC connection is invalidated after server crash
**ID**: scenario-1
**Testable after**: #1753
**Category**: infrastructure
**Environment**: integration (requires tsuku-llm binary)
**Commands**:
- `go test -tags=integration -run TestCrashRecovery ./internal/llm/...`
**Expected**: Test passes. After SIGKILL, the next `Complete()` call invalidates `p.conn`/`p.client`, restarts the daemon, and returns a successful response. No "transport is closing" or "dead connection" error surfaces to the caller on the second request.
**Status**: pending

---

## Scenario 2: LocalProvider.Complete succeeds after server restart following crash
**ID**: scenario-2
**Testable after**: #1753
**Category**: use-case
**Environment**: integration (requires tsuku-llm binary, model CDN reachable)
**Commands**:
- Build binary: `cd tsuku-llm && cargo build --release`
- `go test -tags=integration -run TestCrashRecovery -v ./internal/llm/...`
**Expected**: After SIGKILL of the daemon mid-session, `LocalProvider.Complete` recovers and returns a non-empty recipe for a simple test prompt (e.g., `stern/stern`). The caller receives a successful `CompletionResponse`, not an error.
**Status**: pending

---

## Scenario 3: TestLLMGroundTruth skips when no provider env var is set
**ID**: scenario-3
**Testable after**: #1754
**Category**: infrastructure
**Commands**:
- `env -i HOME=$HOME PATH=$PATH go test -run TestLLMGroundTruth ./internal/builders/`
**Expected**: Test is skipped (not failed) with a message indicating no provider is configured. Exit code 0 (skip is not a failure in `go test`). The skip message should name what env vars to set.
**Status**: pending

---

## Scenario 4: TestLLMGroundTruth selects local provider when TSUKU_LLM_BINARY is set
**ID**: scenario-4
**Testable after**: #1754
**Category**: infrastructure
**Environment**: integration (requires tsuku-llm binary)
**Commands**:
- `TSUKU_LLM_BINARY=/path/to/tsuku-llm go test -tags=integration -run TestLLMGroundTruth/llm_github_stern -v ./internal/builders/`
**Expected**: Test runs using the local provider (not Claude). Test log output shows "provider: local" or equivalent. No `ANTHROPIC_API_KEY` required.
**Status**: pending

---

## Scenario 5: TestLLMGroundTruth selects Claude provider when only ANTHROPIC_API_KEY is set
**ID**: scenario-5
**Testable after**: #1754
**Category**: infrastructure
**Environment**: environment-dependent (requires ANTHROPIC_API_KEY)
**Commands**:
- `ANTHROPIC_API_KEY=<key> go test -run TestLLMGroundTruth/llm_github_stern -v ./internal/builders/`
**Expected**: Test runs using the Claude provider. Test log output shows "provider: claude" or equivalent. The stern baseline test case passes.
**Status**: pending

---

## Scenario 6: Baseline regression is reported when a passing test starts failing
**ID**: scenario-6
**Testable after**: #1754
**Category**: infrastructure
**Commands**:
- Create a synthetic baseline file with `llm_github_stern_baseline` marked as "pass"
- Inject a mock provider that always returns an incorrect recipe for stern
- `go test -run TestLLMGroundTruth/llm_github_stern -v ./internal/builders/`
**Expected**: Test output includes a regression report naming `llm_github_stern_baseline` as a regression (was "pass", now "fail"). The overall test fails (non-zero exit). Output distinguishes regressions from expected failures.
**Status**: pending

---

## Scenario 7: -update-baseline flag writes new baseline file
**ID**: scenario-7
**Testable after**: #1754
**Category**: infrastructure
**Environment**: integration (requires ANTHROPIC_API_KEY or TSUKU_LLM_BINARY)
**Commands**:
- `ANTHROPIC_API_KEY=<key> go test -run TestLLMGroundTruth -update-baseline -v ./internal/builders/`
- `cat testdata/llm-quality-baselines/claude.json`
**Expected**: `testdata/llm-quality-baselines/claude.json` is created or updated. File contains a JSON object with `provider: "claude"`, `model` field, and a `baselines` map with entries for all 21 test cases. Each entry has value "pass" or "fail".
**Status**: pending

---

## Scenario 8: Local baseline covers all 21 test cases with known failures documented
**ID**: scenario-8
**Testable after**: #1755
**Category**: use-case
**Environment**: integration (requires local model, TSUKU_LLM_BINARY)
**Commands**:
- `cat testdata/llm-quality-baselines/local.json`
- `python3 -c "import json,sys; d=json.load(open('testdata/llm-quality-baselines/local.json')); print(len(d['baselines']), 'entries'); print('ast-grep result:', d['baselines'].get('llm_github_ast-grep_rust_triple'))"`
**Expected**: `local.json` exists with 21 entries under `baselines`. The `llm_github_ast-grep_rust_triple` entry is "fail" (known regression documented in design). File contains `"provider": "local"` and a non-empty `"model"` field. Minimum pass rate sanity check: at least 11/21 entries are "pass" (>50%).
**Status**: pending

---

## Scenario 9: TestSequentialInference completes 3-5 requests through one server instance
**ID**: scenario-9
**Testable after**: #1756
**Category**: use-case
**Environment**: integration (requires tsuku-llm binary, model CDN reachable)
**Commands**:
- `go test -tags=integration -run TestSequentialInference -v -timeout=15m ./internal/llm/...`
**Expected**: All 3-5 sequential inference requests succeed. Server is started once and not restarted between requests. Each request returns a non-empty response. Test passes with no gRPC transport errors.
**Status**: pending

---

## Scenario 10: TestCrashRecovery verifies client reconnects after SIGKILL
**ID**: scenario-10
**Testable after**: #1756
**Category**: use-case
**Environment**: integration (requires tsuku-llm binary, model CDN reachable)
**Commands**:
- `go test -tags=integration -run TestCrashRecovery -v -timeout=10m ./internal/llm/...`
**Expected**: After SIGKILL of the server mid-session, the test verifies that: (1) the immediate request fails or detects the crash, (2) the next `Complete()` call auto-restarts the server, (3) that call eventually succeeds. Test file `internal/llm/stability_test.go` exists with the `integration` build tag.
**Status**: pending

---

## Scenario 11: Model is restored from CI cache rather than re-downloaded
**ID**: scenario-11
**Testable after**: #1757
**Category**: infrastructure
**Environment**: CI (GitHub Actions, not local)
**Commands**:
- Trigger CI on a change to `internal/llm/` with a pre-populated cache
- Check CI job log for "Cache restored" in the model cache step
- Observe that the model download step is skipped or takes <5 seconds
**Expected**: The `actions/cache` step reports a cache hit for the model file keyed by its SHA256 checksum. The model download is skipped. Total time for the model setup step is under 10 seconds (versus 2-5 minutes for a cold download).
**Status**: pending

---

## Scenario 12: llm-quality CI job triggers on prompt template changes but not on lifecycle code changes
**ID**: scenario-12
**Testable after**: #1758
**Category**: infrastructure
**Environment**: CI (GitHub Actions)
**Commands**:
- Submit a PR that modifies only `internal/builders/prompts/` (a prompt template file)
- Submit a PR that modifies only `internal/llm/lifecycle.go` (lifecycle code, no prompt changes)
- Check which CI jobs trigger for each PR
**Expected**: PR 1 (prompt change): `llm-quality` job triggers and runs `TestLLMGroundTruth` with local provider. PR 2 (lifecycle change): `llm-quality` job does NOT trigger (change detection filter excludes it). The `llm-integration` job triggers for PR 2 as before.
**Status**: pending

---

## Scenario 13: llm-quality job runs full ground truth suite with local provider in CI
**ID**: scenario-13
**Testable after**: #1758
**Category**: use-case
**Environment**: CI (GitHub Actions, requires tsuku-llm binary build + model)
**Commands**:
- Trigger `llm-quality` CI job via a PR touching `internal/builders/prompts/`
- Check job log for test results and regression output
**Expected**: `llm-quality` job runs `go test -tags=integration ./internal/builders/...` with `TSUKU_LLM_BINARY` set. All 21 test cases run. Known failures (matching `local.json` baseline) are reported as expected, not as regressions. Job passes unless a previously-passing case now fails.
**Status**: pending

---

## Scenario 14: Manual test runbook procedures are complete and executable
**ID**: scenario-14
**Testable after**: #1759
**Category**: use-case
**Environment**: manual (requires local model, human executor)
**Commands**:
- `cat docs/llm-testing.md`
- Follow "Full benchmark" procedure from the runbook
- Follow "Soak test" procedure from the runbook
**Expected**: `docs/llm-testing.md` exists and contains three procedures: full benchmark (10-case with server restarts), soak test (20+ sequential requests with memory monitoring), and new model validation. Each procedure has numbered steps, specific commands to run, and a results recording template. A developer unfamiliar with the LLM runtime can follow the full benchmark procedure without needing to consult other docs. Commands reference `TSUKU_HOME` not hardcoded paths.
**Status**: pending
