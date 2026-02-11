# Design Review Round 2: Abstraction Level Analysis

**Document:** DESIGN-llm-discovery-implementation.md
**Reviewer Role:** Abstraction Level Reviewer
**Review Date:** 2026-02-11

## 1. Assessment of Generalization Changes

### 1.1 Go Struct Definitions to Capability Descriptions

**Verdict: Just Right**

The document now uses capability descriptions like:

> "**Required capabilities:**
> - LLM provider access (via Factory)
> - Message history for potential multi-turn conversations
> - Tool definitions (web_search, extract_source)
> - Usage tracking for cost accounting
> - Timeout enforcement (configurable, default ~15 seconds)"

This is appropriate for a tactical design. The implementer knows WHAT the session needs without being locked into specific field names or types. The document retains essential Go code for tool definitions and interface contracts (SearchProvider, WebSearchHandler) which are necessary for API compatibility.

### 1.2 Threshold Value Generalization

**Verdict: Too Far - Needs Minor Adjustment**

The original thresholds (confidence >= 70, stars >= 50, downloads >= 1000) were removed and replaced with "Threshold design principles." However, the document is inconsistent:

- Line 391-392 still mentions: "confidence >= 70 AND stars >= 50"
- Line 663 still mentions: "Low confidence (<70)"
- The "Threshold design principles" table (lines 329-335) is appropriately abstract

**Issue:** The document tries to be abstract about thresholds but leaks specific values. This creates confusion: are implementers expected to use 70/50 or determine their own values?

**Recommendation:** Either:
1. Fully commit to deferring thresholds and remove the leaked values (lines 391-392, 663)
2. Or acknowledge these as reasonable starting defaults while noting they may be tuned

### 1.3 extract_source Schema Deferral

**Verdict: Appropriate**

The document states:

> "Deferred to implementation: The exact schema for `extract_source` tool and result types will be designed during Phase 2 (Core Discovery Session)."

This is reasonable because:
- The two result types are clearly defined (builder vs. instructions)
- Required fields are listed (confidence, evidence, reasoning, warnings)
- The architectural decision (two result types) is firm; only field-level details are deferred
- Phase 2 is early enough that the schema will be defined before dependent work begins

### 1.4 File Layout Removal

**Verdict: Just Right**

The original design likely had explicit file paths like `internal/discover/session.go`. The current document uses component tables instead:

> | Component | Purpose |
> |-----------|---------|
> | SearchProvider interface | Abstraction for web search (local LLMs only) |
> | DDG/Tavily/Brave searchers | SearchProvider implementations |

This gives implementers freedom to organize files while ensuring all components are built.

## 2. Sections That Need Adjustment

### 2.1 Threshold Inconsistency (Minor Fix Required)

**Location:** Lines 391-392 in Decision Outcome Summary, Line 663 in Error Handling

**Current text (line 391-392):**
```
3. **Threshold Filtering**: Applies deterministic thresholds (confidence >= 70 AND stars >= 50)
```

**Problem:** Contradicts the generalized "Threshold design principles" section.

**Recommended fix:** Change to:
```
3. **Threshold Filtering**: Applies deterministic thresholds (confidence gate + quality signals per design principles)
```

**Also line 663:** Change "Low confidence (<70)" to "Low confidence (below threshold)"

### 2.2 Code Examples Are Well-Calibrated

The remaining code examples are appropriate:
- `WebSearchTool` definition (line 143-156): Needed for API compatibility
- `WebSearchHandler` interface (lines 177-189): Clear contract
- `SearchProvider` interface (lines 192-219): Essential for alternative implementations
- System prompt (lines 481-500): Provides crucial guidance for prompt engineering

These are architectural decisions, not implementation details.

### 2.3 Implementation Phases

**Verdict: Just Right**

The phases provide clear deliverables without micromanaging:
- Phase 1: "Implement DDG scraper with HTML parsing" (what, not how)
- Phase 2: "Create LLMDiscoverySession with conversation management"
- Phase 3: "Implement HTML stripping for search results"

Each phase has clear scope and dependencies.

## 3. Areas Still Too Specific

### 3.1 DDG HTML Endpoint Details

**Location:** Lines 211-212, 465

The document specifies:
- `html.duckduckgo.com/html/?q={query}` endpoint
- "Parse HTML: extract result__a (title/URL), result__snippet (description)"

**Assessment:** This is borderline. The endpoint URL is a fact (not a design choice), but CSS selectors (`result__a`, `result__snippet`) are implementation details that could change.

**Recommendation:** Keep the endpoint URL but remove CSS selector specifics. Change to:
> "**Endpoint**: `html.duckduckgo.com/html/?q={query}`
> **Parsing**: Extract titles, URLs, and snippets from HTML response"

The current text at line 465 is already correctly abstract.

### 3.2 Fork Comparison Logic

**Location:** Lines 572-573

> "3. Compare stars: if parent has 10x more stars, suggest parent instead"

The "10x" multiplier is overly specific for a design document. This is a tunable heuristic.

**Recommendation:** Change to:
> "3. Compare activity/stars: if parent is significantly more active/popular, suggest parent instead"

## 4. Areas That May Be Too Vague

### 4.1 Rate Limit Handling

**Location:** Line 659

> "GitHub API rate limit | VerificationError (retryable) | Soft error, skip verification, higher confirmation bar"

"Higher confirmation bar" is vague. What does this mean in practice?

**Recommendation:** Clarify:
> "Soft error, skip verification, display warning that verification was skipped, require explicit 'yes' even in non-interactive mode"

### 4.2 Budget Integration

**Location:** Phase 7, Lines 806-809

The budget integration section is thin:
> "- Add cost tracking to discovery session
> - Integrate with existing budget tracking infrastructure"

This could benefit from mentioning what the existing infrastructure is (daily limits? per-query limits?).

**Assessment:** Acceptable because the design references "existing budget tracking infrastructure" which presumably is documented elsewhere.

## 5. Readiness Verdict

**Ready with Minor Fixes**

The design document is well-balanced between architectural firmness and implementation flexibility. The generalization from round 1 was largely successful. The document:

**Does well:**
- Separates architectural decisions (tool-based search, quality threshold pattern, verification layers) from implementation details
- Provides clear component responsibilities without dictating code structure
- Keeps essential code examples (interfaces, tool schemas) while removing unnecessary ones
- Defers schema details to appropriate implementation phases

**Needs minor adjustments:**
1. **Threshold inconsistency** (lines 391-392, 663): Remove leaked specific values or acknowledge them as defaults
2. **DDG selector specifics** (lines 211-212): Remove CSS class names
3. **Fork comparison heuristic** (line 573): Generalize "10x" to "significantly more"
4. **Rate limit behavior** (line 659): Clarify "higher confirmation bar"

These are all minor text edits that don't affect the architecture.

## Summary Table

| Area | Assessment | Action |
|------|------------|--------|
| Struct to capability descriptions | Just right | None |
| Threshold generalization | Too far, inconsistent | Fix leaked values |
| extract_source deferral | Appropriate | None |
| File layout removal | Just right | None |
| Code examples retained | Appropriate | None |
| Implementation phases | Just right | None |
| DDG implementation details | Slightly too specific | Minor edit |
| Fork comparison | Too specific | Minor edit |
| Rate limit handling | Slightly vague | Clarify |

**Overall:** The design strikes the right balance. Architectural decisions are firm; implementation details are flexible. The identified issues are cosmetic inconsistencies rather than structural problems.
