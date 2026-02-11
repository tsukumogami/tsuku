# Exploration Summary: LLM Discovery with Quality-Driven Decision Making

## Problem (Phase 1)

The LLM discovery stage is a stub that needs implementation to complete the discovery resolver chain. Users without API keys can only find tools in the registry or ecosystem probes. The LLM stage handles the "long tail" of tools, but its implementation is security-sensitive because LLM output directly influences which source gets installed.

## Decision Drivers (Phase 1)

1. **Reuse existing infrastructure**: LLM sessions, tool patterns, and quality metrics already exist
2. **Deterministic decision-making from LLM outputs**: LLM extracts structured data; algorithm picks the winner
3. **Security through defense-in-depth**: HTML stripping, URL validation, GitHub API verification, user confirmation
4. **Web search integration**: Claude native search ($10/1K) is the simplest path
5. **15-second timeout budget**: Per-discovery timeout to prevent runaway sessions
6. **Quality metrics extension**: Build on existing ProbeResult/QualityFilter patterns

## Research Agents Deployed

1. **Quality Metrics Analysis**: Examined ProbeResult schema, QualityFilter thresholds, per-ecosystem signals
2. **LLM Infrastructure Analysis**: Examined session patterns, tool definitions, provider abstraction
3. **Web Search Tool Patterns**: Examined existing tool schemas, content handling patterns
4. **Prompt Injection Defense**: Examined HTML stripping, URL validation, defense layers
5. **Structured Output Schema**: Examined extract_pattern/extract_recipe schemas, design for extract_source
6. **GitHub API Verification**: Examined existing validator, metadata extraction, rate limits
7. **Deterministic Decisions**: Examined quality filter logic, priority ranking, OR-threshold pattern
8. **Confirmation UX**: Examined existing confirmation patterns, metadata display, non-interactive handling
9. **Web Search Providers**: Compared Claude native, Gemini native, Tavily, SerpAPI
10. **Budget/Timeout Handling**: Examined cost tracking, circuit breaker, state persistence
11. **Session Patterns**: Examined HomebrewSession, GitHubReleaseSession, BuildSession interface

## Key Insights

### 1. Quality Metrics Can Extend to LLM Discovery

The existing pattern: ProbeResult → QualityFilter → DiscoveryResult can be extended:
- LLM extracts structured quality signals from web search results
- Same threshold-based OR logic applies (passes if ANY threshold met)
- Deterministic algorithm makes final decision, not LLM

### 2. Claude Native Web Search is Simplest Path

- $10/1K searches, uses existing ANTHROPIC_API_KEY
- No additional dependencies or API keys
- Integrates with existing provider abstraction

### 3. Session Pattern Should Follow HomebrewBuilder

- Implement BuildSession interface
- Reuse Factory/Provider/Message patterns
- Discovery-specific system prompt and tools

### 4. Defense-in-Depth is Critical

1. Input normalization (homoglyphs, lowercasing)
2. HTML stripping (hidden elements, zero-width characters)
3. Structured output (tool calls, not free-form)
4. URL validation (regex per builder type)
5. GitHub API verification (exists, not archived, owner check)
6. User confirmation (rich metadata display)
7. Sandbox validation (defense in depth)

## Decision (Phase 5)

**Problem:**
The LLM discovery stage in tsuku's resolver chain is an unimplemented stub. Tools not found in the registry or ecosystem probes fall through to a non-functional "not found" error. This blocks the long-tail discovery experience where users expect `tsuku install stripe-cli` to work without knowing the source. The implementation is security-sensitive because LLM output directly influences which binary gets installed.

**Decision:**
Implement LLM discovery as a quality-metric-driven system where the LLM's role is limited to extracting structured data from web search results, while a deterministic algorithm (reusing the existing QualityFilter pattern) makes the final source selection. Use Claude's native web search tool for simplicity. Require GitHub API verification and user confirmation for all LLM-discovered sources.

**Rationale:**
Separating LLM extraction from deterministic decision-making provides reproducibility and auditability. The LLM excels at understanding web content and extracting structured data; the algorithm excels at consistent, threshold-based decisions. Claude's native web search avoids additional API dependencies while the existing quality metrics infrastructure (ProbeResult, QualityFilter) provides a proven pattern to extend. Defense-in-depth through multiple verification layers (HTML stripping, URL validation, GitHub API checks, user confirmation) addresses the security risks of web-sourced data.

## Phase 8 Review Feedback

### Architecture Review

**Key Findings:**
- Clarity: 8/10 - Architecture is well-structured and implementable
- Completeness: 7/10 - Most components identified, some implementation details need decisions
- Sequencing: 8/10 - Phases are logically ordered
- Simplicity: 7/10 - Some complexity could be deferred

**Recommendations Applied:**
1. Error types - noted for Phase 1 implementation
2. `--yes` semantics clarified in design doc
3. HTTP client configuration - follows existing patterns

### Security Review

**Key Strengths:**
- Defense-in-depth with 6 verification layers
- Fork detection designed with parent comparison
- Quality thresholds with explicit rationale

**Key Concerns:**
1. Fork detection needs full implementation
2. `--yes` flag semantics need explicit documentation
3. Budget tracking implementation required
4. "Not applicable" justification for download verification challenged - rephrased as "misdirection equivalence"

**All critical feedback addressed in design document.**

## Current Status

**Phase:** Phase 8 complete - Final review done
**Last Updated:** 2026-02-10
