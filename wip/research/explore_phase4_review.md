# Design Review: DESIGN-homebrew-deterministic-mode.md

## 1. Problem Statement Clarity

The problem statement is well-structured and specific. It identifies three concrete blockers (RequiresLLM returning true unconditionally, no structured failure type, no way to prevent LLM fallback) and ties each to a downstream consumer (the batch pipeline). This makes it straightforward to evaluate whether each option solves the stated problems.

One minor gap: the problem statement doesn't quantify the impact. It mentions "most formulas succeed deterministically" but the 85-90% figure only appears later in Uncertainties. Moving an estimate into the problem statement would strengthen the case for why this is worth doing now.

**Verdict: Sufficiently specific. Minor improvement possible by quantifying impact upfront.**

## 2. Missing Alternatives

The options space is well-covered for each decision. A few alternatives worth noting:

**Decision 1 (signaling mode):** The three options (session flag, separate builder, constructor option) cover the reasonable design space. One could imagine a context-based approach (passing deterministic-only via `context.Context` values), but this would be worse than all listed options since it hides control flow in implicit context. No meaningful gap.

**Decision 2 (failure reporting):** The doc doesn't consider returning a structured result instead of an error -- e.g., `Generate()` returning `(*BuildResult, *DeterministicFailure, error)` where a nil BuildResult + non-nil DeterministicFailure signals structured failure distinct from unexpected errors. This would separate "I tried and classified the failure" from "something unexpected went wrong." However, this changes the interface signature, which the design explicitly wants to avoid. The chosen approach (type-asserting the error) is idiomatic Go and pragmatic.

**Decision 3 (RequiresLLM):** A fourth option could be making `RequiresLLM()` context-dependent (e.g., `RequiresLLM(opts SessionOptions) bool`), but this changes the interface signature. Not worth adding.

**Verdict: No significant missing alternatives.**

## 3. Pros/Cons Fairness

The pros and cons are balanced and honest. Specific observations:

- **Option 1B (separate builder):** The cons fairly call it over-engineering. The pros don't oversell it. Not a strawman -- it's a legitimate pattern for more divergent behavior, just disproportionate here.

- **Option 3A (return false):** The con about losing LLM cost warnings is real and well-stated. The rationale section explains why it's acceptable (the `create` command has separate LLM checks via `CheckLLMPrerequisites`). I verified this claim against `builder.go` -- `CheckLLMPrerequisites` does exist and checks `LLMConfig.LLMEnabled()` independently of `RequiresLLM()`. The claim holds.

- **Option 3B (CanRunDeterministic):** The con "all builders need to implement it" is accurate since it proposes adding to the `SessionBuilder` interface. However, the doc doesn't mention that a default implementation returning `false` via embedding would make this low-cost. This slightly overstates the con for 3B.

- **Option 3C (batch pipeline handles differently):** The con about Homebrew-specific knowledge in the pipeline is fair but understated. If more builders gain deterministic paths later (likely), this approach creates per-builder special cases in the pipeline.

**Verdict: Pros/cons are fair. Option 3B's con is slightly overstated but doesn't change the conclusion.**

## 4. Unstated Assumptions

Several assumptions deserve explicit mention:

1. **Single builder per formula.** The design assumes each formula goes through one builder. If multiple builders could handle the same formula, the DeterministicOnly flag's per-session scope might interact with builder selection logic. This is likely already true but not stated.

2. **Failure categories are exhaustive for Homebrew.** The six categories listed are assumed to cover all deterministic failure modes. The Uncertainties section partially addresses this ("complex_archive may not occur") but doesn't address the inverse: are there Homebrew-specific failure modes not captured? For example, what about architecture mismatches (formula has bottles for darwin-arm64 but not linux-amd64)?

3. **`generateDeterministic()` is already a cleanly separable function.** The design assumes the deterministic path can be intercepted at a single point. If the LLM fallback is woven into multiple places in the current code, the "check one flag" approach might be more invasive than described.

4. **Nil dereference fix (homebrew.go:430) is straightforward.** This is listed in scope but not discussed in the architecture section. If it requires restructuring initialization order, it could affect the session options design.

**Verdict: Assumptions 2 and 4 should be made explicit. The others are low-risk.**

## 5. Strawman Analysis

No option appears designed to fail. Each has genuine trade-offs:

- Option 1B (separate builder) is the weakest but remains a legitimate pattern used in other codebases. The doc correctly identifies it as disproportionate for a one-flag change.
- Option 3C (pipeline handles differently) is the simplest and could be a valid interim solution. It's not dismissed unfairly.

**Verdict: No strawmen detected.**

## 6. Additional Observations

**Coupling to failure schema:** The design acknowledges that `DeterministicFailureCategory` constants must stay in sync with `failure-record.schema.json`. The mitigation (CI validation via #1201) is appropriate but introduces a dependency on work that may not be done yet. If #1201 ships after this design, there's a window where they can drift. The design should note whether this is acceptable or if sync validation is a prerequisite.

**RepairNotSupportedError:** The design references this error type in the Repair guard, but it doesn't appear in `errors.go`. Either it already exists elsewhere (perhaps in `DeterministicSession`) or it needs to be created as part of this work. The implementation plan should clarify this.

**Thread safety:** If a builder instance is shared between interactive and batch callers (as Option 1A enables), `deterministicOnly` is set per-session, not per-builder. The design correctly scopes it to the session, but this should be stated explicitly since it's the key advantage of 1A over 1C.

## Summary of Recommendations

1. **Quantify impact in the problem statement** -- move the ~85-90% deterministic success estimate into the problem section.
2. **Address architecture mismatch failures** -- confirm the category list covers "no bottle for this platform" (likely maps to `no_bottles` but worth stating).
3. **Clarify RepairNotSupportedError origin** -- note whether it exists or needs creation.
4. **Note CI sync validation dependency** -- state whether #1201 is a prerequisite or if temporary drift is acceptable.
5. **Make the nil dereference fix visible** -- add a brief note in the architecture section or remove it from scope if it's truly orthogonal.

Overall, this is a well-structured design document. The problem is clear, the options are fair, and the chosen solution (1A + 2A + 3A) is well-justified. The recommendations above are refinements, not structural issues.
