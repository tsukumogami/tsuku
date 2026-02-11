# LLM Discovery Implementation Design Review

## Executive Summary

The DESIGN-llm-discovery-implementation.md document provides a solid tactical design for implementing the LLM discovery stage. The core insight about separating LLM extraction from deterministic decision-making is well-developed and aligns with the existing ecosystem probe patterns. However, there are gaps in the alternatives analysis, some unstated assumptions, and areas where the security model could be strengthened.

**Overall Assessment**: Approve with modifications. The design is implementable but would benefit from addressing the gaps identified below.

---

## 1. Problem Statement Evaluation

### Strengths
- The problem is concrete and measurable: "Tools not found in the registry or ecosystem probes fall through to a non-functional 'not found' error"
- Clear user story: Users expect `tsuku install stripe-cli` to work without knowing the source
- Security sensitivity is appropriately emphasized upfront

### Gaps
- **Missing success criteria**: The problem statement doesn't define what "working" looks like. What's the expected success rate? What latency is acceptable? The 15-second timeout is mentioned later but not tied to a user experience goal.
- **Missing scope boundary**: The problem implicitly assumes GitHub is the primary distribution channel for long-tail tools. This assumption should be explicit since it justifies the GitHub-only verification choice.
- **No baseline comparison**: How many tools fall into the "LLM discovery needed" bucket? Is this 5% of install attempts or 50%? This affects whether the complexity is justified.

### Recommendation
Add a "Success Criteria" section defining:
- Target: 80%+ of LLM discovery attempts should find the correct source
- Latency: 95th percentile under 10 seconds, hard timeout at 15 seconds
- Cost: Average discovery cost under $0.05 per attempt

---

## 2. Missing Alternatives Analysis

### Decision 1: Web Search Integration

**Missing alternatives:**

1. **Perplexity API**: Purpose-built for search-with-LLM. Provides structured citations and factual answers. Pricing is competitive ($0.005 per search at scale). Should be considered as a serious alternative to Claude native, not just Tavily/SerpAPI.

2. **Hybrid approach with caching**: Use a cheaper search API (like Bing) for the initial search, then use Claude only for structured extraction. This separates the search cost from the LLM cost and allows caching search results.

3. **Pre-built GitHub search**: Many tools can be found via GitHub search API directly (`q=stripe-cli in:name`). This is free within rate limits and doesn't require LLM for common cases. Could be a fast-path before full web search.

### Decision 2: Structured Output Schema

**Missing alternatives:**

1. **JSON Schema-constrained generation**: Claude supports `response_format` with JSON schema validation. This guarantees valid output and eliminates parsing failures. The design uses tool calls which achieve similar reliability, but direct JSON mode should be mentioned as a considered option.

2. **Multi-step extraction**: First extract candidate URLs, then verify each independently. This is dismissed too quickly with "adds complexity and latency" but would catch cases where the LLM conflates multiple sources in a single response.

### Decision 3: Verification Depth

**Missing alternatives:**

1. **Binary verification via HEAD request**: Before full GitHub API verification, a simple HEAD request to `https://github.com/{owner}/{repo}/releases` confirms the repo has releases. This is faster than the full API call and catches the common case of repos without releases.

2. **Community trust signals**: Check if the repo is starred by trusted accounts (e.g., GitHub employees, major tech company orgs). This is a weak signal but could be part of the quality score.

### Decision 4: Decision Algorithm

**Missing alternatives:**

1. **LLM confidence calibration**: The 70% confidence threshold assumes the LLM's self-reported confidence is well-calibrated. An alternative is to always present results to the user with metadata and only auto-select when GitHub metadata (stars, activity) alone passes thresholds. This removes reliance on LLM confidence.

2. **Ensemble approach**: Query multiple times with different prompts and require consensus. More expensive but catches LLM hallucinations through disagreement.

### Decision 5: Session Architecture

**No major gaps**, though the rationale for rejecting BuildSession is weak. The interface mismatch is real, but the design could have considered a shared base type that both BuildSession and DiscoverySession implement.

---

## 3. Rejection Rationale Analysis

### Fair Rejections

- **Implement custom search scraping**: Legitimately rejected. ToS violations and anti-bot measures make this fragile.
- **LLM picks the winner**: Correctly identifies the non-determinism and auditability issues.
- **Separate search and extraction phases**: The complexity argument is valid, though the alternative has merit for edge cases.

### Potentially Unfair Rejections

1. **Gemini with Google Search Grounding**
   - Rejection rationale: "switching providers mid-discovery adds complexity"
   - Issue: This isn't a fair characterization. The design already uses Factory/Provider abstractions. Supporting Gemini as a fallback (or primary) would use the same abstraction. The real reason seems to be preference for consistency, which is valid but should be stated clearly.
   - Recommendation: Reframe as "defer Gemini support until we have telemetry showing Claude's web search is insufficient"

2. **Full ecosystem verification in LLM stage**
   - Rejection rationale: "the ecosystem probe would have found genuine ecosystem packages"
   - Issue: This isn't always true. The ecosystem probe uses `Probe()` which checks for exact name matches. If the LLM discovers that `prettier` is actually `@prettier/prettier-cli` on npm, the ecosystem probe wouldn't have found it because it searched for `prettier`.
   - Recommendation: Add a "re-probe" step when LLM suggests an ecosystem source with a different package name than the query.

3. **No verification, just confirmation**
   - Rejection rationale: "too risky"
   - Issue: This alternative is presented unfairly. The real comparison should be against the marginal security value of GitHub API verification. Since sandbox validation runs regardless, the question is whether API verification catches attacks that sandbox validation would miss. The design doesn't analyze this.
   - Recommendation: Explicitly state what attack vectors GitHub API verification catches that sandbox validation doesn't.

---

## 4. Unstated Assumptions

### Explicit Assumptions Needed

1. **GitHub dominates long-tail distribution**
   - The design assumes most tools not in ecosystem registries distribute via GitHub releases
   - This justifies GitHub-only verification
   - Should cite data: "Based on the existing registry, X% of GitHub-sourced tools use releases for distribution"

2. **LLM confidence is meaningful**
   - The threshold logic assumes `confidence: 85` from the LLM correlates with accuracy
   - LLM self-reported confidence is often poorly calibrated
   - Should acknowledge this and describe how thresholds will be tuned

3. **Single source is sufficient**
   - The design assumes tools have one "correct" source
   - Some tools (like Stripe CLI) are legitimately available from both GitHub releases AND npm
   - Should describe behavior when multiple valid sources exist

4. **Web search results contain sufficient metadata**
   - The design assumes the LLM can extract stars/downloads/last_update from web search snippets
   - Web search results are summaries; full metadata may not be present
   - GitHub API verification is the real source of metadata, which makes the extraction schema partially redundant

5. **15-second timeout is acceptable**
   - The design assumes users will tolerate 15-second worst-case latency
   - No user research or comparison to competitor behavior is cited

### Implicit Security Assumptions

1. **Claude's web search is trustworthy**
   - The design assumes Claude's web search tool returns legitimate results
   - No discussion of whether Claude's search could be poisoned or manipulated

2. **GitHub API is authoritative**
   - The design treats GitHub API responses as ground truth
   - A compromised GitHub account with a legitimate-looking repo would pass all checks

3. **HTML stripping is sufficient**
   - The design assumes stripping HTML tags and hidden elements defeats prompt injection
   - Novel attacks via visible text in search results are acknowledged as residual risk but underexplored

---

## 5. Strawman Analysis

### No Clear Strawmen

All alternatives appear to be genuine options that were considered. However, some rejections are too brief:

1. **"Separate search and extraction phases"**: Dismissed in one sentence. A fair treatment would acknowledge the traceability benefits (knowing exactly which URLs the LLM examined) before rejecting on complexity grounds.

2. **"Simple builder+source output"**: This is close to a strawman. It's presented as obviously inferior ("doesn't enable quality-based decision making"), but the design doesn't explain why LLM confidence alone isn't sufficient given that GitHub API verification happens anyway.

---

## 6. Quality Metrics Insight Evaluation

The design does address the user's key insight about quality metrics enabling deterministic decisions. The approach is sound:

- LLM extracts structured data (stars, downloads, confidence, evidence)
- Algorithm applies thresholds (confidence >= 70 AND stars >= 50)
- Same pattern as ecosystem probe's QualityFilter

### Gaps in the Quality Metrics Implementation

1. **Threshold values are arbitrary**: Stars >= 50 and confidence >= 70 are stated without justification. The existing QualityFilter uses different thresholds per ecosystem (crates.io: 100 downloads, RubyGems: 1000 downloads). Why is GitHub different?

2. **OR logic vs AND logic inconsistency**: The ecosystem probe uses OR logic (passes if ANY threshold met). The LLM discovery uses AND logic (confidence >= 70 AND stars >= 50). This inconsistency should be explained.

3. **No fallback for high-star/low-confidence**: A repo with 10,000 stars but only 60% LLM confidence would fail the threshold. The "strong signals override" path (stars >= 500) addresses this but creates magic numbers.

4. **Evidence array is underutilized**: The schema includes `evidence: []string` but the threshold logic doesn't use it. How does evidence factor into the decision?

---

## 7. Security Analysis Gaps

### Well-Covered Areas

- Prompt injection via HTML: Stripping is mentioned
- Hallucinated repos: GitHub API 404 catches this
- Typosquatting: Quality thresholds + confirmation metadata
- Budget exhaustion: Timeout and daily limits

### Gaps

1. **Clone attacks not addressed**
   - Attacker forks a legitimate repo, maintains it for a while to build stars/activity, then adds malware
   - GitHub API verification would pass (exists, not archived, has stars)
   - The "residual risk" column mentions "sophisticated clone with stars" but offers no mitigation
   - Recommendation: Check if repo is a fork of another repo

2. **GitHub repo takeover**
   - Original maintainer loses access, attacker gains control
   - No ownership change detection is proposed for real-time discovery (only mentioned for registry freshness checks)
   - Recommendation: Note when GitHub owner login differs from repo name patterns

3. **SEO poisoning**
   - Attacker creates content that ranks highly for tool searches
   - Claude's web search might surface this content
   - The "novel visible-text injection" risk is acknowledged but underexplored
   - Recommendation: Weight official documentation sites higher than general results

4. **Redirect attacks**
   - Attacker creates a site that initially looks legitimate, then redirects to malicious content
   - HTML stripping happens at search time, but the final GitHub URL could be a redirect
   - Recommendation: Resolve GitHub URLs and verify final destination matches expected pattern

5. **Rate limit gaming**
   - Attacker triggers many LLM discovery attempts to exhaust the user's budget
   - The daily budget limit helps, but an attacker could craft a tool name that takes maximum time to process
   - Recommendation: Per-tool rate limiting, not just daily budget

### Missing Threat Model

The design lacks a formal threat model. Adding a table like:

| Attacker | Goal | Capability | Mitigations |
|----------|------|------------|-------------|
| Script kiddie | Install malware | Create fake GitHub page | Quality thresholds, API verification |
| Sophisticated actor | Target specific user | SEO + legitimate-looking repo | User confirmation, sandbox validation |
| State actor | Supply chain attack | Long-term repo compromise | (None - out of scope?) |

---

## 8. Scope Appropriateness

### Alignment with Parent Design

The design correctly implements the requirements from DESIGN-discovery-resolver.md:
- Web search via LLM
- Structured JSON extraction
- GitHub API verification
- User confirmation with metadata
- Prompt injection defenses

### Scope Creep Risks

1. **Multi-turn conversation**: The design mentions "max 3 turns" for disambiguation. This adds complexity not required by the parent design. A single-turn approach might be sufficient.

2. **Evidence array**: Nice for debugging but not required. Adds to token costs and extraction complexity.

3. **Reasoning field**: Similar to evidence - useful for debugging but not core functionality.

### Potentially Under-Scoped

1. **Telemetry**: Mentioned in Phase 5 but not detailed. What events? What fields?

2. **Caching**: Explicitly out of scope, but the design should note that repeated searches for the same tool will hit the LLM every time until a recipe is saved.

3. **Offline behavior**: What happens when web search fails? The error handling table shows timeouts but not network unreachability.

---

## 9. Actionable Recommendations

### Must Fix (Blocking Issues)

1. **Add success criteria to problem statement**: Define measurable targets for accuracy, latency, and cost.

2. **Justify threshold values**: Explain why stars >= 50 and confidence >= 70 were chosen. Reference similar thresholds in QualityFilter.

3. **Clarify AND vs OR logic**: Explain why LLM discovery uses AND logic when ecosystem probe uses OR logic.

4. **Add fork detection**: Check if discovered repos are forks; surface this in confirmation metadata.

### Should Fix (Improve Quality)

5. **Add Perplexity as considered alternative**: It's a legitimate option that should be addressed.

6. **Reframe Gemini rejection**: State "defer to future iteration" rather than implying architectural difficulty.

7. **Add re-probe step for ecosystem sources**: When LLM suggests an ecosystem package with a different name, re-run the ecosystem probe with that name.

8. **Explicit assumption list**: Add a section listing assumptions about GitHub dominance, LLM confidence calibration, etc.

9. **Formalize threat model**: Add an attacker/goal/capability table.

### Nice to Have (Polish)

10. **Single-turn simplification**: Consider whether multi-turn is necessary for v1.

11. **Simplify extraction schema**: Evidence and reasoning are nice-to-have; consider making them optional.

12. **Add telemetry event list**: Specify what discovery telemetry will capture.

---

## 10. Summary

The LLM Discovery Implementation design is fundamentally sound. The key architectural decision - separating LLM extraction from deterministic decision-making - is well-reasoned and aligns with existing patterns. The security model has appropriate depth with multiple verification layers.

The main weaknesses are:

1. **Incomplete alternatives analysis**: Some options (Perplexity, GitHub search fast-path) weren't considered
2. **Unstated assumptions**: GitHub dominance, LLM confidence calibration, threshold values
3. **Security gaps**: Clone attacks, ownership changes, SEO poisoning
4. **Scope clarity**: Success criteria undefined, telemetry details missing

Addressing the "Must Fix" items would make this design ready for implementation. The "Should Fix" items would improve confidence in the design but aren't blocking.
