# Decision Validation Review: LLM Discovery Implementation

**Design Document:** `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/docs/designs/DESIGN-llm-discovery-implementation.md`

**Review Date:** 2026-02-10

---

## 1. Decisions That Are Well-Justified

### 1.1 Search as a Tool (Decision 1)

**Decision:** Expose web search as a tool that the LLM invokes, with Cloud LLMs using native search and local LLMs using a tsuku-provided handler.

**Why this is well-justified:**
- The design correctly identifies that Cloud LLMs (Claude, Gemini) have native search capabilities that are higher quality than any tsuku-provided alternative
- Unified architecture means the same extraction/verification flow applies regardless of provider
- The LLM controls search strategy (may refine queries, search multiple times), which is more flexible than a fixed "search then LLM" approach
- Rejected alternatives are fairly considered: tsuku-driven search for all providers would waste Cloud LLM native capabilities; separate search phase prevents iterative refinement

**Strength of rationale:** Strong. The trade-offs are clearly articulated.

### 1.2 Deterministic Decision Algorithm (Decision 4)

**Decision:** LLM extracts data, deterministic algorithm makes the final decision using quality thresholds.

**Why this is well-justified:**
- Clear separation of concerns: LLMs excel at understanding web content; algorithms excel at consistent, auditable decisions
- Reuses the existing QualityFilter pattern from the ecosystem probe
- Provides transparency: users can understand why a source was selected or rejected
- Aligns with security requirements: no implicit trust in LLM output

**Strength of rationale:** Strong. This is a key architectural insight that the design executes well.

### 1.3 Dedicated DiscoverySession (Decision 5)

**Decision:** Create a separate LLMDiscoverySession rather than reusing BuildSession.

**Why this is well-justified:**
- Discovery returns DiscoveryResult (builder + source), not a recipe
- Follows established patterns (HomebrewBuilder session architecture)
- Clean separation between finding sources and building recipes

**Strength of rationale:** Adequate. Could have explained more about why discovery might need multi-turn conversations (the current design implies single-turn).

### 1.4 Fork Detection

**Decision:** Forks never auto-pass thresholds; require explicit confirmation even with --yes.

**Why this is well-justified:**
- Forks can represent abandoned copies of legitimate projects
- Comparing to parent repo (10x stars) provides objective guidance
- Never auto-selecting forks is a conservative, safe default

**Strength of rationale:** Strong. This addresses a real attack vector.

---

## 2. Decisions with Weak or Missing Justification

### 2.1 Threshold Values: 70/50/1000

**Decision:** Confidence >= 70, Stars >= 50, Downloads >= 1000

**Justification provided:**
- Confidence 70%: "Below 70%, false positive rate in testing exceeded 15%"
- Stars 50: "Analysis of tsuku registry shows 95% of legitimate GitHub tools have 50+ stars"
- Downloads 1000: "Consistent with ecosystem probe"

**Why the justification is weak:**

1. **Confidence threshold (70%):**
   - What testing? There's no reference to any validation data
   - What is "false positive" in this context? Wrong repo? Non-existent repo?
   - Why is 15% the threshold for "too many" false positives?
   - The design explicitly says LLM extraction accuracy hasn't been validated (in Uncertainties)

2. **Stars threshold (50):**
   - "Analysis of tsuku registry" suggests existing registry entries, but registry entries were curated for known-popular tools
   - This is survivorship bias: registry has popular tools, therefore popular tools have stars
   - What about legitimate but niche tools with 20-49 stars?
   - The design says this "may exclude legitimate but obscure tools" but doesn't quantify

3. **Downloads threshold (1000):**
   - The existing QualityFilter uses 100 for crates.io and npm, not 1000
   - 1000 for rubygems is different from 1000 for npm (different ecosystem scales)
   - Claiming "consistent with ecosystem probe" is incorrect based on actual code

**Impact:** These thresholds will directly affect discovery accuracy but lack empirical foundation.

### 2.2 GitHub-Only Verification

**Decision:** Verify GitHub sources via API; for non-GitHub sources, "defer to existing ecosystem probe."

**Justification:** "If a tool exists in npm, the ecosystem probe would have found it - LLM discovery only runs after probe misses."

**Why the justification is problematic:**

The reasoning contains a logical gap:

1. LLM discovery runs after ecosystem probe misses
2. LLM might suggest an npm package that the probe missed (typo in query, API timeout, rate limit)
3. LLM might suggest homebrew formula or cask (not covered by ecosystem probe at all)
4. LLM might suggest sources that need verification but aren't GitHub (GitLab, Bitbucket, direct download URLs)

**The design's claim that "ecosystem probe would have found it" assumes the probe is infallible.** But the design explicitly lists probe failures as soft errors that log and continue. An LLM might re-suggest something the probe failed to find.

**Missing analysis:** What happens when LLM suggests:
- A GitLab repository?
- A direct download URL from a project website?
- A homebrew formula (probe doesn't cover this)?

The design says these are "out of scope" but doesn't explain why GitHub-only is the right boundary for v1.

### 2.3 "Required Subsystem Designs" for Non-Deterministic Results

**Decision:** Support non-deterministic results (instructions) but defer implementation to a separate design.

**Why this is concerning:**

The design includes `instructions` as a result type but:
1. Doesn't define what makes a result "non-deterministic"
2. Doesn't specify how to detect when to return instructions vs. keep searching
3. Creates a schema that supports something that won't work

**Question:** Why include `instructions` result type at all if the handling doesn't exist? This creates dead code paths and confuses implementers.

**Alternative not considered:** Start with builder results only, add instructions result type when the subsystem is designed.

### 2.4 15-Second Timeout

**Decision:** Discovery has a 15-second timeout budget.

**Justification:** Brief mention in Decision Drivers ("15-second budget: Discovery shouldn't block interactive use indefinitely")

**Missing justification:**
- Why 15 seconds specifically? (Parent design also uses 15s, but without explanation)
- What's the expected latency breakdown? (Search: X, LLM: Y, Verification: Z)
- Is 15 seconds enough for multi-turn conversations if the LLM does multiple searches?
- The success criteria show P95 < 10s, P99 < 15s - so timeout is set at P99. Is that intentional?

---

## 3. Alternative Approaches That Deserve More Consideration

### 3.1 Cached LLM Discovery Results

**Rejected reason:** "recipes serve this purpose"

**Why this deserves reconsideration:**

1. Recipe creation requires sandbox validation, which is heavyweight
2. A user might run `tsuku install stripe-cli`, see the confirmation, say no, then later run it again
3. Without discovery caching, the second run repeats the full LLM + search + verification cycle
4. Cache could store DiscoveryResult (not recipe) for quick re-confirmation

**Cost-benefit not analyzed:** How often do users decline then retry? How much LLM cost does this save?

### 3.2 Pre-Fetch Search Before LLM Invocation

**Rejected reason:** "Wastes Cloud LLM native search capability"

**Why this deserves reconsideration for local LLMs:**

For local LLMs specifically:
1. Local LLMs have smaller context windows (~4K tokens)
2. Search results take ~2-3K tokens (per design)
3. Pre-fetching lets tsuku control result count and format precisely
4. Could enable caching of search results across tool queries ("cli tool" might help with multiple tools)

The design correctly notes this is relevant only for local LLMs. The rejection focuses on Cloud LLMs, but local LLM workflow might benefit from pre-fetch.

### 3.3 Verification for Non-GitHub Sources

**Rejected reason:** "deferred - GitHub covers most cases"

**Why this deserves reconsideration:**

1. The design doesn't quantify "most cases"
2. Homebrew casks are a common source for macOS tools (not in ecosystem probe)
3. GitLab is increasingly popular for open-source projects
4. If LLM suggests a non-GitHub source with high confidence, what happens?

**Specific gap:** The design says "GitHub API verification" runs for GitHub sources, but doesn't say what verification (if any) runs for non-GitHub sources. Complete omission creates silent acceptance of unverified sources.

### 3.4 Structured Grounding for Search Results

**Not mentioned:** Using structured search prompts that guide search engines toward official sources.

Example: Instead of searching "stripe-cli github official", search:
- `site:github.com stripe-cli releases`
- `stripe-cli official documentation install`

This could improve search result quality before LLM sees them. The design relies on the LLM to craft good queries, but doesn't discuss search query engineering.

---

## 4. Trade-offs That Need Clearer Acknowledgment

### 4.1 Stars as Quality Signal

**Hidden cost:** Stars can be gamed, purchased, or inflated by viral moments unrelated to tool quality.

**Not acknowledged:**
- Recently starred repos (star farms) vs. gradually accumulated stars
- Stars don't indicate security posture or maintenance quality
- A malicious fork could accumulate stars before being detected

**Suggestion:** Consider star velocity (stars per month over lifetime) rather than absolute count.

### 4.2 DDG Dependency for Local LLMs

**Acknowledged:** "DDG endpoint stability" is listed as uncertainty.

**Not adequately addressed:**
- What happens when DDG scraping fails? Error? Fall through?
- Rate limiting from DDG is likely at scale
- No offline fallback for local LLMs

The design lists Tavily and Brave as alternatives, but these require API keys. Users who specifically want local (no cloud APIs) are stuck with DDG.

### 4.3 Cost Per Discovery

**Success metric:** < $0.05 per discovery

**Hidden costs not analyzed:**
- Multiple search calls per discovery (LLM controls iteration)
- Failed discoveries (timeout, no match) still cost tokens
- GitHub API rate limits without authentication

**Missing:** Expected cost breakdown by component and failure mode.

### 4.4 The Confidence Score Paradox

**The design states:** "The LLM doesn't 'decide' which source to use - it extracts data, and the algorithm decides."

**But also:** The LLM provides a confidence score (0-100), and confidence >= 70 is required.

**Tension:** The confidence score IS a decision by the LLM. The LLM is deciding how confident it is, which gates the algorithmic decision. If the LLM says 71% for the wrong repo, it passes. If it says 69% for the right repo, it fails.

**Not acknowledged:** Confidence calibration. Are LLM confidence scores well-calibrated? A 70% confidence should mean "correct 70% of the time," but LLMs often aren't calibrated this way.

---

## 5. Recommendations for Strengthening Rationale

### 5.1 Empirical Validation Plan

Add a section describing how thresholds will be validated before they're frozen:

1. Run discovery against a known-answer test set (100 tools with known sources)
2. Measure false positive and false negative rates at different thresholds
3. Document the test set and results in the design

### 5.2 Non-GitHub Source Handling

Explicitly define behavior:

| Source Type | Verification | Confirmation |
|-------------|--------------|--------------|
| GitHub | API check (exists, not archived, not fork) | Rich metadata |
| GitLab | [define: API check? none?] | [define] |
| Homebrew | [define] | [define] |
| Direct URL | [define: domain allowlist?] | [define] |

If "reject all non-GitHub" is the v1 answer, say so explicitly.

### 5.3 Confidence Calibration

Either:
1. Remove confidence from threshold logic (use only objective metrics like stars)
2. Add a calibration mechanism (historical accuracy tracking)
3. Document that confidence is a soft signal and describe how miscalibration affects behavior

### 5.4 Local LLM Fallback Chain

Define explicit behavior when DDG scraping fails:

```
DDG scraping fails
  -> If TAVILY_API_KEY set: try Tavily
  -> If BRAVE_API_KEY set: try Brave
  -> Else: return error with "Web search unavailable. Use --from to specify source."
```

The design implies this but doesn't make it explicit.

### 5.5 Cost Accounting Detail

Add expected cost breakdown:

| Component | Expected Cost | Notes |
|-----------|--------------|-------|
| Cloud search (Claude) | Included in token cost | Native search |
| LLM tokens (input) | ~X tokens * $Y/1K | System prompt + results |
| LLM tokens (output) | ~Z tokens * $W/1K | Extraction response |
| DDG search | $0 | Free |
| Tavily search | $0.01/search | If configured |
| GitHub API | $0 (rate-limited) | 60/hr unauthenticated |

---

## Summary Assessment

**Overall:** The design makes sound high-level decisions (tool-based search, deterministic algorithm, GitHub verification) but lacks empirical grounding for specific threshold values. The treatment of non-GitHub sources and non-deterministic results creates ambiguity that will surface during implementation.

**Key recommendations:**
1. Validate thresholds empirically before finalizing
2. Explicitly handle (or reject) non-GitHub sources
3. Remove instructions result type until the subsystem design exists
4. Document confidence calibration assumptions
5. Add cost breakdown and timeout budget allocation

**Risk level:** Medium. The architecture is sound, but threshold values could significantly affect user experience (too conservative = missed tools; too permissive = security incidents). The GitHub-only verification creates a gap that should be documented or closed.
