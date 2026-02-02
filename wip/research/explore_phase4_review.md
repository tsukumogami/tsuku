# Design Review: Probe Quality Filtering

## Document Overview

This design addresses a critical correctness issue in tsuku's ecosystem probe where package names resolve to name-squatters on registries instead of the intended tools (e.g., `prettier` resolving to crates.io instead of npm).

## Question 1: Is the problem statement specific enough to evaluate solutions against?

**Assessment: YES, with minor gaps**

The problem statement is concrete and measurable:
- **What's broken**: Ecosystem probe accepts any package that exists, regardless of quality
- **Impact**: Specific examples provided (prettier → crates.io squatter, httpie → crates.io instead of pypi)
- **Root cause**: Static priority ranking combined with lack of quality filtering
- **Evidence**: Quantified metrics for squatters (87 downloads, 3 versions, max v0.1.5)

**Strengths:**
- Falsifiable claim: "prettier should resolve to npm, not crates.io"
- Clear success criteria: Quality filtering should reject low-signal packages
- Specific data points about what registries expose (recent_downloads, version counts)

**Minor gaps:**
- No threshold for "how many tools are affected" - is this 5 tools or 500?
- No performance budget stated (though Decision Driver mentions "no extra latency where possible")
- Doesn't quantify impact severity beyond "wrong results" - is this breaking production workflows or theoretical?

**Recommendation:** Add a scope statement like "Affects approximately 15-20 commonly requested tools based on initial testing" and define acceptable latency increase (e.g., "no more than 200ms added to probe resolution").

## Question 2: Are there missing alternatives we should consider?

**Assessment: One significant gap, several minor ones**

### Missing Alternative: Client-side curation database

**What it is:** Maintain a curated mapping file (e.g., `tool-registry-hints.json`) that explicitly maps known tools to their canonical registries:
```json
{
  "prettier": "npm",
  "httpie": "pypi",
  "ripgrep": "cargo"
}
```

**Why consider it:**
- Solves the specific examples (prettier, httpie) immediately with zero latency
- No heuristic threshold tuning required
- Could be generated from the discovery registry seeding pipeline
- Falls back to quality filtering for tools not in the database

**Why it might be rejected:**
- Requires manual maintenance or automated curation process
- Doesn't scale to long-tail tools
- Could become stale as new tools emerge
- Adds a new file to ship and update

**Verdict:** This should be explicitly considered and rejected (or accepted as complementary) because it's a common pattern in package managers (e.g., Homebrew's tap priority system).

### Other missing alternatives worth brief mention:

1. **Registry-specific deny lists**: Maintain a list of known squatter package names per registry (e.g., "ignore npm package 'python'"). Simpler than quality filtering but requires ongoing maintenance.

2. **Fuzzy name matching with authoritative registry hints**: If tool name contains patterns like "prettier" and matches npm exactly, boost npm's priority. Heuristic-based but could complement quality filtering.

3. **Defer to highest-signal registry first**: Instead of static priority, probe all registries and use the one with the highest quality score. This inverts the current architecture but might produce better results.

4. **Community voting/reporting**: Allow users to report misresolved tools, feeding into either curation database or threshold tuning. Out of scope for MVP but worth acknowledging as future work.

## Question 3: Is the rejection rationale for each alternative specific and fair?

**Assessment: Mostly FAIR, one rationale is underspecified**

### Decision 1: Where to filter

**"Filter inside each Probe()"**
- Rejection: "scatters policy across 7 files, makes thresholds hard to find"
- **Fair**: This is a valid maintainability concern
- **Specific**: Identifies concrete issue (7 files, threshold discoverability)
- **Complete**: Also notes seeding pipeline can't apply different thresholds - this is a strong architectural argument

**"Filter only in the resolver"**
- Rejection: "seeding pipeline would need to duplicate the logic or depend on the discover package in a way that creates circular dependencies"
- **Fair**: Reusability is a stated decision driver
- **Could be more specific**: What circular dependency would be created? The resolver and seeding pipeline are both part of the same system - why would this create a cycle? This needs elaboration.

### Decision 2: What metadata to collect

**"Downloads only"**
- Rejection: "PyPI and Go don't expose downloads, leaving those registries without any filtering"
- **Fair and specific**: Correctly identifies registries that would be unfiltered
- **Complete**: Makes the case that version count fills this gap

**"Full metadata extraction"**
- Rejection: "adds complexity and fragility for marginal benefit"
- **Fair but vague**: What specific complexity? What makes it fragile? Which fields would be parsed?
- **Questionable**: "marginal benefit" is asserted without evidence. Description text, maintainer activity, or README presence might be strong signals. This feels like an assumption rather than analysis.
- **Recommendation**: Be more specific - "Parsing descriptions requires NLP heuristics. Maintainer lists vary by registry format. These add 100+ LOC for unclear signal value."

### Decision 3: How to set thresholds

**"Single universal threshold"**
- Rejection: "registries have wildly different scales"
- **Fair and specific**: Gives examples of scale differences (RubyGems total vs npm weekly)
- **Complete**: Notes some registries don't expose downloads

**"Relative scoring"**
- Rejection: "overengineered for the current problem"
- **Fair**: YAGNI principle is valid
- **Specific enough**: Distinguishes ranking from binary accept/reject
- **Minor concern**: This is somewhat dismissive. Scoring could make thresholds self-tuning. Worth acknowledging that "if thresholds prove hard to tune, revisit scoring approach."

## Question 4: Are there unstated assumptions that need to be explicit?

**Assessment: Several critical assumptions are implicit**

### Unstated Assumption 1: Download counts reflect legitimacy

**Implicit claim:** A package with 100+ downloads is likely legitimate; one with <100 is likely a squatter.

**Why this matters:**
- Malicious actors could inflate downloads through bots
- Legitimate niche tools might have low downloads
- Regional or domain-specific tools might be filtered incorrectly

**What to make explicit:** Add to "Uncertainties" or "Trade-offs":
> "This design assumes download counts are honest signals. A sophisticated attacker could inflate downloads to bypass filtering. We accept this risk because:
> 1. Most squatting is opportunistic, not targeted
> 2. Version count + repository URL provide cross-validation
> 3. The current state (no filtering) is strictly worse"

### Unstated Assumption 2: Registry APIs won't rate-limit the npm downloads endpoint

**Implicit claim:** The npm downloads API can be called on every probe without triggering rate limits.

**Why this matters:**
- If npm rate-limits after 100 requests/hour, the probe would fail for heavy users
- The design doesn't specify retry logic or fallback behavior

**What to make explicit:** In "Implementation Approach" or "Uncertainties":
> "We assume npm's downloads API has similar rate limits to the registry endpoint. If rate limiting becomes an issue, the Probe() should handle 429 responses by returning Downloads=0 and relying on version count filtering."

### Unstated Assumption 3: "Recent downloads" timeframes are comparable across registries

**Implicit claim:** crates.io's "recent downloads," npm's "weekly downloads," and RubyGems "total downloads" can all use thresholds in the same ballpark.

**Why this matters:**
- crates.io's "recent" might be 90 days
- npm's is 7 days
- RubyGems is all-time
- These are not comparable scales, yet the design proposes thresholds of 100, 100, and 1000

**What to make explicit:** In the "Solution Architecture" or threshold table, clarify:
> "Thresholds are calibrated per registry's specific download metric:
> - crates.io: recent_downloads (90-day window) >= 100
> - npm: weekly_downloads (7-day window) >= 100
> - RubyGems: total downloads (all-time) >= 1000"

### Unstated Assumption 4: Version count is a reliable signal

**Implicit claim:** A package with 5+ versions is more likely legitimate than one with 2 versions.

**Why this matters:**
- A squatter could trivially publish 10 empty versions to bypass filtering
- Some legitimate tools have slow release cycles (1 version/year)

**What to make explicit:** In "Rationale" or "Trade-offs":
> "Version count assumes squatters won't bother publishing many versions. This is historically true (most squatters publish 1-3 placeholder versions), but a determined attacker could bypass this. The OR logic (downloads OR version count) means both signals must fail for rejection."

### Unstated Assumption 5: Probe() methods are called sequentially, not cached

**Implicit claim:** Each `tsuku create` invocation makes fresh API calls to all registries.

**Why this matters:**
- If probes are cached, stale quality metadata could persist
- If multiple probes happen in parallel for different tools, the npm downloads API might get hammered

**What to make explicit:** In "Data flow" or "Implementation Approach":
> "Probe results are not cached across invocations. Each `tsuku create` call fetches fresh quality metadata. This ensures up-to-date signals but means high-frequency users will make repeated API calls."

## Question 5: Is any option a strawman (designed to fail)?

**Assessment: One borderline strawman, others are legitimate**

### Borderline strawman: "Full metadata extraction"

**Strawman indicators:**
- Rejection uses subjective language ("overengineered," "marginal benefit")
- No attempt to quantify what would be extracted or how much benefit it provides
- Dismisses without considering hybrid approach (e.g., "parse description IF downloads=0")

**Counter-evidence:**
- The design genuinely doesn't need this complexity for the stated problem
- Downloads + version count demonstrably solve the prettier/httpie examples
- Extending ProbeResult is mentioned as future-compatible

**Verdict:** Borderline. It reads like a strawman because the rejection is vague, but the decision is probably correct. Strengthen the rationale by being specific about what "full metadata" means and why each field doesn't add signal value.

### Not strawmen:

**"Filter inside each Probe()"**
- This is a legitimate design choice (common in OOP patterns)
- Rejection is based on concrete architectural concerns (7 files, seeding reuse)
- Not designed to fail

**"Filter only in the resolver"**
- Also a legitimate choice (centralized logic)
- Rejection is based on reusability concerns
- Not designed to fail, though the circular dependency claim needs elaboration

**"Downloads only"**
- This is actually a simpler approach than the chosen solution
- Rejection is based on coverage (PyPI, Go wouldn't be filtered)
- Not a strawman

**"Single universal threshold"**
- Common approach in filtering systems
- Rejection is based on registry scale heterogeneity (valid concern)
- Not a strawman

**"Relative scoring"**
- Sophisticated approach used in search ranking, spam detection
- Rejection is based on complexity vs. benefit trade-off
- Not designed to fail, but dismissed perhaps too quickly

## Summary: Key Findings

### Strengths of the design

1. **Problem is concrete and measurable**: Specific examples with data
2. **Most alternatives are fairly evaluated**: Rejections are generally justified
3. **Decision drivers are explicit**: Correctness over speed, fail-open for unknowns
4. **Phased implementation plan**: Logical breakdown

### Critical gaps

1. **Missing alternative: Curated tool-registry mappings** - This is a common pattern in package managers and should be explicitly considered (even if rejected)
2. **Unstated assumptions about download count legitimacy** - Design assumes honest signals without addressing inflation risk
3. **Vague rejection of "full metadata extraction"** - Reads like a strawman due to lack of specificity
4. **No explicit handling of rate limiting** - npm downloads API could be rate-limited
5. **Circular dependency claim needs elaboration** - Why would shared QualityFilter create a cycle?

### Recommendations

1. **Add curated mappings alternative** with rejection rationale (or accept as Phase 4 enhancement)
2. **Make download count assumptions explicit** - Address inflation risk, rate limiting, and signal honesty
3. **Strengthen "full metadata" rejection** - List specific fields that would be parsed and why each lacks value
4. **Clarify download metric timeframes** - Document that "recent_downloads" is 90 days for crates.io, 7 days for npm, etc.
5. **Add scope quantification** - How many tools are affected? What's the acceptable latency budget?
6. **Consider hybrid with scoring** - If thresholds prove hard to tune, note that scoring could be revisited

## Verdict

**The design is sound and solves the stated problem, but needs refinement before implementation:**

- Problem statement: **90% complete** (add scope/latency budget)
- Alternatives: **70% complete** (missing curated mappings, rate limiting fallback)
- Rejection rationales: **80% fair** (strengthen "full metadata," clarify circular dependency)
- Assumptions: **60% explicit** (need to surface 5 critical assumptions)
- Strawman check: **One borderline case** (full metadata extraction)

**Overall recommendation: APPROVE with revisions.** Address the missing alternative and unstated assumptions before implementation. The core architectural choice (shared QualityFilter) is correct.
