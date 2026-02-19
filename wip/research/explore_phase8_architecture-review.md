# Architecture and Security Review: DESIGN-llm-testing-strategy.md

Reviewer: architect-reviewer
Upstream: DESIGN-local-llm-runtime.md

---

## Architecture Review

### 1. Is the architecture clear enough to implement?

**Verdict: Yes, with two gaps that need clarification before implementation.**

The design is implementable as written. It identifies specific files to change (`internal/builders/llm_integration_test.go`, `internal/llm/stability_test.go`, `internal/llm/local.go`), describes concrete function signatures (`detectProvider`, `loadBaseline`, `reportRegressions`), and shows data flow for both CI and manual paths. The phasing is incremental and each phase produces a testable artifact.

**Gap 1: Factory injection path for HomebrewBuilder differs from GitHubReleaseBuilder.**

The design says the test will "inject via `WithFactory`" but `TestLLMGroundTruth` at `internal/builders/llm_integration_test.go:70-71` constructs both builders without factory injection:

```go
githubBuilder := NewGitHubReleaseBuilder()
homebrewBuilder := NewHomebrewBuilder()
```

The GitHub builder uses `WithFactory(f)` while the Homebrew builder uses `WithHomebrewFactory(f)` -- different option function names. The design's `detectProvider` returns a `(llm.Provider, string)` but the builders take a `*llm.Factory`, not a `Provider` directly. The implementer needs to:

1. Create a `*llm.Factory` wrapping the detected provider (via `NewFactoryWithProviders`).
2. Pass it to *both* builders using their respective option functions.

This is straightforward but the design should mention it explicitly. An implementer who reads "inject via `WithFactory`" might only wire up the GitHub builder and miss the Homebrew path. **Advisory** -- contained to Phase 2 implementation, won't cause structural drift.

**Gap 2: `detectProvider` signature returns `llm.Provider` but test needs `*llm.Factory`.**

The Key Interfaces section shows:

```go
func detectProvider(t *testing.T) (llm.Provider, string)
```

But the builders accept `*llm.Factory`, not `llm.Provider`. The function should either return a factory directly, or the design should show the wrapping step. This is a minor API mismatch in the design pseudocode. **Advisory.**

### 2. Are there missing components or interfaces?

**One missing component: baseline update tooling.**

The design describes baselines as JSON files that are "updated manually when expectations change" but doesn't specify how. After a test run, the implementer needs to know which results to promote to the baseline. The `reportRegressions` function reports diffs, but there's no mechanism to write a new baseline from test results.

The practical workflow is: run the suite, see the results, manually edit JSON. This works but is error-prone -- someone will inevitably get a key wrong or miss a test case. A flag like `go test -run TestLLMGroundTruth -update-baseline` that writes the current results to the baseline file is a common pattern (similar to Go's `-update` flag for golden files).

This isn't blocking -- manual editing works and the baseline files are small. But it's worth noting because the design lists "baseline maintenance" as a negative consequence and this tooling would reduce that cost. **Advisory.**

### 3. Are the implementation phases correctly sequenced?

**Yes. The sequencing is correct and has no circular dependencies.**

Phase 1 (connection recovery) is a prerequisite for Phase 4 (stability tests -- specifically `TestCrashRecovery`). Phases 2-3 (quality suite parameterization) are independent of Phase 4 (stability tests). Phase 5 (CI quality gate) depends on Phases 2-3 being complete. Phase 6 (runbook) has no code dependencies and can be written at any time.

One observation: Phase 2 creates baselines for Claude only, and Phase 3 creates baselines for local. If the implementer wants to validate the baseline comparison logic during Phase 2 (before local baselines exist), they need at least one Claude test run to produce `claude.json`. The design implies this ("run full suite once, record results") but the ordering means you can't test regression detection without a real Claude API run. In CI without `ANTHROPIC_API_KEY`, baseline comparison for Claude will effectively be untestable until Phase 3 adds local baselines. This is fine -- Phase 2 can be validated in development with a Claude key.

### 4. Are there simpler alternatives we overlooked?

**One alternative worth acknowledging: skip baselines, use structural validation only.**

The existing `validateGitHubRecipe` and `validateHomebrewSourceRecipe` functions in `llm_integration_test.go` already do structural validation -- they check action types, archive formats, mapping keys, and patch counts against ground truth recipes. These structural checks catch the most important class of regression: a prompt change that causes the model to produce a structurally different recipe.

The baseline system adds regression tracking on top of structural validation: it records which tests pass/fail per provider and alerts when a previously-passing test starts failing. This is valuable, but it's also the highest-maintenance component of the design.

A simpler alternative: keep structural validation as the quality gate and add a `t.Log` summary of pass/fail counts without formal baselines. Regressions would still be visible in CI logs (a test that starts failing shows up as a failure), just without the "this used to pass" context.

The design's approach is better for the stated goals (detecting quality regressions across releases, comparing providers). But for a team of one or two, the simpler alternative might deliver 80% of the value. This is a scope judgment, not an architectural concern. **Not blocking.**

### 5. Structural fit with existing codebase

**The design fits the existing architecture well in three ways:**

**Provider interface respected.** The design routes through the `Provider` interface and `Factory` for parameterization. It uses the existing `WithFactory`/`WithHomebrewFactory` option pattern for dependency injection, and `NewFactoryWithProviders` for test factory construction. No bypass of the provider dispatch.

**Build tag convention followed.** The existing codebase uses `//go:build integration` for tests that need the addon binary (see `lifecycle_integration_test.go:1`). The design introduces `llmquality` as a new build tag for quality tests. This is an extension of the existing pattern, not a parallel one. The tag name distinguishes "needs addon for lifecycle" from "needs addon for quality testing" -- a valid distinction since quality tests have different CI trigger conditions.

**Test infrastructure reuse.** The stability tests follow the patterns in `lifecycle_integration_test.go`: `startDaemon`, `isDaemonReady`, `isDaemonRunning`, `skipIfModelCDNUnavailable`, `getAddonBinary`. The design explicitly states "uses the existing daemon management patterns." No parallel test framework.

**One structural concern: the `llmquality` build tag creates a split in `llm_integration_test.go`.**

Currently, `TestLLMGroundTruth` in `internal/builders/llm_integration_test.go` has no build tag -- it runs as a regular test gated by the `ANTHROPIC_API_KEY` environment variable check. The design adds the `llmquality` build tag "for local provider execution." This means:

- Claude/Gemini tests: no build tag needed, gated by env vars (existing behavior)
- Local tests: `llmquality` build tag required

This creates an asymmetry: the same `TestLLMGroundTruth` function behaves differently depending on build tags. If `llmquality` is set but no env vars are present, the test detects local provider. If `llmquality` is NOT set, the test falls through to Claude/Gemini detection or skips.

The implementation needs to handle this cleanly. The simplest approach: the `llmquality` build tag gates a separate file that registers a test init or sets a package-level variable. The main test function checks for provider availability regardless of build tag. This avoids conditional compilation within the test function itself.

This is an implementation detail, not an architectural problem. **Advisory.**

---

## Security Review

### 1. Are there attack vectors we haven't considered?

**One unconsidered vector: baseline poisoning via PR.**

The quality baselines in `testdata/llm-quality-baselines/` are JSON files committed to the repository. A malicious PR could modify `local.json` to set all tests to `"fail"` as their expected baseline. This would cause the CI quality gate to pass even when the local model produces zero correct recipes -- because the baseline says failure is expected.

This is a social engineering attack, not a technical one. It requires the reviewer to not notice the baseline change. The design mitigates this somewhat by listing `testdata/llm-quality-baselines/` as a trigger for the quality CI job (changes to baselines trigger quality tests), but the test would pass because the baselines match.

**Mitigation:** Code review is the primary defense. Baseline changes should get the same scrutiny as production code. Consider adding a CI check that rejects baselines where more than N% of tests are marked `"fail"` for any provider -- a sanity bound that catches wholesale baseline degradation. **Advisory** -- this is a process concern, not a code vulnerability.

**No other unconsidered attack vectors.** The design correctly identifies that:
- Model downloads use the same SHA256 verification as production
- Tests run with the same isolation (Unix domain socket, 0600 permissions)
- No new filesystem paths or permissions are introduced
- API keys are handled as GitHub Actions secrets

### 2. Are the mitigations sufficient for the risks identified?

**Yes. The mitigations match the threat model.**

The security section identifies four risks and all mitigations are grounded in existing infrastructure:

| Risk | Mitigation | Assessment |
|------|------------|------------|
| Malicious model in CI | SHA256 verification against embedded manifest | Sufficient -- same trust chain as production |
| Stale socket from SIGKILL test | Existing stale-socket cleanup in ServerLifecycle | Sufficient -- already tested in `TestIntegration_StaleSocketCleanup` |
| API key exposure in CI logs | GitHub Actions secret masking | Sufficient -- standard practice |
| Non-determinism causing flakiness | Per-provider baselines + `skipIfModelCDNUnavailable` | Sufficient -- known failures are expected, network issues cause skips |

The design correctly notes that "the SHA256 verification checks against the manifest embedded in the addon binary, not a remote manifest." This is the critical detail -- a compromised CI environment can't serve a different manifest to match a malicious model because the manifest is baked into the compiled addon binary.

### 3. Is there residual risk we should escalate?

**One residual risk worth documenting explicitly: CI runner compromise allowing model substitution at the filesystem level.**

The design says "if CI is compromised and serves a malicious model, the SHA256 verification would reject it." This is true for network-level attacks (MITM on the model download). But if the CI runner itself is compromised, an attacker could:

1. Wait for the model to download and pass verification
2. Replace the model file on disk after verification but before inference
3. The addon re-verifies before each model load (per the upstream design), so this would be caught

Step 3 closes the gap -- the upstream design specifies re-verification before each model load. The testing design inherits this protection. But it's worth verifying that the test infrastructure doesn't bypass this re-verification step. If stability tests use `startDaemon` directly (which they do), the daemon performs its own verification on startup, so the protection is maintained.

**No escalation needed.** The residual risk is a CI runner compromise, which is outside the threat model for test infrastructure. The re-verification behavior provides defense-in-depth even in this scenario.

### 4. Are any "not applicable" justifications actually applicable?

**The security section doesn't use "not applicable" for any category, which is correct.** All four standard categories (download verification, execution isolation, supply chain, user data) are addressed with specific mitigations.

One implicit "not applicable": the design doesn't discuss **denial of service** against CI resources. The quality tests download a 500MB model and run CPU inference for 10-15 minutes. A malicious PR could modify the test matrix to include hundreds of test cases, exhausting CI runner time. This is mitigated by: (1) the quality job only runs when specific paths change, (2) the test matrix is committed to the repo and reviewable, (3) CI has overall timeout limits. This is a standard CI resource concern, not specific to this design. **Not worth adding to the design.**

### 5. SIGKILL crash-recovery test: process cleanup

The `TestCrashRecovery` test sends SIGKILL to the addon process. This leaves:
- A stale socket file (no SIGTERM handler runs)
- A stale lock file (kernel releases the flock, but the file persists)
- Potentially orphaned child processes if llama.cpp spawns any

The design says "the existing stale-socket cleanup in `ServerLifecycle.EnsureRunning()` handles this case." Verified: `TestIntegration_StaleSocketCleanup` in `lifecycle_integration_test.go:261-279` confirms this path works. The lock file is released by the kernel on process death (flock semantics), so the lock-based daemon detection works correctly after SIGKILL.

One edge case: if the SIGKILL test runs and the test process itself crashes before cleanup, the next test run will find a stale socket. The `ServerLifecycle` handles this, but the test should verify cleanup in a `t.Cleanup` handler rather than relying on the next test to detect stale state. The existing `startDaemon` function already registers a cleanup handler that kills the process, so this is covered. **No issue.**

---

## Summary of Findings

### Blocking

None. The design is structurally sound and fits the existing architecture.

### Advisory

1. **Factory injection asymmetry** (Section 1, Gap 1): The design says "inject via `WithFactory`" but the Homebrew builder uses `WithHomebrewFactory`. Document both injection points to avoid partial implementation.

2. **`detectProvider` return type mismatch** (Section 1, Gap 2): The signature returns `(llm.Provider, string)` but builders need `*llm.Factory`. Show the wrapping step or change the return type.

3. **Baseline update tooling** (Section 2): No mechanism to generate baselines from test results. Consider an `-update-baseline` flag to reduce manual editing errors.

4. **Build tag interaction with provider detection** (Section 5): The `llmquality` build tag gates local provider availability but the same test function handles all providers. Document how provider detection works when `llmquality` is or isn't set.

5. **Baseline poisoning via PR** (Security, Section 1): Malicious baseline changes could suppress quality regressions. Add reviewer guidance or a CI sanity check on baseline pass rates.

### Out of Scope

- Whether 0.5B is a good enough canary for 3B quality (product decision)
- Whether the CI time budget of 15-20 minutes is acceptable (operational decision)
- The specific fix for the fly/trivy crashes (separate issue per design scoping)
