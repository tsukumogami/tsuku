# Architecture Review Round 2: DESIGN-llm-discovery-implementation.md

**Reviewer Role**: Architecture Reviewer
**Review Date**: 2026-02-11
**Review Type**: Second Round (Post-Revision)

---

## Changes Assessed

The design was revised to address first-round feedback:

| First-Round Issue | Change Made | Assessment |
|-------------------|-------------|------------|
| Go struct definitions were over-specified | Replaced with capability descriptions | Successful |
| File layout was overly prescriptive | Replaced with component table (lines 434-443) | Successful |
| Specific threshold values (70/50/1000) | Generalized to "configurable, default ~15s" pattern | Successful |
| Frontmatter/body contradiction | Fixed - frontmatter now aligns with body | Successful |
| Non-deterministic scope unclear | Clarified as out-of-scope with explicit subsystem design requirement | Successful |

---

## Assessment of Changes

### 1. Over-Specification Addressed Successfully

**Before**: The design included concrete Go type definitions like:
```go
type LLMDiscoverySession struct {
    provider     llm.Provider
    messages     []llm.Message
    ...
}
```

**After**: Replaced with capability descriptions (lines 469-476):
> The discovery session orchestrates the LLM conversation:
> 1. Apply timeout
> 2. Build initial message
> 3. Run conversation loop
> ...

This is the right level of abstraction. It tells implementers what the component must do without constraining how. The existing HomebrewSession in `internal/builders/homebrew.go` serves as a working example for session patterns.

**Verdict**: Improvement achieved.

### 2. Component Table is Clear (Lines 434-443)

The component table format:

| Component | Purpose |
|-----------|---------|
| SearchProvider interface | Abstraction for web search (local LLMs only) |
| DDG/Tavily/Brave searchers | SearchProvider implementations |
| LLMDiscoverySession | Manages LLM conversation for discovery |
| ... | ... |

This is more appropriate than a file layout. It describes responsibilities without mandating package structure. Implementers can organize files based on actual dependencies and test needs.

**Verdict**: Improvement achieved.

### 3. Threshold Generalization Works

**Before**: Hardcoded values like `confidence >= 70`, `stars >= 50`

**After**: Lines 315-336 now describe threshold design principles:

> | Signal | Purpose | Tuning Approach |
> | Confidence (gate) | Reject uncertain extractions | Set to balance precision vs. recall... |
> | Stars (OR) | Filter low-quality repos | Set to exclude typosquat-risk repos... |

The text "Specific threshold values will be determined during implementation and tuned via telemetry" (line 336) is appropriate for a design document. Implementation can iterate on values.

**Verdict**: Improvement achieved.

### 4. extract_source Schema Still Deferred

The first-round review flagged the deferred schema as **High Severity**. The current design still states (line 277):
> "Exact schema to be designed during Phase 2 implementation."

However, the design now includes more context about what the schema must support (lines 259-276):
- Two result types: `builder` and `instructions`
- Common fields: confidence, evidence, reasoning, warnings
- Builder type: builder name, source arg, quality signals
- Instructions type: URL, instruction text, platform

This is borderline acceptable. The schema details are clearer even if not fully specified. The tool call example in the conversation flow (lines 542-549) shows concrete field names:
```
builder: "github"
source: "stripe/stripe-cli"
confidence: 95
evidence: [...]
```

**Verdict**: Partially addressed. Not blocking, but implementers will need to define the schema early in Phase 2.

---

## Remaining Issues from Round 1

### Issue 1: Session/Resolver Integration (Previously Lines 556-567)

**Status**: Still Unclear

The design describes `LLMDiscoverySession` capabilities (lines 469-476) and shows it in the data flow (lines 679-699), but doesn't show how `LLMDiscovery.Resolve()` creates and uses sessions.

The data flow diagram shows:
```
LLMDiscovery.Resolve("stripe-cli")
    |
    ├── Create LLMDiscoverySession with tools: [web_search, extract_source]
```

But no pseudocode shows the integration. This is a minor gap since the pattern is obvious from HomebrewBuilder:

```go
// Implied pattern (not in design):
func (d *LLMDiscovery) Resolve(ctx context.Context, name string) (*DiscoveryResult, error) {
    session, err := d.newSession(ctx, name)
    if err != nil {
        return nil, nil // soft miss
    }
    defer session.Close()
    return session.Discover(ctx)
}
```

**Severity**: Low. Pattern is clear from existing code.

### Issue 2: Interim Handling of Instructions Results

**Status**: Addressed

Lines 95-99 now explicitly state:
> **Non-Deterministic Builder** is the critical dependency. When LLM discovery finds install instructions instead of a builder-mappable source, this subsystem...

And lines 85-86:
> Non-deterministic result handling (see Required Subsystem Designs below)

The design defers `instructions` handling to a separate design. This is acceptable because:
1. It's explicitly out of scope
2. Phase 8 is documented as blocked by that design
3. For now, discoverable tools are those with builder-mappable sources

**Severity**: Resolved via explicit scoping.

### Issue 3: DDG Fallback Chain

**Status**: Partially Addressed

Lines 462-465 describe DDG scraper:
> - Endpoint: `html.duckduckgo.com/html/?q={query}`
> - Requirements: Browser-like headers to avoid bot detection
> - Parsing: Extract titles, URLs, and snippets from HTML response
> - Limitations: May be rate-limited; HTML structure may change

Uncertainties section (line 375) notes:
> Mitigation: the lite POST endpoint is a fallback, and we can add API-based search later if needed.

The design doesn't include explicit fallback chain logic in Phase 1, but this is an implementation detail. The SearchProvider interface allows swapping implementations.

**Severity**: Low. Implementation can add fallback logic.

---

## New Issues Introduced by Changes

### New Issue 1: Web Search Tool Shown Twice

The `web_search` tool definition appears in two places:
1. Lines 143-155 (Decision 1 explanation)
2. Lines 504-517 (Tool Definitions section)

Both show the same Go struct definition. This creates a maintenance burden if the schema changes.

**Severity**: Low (documentation consistency issue, not architectural).

**Recommendation**: Reference one location from the other, or remove the duplicate.

### New Issue 2: Confidence Threshold Discussion Removed Prematurely

The first-round review noted a bug in the threshold logic pseudocode (redundant Stars >= 500 check). The revised design removed the pseudocode entirely, replacing it with:

> 1. Confidence is REQUIRED above a minimum threshold (gates all other checks)
> 2. THEN apply OR logic: passes if stars >= threshold OR downloads >= threshold

This is correct and cleaner. However, the design loses the explicit statement that "forks never auto-pass." This was in the previous pseudocode.

Lines 584-585 do state:
> 2. **Fork check**: Forks never auto-pass - always require explicit confirmation

**Severity**: None. The information is preserved in the threshold logic section.

---

## Pattern Consistency Check

Comparing to existing codebase patterns:

| Pattern | Codebase Example | Design Alignment |
|---------|------------------|------------------|
| Tool definitions | `homebrew.go:1711-1768` | Matches (map[string]any style) |
| Session with messages | `HomebrewSession` (lines 99-127) | Design follows same structure |
| Factory for providers | `llm.Factory` | Design explicitly uses Factory |
| Quality filtering | `quality_filter.go` | Extends pattern appropriately |
| Error types | `DeterministicFailedError` | Design follows same approach |

**Verdict**: Patterns are consistent with codebase.

---

## Component Boundaries and Coupling

The design maintains clear boundaries:

1. **Search Layer** (internal/discover/search/)
   - SearchProvider interface
   - DDG/Tavily/Brave implementations
   - Isolated from LLM layer

2. **Session Layer** (internal/discover/)
   - LLMDiscoverySession
   - Uses LLM via Factory/Provider abstraction
   - Uses Search via SearchProvider abstraction

3. **Verification Layer** (internal/discover/)
   - GitHub API verification
   - Reuses existing HTTP client patterns

4. **Decision Layer** (internal/discover/)
   - Threshold logic
   - Extends QualityFilter pattern

**Coupling Assessment**: Low coupling. Each layer communicates via interfaces. The only tight coupling is between LLMDiscoverySession and its tool handlers, which is appropriate.

---

## Readiness Verdict

### Ready for Approval

The design has addressed the substantive concerns from round 1:
- Over-specification replaced with capability descriptions
- Thresholds generalized appropriately
- Non-deterministic scope clarified
- Component structure defined without file-level prescription

Remaining gaps are minor:
- Session/Resolver integration pattern is implied but not explicit (easily inferred from HomebrewBuilder)
- extract_source schema still deferred but clearer context provided
- DDG fallback is mentioned but not detailed (implementation detail)

The design provides sufficient guidance for implementation while leaving appropriate flexibility for engineering decisions.

### Minor Recommendations (Non-Blocking)

1. **Deduplicate web_search tool definition** - Reference one location from the other
2. **Add integration pseudocode** - One paragraph showing LLMDiscovery.Resolve() -> session lifecycle
3. **Phase 1 scope clarification** - Explicitly note that DDG fallback to lite endpoint is part of Phase 1

These are polish items, not blockers.

---

## Summary

| Assessment Area | Round 1 | Round 2 |
|-----------------|---------|---------|
| Pattern Consistency | 7/10 | 8/10 |
| Component Boundaries | 8/10 | 8/10 |
| Data Flow | 7/10 | 8/10 |
| Extensibility | 6/10 | 7/10 |
| Interface Design | 7/10 | 8/10 |
| **Overall** | **7/10** | **8/10** |

**Final Verdict**: Ready for approval.

The changes successfully addressed over-specification while maintaining clarity. The design is now at the right level of abstraction for a tactical design document: it specifies what components do without constraining implementation details unnecessarily.
