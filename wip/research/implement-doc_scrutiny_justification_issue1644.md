# Scrutiny Review: Justification -- Issue #1644

**Issue**: #1644 test(llm): add end-to-end integration test without cloud keys
**Focus**: justification (quality of deviation explanations)
**Reviewer**: architect-reviewer

## Deviations Under Review

### Deviation 1: "TOML validation" -- "validates tool call structure at provider layer instead"

**AC claim**: TOML validation
**Claimed status**: deviated
**Claimed reason**: "validates tool call structure at provider layer instead"

**Assessment**: Advisory

The issue description says "Validates full flow from factory fallback to recipe output." The test plan scenario 13 specifies: "The response contains an extract_pattern call with valid mappings." The implementation validates tool call structure (tool name is one of the defined tools, extract_pattern has required fields like mappings/executable/verify_command, platform coverage includes linux/amd64 and darwin/arm64).

The deviation reason is genuine: the test validates at the provider response layer rather than at the TOML output layer. This is a meaningful shift -- the test verifies that the local LLM returns structured tool calls with correct fields, not that those tool calls can be converted to a valid TOML recipe. The design doc's Phase 5 says "End-to-end test: `tsuku create` with addon, no cloud keys," which implies the full pipeline through to recipe file output.

However, the reason has substance. The `validateExtractPattern` function (lines 200-241 of `local_e2e_test.go`) checks the specific fields and platform coverage that matter for recipe generation. The tool call arguments mirror the recipe structure closely. The gap between "validated tool call JSON" and "valid TOML file" is bridged by existing code that converts tool call results to TOML -- code already tested elsewhere. Testing TOML serialization again here would test the converter, not the local LLM.

The explanation could be stronger: it should note that the TOML conversion from tool call output is already tested in the builder layer, so re-testing it here would test the serializer, not the provider. But the core trade-off is real.

**Severity**: Advisory. The deviation is genuine, not a shortcut disguise. The tool call structure validation covers the LLM-specific concern (does the model produce correct structured output). TOML conversion is a separate, already-tested concern.

### Deviation 2: "CI workflow" -- "optional per issue, deferred"

**AC claim**: CI workflow
**Claimed status**: deviated
**Claimed reason**: "optional per issue, deferred"

**Assessment**: Advisory (borderline)

The reason "optional per issue, deferred" is thin. It doesn't explain what makes it optional, or when it will be addressed. This matches the avoidance pattern of "can be added later."

However, the context makes the deferral reasonable. The e2e test requires a running tsuku-llm addon with a loaded model -- hardware that CI runners don't have. A CI workflow for this test would need either self-hosted runners with GPU, or a way to run with CPU inference (slow but possible). No existing CI infrastructure supports this. The `e2e` build tag already ensures the test is excluded from standard CI runs (`go test ./...` won't trigger it).

The explanation should have stated: "CI runners lack the tsuku-llm addon and model files; running this test in CI requires self-hosted GPU runners or CPU inference with long timeouts, neither of which exists yet." That would be a genuine technical constraint, not a deferral of convenience.

This issue is the second-to-last in the design doc (#1645 documentation follows). There's no downstream issue that depends on the CI workflow being present. The test itself is manually runnable via `go test -tags=e2e`.

**Severity**: Advisory. The deferral is practically justified (no CI hardware for LLM inference) but the explanation is weak. A stronger reason would reference the infrastructure gap rather than claiming the AC is "optional."

## Proportionality Check

Two deviations out of seven ACs. Both are on peripheral concerns (output format validation and CI automation) rather than core test behavior (factory fallback, recipe generation, skip behavior, build tag). The core ACs are all claimed as implemented and verified in the diff. This ratio is proportional -- the deviations don't suggest selective effort.

## Overall Assessment

Both deviations are genuine trade-offs, not disguised shortcuts. The TOML validation deviation has a real technical rationale (testing the converter vs. testing the LLM output). The CI workflow deviation has a practical justification that the explanation doesn't adequately surface. Neither deviation undermines the issue's primary purpose: proving that local inference works end-to-end without cloud keys.
