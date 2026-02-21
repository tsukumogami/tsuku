# Test Plan: Local LLM Runtime (M3: Production Ready)

Generated from: docs/designs/DESIGN-local-llm-runtime.md
Issues covered: 5
Total scenarios: 16

---

## Scenario 1: Benchmark suite runs against local provider
**ID**: scenario-1
**Testable after**: #1641
**Category**: infrastructure
**Commands**:
- `TSUKU_LLM_BINARY=$TSUKU_HOME/tools/tsuku-llm-*/bin/tsuku-llm go test -v -run TestLLMGroundTruth ./internal/builders/ -count=1`
**Expected**: Test completes without panics, produces pass/fail results for all 18 test matrix cases, and results are serialized per the existing baseline format in `testdata/llm-quality-baselines/local.json`.
**Status**: pending

---

## Scenario 2: Benchmark detects regressions against saved baseline
**ID**: scenario-2
**Testable after**: #1641
**Category**: infrastructure
**Commands**:
- `go test -v -run TestLLMGroundTruth ./internal/builders/ -count=1`
**Expected**: When a test case that previously passed now fails, the test output contains "Quality regressions detected". When a previously failing case now passes, the output contains "Improvements". The `-update-baseline` flag writes a new baseline file that includes all current results.
**Status**: pending

---

## Scenario 3: Benchmark compares local vs cloud provider quality
**ID**: scenario-3
**Testable after**: #1641
**Category**: use-case
**Environment**: manual -- requires tsuku-llm addon running with GPU, and ANTHROPIC_API_KEY or GOOGLE_API_KEY set
**Commands**:
- `TSUKU_LLM_BINARY=/path/to/tsuku-llm go test -v -run TestLLMGroundTruth ./internal/builders/ -count=1 -update-baseline`
- `ANTHROPIC_API_KEY=sk-... go test -v -run TestLLMGroundTruth ./internal/builders/ -count=1 -update-baseline`
- Compare `testdata/llm-quality-baselines/local.json` vs `testdata/llm-quality-baselines/claude.json`
**Expected**: Local provider baseline shows pass rate within 10% of Claude baseline. Both baselines exist and contain results for all 18 test matrix cases. The local baseline is writable (requires >= 50% pass rate per `writeBaseline` threshold).
**Status**: pending

---

## Scenario 4: Benchmark records latency metrics per hardware profile
**ID**: scenario-4
**Testable after**: #1641
**Category**: use-case
**Environment**: manual -- requires running on hardware with known GPU profile
**Commands**:
- `TSUKU_LLM_BINARY=/path/to/tsuku-llm go test -v -run TestRecipeQualityBenchmark ./internal/llm/ -count=1`
**Expected**: Test output logs latency per test case (p50, p99). Metrics are exportable to JSON. Hardware profile (GPU type, VRAM, model selected) is logged at test start. Latency falls within documented expectations: GPU inference <10s/turn, CPU inference <60s/turn.
**Status**: pending

---

## Scenario 5: Permission prompt before addon download
**ID**: scenario-5
**Testable after**: #1642
**Category**: infrastructure
**Commands**:
- Build tsuku-test with `make build-test`
- Run in isolated environment with no addon installed: `tsuku create test-tool --from github:cli/cli`
- Pipe "n" to stdin to decline download
**Expected**: User sees a prompt mentioning the download size (approximately 50MB). Responding "n" or declining aborts the operation with a clear message (not a panic or cryptic error). No addon binary is downloaded. The exit code is non-zero.
**Status**: pending

---

## Scenario 6: Permission prompt before model download
**ID**: scenario-6
**Testable after**: #1642
**Category**: use-case
**Environment**: manual -- requires addon installed but no model downloaded
**Commands**:
- `tsuku create test-tool --from github:cli/cli`
- Observe stderr for model download prompt
**Expected**: User sees a prompt mentioning model name and size (e.g., "qwen2.5-3b-instruct-q4 (2.5 GB)"). Default is "Y" (continue). Pressing Enter without input accepts the download. Typing "n" aborts with a clear message. Progress bar appears during download showing bytes downloaded / total.
**Status**: pending

---

## Scenario 7: Progress bars during downloads
**ID**: scenario-7
**Testable after**: #1642
**Category**: use-case
**Environment**: manual -- requires network access for actual downloads
**Commands**:
- `tsuku create test-tool --from github:cli/cli --yes`
- Observe stderr during addon and model downloads
**Expected**: Progress bars show during both addon and model downloads. Each bar shows current bytes, total bytes, and percentage. After download completes, progress line is replaced with completion message. Non-interactive mode (piped stdin) skips prompts when --yes is passed but still shows progress.
**Status**: pending

---

## Scenario 8: Spinner during inference
**ID**: scenario-8
**Testable after**: #1642
**Category**: use-case
**Environment**: manual -- requires full local LLM stack running
**Commands**:
- `tsuku create jq --from github:jqlang/jq --yes`
- Observe stderr during inference turns
**Expected**: A spinner or status message appears during inference (e.g., "Generating recipe..." or "Analyzing release assets..."). The spinner updates between inference turns. Once generation completes, the spinner stops and recipe output is shown.
**Status**: pending

---

## Scenario 9: `tsuku llm download` with no prior installation
**ID**: scenario-9
**Testable after**: #1643
**Category**: infrastructure
**Commands**:
- `tsuku llm download`
**Expected**: Command downloads addon binary and model. Output shows hardware detection results (GPU type, VRAM, selected model). Progress bars appear during download. Exit code 0. Files exist at `$TSUKU_HOME/tools/tsuku-llm-*/bin/tsuku-llm` and `$TSUKU_HOME/models/*.gguf` after completion.
**Status**: pending

---

## Scenario 10: `tsuku llm download` skips when already present
**ID**: scenario-10
**Testable after**: #1643
**Category**: infrastructure
**Commands**:
- `tsuku llm download` (first run to install)
- `tsuku llm download` (second run)
**Expected**: Second run completes quickly without re-downloading. Output indicates addon and model are already present. Exit code 0 on both runs.
**Status**: pending

---

## Scenario 11: `tsuku llm download --force` re-downloads
**ID**: scenario-11
**Testable after**: #1643
**Category**: infrastructure
**Commands**:
- `tsuku llm download` (first run)
- `tsuku llm download --force`
**Expected**: The --force flag triggers re-download of both addon and model even when they already exist. Progress bars appear during download. Exit code 0.
**Status**: pending

---

## Scenario 12: `tsuku llm download --model` overrides selection
**ID**: scenario-12
**Testable after**: #1643
**Category**: infrastructure
**Commands**:
- `tsuku llm download --model qwen2.5-0.5b-instruct-q4`
**Expected**: Downloads the specified model instead of the hardware-auto-selected one. Output confirms the override ("Using specified model: qwen2.5-0.5b-instruct-q4"). Invalid model names produce exit code 2 with an error listing valid model names.
**Status**: pending

---

## Scenario 13: E2E recipe generation without cloud keys
**ID**: scenario-13
**Testable after**: #1644
**Category**: use-case
**Environment**: manual -- requires tsuku-llm addon + model installed, GPU recommended
**Commands**:
- `unset ANTHROPIC_API_KEY; unset GOOGLE_API_KEY; unset GEMINI_API_KEY`
- `go test -v -run TestE2E_CreateWithLocalProvider ./internal/llm/ -tags=e2e -count=1`
**Expected**: Test verifies factory falls through to LocalProvider when no cloud keys are set. LocalProvider.Complete() succeeds and returns a response with tool calls. The response contains an extract_pattern call with valid mappings (linux/amd64 and darwin/arm64 at minimum). No cloud API calls are made during the test.
**Status**: pending

---

## Scenario 14: Factory fallback to LocalProvider
**ID**: scenario-14
**Testable after**: #1644
**Category**: infrastructure
**Commands**:
- `go test -v -run TestNewFactoryWithLocalProviderOnly ./internal/llm/ -count=1`
**Expected**: With no ANTHROPIC_API_KEY or GOOGLE_API_KEY set, `NewFactory()` succeeds and returns a factory with exactly 1 provider (local). `factory.GetProvider()` returns the local provider. The provider's `Name()` returns "local".
**Status**: pending

---

## Scenario 15: Documentation covers all config options
**ID**: scenario-15
**Testable after**: #1645
**Category**: infrastructure
**Commands**:
- Verify documentation file exists at expected location (e.g., `docs/local-llm.md` or section in README)
- Check file content for all config keys: `local_enabled`, `local_preemptive`, `local_model`, `local_backend`, `idle_timeout`
- Check for `TSUKU_LLM_IDLE_TIMEOUT` env var documentation
- Check for hardware requirements table
- Check for troubleshooting section
**Expected**: Documentation file exists and contains: (1) All five [llm] config options with descriptions and defaults. (2) TSUKU_LLM_IDLE_TIMEOUT env var override. (3) Hardware requirements table with minimum (4GB RAM) and recommended (8GB+ VRAM) specs. (4) Model selection table mapping resources to models. (5) Troubleshooting section covering: server start failure, OOM, slow inference, stale socket cleanup. (6) Uses `$TSUKU_HOME` convention per project CLAUDE.md.
**Status**: pending

---

## Scenario 16: Documentation hardware requirements match code
**ID**: scenario-16
**Testable after**: #1645
**Category**: infrastructure
**Commands**:
- Read documentation hardware table
- Compare against model selection logic in design doc (Section "ModelSelector")
- Verify thresholds match: 8GB+ VRAM -> 3B, 4-8GB VRAM -> 1.5B, CPU 8GB+ RAM -> 1.5B, CPU <8GB RAM -> 0.5B, <4GB RAM -> disabled
**Expected**: Documentation thresholds exactly match the design document's model selection table. No discrepancies between documented hardware requirements and actual selection logic.
**Status**: pending
