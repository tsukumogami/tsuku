# Security Review: LLM Discovery Implementation (Round 2)

**Document:** DESIGN-llm-discovery-implementation.md
**Reviewer:** Security Review
**Date:** 2026-02-11
**Round:** 2 (post-revision review)

---

## 1. Assessment of Security Risk Acknowledgments

The design now acknowledges two key risks in the Uncertainties section:

### Star Gaming (lines 377-378)

> "Quality thresholds based on star counts can be gamed (stars can be purchased). May need velocity checking or multi-source corroboration in future iterations if attacks materialize."

**Assessment:** Adequate for v1. The acknowledgment is appropriately scoped:
- Recognizes the attack vector exists
- Proposes reasonable future mitigations (velocity checking, multi-source corroboration)
- Defers to "if attacks materialize" which is pragmatic for a v1

Star gaming is a known issue across the GitHub ecosystem. Tsuku isn't unique in facing this, and the existing mitigations (user confirmation, multiple evidence sources, human-readable metadata display) provide reasonable protection for v1.

### Visible Text Injection (lines 379-380)

> "HTML stripping addresses hidden injection but not SEO-optimized attack pages with malicious visible content. Multi-source verification (official docs -> repo link) may be needed if this attack vector is exploited."

**Assessment:** Adequate acknowledgment. The design correctly identifies that:
- HTML stripping is necessary but not sufficient
- Visible text can still be weaponized via SEO optimization
- Future mitigation path exists (multi-source verification)

The system prompt (lines 483-500) already encourages cross-referencing sources and looking for official documentation, which provides partial protection.

---

## 2. Critical Blockers

**None identified.**

The design provides adequate defense-in-depth for a v1 implementation. The verification chain is:

1. LLM extraction (can be fooled, but constrained by structured output)
2. Quality thresholds (confidence gate + star/download thresholds)
3. GitHub API verification (authoritative existence check)
4. User confirmation with rich metadata
5. Sandbox validation before real installation

Each layer catches different attack categories:
- Layers 1-2 filter low-confidence/low-quality suggestions
- Layer 3 catches hallucinations and non-existent repos
- Layer 4 enables human judgment on edge cases
- Layer 5 catches malicious binaries

The `--yes` flag behavior (line 634) correctly skips only confirmation, not verification. This is security-critical and implemented correctly.

---

## 3. Acceptable Risks for v1

### GitHub-Only Verification
The design explicitly limits verification to GitHub sources (lines 289-306). Non-GitHub ecosystem sources (npm, PyPI, etc.) defer to the ecosystem probe. This is acceptable because:
- The ecosystem probe would have found genuine ecosystem packages
- LLM discovery only runs after ecosystem probe misses
- GitHub releases are the primary distribution method for tools not in registries

### Conservative Thresholds May Exclude Legitimate Tools
Stars >= 50 may exclude obscure but legitimate tools (line 413). This is the right trade-off for v1:
- Users can override with `--from`
- Better to be conservative initially and relax based on data

### Fork Detection Relies on User Judgment
Forks are flagged but the final decision is the user's (lines 567-576). The mitigations are reasonable:
- Warning displayed
- Parent comparison (10x stars triggers suggestion)
- Never auto-select forks

### DuckDuckGo Endpoint Stability (Local LLMs)
The DDG HTML endpoint could change (line 365). This only affects local LLMs and has fallback options:
- Cloud LLMs use native search
- Tavily/Brave API options exist
- Lite POST endpoint is documented as fallback

### Prompt Injection Residual Risk
Novel visible-text injection attacks remain possible (line 891). The defense-in-depth layers provide acceptable protection:
- Structured output constrains what the LLM can suggest (builder types only)
- Quality thresholds filter low-confidence results
- User confirmation shows evidence for human review
- Sandbox catches bad binaries

---

## 4. New Security Concerns from Changes

**None introduced.**

The additions (star gaming and visible text injection acknowledgments) are purely documentation additions to the Uncertainties section. They don't introduce new code paths or weaken existing mitigations.

---

## 5. Verification of Key Security Properties

| Property | Status | Evidence |
|----------|--------|----------|
| LLM output doesn't directly control installation | Pass | Structured output -> thresholds -> verification -> confirmation chain |
| GitHub verification is not skippable | Pass | `--yes` skips confirmation only (line 634) |
| Forks never auto-selected | Pass | Explicit in threshold logic (line 582-583) |
| Confidence gate is AND not OR | Pass | Lines 318-326 explain rationale |
| HTML stripping removes hidden content | Pass | Lines 611-615 specify removal targets |
| URL validation prevents injection | Pass | Lines 621-624 specify restrictions |
| Rate limit handling is graceful | Pass | Soft error, skip verification, higher confirmation bar (line 659) |
| No code execution during discovery | Pass | Line 844 explicitly states this |

---

## 6. Readiness Verdict

**Ready**

The design adequately addresses v1 security requirements:

1. **Defense-in-depth is appropriate.** Five verification layers with different failure modes provide reasonable protection.

2. **Known risks are acknowledged.** Star gaming and visible text injection are documented as uncertainties with future mitigation paths.

3. **No critical gaps.** The verification chain is complete: LLM -> thresholds -> GitHub API -> confirmation -> sandbox.

4. **Trade-offs are explicit.** Conservative thresholds, GitHub-only verification, and fork handling are documented with rationale.

5. **Fail-safe defaults.** `--yes` skips confirmation but not verification. Forks never auto-select. Low confidence sources are presented as options, not auto-selected.

The design is ready for implementation. The acknowledged risks (star gaming, visible text injection) are acceptable for v1 and have reasonable future mitigation paths if exploitation is observed.
