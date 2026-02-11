# Decision Validation Review - Round 2

**Design:** DESIGN-llm-discovery-implementation.md
**Reviewer Role:** Decision Validation
**Review Date:** 2026-02-11

## Summary

This design has improved significantly from the first round. The key changes (threshold generalization, security concerns moved to Uncertainties, non-deterministic handling deferred) address the main feedback. The document now makes appropriate decisions for a tactical design while deferring implementation details correctly.

---

## 1. Decisions That Are Now Well-Justified

### Decision 1: Web Search as a Tool (Provider-Transparent)

**Justification Quality: Strong**

The design clearly articulates why this approach was chosen:
- Cloud LLMs automatically use native search (higher quality, API-handled)
- Local LLMs get capability via tsuku-provided handler
- LLM controls search strategy (can refine queries, search multiple times)
- Unified architecture regardless of provider

The alternatives considered section effectively explains why pre-search or separate search phases were rejected. The trade-off (DDG dependency for local LLMs) is explicitly acknowledged.

### Decision 2: Flexible Result Interface (Builder + Instructions)

**Justification Quality: Strong**

The design explains the two result types and provides concrete examples of why non-deterministic results matter (proprietary installers, platform-specific instructions). The deferral to implementation for exact schema is appropriate—this is the right level of detail for a tactical design.

### Decision 3: GitHub-Only Verification

**Justification Quality: Adequate**

The rationale is sound: GitHub releases are the primary distribution method for tools not in ecosystem registries, and ecosystem packages would have been found by the probe already. The trade-off is explicitly acknowledged in section "Trade-offs Accepted."

One minor concern: The design could more explicitly state *why* GitHub-only is sufficient for v1 (e.g., "Based on analysis of the current registry, X% of non-ecosystem tools are distributed via GitHub releases"). However, this is a minor gap—the decision is defensible without this data.

### Decision 4: Threshold + Priority + Confirmation

**Justification Quality: Strong (improved)**

The revision successfully removed specific threshold values while preserving the decision logic. Key improvements:
- The table explaining signal purposes and tuning approaches (lines 330-334) provides justification without locking in numbers
- The explanation of why AND logic for confidence vs OR logic for quality signals is clear
- Explicit acknowledgment that values will be tuned via telemetry

This is exactly the right approach: define the algorithm shape, defer specific parameters to implementation.

### Decision 5: Dedicated DiscoverySession

**Justification Quality: Strong**

The rationale for a separate session type (discovery is not building a recipe) is clear. The alternatives considered effectively explain why existing patterns don't fit.

---

## 2. Decisions That Still Have Weak Justification (If Any)

### Minor: 15-Second Timeout

The 15-second timeout appears in multiple places but the justification is minimal. Line 79 mentions it as a budget control, but there's no analysis of:
- Expected latency distribution for LLM + search + verification
- Why 15 seconds specifically (vs 10 or 20)
- Whether this is configurable at runtime

This is a **minor** issue because:
1. The timeout is described as "configurable" (line 475)
2. The success criteria table (line 117) shows P95 < 10s, P99 < 15s, suggesting 15s is the ceiling
3. Implementation can tune this based on real latency data

**Verdict:** Acceptable for this design level. Implementation should validate the timeout.

### Minor: Fork Detection Threshold (10x Stars)

Line 573 states: "if parent has 10x more stars, suggest parent instead"

This specific multiplier appears without justification. Why 10x vs 5x or 20x? This is a similar situation to the threshold values that were generalized—except this one remained specific.

**Recommendation:** Either generalize this to "significantly more stars" with implementation-time tuning, or add a brief rationale for the 10x choice.

**Verdict:** Minor issue. The behavior (detect forks, compare to parent, warn user) is correct; the specific threshold is an implementation detail.

---

## 3. "Required Subsystem Designs" Approach

**Assessment: Appropriate and Well-Executed**

The design correctly identifies the Non-Deterministic Builder as a dependency requiring its own design (lines 89-109). This is the right call because:

1. **Scope containment:** The non-deterministic builder involves LLM-driven code generation, sandbox execution, and verification—each complex enough to warrant dedicated design attention.

2. **Clear blocking relationship:** Phase 8 explicitly states "Blocked by: DESIGN-non-deterministic-builder.md" with a clear statement that LLM Discovery is feature-complete only when that phase is done.

3. **Value without the subsystem:** The design delivers value (deterministic builder results) without waiting for the subsystem. This enables incremental delivery.

4. **DDG handler inline:** The design correctly notes that the DDG search handler is "simple enough" for inline treatment, showing appropriate judgment about what needs separate design vs inline implementation.

---

## 4. Trade-offs Acknowledgment Review

The "Trade-offs Accepted" section (lines 409-415) explicitly lists five trade-offs. Each is stated clearly with mitigation where applicable:

| Trade-off | Mitigation Stated | Assessment |
|-----------|------------------|------------|
| DDG dependency (local only) | Endpoint designed for accessibility; API fallbacks possible | Clear |
| GitHub verification only | Ecosystem probe handles other sources | Clear |
| Conservative thresholds | Users can override with --from | Clear |
| Always confirm LLM sources | Sandbox validation still runs | Clear |
| Local LLM context limits | Results trimmed; may reduce accuracy | Clear |

**Verdict:** Trade-offs are well-acknowledged. No hidden assumptions.

---

## 5. Uncertainties Section Review

The Uncertainties section (lines 374-379) appropriately captures risks moved from earlier in the document:

- **Star gaming** moved here with mitigation (velocity checking, multi-source corroboration in future)
- **Visible text injection** moved here with mitigation (multi-source verification)
- **Other uncertainties** (DDG stability, extraction accuracy, false positive rate, context limits) are appropriate for this section

**Assessment:** The security concerns are now appropriately framed as known risks to monitor rather than solved problems. This is more honest than the original framing.

---

## 6. Readiness Verdict

### Ready with Minor Fixes

The design is ready for implementation with two minor fixes:

1. **Fork detection threshold (10x stars):** Either generalize to "significantly more" or add a one-sentence rationale for 10x. This is on line 573.

2. **Optional: GitHub-only justification strengthening:** Consider adding a sentence like "Analysis of the existing recipe registry shows the majority of non-ecosystem tools use GitHub releases" to strengthen the GitHub-only verification decision.

Neither of these blocks implementation. The design makes correct decisions at the right level of abstraction, defers appropriately to implementation for threshold tuning, and correctly identifies the non-deterministic builder as a separate design dependency.

---

## Summary Table

| Decision | Justification Quality | Status |
|----------|----------------------|--------|
| Web search as tool | Strong | Ready |
| Flexible result interface | Strong | Ready |
| GitHub-only verification | Adequate | Ready (minor improvement possible) |
| Threshold + Priority + Confirmation | Strong (improved) | Ready |
| Dedicated DiscoverySession | Strong | Ready |
| Required Subsystem approach | Appropriate | Ready |
| Trade-offs acknowledgment | Complete | Ready |

**Overall Verdict: Ready with Minor Fixes**

The two minor items (fork threshold specificity, optional GitHub justification strengthening) do not block approval. The design can proceed to implementation.
