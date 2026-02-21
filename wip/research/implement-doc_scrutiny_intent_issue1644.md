# Scrutiny Review: Intent -- Issue #1644

**Issue**: #1644 test(llm): add end-to-end integration test without cloud keys
**Focus**: intent
**Reviewer**: maintainer-reviewer (scrutiny agent)

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

## Sub-check 1: Design Intent Alignment

### Design doc description for #1644

From the implementation issues table:

> _Test creates recipe using local inference with no cloud API keys configured. Validates full flow from factory fallback to recipe output._

From Phase 5 (Testing and Quality Validation):

> End-to-end test: `tsuku create` with addon, no cloud keys

From the Data Flow section ("First-time user"):

> Recipe generated (3-5 turns), validated in sandbox, written to local recipes

### What the implementation actually does

The test file (`internal/llm/local_e2e_test.go`) contains two tests:

1. **TestE2E_FactoryFallbackToLocal**: Verifies factory behavior with no cloud API keys. Creates factory with `WithLocalEnabled(true)` and `WithPrompter(&addon.AutoApprovePrompter{})`, checks it has exactly 1 provider named "local". This test does NOT require the addon binary -- it only tests factory registration logic.

2. **TestE2E_CreateWithLocalProvider**: Full end-to-end test. Clears cloud API keys, creates factory, gets provider, sends a `CompletionRequest` with recipe generation prompts for `jq`. Validates:
   - `Complete()` succeeds
   - Response contains at least one tool call
   - Each tool call name is one of the defined tools (fetch_file, inspect_archive, extract_pattern)
   - If extract_pattern is returned, validates its structure (mappings array with os/arch/format/asset, executable, verify_command, platform coverage for linux/amd64 and darwin/arm64)

### Intent alignment assessment

**The "TOML validation" deviation is aligned with design intent. Advisory.**

The design doc describes the test as validating "full flow from factory fallback to recipe output." The implementation validates from factory fallback through to the Provider.Complete() response, including tool call structure. The gap between "tool call structure" and "recipe output" is real but narrow: the tool call IS the structured recipe data (extract_pattern contains the platform mappings, executable name, and verify command that become the TOML recipe). The test validates the semantic content of extract_pattern, not just that it is valid JSON.

The design doc's Phase 5 description says "run tsuku's existing recipe test suite against local models" as a separate bullet from "end-to-end test." The E2E test was never intended to duplicate the recipe test suite. The test validates that local inference produces the right structured data; the existing recipe test suite (in #1641's benchmark tests) validates that the data produces valid recipes.

**The test correctly exercises the design's "first-time user" data flow. No finding.**

The test follows the first-time user flow from the design doc: no cloud API keys -> factory falls through to LocalProvider -> LocalProvider.Complete() sends inference requests -> response contains structured tool calls. The only steps not covered are sandbox validation and TOML serialization, which are downstream of the Provider interface and tested elsewhere.

**The "CI workflow" deviation is aligned with the issue's scope. No finding.**

The design doc's implementation issues table marks #1644 as "testable" tier, not "critical." The test plan (scenario 13) describes the CI execution command but doesn't list a dedicated CI workflow as a hard requirement. The build tag `e2e` ensures the test doesn't run in normal CI, which is the expected behavior for tests requiring hardware.

### Backward coherence

The test reuses patterns established in prior issues:
- `AutoApprovePrompter{}` from #1642
- `IdleTimeoutEnvVar` from #1630
- `buildToolDefs()` from the tools package (pre-existing)
- Factory options pattern from #1632

The test's approach of validating at the tool call level (not TOML level) is consistent with how #1641's benchmark suite validates quality -- it also checks tool call structure and repair attempts rather than TOML output.

No contradictions with prior issues found.

## Sub-check 2: Cross-Issue Enablement

#1644 has no downstream issues in the dependency graph. The only other pending issue is #1645 (docs), which depends on #1632 and #1636, not #1644. Skipped per instructions.

## Findings

### Finding 1: TOML validation deviation is design-aligned (Advisory)

**AC**: "TOML validation"
**Claimed status**: deviated
**Assessment**: The deviation is honest and the alternative (validating tool call structure) captures the design's intent. The extract_pattern tool call contains the same data that becomes TOML -- the test validates the semantic content (platform mappings, executable, verify_command) rather than the serialization format. This is a reasonable boundary for a Provider-level E2E test.

**Severity**: Advisory. The design doc's description says "recipe output" which could mean TOML, but in context it refers to the Provider's output (tool calls with recipe data). The test validates the substance of that output.

### Finding 2: CI workflow deferral is proportionate (Advisory)

**AC**: "CI workflow"
**Claimed status**: deviated
**Assessment**: The build tag `e2e` already prevents accidental execution in normal CI. A dedicated CI workflow would require hardware (GPU or large CPU) that most CI runners don't have. Deferring this is reasonable for a "testable" tier issue.

**Severity**: Advisory. The test is runnable manually with clear instructions in the test comment (line 57).

## Summary

| Level | Count |
|-------|-------|
| Blocking | 0 |
| Advisory | 2 |
