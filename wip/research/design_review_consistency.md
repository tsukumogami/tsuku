# Consistency and Proofreading Review: DESIGN-llm-discovery-implementation.md

## Summary

This review identifies internal contradictions, terminology inconsistencies, unsupported claims, missing content, and broken references in the LLM Discovery Implementation design document.

---

## 1. Contradictions Found

### 1.1 Threshold Logic Description vs. Implementation (Lines 324-334 vs. 683-708)

**Contradiction:** The Decision 4 section states:
> "1. Confidence >= 70 is REQUIRED (gates all other checks)
> 2. THEN apply OR logic: passes if stars >= 50 OR downloads >= 1000"

But the `passesThresholds()` implementation in lines 683-708 has an additional condition not mentioned in Decision 4:

```go
// Very strong signals can override threshold requirements
if metadata != nil && metadata.Stars >= 500 {
    return true
}
```

This creates a path where a 500+ star repo passes even if confidence < 70 was already checked and should have failed. Looking more carefully, the confidence check is first and returns false early, so this code path is unreachable given the current ordering. However, the text in lines 325-329 doesn't mention the 500-star "very strong signal" override at all, making the implementation appear to have secret logic.

**Fix needed:** Add the 500-star override to the decision rules in lines 325-329, or remove it from the implementation code if it's redundant (which it appears to be since confidence is checked first).

### 1.2 Frontmatter Decision vs. Body Content Mismatch (Lines 9-15 vs. 392-409)

**Contradiction:** The frontmatter `decision:` says:
> "Expose web_search as a tool: Cloud LLMs use their native search capability; local LLMs use a tsuku-provided handler backed by DuckDuckGo."

But the Decision Outcome section (line 394) says:
> "Chosen: Quality-metric-driven LLM discovery with Claude native search and deterministic decision algorithm"

The frontmatter correctly describes the provider-transparent approach, but the Decision Outcome summary specifically names "Claude native search" which contradicts the provider-transparent architecture. The summary should mention the tool-based approach, not specifically Claude.

**Fix needed:** Line 394 should read something like "Quality-metric-driven LLM discovery with provider-transparent web search tool and deterministic decision algorithm"

### 1.3 Scope: In-scope vs. Required Subsystem Designs (Lines 73-87 vs. 89-108)

**Contradiction:** Line 81 lists as in-scope:
> "Non-deterministic result handling (requires separate subsystem design)"

But lines 89-108 then say:
> "This design identifies building blocks that require their own tactical designs. LLM Discovery is not complete until these subsystems are designed and implemented."

If non-deterministic handling requires a separate design, it shouldn't be listed as in-scope for THIS design. The scope section mixes things being designed here with things being deferred.

**Fix needed:** Move "Non-deterministic result handling" from in-scope to out-of-scope with a note that it's covered by the Required Subsystem Designs section, OR remove the Required Subsystem Designs section and incorporate the non-deterministic builder as a future phase within this design's scope.

### 1.4 DDG as Default vs. Trade-offs (Lines 420-426)

**Inconsistency:** The Trade-offs section (line 420) says:
> "DuckDuckGo dependency: Web search relies on DDG's HTML endpoint continuing to work."

But this only applies to local LLMs. The architecture states Cloud LLMs use native search. The trade-off should clarify this is only for local LLMs.

**Fix needed:** Line 420 should say "DuckDuckGo dependency (local LLMs only): ..."

---

## 2. Terminology Inconsistencies

### 2.1 "Builder" vs. "Result Type" Confusion (Lines 257-276)

The schema in Decision 2 uses `result_type` values of `builder` and `instructions`:
> "| Result Type | Example | Routing |
> | `builder` | `github:stripe/stripe-cli` | Existing builder pipeline |
> | `instructions` | Install page URL + text | Future: instruction-following builder |"

But throughout the document, "builder" is also used to mean the existing builder infrastructure (github, npm, cargo builders). This overloads the term confusingly - is "builder" a result type or a component?

**Fix needed:** Consider renaming the result type to something like `direct_source` or `mappable_source` to distinguish from the builder components.

### 2.2 "Deterministic" Overloaded Meaning (Multiple Locations)

The term "deterministic" is used in two different senses:
1. Lines 252-277: "Deterministic sources" meaning mappable to existing builders
2. The `--deterministic-only` flag meaning "no LLM usage"

These are related but distinct concepts. A github builder result is "deterministic" per the schema but the github builder itself `RequiresLLM()` for recipe generation.

**Fix needed:** Add a clarifying note that "deterministic sources" in the schema context means "sources that map to existing builders" while `--deterministic-only` means "avoid LLM usage entirely."

### 2.3 "Threshold" vs. "Quality Filter" (Lines 315-344)

Decision 4 is titled "Decision Algorithm" but describes thresholds. The term "quality filter" appears in the Key Insight section (lines 61-68) but "QualityFilter" appears only once in the frontmatter. The relationship between:
- Quality metrics
- Quality filtering
- Threshold filtering
- DiscoveryThreshold type

...is not clearly explained. Are these the same concept with different names?

**Fix needed:** Consistently use either "threshold filtering" or "quality filtering" and explain the relationship to the ecosystem probe's QualityFilter pattern.

### 2.4 "Session" vs. "Resolver" (Lines 357-376)

Decision 5 creates `LLMDiscoverySession` but the document elsewhere refers to `LLMDiscovery` as a resolver stage. The session is the implementation while the resolver is the interface, but this distinction isn't clearly drawn.

**Fix needed:** Add a clarifying sentence like "The `LLMDiscovery` resolver delegates to `LLMDiscoverySession` which manages the LLM conversation."

---

## 3. Unsupported Claims

### 3.1 Threshold Values (Lines 336-344)

The threshold justifications cite testing data that isn't available:
> "Below 70%, false positive rate in testing exceeded 15%"

What testing? This is a proposed design - no testing has occurred. The upstream design (DESIGN-discovery-resolver.md) lists these as uncertainties (line 243):
> "False positive rate: Threshold values are estimates."

**Fix needed:** Change "in testing exceeded 15%" to "is expected to exceed 15% based on initial estimates" or similar wording acknowledging these are projections.

### 3.2 Context Window Limits (Lines 389-390)

> "Local models have smaller context windows (~4K tokens)"

This is outdated. Many local models now support 8K-128K+ contexts. The 4K figure seems arbitrary.

**Fix needed:** Either cite specific models this targets or use more general language like "some local models have limited context windows."

### 3.3 Cost Claims (Line 1089)

> "$10/1K search cost (minimal at expected volume)"

No source or calculation is provided for this figure. The Success Criteria section (line 118) says "<$0.05 per discovery" but doesn't explain how this relates to the $10/1K claim.

**Fix needed:** Either remove the specific cost claim or provide the calculation.

---

## 4. Missing Content or Broken References

### 4.1 Deferred Schema Design (Line 277)

> "Deferred to implementation: The exact schema for `extract_source` tool and result types will be designed during Phase 2"

But the Solution Architecture section (lines 613-622) then shows schema details. This is inconsistent - is the schema deferred or specified?

**Fix needed:** Either remove the "deferred" statement or mark the Solution Architecture schema as "preliminary" or "illustrative."

### 4.2 Missing Integration Test Strategy

The Success Criteria (lines 110-122) define metrics but don't explain how they'll be measured during development. The Implementation Approach phases don't mention integration testing.

**Fix needed:** Add a note about how accuracy, latency, and confirmation rate will be validated before launch.

### 4.3 Missing llm_quality.go (Phase 4)

Phase 4 (lines 921-933) lists:
> "**Files:**
> - `internal/discover/llm_quality.go`"

But the Components section (lines 443-455) doesn't list this file. It lists `llm_verify.go`, `llm_confirm.go`, `llm_sanitize.go` but not `llm_quality.go`.

**Fix needed:** Add `llm_quality.go` to the Components file list.

### 4.4 SearchProvider Not Listed in Components (Lines 443-455)

The components section lists `search.go` but describes it as "SearchProvider interface (for local LLM tool handler)" - it should clarify this is only used for local LLMs, not Cloud LLMs.

**Fix needed:** Add "(local LLMs only)" annotation to search-related files.

### 4.5 Phase Coverage Gap (Lines 849-999)

The Implementation Approach has 8 phases, and the Solution Architecture describes various components. Cross-checking:

- **Covered:** Tool definitions (Phase 1), DDG handler (Phase 1), Core session (Phase 2), Verification (Phase 3), Thresholds (Phase 4), Confirmation (Phase 5), Alternative providers (Phase 6), Telemetry (Phase 7)
- **Missing from phases:** Fork detection logic (mentioned in lines 673-679 but not assigned to a phase)

**Fix needed:** Fork detection should be assigned to Phase 3 (Verification) or Phase 4 (Quality Thresholds).

### 4.6 Error Types Not Fully Covered (Lines 773-807)

The Error Types section defines:
- TimeoutError
- BudgetError
- VerificationError
- ConfirmationDeniedError

The Error Handling table then lists these conditions:
- "No LLM provider configured" -> returns nil, nil (not an error type)
- "Fork detected" -> None (handled differently)
- "Low confidence (<70)" -> None (handled differently)

But there's no error type for:
- Search failures (DDG down, API errors)
- Parse failures (LLM returns unparseable response)
- Rate limit errors (distinct from BudgetError?)

**Fix needed:** Either add SearchError and ParseError types, or explain why these cases are handled via the generic error interface.

---

## 5. Specific Fixes Needed

### High Priority (Contradictions)

1. **Line 394:** Change "Claude native search" to "provider-transparent web search tool"
2. **Line 81:** Move "Non-deterministic result handling" to out-of-scope or clarify its status
3. **Lines 325-329:** Add the 500-star override to the decision rules OR remove it from the implementation if redundant

### Medium Priority (Clarity)

4. **Line 420:** Add "(local LLMs only)" to the DDG dependency trade-off
5. **Lines 257-276:** Rename `builder` result type to avoid confusion with builder components
6. **Line 277 vs. 613-622:** Reconcile the "deferred schema" statement with the actual schema shown
7. **Lines 443-455:** Add `llm_quality.go` to the Components list
8. **Throughout:** Add a terminology note distinguishing "deterministic sources" from `--deterministic-only`

### Low Priority (Polish)

9. **Lines 336-344:** Soften the "testing" claims to "estimates" or "projections"
10. **Line 389-390:** Update the 4K context limit claim to be less specific
11. **Line 1089:** Remove or justify the "$10/1K search cost" claim
12. **Lines 849-999:** Assign fork detection to a specific implementation phase

---

## 6. Cross-Reference Check with Upstream Design

Comparing DESIGN-llm-discovery-implementation.md against DESIGN-discovery-resolver.md:

### Consistent Elements
- Three-stage resolver chain (registry, ecosystem, LLM)
- 15-second timeout for LLM discovery
- Quality threshold filtering
- GitHub API verification
- User confirmation requirement
- Defense layers (HTML stripping, URL validation)

### Potential Inconsistencies

1. **Registry Size:** Upstream says "~500 entries" (line 14) but this design says "~800 entries" (line 47). Which is correct?

   **Fix needed:** Reconcile the registry size claim. The upstream design's Implementation Issues table (line 36) mentions "~500 entries" while the parent design says "500" consistently. This design should use 500 or explain why it's now 800.

2. **Timeout Budget:** Upstream says "LLM discovery under 15 seconds" (line 158). This design says "P95 < 10s, P99 < 15s" (line 117). These are compatible but could be stated more consistently.

3. **The "extract_source" tool:** The upstream design (line 453) mentions "Structured JSON output from LLM (`extract_source` tool call)" which matches this design. Consistent.

---

## 7. Completeness Check

### Frontmatter vs. Body Alignment

| Frontmatter Element | Body Coverage | Status |
|---------------------|---------------|--------|
| problem statement | Lines 43-55 | Complete |
| decision: quality-metric-driven | Lines 56-68, 309-350 | Complete |
| decision: web_search as tool | Lines 135-240 | Complete |
| decision: GitHub verification | Lines 285-306, 649-679 | Complete |
| decision: user confirmation | Lines 405-406, 711-731 | Complete |
| rationale: unified architecture | Lines 139-172 | Complete |
| rationale: defense-in-depth | Lines 1001-1073 | Complete |

### Phases Cover Architecture

| Architecture Component | Implementation Phase | Status |
|-----------------------|---------------------|--------|
| SearchProvider | Phase 1 | Covered |
| DDGSearcher | Phase 1 | Covered |
| LLMDiscoverySession | Phase 2 | Covered |
| llm_sanitize.go | Phase 3 | Covered |
| llm_verify.go | Phase 3 | Covered |
| llm_quality.go | Phase 4 | Covered (but missing from Components list) |
| llm_confirm.go | Phase 5 | Covered |
| TavilySearcher, BraveSearcher | Phase 6 | Covered |
| Telemetry integration | Phase 7 | Covered |
| Non-deterministic builder | Phase 8 | Covered (marked as blocked) |

### Error Types Cover Error Handling

| Error Condition | Error Type | Status |
|-----------------|------------|--------|
| 15-second timeout | TimeoutError | Covered |
| Budget exhausted | BudgetError | Covered |
| GitHub 404 | VerificationError | Covered |
| GitHub rate limit | VerificationError (retryable) | Covered |
| Archived repository | VerificationError | Covered |
| User denies | ConfirmationDeniedError | Covered |
| Fork detected | (no error, warning) | Covered |
| Low confidence | (no error, present options) | Covered |
| No LLM provider | nil, nil | Covered |
| Search failure | ??? | NOT COVERED |
| LLM parse failure | ??? | NOT COVERED |

---

## Conclusion

The design document is largely well-structured and internally consistent. The main issues are:

1. **One significant contradiction:** The frontmatter and Decision Outcome use "Claude native search" when the design is actually provider-transparent
2. **Terminology overloading:** "builder" and "deterministic" are used in multiple senses
3. **Registry size discrepancy:** 500 vs. 800 entries needs reconciliation
4. **Missing error types:** Search and parse failures need explicit handling
5. **Threshold claims:** Testing data cited doesn't exist yet

Addressing these issues would strengthen the document's coherence and make implementation clearer.
