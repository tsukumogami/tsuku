# Exploration Summary: LLM Discovery with Quality-Driven Decision Making

## Problem (Phase 1)

The LLM discovery stage is a stub that needs implementation to complete the discovery resolver chain. Users without API keys can only find tools in the registry or ecosystem probes. The LLM stage handles the "long tail" of tools, but its implementation is security-sensitive because LLM output directly influences which source gets installed.

## Decision Drivers (Phase 1)

1. **Reuse existing infrastructure**: LLM sessions, tool patterns, and quality metrics already exist
2. **Deterministic decision-making from LLM outputs**: LLM extracts structured data; algorithm picks the winner
3. **Security through defense-in-depth**: HTML stripping, URL validation, GitHub API verification, user confirmation
4. **Web search integration**: Support all LLM providers (Claude, Gemini, local) with unified search
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

### 2. DuckDuckGo HTML Scraping Enables Unified Architecture

Research discovered that DDG's HTML endpoint (`html.duckduckgo.com/html/?q=`) works with browser-like headers:
- GET requests with proper headers return real search results
- No API key, rate limits, or usage-based pricing
- Simple HTML structure: `result__a` for titles, `result__snippet` for descriptions
- URLs decoded from `uddg=` parameter

This enables a **unified architecture** where:
- tsuku performs web search itself (no per-provider search implementation)
- All LLM providers (Claude, Gemini, local) receive the same search context
- Local LLMs (#1421) get full discovery capability without cloud dependencies
- Optional API-based providers (Tavily, Brave) available for users who prefer them

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

## Decision (Phase 5, Updated)

**Problem:**
The LLM discovery stage in tsuku's resolver chain is an unimplemented stub. Tools not found in the registry or ecosystem probes fall through to a non-functional "not found" error. This blocks the long-tail discovery experience where users expect `tsuku install stripe-cli` to work without knowing the source. The implementation is security-sensitive because LLM output directly influences which binary gets installed.

**Decision:**
Implement LLM discovery as a quality-metric-driven system where the LLM's role is limited to extracting structured data from web search results, while a deterministic algorithm (reusing the existing QualityFilter pattern) makes the final source selection. Use tsuku-driven web search via DuckDuckGo HTML scraping as the default, with optional API-based providers (Tavily, Brave) for users who prefer them. This unified architecture supports all LLM providers including future local models (#1421). Require GitHub API verification and user confirmation for all LLM-discovered sources.

**Rationale:**
Separating LLM extraction from deterministic decision-making provides reproducibility and auditability. The LLM excels at understanding web content and extracting structured data; the algorithm excels at consistent, threshold-based decisions. Tsuku-driven search via DuckDuckGo scraping requires no API keys and enables the same discovery flow for all LLM providers—cloud and local alike. The swappable SearchProvider interface allows users with API keys (Tavily, Brave) to use higher-quality search if they prefer, while the existing quality metrics infrastructure (ProbeResult, QualityFilter) provides a proven pattern to extend. Defense-in-depth through multiple verification layers addresses security risks.

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

## Web Search Research (Post Phase 8)

After the initial design was complete, additional research was conducted to find a unified search architecture that supports all LLM providers including local models (#1421).

### Research Findings

| Endpoint | Method | Result |
|----------|--------|--------|
| `duckduckgo.com/html/?q=` | GET | CAPTCHA ("Select all squares containing a duck") |
| `lite.duckduckgo.com/lite/?q=` | GET | 400 error or CAPTCHA |
| `lite.duckduckgo.com/lite/` | POST | **Works** (with browser headers) |
| `html.duckduckgo.com/html/?q=` | GET | **Works** (with browser headers) |
| SearXNG public instances | GET | JavaScript bot verification |
| Google Search | GET | Aggressive CAPTCHAs, requires JS rendering |

### Key Finding

The `html.duckduckgo.com/html/?q=` endpoint returns real search results when accessed with browser-like headers:

```bash
curl -sL "https://html.duckduckgo.com/html/?q=ripgrep" \
  -H "User-Agent: Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36..." \
  -H "Accept: text/html,application/xhtml+xml,application/xml;q=0.9"
```

HTML structure:
- `<a class="result__a">` - Title and link (URL in `uddg=` param)
- `<a class="result__snippet">` - Description/snippet

### Architecture Change

The design was updated to use **search as a tool** that the LLM invokes:

| Provider | web_search Implementation |
|----------|--------------------------|
| Claude | Native (API handles internally) |
| Gemini | Native (Google Search Grounding) |
| Local LLM | tsuku-provided handler (DDG/Tavily/Brave) |

The LLM just calls `web_search` when it needs information. Cloud providers handle it natively; local providers use our DDG-based handler.

**Benefits:**
1. Cloud LLMs automatically use native search (no extra work)
2. Local LLMs get search capability via DDG handler
3. LLM controls search strategy (may search multiple times)
4. Unified extraction/verification flow for all providers

## Current Status

**Phase:** Phase 8 complete + Web Search Research
**Last Updated:** 2026-02-10
