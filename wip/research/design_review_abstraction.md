# Abstraction Level Review: DESIGN-llm-discovery-implementation.md

## Overall Assessment

The design document is at an **appropriate abstraction level for a tactical implementation design**, with some sections that over-specify and a few that under-specify. It successfully separates architectural decisions (which need to be firm) from implementation details (which should remain flexible).

**Overall Rating: 7/10** - Good balance, but some adjustments would improve implementer flexibility and reduce unnecessary lock-in.

---

## Sections That Are TOO SPECIFIC

### 1. Go Code Examples with Full Function Signatures

**Problem:** The document includes detailed Go struct definitions and function implementations throughout (lines 143-189, 317-329, 464-489, 494-517, 524-540, 546-568, 596-621, 652-669, 683-708, 742-758).

**Examples:**
```go
type LLMDiscoverySession struct {
    provider     llm.Provider
    messages     []llm.Message
    systemPrompt string
    tools        []llm.ToolDef
    totalUsage   llm.Usage
    genCtx       *discoveryContext
    timeout      time.Duration // 15 seconds
}
```

**Impact:** These examples lock implementers into specific struct field names and types. The `genCtx *discoveryContext` field, for example, may not even be needed. The existing `HomebrewSession` uses `genCtx *homebrewGenContext`, which is builder-specific. LLM Discovery may need different context.

**Recommendation:** Replace full struct definitions with descriptions of required capabilities:
- "LLM Discovery Session needs: provider access, message history for multi-turn, usage tracking, timeout enforcement"
- Keep the data flow diagrams but remove struct definitions

### 2. File Layout in Solution Architecture

**Problem:** Lines 443-455 specify exact file names:
```
internal/discover/
├── search.go             # SearchProvider interface (for local LLM tool handler)
├── search_ddg.go         # DuckDuckGo HTML scraper (default for local)
├── search_tavily.go      # Tavily API provider (optional)
├── search_brave.go       # Brave Search API provider (optional)
├── llm_discovery.go      # LLMDiscoverySession implementation
├── llm_tools.go          # Tool definitions (web_search, extract_source)
├── llm_tool_handler.go   # Tool call handlers (web_search for local LLMs)
├── llm_verify.go         # GitHub API verification
├── llm_confirm.go        # User confirmation flow
└── llm_sanitize.go       # HTML stripping, URL validation
```

**Impact:** Forces artificial file boundaries. Implementers may prefer different organization (e.g., combining `llm_verify.go` and `llm_confirm.go`, or putting all tool definitions with handlers).

**Recommendation:** Remove the explicit file list. State the required components without dictating file organization.

### 3. Threshold Values (70, 50, 1000)

**Problem:** Lines 318-343 specify exact threshold values with detailed justifications:
- `MinConfidence: 70`
- `MinStars: 50`
- `MinDownloads: 1000`

**Impact:** While the rationale is sound, these are empirical values that will need tuning. The document acknowledges this ("thresholds are initial estimates") but then includes them in the design rather than leaving them to implementation/configuration.

**Recommendation:**
- State the pattern: "Confidence gates all checks (AND logic); quality signals use OR logic"
- State the heuristics: "Reject low-star repos to filter fly-by-night projects"
- Defer specific values to implementation: "Initial values will be determined during implementation and tuned via telemetry"

### 4. DDG HTML Parsing Details

**Problem:** Lines 527-540 specify implementation details:
```go
url := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
// Parse: <a class="result__a"> for title/URL, <a class="result__snippet"> for description
```

**Impact:** DDG's HTML structure may change. The specific CSS classes are implementation details that shouldn't be in a design doc.

**Recommendation:** State "DuckDuckGo HTML endpoint scraping" as the approach, but leave parsing details to implementation.

### 5. Error Types with Exact Struct Definitions

**Problem:** Lines 776-793 define exact error types with field names:
```go
type TimeoutError struct{ Duration time.Duration }
type BudgetError struct{ Limit, Used float64 }
type VerificationError struct{ Reason string }
type ConfirmationDeniedError struct{}
```

**Recommendation:** State the error categories needed (timeout, budget, verification, user rejection) without specifying exact struct shapes.

---

## Sections That Are TOO VAGUE

### 1. extract_source Tool Schema

**Problem:** Lines 613-620 defer the schema entirely:
> "Exact schema to be designed during Phase 2 implementation."

**Impact:** The schema is a critical architectural decision. It determines:
- What quality signals are captured
- How deterministic vs. non-deterministic results are distinguished
- What evidence/reasoning fields are available for verification

The existing ecosystem probe (`ProbeResult` in `internal/builders/probe.go`) has a well-defined schema. LLM discovery's output schema is equally important.

**Recommendation:** Define the schema at a structural level:
- Required fields for builder-mappable results (builder, source, confidence, evidence)
- Required fields for instruction results (url, instruction_text, platform)
- Leave exact JSON property names to implementation

### 2. Non-Deterministic Builder

**Problem:** Lines 89-109 acknowledge this is a critical dependency but provide minimal detail:
> "This subsystem needs its own design because it involves... LLM-driven code/command generation (security-sensitive)"

**Impact:** Without understanding how instruction-based results will be processed, the design can't fully specify what information LLM Discovery needs to capture. The `instructions` result type is mentioned but its schema is undefined.

**Recommendation:** Add a brief sketch of the non-deterministic builder's input requirements. What must LLM Discovery provide? URL? Full page content? Structured instruction steps? This doesn't need a full design, but the interface between the two systems should be clearer.

### 3. Multi-Turn Conversation Strategy

**Problem:** Lines 625-647 describe a conversation flow, but don't specify:
- When should the LLM do multiple searches?
- How many tool calls are acceptable?
- What triggers termination?

The document mentions "max 3 turns" (line 562) but doesn't define what constitutes a turn or when to give up.

**Recommendation:** Add decision criteria:
- Max tool calls per discovery (not just turns)
- When to terminate early (e.g., if two searches return similar results)
- Handling of LLM that doesn't call extract_source

### 4. GitHub Rate Limit Handling

**Problem:** Line 802-803 mentions "higher confirmation bar" for rate limit cases but doesn't specify what this means:
> "GitHub API rate limit | VerificationError (retryable) | Soft error, skip verification, higher confirmation bar"

**Impact:** What does "higher confirmation bar" mean operationally? Does it require manual confirmation even with `--yes`? Does it display additional warnings?

**Recommendation:** Specify the UX for rate-limited verification.

---

## Decisions Made at Wrong Level

### 1. 15-Second Timeout

**Location:** Throughout (lines 79, 131, 375, 559)

**Issue:** This is presented as an architectural decision, but it's really a configuration value. The design should specify that discovery has a timeout to prevent blocking interactive use, but the specific value is operational.

**Recommendation:** State "configurable timeout with reasonable default" rather than locking in 15 seconds.

### 2. Provider Selection Priority for Local LLMs

**Location:** Lines 225-228, 958-965

**Issue:** The env var priority (TAVILY > BRAVE > DDG) is configuration policy, not architecture.

**Recommendation:** State "configurable search provider with sensible defaults" and move priority to implementation.

### 3. User-Agent String

**Location:** Line 534

**Issue:** Including a specific browser User-Agent string in a design doc is too specific.

**Recommendation:** Remove. State "browser-like headers" if needed, but implementation handles specifics.

---

## Decisions Made at Right Level

### 1. Web Search as a Tool (Decision 1)

**Location:** Lines 135-246

**Analysis:** This is exactly the right level. It establishes:
- The architectural pattern (tool-based, provider-transparent)
- The key insight (Cloud LLMs use native, local LLMs use our handler)
- The rationale for alternatives rejected

This is a real architectural decision that shapes the entire implementation.

### 2. Quality-Metric-Driven Decision Algorithm (Decision 4)

**Location:** Lines 309-350

**Analysis:** The pattern of "LLM extracts, algorithm decides" is an architectural decision. The specific threshold values are too detailed (see above), but the decision to separate extraction from selection is exactly right.

### 3. GitHub-Only Verification (Decision 3)

**Location:** Lines 285-307

**Analysis:** Correctly identifies that GitHub verification is critical while ecosystem verification is handled by the existing probe. This is a scoping decision that affects implementation complexity.

### 4. Dedicated DiscoverySession (Decision 5)

**Location:** Lines 352-379

**Analysis:** The decision to create a new session type rather than reusing BuildSession is correct and well-justified. The interface differences (Resolver returns DiscoveryResult, BuildSession returns Recipe) are fundamental.

### 5. Defense-in-Depth Layers

**Location:** Lines 735-770

**Analysis:** The six defense layers are appropriately architectural:
1. Input normalization (references existing)
2. HTML stripping (new)
3. URL validation (new)
4. GitHub API verification (new)
5. User confirmation (new)
6. Sandbox validation (existing)

These establish what protections exist without over-specifying implementation.

---

## Recommendations for Adjustment

### High Priority

1. **Remove struct definitions** - Replace with capability descriptions
2. **Add extract_source schema** - Define at structural level (required vs optional fields)
3. **Define non-deterministic builder interface** - What does LLM Discovery provide?
4. **Generalize threshold values** - Keep the pattern, defer the numbers

### Medium Priority

5. **Remove file layout** - Let implementers organize code
6. **Clarify rate-limit UX** - What happens when GitHub verification is skipped?
7. **Define conversation termination** - When does multi-turn give up?

### Low Priority

8. **Remove User-Agent string** - Implementation detail
9. **Generalize timeout** - "Configurable with default" rather than "15 seconds"
10. **Remove DDG class names** - Parsing is implementation detail

---

## Comparison with Parent Design

The parent design (DESIGN-discovery-resolver.md) operates at a higher level:
- Defines the 3-stage resolver chain (registry, ecosystem, LLM)
- Specifies the Resolver interface
- Defers LLM implementation details to this design

This child design appropriately fills in those details while maintaining some over-specification in Go code examples. The relationship between the two documents is well-structured.

---

## Summary

| Aspect | Assessment |
|--------|------------|
| Architectural decisions | Mostly appropriate |
| Go code examples | Too specific - remove struct definitions |
| Threshold values | Too specific - keep patterns, defer values |
| File organization | Too specific - remove |
| Tool schema | Too vague - add structural definition |
| Non-deterministic interface | Too vague - add sketch |
| Security model | Appropriate level |
| Decision rationale | Well-documented |
| Implementer flexibility | Moderate - could improve |

The design document makes the right high-level decisions (tool-based search, quality-metric filtering, GitHub verification) but over-specifies implementation details (struct fields, file names, threshold values). With the adjustments above, it would provide clear direction while preserving implementer flexibility.
