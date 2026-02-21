# Scrutiny Review: Completeness - Issue #1644

## Issue: test(llm): add end-to-end integration test without cloud keys

## Requirements Mapping (Untrusted)

--- BEGIN UNTRUSTED REQUIREMENTS MAPPING ---
[
  {"ac":"test file exists","status":"implemented"},
  {"ac":"factory fallback","status":"implemented"},
  {"ac":"generates recipe","status":"implemented"},
  {"ac":"TOML validation","status":"deviated","reason":"validates tool call structure at provider layer instead"},
  {"ac":"skips cleanly","status":"implemented"},
  {"ac":"build tag","status":"implemented"},
  {"ac":"CI workflow","status":"deviated","reason":"optional per issue, deferred"}
]
--- END UNTRUSTED REQUIREMENTS MAPPING ---

## Independent Diff Assessment

The implementation is a single file: `internal/llm/local_e2e_test.go` (277 lines). It contains:

1. Build tag `//go:build e2e` gating the tests
2. `TestE2E_FactoryFallbackToLocal` -- factory-only test clearing cloud keys, asserting local provider
3. `TestE2E_CreateWithLocalProvider` -- full inference test: factory fallback, Complete() call with recipe prompts, tool call validation
4. Helper functions: `findAddonBinary`, `validateExtractPattern`, `recipeGenerationSystemPrompt`, `recipeGenerationUserPrompt`
5. Skip logic: `findAddonBinary` returns empty string -> `t.Skip()`

No other files were changed.

## AC-by-AC Verification

### AC 1: "test file exists" -- Claimed: implemented
**Assessment: CONFIRMED.** File exists at `internal/llm/local_e2e_test.go`.

### AC 2: "factory fallback" -- Claimed: implemented
**Assessment: CONFIRMED.** `TestE2E_FactoryFallbackToLocal` (lines 22-41) clears all three cloud API key env vars, creates factory with `WithLocalEnabled(true)`, asserts provider count is 1, asserts `HasProvider("local")`, and verifies `GetProvider()` returns a provider with `Name() == "local"`.

### AC 3: "generates recipe" -- Claimed: implemented
**Assessment: CONFIRMED.** `TestE2E_CreateWithLocalProvider` (lines 58-154) sends a full `CompletionRequest` with system prompt, user prompt containing jq release assets, and tool definitions. It calls `provider.Complete()` and asserts the response contains tool calls. If the model returns `extract_pattern`, `validateExtractPattern` checks for `mappings`, `executable`, `verify_command` fields and linux/amd64 + darwin/arm64 platform coverage. This validates the full generation flow.

### AC 4: "TOML validation" -- Claimed: deviated, reason: "validates tool call structure at provider layer instead"
**Assessment: CONFIRMED deviation is reasonable.** The test validates tool call structure (lines 132-147): it checks that returned tool names are in the valid set (`fetch_file`, `inspect_archive`, `extract_pattern`) and that `extract_pattern` calls have correct field structure. This validates structured output at the provider layer rather than parsing TOML output. Since the local provider uses gRPC+GBNF grammar to constrain output to tool calls (not raw TOML), validating tool call structure is the correct level of abstraction. The AC text "TOML validation" likely referred to validating the recipe output format, and tool call structure validation is the equivalent check for this architecture.

### AC 5: "skips cleanly" -- Claimed: implemented
**Assessment: CONFIRMED.** `findAddonBinary` (lines 157-196) returns empty string if no addon found. Line 62-63: `if addonPath == "" { t.Skip(...) }`. The skip message is descriptive. `TestE2E_FactoryFallbackToLocal` does not require the addon binary (it only tests factory registration, not inference), so it doesn't skip -- this is correct behavior since factory fallback can be tested without the addon.

### AC 6: "build tag" -- Claimed: implemented
**Assessment: CONFIRMED.** Line 1: `//go:build e2e`. Tests won't run in normal `go test ./...`.

### AC 7: "CI workflow" -- Claimed: deviated, reason: "optional per issue, deferred"
**Assessment: CONFIRMED deviation is acceptable.** No CI workflow file was added. The AC coverage warnings from the orchestrator say "none", and the issue description in the design doc says "Test creates recipe using local inference with no cloud API keys configured. Validates full flow from factory fallback to recipe output." -- the design doc description doesn't mention CI workflow as a core deliverable. E2E tests requiring a running addon with GPU inference are not trivially CI-able. Deferral is reasonable.

## Missing ACs

None identified. The mapping covers the ACs provided.

## Phantom ACs

None identified. All mapping entries correspond to plausible ACs for an E2E test issue.

## Evidence Quality

All "implemented" claims have direct code evidence in the single changed file. References to specific functions and line numbers check out against the actual file contents.

## Summary

No blocking findings. All "implemented" claims are verified against the code. Both deviations are reasonable: TOML validation was replaced with equivalent tool call structure validation (correct for the gRPC/GBNF architecture), and CI workflow deferral is sensible for an E2E test requiring hardware inference. The implementation is well-scoped to the issue's purpose.
