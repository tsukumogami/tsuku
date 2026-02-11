---
status: Accepted
problem: |
  The LLM discovery stage in tsuku's resolver chain is an unimplemented stub. Tools not
  found in the registry or ecosystem probes fall through to a non-functional "not found"
  error. This blocks the long-tail discovery experience where users expect
  `tsuku install stripe-cli` to work without knowing the source. The implementation is
  security-sensitive because LLM output directly influences which binary gets installed.
decision: |
  Implement LLM discovery as a quality-metric-driven system where the LLM's role is
  limited to extracting structured data from web search results, while a deterministic
  algorithm (reusing the existing QualityFilter pattern) makes the final source selection.
  Expose web_search as a tool: Cloud LLMs use their native search capability; local LLMs
  use a tsuku-provided handler backed by DuckDuckGo. Require GitHub API verification and
  user confirmation for all LLM-discovered sources.
rationale: |
  Exposing search as a tool provides a unified architecture across all LLM providers.
  Cloud LLMs automatically use native search (higher quality, API-handled). Local LLMs
  get search capability via the DDG tool handler, enabling full discovery without cloud
  dependencies. The LLM controls search strategy (may search multiple times), and the
  same extraction/verification flow applies regardless of provider. Defense-in-depth
  through multiple verification layers addresses security risks of web-sourced data.
---

# DESIGN: LLM Discovery Implementation

## Status

Accepted

## Upstream Design Reference

This design implements Phase 5 (LLM Discovery) of [DESIGN-discovery-resolver.md](DESIGN-discovery-resolver.md). It addresses the design questions raised in [issue #1318](https://github.com/tsukumogami/tsuku/issues/1318):

- LLM integration: How to connect to the existing LLM infrastructure
- Prompt design: System prompts for reliable structured output
- Web search tool: Integration with web search capabilities
- Verification depth: GitHub API checks and non-GitHub verification
- Prompt injection defense: HTML stripping and URL validation
- Confirmation UX: Metadata display and user approval
- Budget/cost controls: Per-discovery limits and timeout handling

## Context and Problem Statement

The discovery resolver chain has three stages: registry lookup, ecosystem probe, and LLM discovery. The first two are implemented and working. The LLM discovery stage is a stub that returns `nil, nil`, meaning every tool not found by earlier stages produces a "not found" error.

This creates a poor experience for the long tail of developer tools. Popular tools like `kubectl`, `ripgrep`, and `jq` are covered by the registry (~800 entries). Ecosystem tools like crates, npm packages, and PyPI distributions are found by the ecosystem probe. But tools distributed primarily via GitHub releases (like `stripe-cli`, `lazygit`, `delta`) require manual `--from github:owner/repo` specification.

The challenge is that LLM-based discovery is security-sensitive. Web search results can contain malicious content attempting prompt injection. An attacker could:
- Create a fake GitHub page mentioning a tool name to steal installs
- Hide injection prompts in CSS/HTML to steer the LLM
- Use homoglyph Unicode characters to impersonate legitimate repos

The parent design specifies defense mechanisms (HTML stripping, URL validation, GitHub API verification, user confirmation), but leaves implementation details to this tactical design.

### Key Insight: Quality Metrics for LLM Discovery

The existing ecosystem probe uses a quality metrics pattern:
1. Builders extract structured data (ProbeResult with Downloads, VersionCount, HasRepository)
2. QualityFilter applies per-registry thresholds (OR logic: passes if ANY threshold met)
3. Priority ranking selects the best match when multiple pass

This pattern can extend to LLM discovery:
1. LLM extracts structured quality signals from web search results
2. The same threshold-based decision algorithm applies
3. Deterministic, auditable decisions even though extraction used LLM

This separation is powerful: LLMs excel at understanding web content; algorithms excel at consistent decisions. The LLM doesn't "decide" which source to use—it extracts data, and the algorithm decides.

### Scope

**In scope:**
- `LLMDiscovery` resolver implementation replacing the stub
- Web search as a tool (native for Cloud LLMs, DDG handler for local)
- Structured output schema supporting deterministic and non-deterministic results
- GitHub API verification with metadata extraction (for deterministic builder results)
- User confirmation flow with rich metadata display
- Defense layers (HTML stripping, URL validation, prompt injection defenses)
- 15-second timeout and per-discovery budget controls
- Integration with existing BuildSession and Factory patterns

**Out of scope:**
- Non-deterministic result handling (see Required Subsystem Designs below)
- Non-GitHub source verification (deferred—GitHub covers most cases)
- Caching of LLM discovery results (recipes serve this purpose)
- Automated learning from user confirmations
- Changes to the ecosystem probe or registry lookup stages

### Required Subsystem Designs

This design identifies building blocks that require their own tactical designs. LLM Discovery is not complete until these subsystems are designed and implemented.

| Subsystem | Purpose | Design Status |
|-----------|---------|---------------|
| **Non-Deterministic Builder** | Handle `instructions` results: follow install instructions via LLM, execute platform-specific setup | Needs design |
| **DDG Search Handler** | Web search tool implementation for local LLMs | Inline (simple enough) |

**Non-Deterministic Builder** is the critical dependency. When LLM discovery finds install instructions instead of a builder-mappable source, this subsystem:
1. Parses the instructions (may require fetching the full page)
2. Uses LLM to generate executable install steps
3. Executes steps in sandbox with appropriate verification
4. Handles platform-specific branching

This subsystem needs its own design because it involves:
- LLM-driven code/command generation (security-sensitive)
- Sandbox execution model
- Verification strategy for non-deterministic installs
- Rollback/cleanup on failure

## Success Criteria

To measure whether LLM discovery is working as intended, we define:

| Metric | Target | Measurement |
|--------|--------|-------------|
| **Accuracy** | >= 95% correct sources | Validated via telemetry: confirmed sources that pass sandbox without errors |
| **Latency** | P95 < 10s, P99 < 15s | Time from query to confirmation prompt |
| **Cost** | < $0.05 per discovery | LLM tokens + web search costs combined |
| **Confirmation rate** | >= 80% accepted | User confirmations vs. denials (excluding timeouts) |
| **Fallback rate** | < 5% to manual --from | Users who abandon discovery and specify source manually |

These metrics are tracked via the existing telemetry infrastructure. Initial thresholds are estimates that will be tuned based on real usage data.

## Decision Drivers

- **Security first**: LLM output must never directly control installation without verification
- **Reuse infrastructure**: Sessions, tools, quality filters, and cost tracking already exist
- **Deterministic decisions**: Algorithm makes the choice; LLM provides structured input
- **Simplicity**: Minimal new dependencies; Claude native search over third-party APIs
- **Graceful degradation**: Works without API key (earlier stages); clear errors when needed
- **15-second budget**: Discovery shouldn't block interactive use indefinitely

## Considered Options

### Decision 1: Web Search Integration

The LLM needs web search capability to find tool sources. The question is how to provide it.

#### Chosen: Search as a Tool (Provider-Transparent)

The architecture exposes web search as a **tool** that the LLM can invoke during discovery. Cloud LLM providers (Claude, Gemini) use their native search capability transparently; local LLMs use a tsuku-provided implementation.

```go
// The LLM sees this tool definition regardless of provider
var WebSearchTool = llm.ToolDef{
    Name:        "web_search",
    Description: "Search the web for information about developer tools",
    Parameters: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "query": {"type": "string", "description": "Search query"},
        },
        "required": []string{"query"},
    },
}
```

**How it works per provider:**

| Provider | web_search Implementation | Behavior |
|----------|--------------------------|----------|
| Claude | Native (API-handled) | Claude's API executes search internally |
| Gemini | Native (grounding) | Gemini's API uses Google Search |
| Local LLM | tsuku-provided | We implement via DDG/Tavily/Brave |

The LLM doesn't know or care where search results come from. It just calls `web_search` when it needs information, and results appear. This means:

1. **Cloud providers automatically use native search** (better quality, no extra implementation)
2. **Local providers get search capability** via our DDG/Tavily/Brave implementation
3. **The LLM controls search strategy** (may search multiple times, refine queries)
4. **No separate "search phase"** before LLM invocation

**Tool Handler for Local LLMs:**

When the LLM provider doesn't have native search, we register a tool handler:

```go
type WebSearchHandler struct {
    searcher SearchProvider // DDG, Tavily, or Brave
}

func (h *WebSearchHandler) Handle(ctx context.Context, args map[string]any) (string, error) {
    query := args["query"].(string)
    results, err := h.searcher.Search(ctx, query)
    if err != nil {
        return "", err
    }
    return formatResultsAsText(results), nil
}
```

**SearchProvider implementations (for local LLMs):**

```go
type SearchProvider interface {
    Search(ctx context.Context, query string) ([]SearchResult, error)
    Name() string
}

type SearchResult struct {
    URL         string
    Title       string
    Description string
}

// DDG scraper - default for local LLMs (free, no API key)
type DDGSearcher struct {
    client *http.Client
}

func (s *DDGSearcher) Search(ctx context.Context, query string) ([]SearchResult, error) {
    // GET https://html.duckduckgo.com/html/?q={url-encoded-query}
    // Parse HTML: extract result__a (title/URL), result__snippet (description)
}

// Optional API-based alternatives
type TavilySearcher struct { /* uses TAVILY_API_KEY */ }
type BraveSearcher struct { /* uses BRAVE_API_KEY */ }
```

**Provider Selection for Local LLMs:**

| Condition | SearchProvider |
|-----------|---------------|
| Default | DDG (free, no key needed) |
| `TAVILY_API_KEY` set | Tavily |
| `BRAVE_API_KEY` set | Brave |
| `--search-provider=X` | Explicit override |

#### Alternatives Considered

**Tsuku-driven search for all providers**: Always fetch search results before calling the LLM, pass as context. Rejected because:
- Wastes Cloud LLM native search capability
- Adds latency (search then LLM vs LLM with search tool)
- Cloud native search is higher quality (integrated, no scraping)

**Separate search phase**: Fetch results first, then call LLM with results as context. Rejected because:
- Prevents LLM from refining searches or doing multiple queries
- Less flexible than tool-based approach
- Doesn't match how Claude/Gemini native search works

**Google Search scraping for local LLMs**: Direct scraping of Google. Rejected because:
- Aggressive bot detection (CAPTCHAs, rate limiting)
- Requires JavaScript rendering
- Terms of Service prohibit automated access

### Decision 2: Structured Output Schema

The LLM needs to return structured data that the algorithm can evaluate. The question is what schema to use.

#### Chosen: Flexible Result Interface (Deterministic + Non-Deterministic)

The LLM discovery may find either:
1. **Deterministic sources**: Mappable to existing builders (github, npm, cargo, etc.)
2. **Non-deterministic findings**: Install instructions, documentation, download pages

The result interface must support both types:

| Result Type | Example | Routing |
|-------------|---------|---------|
| `builder` | `github:stripe/stripe-cli` | Existing builder pipeline |
| `instructions` | Install page URL + text | Future: instruction-following builder |

**Why support non-deterministic results:**

Many tools don't have clean builder mappings:
- Proprietary tools with custom installers
- Tools requiring manual download + PATH setup
- Platform-specific instructions
- Build-from-source tools without release binaries

Capturing these findings enables:
1. Displaying helpful instructions to the user (immediate value)
2. Future: LLM-driven builder that follows install instructions
3. Analytics on what types of tools need non-deterministic handling

**Deferred to implementation:** The exact schema for `extract_source` tool and result types will be designed during Phase 2 (Core Discovery Session).

#### Alternatives Considered

**Simple builder+source output**: Just return the minimal fields needed. Rejected because it doesn't enable quality-based decision making and puts all trust in LLM confidence.

**Separate search and extraction phases**: First search, then extract from each result. Rejected as adding complexity and latency. The LLM can do both in one conversation.

### Decision 3: Verification Depth

The parent design specifies GitHub API verification. The question is how deep to go.

#### Chosen: GitHub-Only with Deferred Ecosystem Verification

Verify GitHub sources via API:
1. Repository exists (404 check)
2. Repository is not archived
3. Owner name matches extraction
4. Collect metadata: stars, created_at, pushed_at, description

For non-GitHub sources (ecosystem packages found via LLM), defer to the existing ecosystem probe:
- If LLM suggests "npm:prettier", verify by calling npm's Probe()
- This reuses existing validation and quality filtering

GitHub-specific verification is critical because GitHub releases are the primary distribution method for tools not in ecosystem registries. Ecosystem verification is already handled by the probe.

#### Alternatives Considered

**Full ecosystem verification in LLM stage**: Verify all ecosystem suggestions via their APIs. Rejected because the ecosystem probe already does this. If a tool exists in npm, the ecosystem probe would have found it—LLM discovery only runs after probe misses.

**No verification, just confirmation**: Trust user judgment from metadata display. Rejected as too risky. The verification catches obvious attacks (non-existent repos, archived projects) that users might miss.

### Decision 4: Decision Algorithm

The question is how to decide when multiple potential sources are found or when confidence is uncertain.

#### Chosen: Threshold + Priority + Confirmation

Apply the existing quality filter pattern with discovery-specific thresholds:

**Decision rules:**
1. Confidence is REQUIRED above a minimum threshold (gates all other checks)
2. THEN apply OR logic: passes if stars >= threshold OR downloads >= threshold
3. If multiple pass: highest confidence wins, then highest stars
4. If none pass: present options with metadata, let user choose
5. Always require user confirmation for LLM sources (unless --yes)

**Why AND logic for confidence, not pure OR like ecosystem probe?**

The ecosystem probe uses pure OR logic because its data comes from authoritative registry APIs—if crates.io says a package has 500 downloads, that's ground truth. LLM discovery is different: the confidence score reflects how certain the LLM is about its extraction, not objective quality. A high-star repo found with low confidence might mean the LLM is unsure whether it found the right repo. Using confidence as a gate ensures we only auto-select sources where the LLM is reasonably certain it found the correct one.

**Threshold design principles:**

| Signal | Purpose | Tuning Approach |
|--------|---------|-----------------|
| Confidence (gate) | Reject uncertain extractions | Set to balance precision vs. recall; tune via false positive rate |
| Stars (OR) | Filter low-quality repos | Set to exclude typosquat-risk repos; tune via registry analysis |
| Downloads (OR) | Quality signal for ecosystem sources | Align with ecosystem probe thresholds for consistency |

Specific threshold values will be determined during implementation and tuned via telemetry tracking of false positives and missed tools.

#### Alternatives Considered

**LLM picks the winner**: Let the LLM return only its top choice. Rejected because it puts decision authority in the LLM, which is non-deterministic and harder to audit.

**Always present multiple options**: Show all candidates every time. Rejected as noisy when one option clearly dominates. Better to auto-select clear winners and only prompt for ambiguous cases.

### Decision 5: Session Architecture

The question is whether LLM Discovery needs its own session type or can reuse existing patterns.

#### Chosen: Dedicated DiscoverySession Following BuildSession Pattern

Create a new session type that follows the HomebrewSession pattern:
- Implements a similar interface for conversation management
- Uses the existing Factory to get providers
- Maintains message history for potential multi-turn conversations
- Tracks cost via totalUsage accumulation

The session lives in `internal/discover/` rather than `internal/builders/` because discovery is not building a recipe—it's finding a source.

**Required capabilities:**
- LLM provider access (via Factory)
- Message history for potential multi-turn conversations
- Tool definitions (web_search, extract_source)
- Usage tracking for cost accounting
- Timeout enforcement (configurable, default ~15 seconds)

#### Alternatives Considered

**Direct Client usage**: Use the legacy llm.Client directly. Rejected because Client is tightly coupled to GitHub release analysis and doesn't fit the discovery use case.

**BuildSession implementation**: Have discovery implement BuildSession. Rejected because discovery doesn't generate recipes—it returns DiscoveryResult. The interfaces don't align.

### Uncertainties

- **DuckDuckGo endpoint stability**: The HTML endpoint works today but DDG could add bot protection. Mitigation: the lite POST endpoint is a fallback, and we can add API-based search later if needed.
- **LLM extraction accuracy for tool discovery**: Haven't validated whether LLMs reliably extract the correct source from search results. May need prompt engineering.
- **Non-GitHub distribution patterns**: Some tools (like Stripe CLI) have multiple distribution channels. The algorithm may need tuning for these cases.
- **False positive rate**: Threshold values are estimates. Real usage will inform tuning.
- **Local LLM context limits**: Some local models have limited context windows. Search results must fit within these limits while leaving room for system prompt and response.
- **Star gaming**: Quality thresholds based on star counts can be gamed (stars can be purchased). May need velocity checking or multi-source corroboration in future iterations if attacks materialize.
- **Visible text injection**: HTML stripping addresses hidden injection but not SEO-optimized attack pages with malicious visible content. Multi-source verification (official docs → repo link) may be needed if this attack vector is exploited.

## Decision Outcome

**Chosen: Quality-metric-driven LLM discovery with provider-transparent web search tool and deterministic decision algorithm**

### Summary

LLM Discovery is implemented as a `LLMDiscoverySession` that:

1. **Web Search**: Uses Claude's native web search tool to find information about the tool
2. **Structured Extraction**: Calls `extract_source` tool to output quality signals (stars, downloads, confidence, evidence)
3. **Threshold Filtering**: Applies deterministic thresholds (confidence >= 70 AND stars >= 50)
4. **GitHub Verification**: Calls GitHub API to verify repository exists and isn't archived
5. **User Confirmation**: Displays rich metadata (stars, age, owner, last commit) and requires approval

The session has a 15-second timeout and respects the existing daily budget tracking. If thresholds aren't met, the session presents multiple options with metadata for user selection. The `--yes` flag skips confirmation but not verification.

The key architectural insight is separating LLM extraction from algorithmic decision-making. The LLM is good at understanding web content and extracting structured data. The algorithm is good at making consistent, auditable decisions. Combining them provides the best of both.

### Rationale

This approach builds on proven patterns:
- **Quality filtering** from ecosystem probe (OR logic, per-source thresholds)
- **Session architecture** from HomebrewBuilder (Factory, Provider, Messages, Usage)
- **Tool calls** from recipe generation (fetch_file, extract_pattern schemas)
- **Verification** from registry bootstrap (GitHub API checks)

Using Claude's native web search avoids new dependencies. The structured output schema ensures the algorithm has the data it needs. Verification and confirmation provide defense-in-depth even if the LLM is fooled.

### Trade-offs Accepted

- **DuckDuckGo dependency (local LLMs only)**: Local LLM web search relies on DDG's HTML endpoint continuing to work. Cloud LLMs use native search. Mitigation: the endpoint is designed for accessibility and low-bandwidth use; if it changes, we can add API-based fallbacks.
- **GitHub verification only**: Non-GitHub sources (npm, PyPI) rely on ecosystem probe fallback rather than separate verification. This is acceptable because the ecosystem probe would have found genuine ecosystem packages.
- **Conservative thresholds**: Stars >= 50 may exclude legitimate but obscure tools. Users can override with `--from`.
- **Always confirm LLM sources**: Even with `--yes`, sandbox validation still runs. The confirmation displays metadata; verification always runs.
- **Local LLM context limits**: Search results are trimmed to fit ~4K context windows, which may reduce discovery accuracy for obscure tools with many similar-named results.

## Solution Architecture

### Overview

The LLM Discovery stage sits between the ecosystem probe and "not found" in the resolver chain. It only runs when registry and ecosystem both miss, and only when an LLM provider is available.

```
Chain Resolver
  ├── Stage 1: RegistryLookup (instant, cached)
  ├── Stage 2: EcosystemProbe (parallel queries, 3s timeout)
  └── Stage 3: LLMDiscovery (web search, verification, 15s timeout)
```

### Components

The implementation adds several components to `internal/discover/`:

| Component | Purpose |
|-----------|---------|
| SearchProvider interface | Abstraction for web search (local LLMs only) |
| DDG/Tavily/Brave searchers | SearchProvider implementations |
| LLMDiscoverySession | Manages LLM conversation for discovery |
| Tool definitions | web_search and extract_source tool schemas |
| Tool handler | Handles web_search calls for local LLMs |
| GitHub verification | API calls to validate repository metadata |
| User confirmation | Interactive approval flow with metadata display |
| Sanitization | HTML stripping, URL validation, input normalization |

### Web Search Tool Handler (Local LLMs)

For local LLMs that don't have native search, tsuku provides a tool handler that:
1. Receives the query from the LLM's web_search tool call
2. Delegates to a SearchProvider implementation
3. Formats results as text for LLM consumption (numbered list with title, URL, snippet)

### Search Provider Interface (Local LLMs)

The SearchProvider interface abstracts web search backends:
- **Required methods**: Search(query) returning results with URL, title, description
- **Provider selection**: Check for API keys (Tavily, Brave), fall back to DDG
- **Configuration**: Environment variables or explicit `--search-provider` flag

### DDG Scraper (Default for Local LLMs)

DuckDuckGo HTML scraping provides free search without API keys:
- **Endpoint**: `html.duckduckgo.com/html/?q={query}`
- **Requirements**: Browser-like headers to avoid bot detection
- **Parsing**: Extract titles, URLs, and snippets from HTML response
- **Limitations**: May be rate-limited; HTML structure may change

### LLMDiscoverySession

The discovery session orchestrates the LLM conversation:

1. **Apply timeout** - Enforce configurable timeout (default ~15s)
2. **Build initial message** - Include tool name and discovery prompt
3. **Run conversation loop** - Allow LLM to search and refine (bounded iterations)
4. **Process extract_source** - Parse structured output from tool call
5. **Apply quality thresholds** - Filter based on confidence and quality signals
6. **Verify via GitHub API** - Confirm repository exists and is legitimate
7. **Return result** - Either auto-select or prompt for confirmation

### System Prompt

```
You are a developer tool discovery assistant. Your task is to find the official
source for a developer tool so it can be installed.

When asked about a tool:
1. Use web_search to find information about the tool
2. Look for the OFFICIAL source: GitHub releases, npm package, crates.io, etc.
3. Prefer GitHub releases for CLI tools that publish binaries
4. Cross-reference multiple sources for confidence
5. Report findings via extract_source

Be skeptical of unofficial sources. Look for:
- Official project websites and documentation
- High star counts and recent activity
- Matching maintainer names across platforms
- Links from official documentation to download sources

If your first search is ambiguous, refine your query or search again.
Never trust a single web result. Cross-reference multiple sources.
```

### Tool Definitions

**web_search** (native for Cloud LLMs, tsuku-provided for local):
```go
{
    Name: "web_search",
    Description: "Search the web for information about developer tools",
    Parameters: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "query": {"type": "string", "description": "Search query"},
        },
        "required": []string{"query"},
    },
}
```

For Claude/Gemini: This maps to their native search capability.
For local LLMs: tsuku handles the tool call via DDG/Tavily/Brave.

**extract_source** (reports findings - deterministic or non-deterministic):

The tool must support two result types:
- `result_type: "builder"` - includes builder name, source arg, quality signals
- `result_type: "instructions"` - includes URL, instruction text, platform

Both types include common fields: confidence, evidence, reasoning, warnings.

Exact schema to be designed during Phase 2 implementation.

### Conversation Flow

```
Turn 1:
  User: "Find the official source for stripe-cli"

  LLM: [calls web_search with query "stripe-cli github official"]
       → Results returned (native for Cloud, DDG for local)

  LLM: [calls web_search with query "stripe cli documentation"]
       → Additional results (LLM decides if needed)

  LLM: [calls extract_source with structured analysis]
      - builder: "github"
      - source: "stripe/stripe-cli"
      - confidence: 95
      - evidence: ["Official Stripe documentation links to this repo", "8k+ stars"]
```

The LLM controls the search strategy. It may:
- Do a single search if results are clear
- Do multiple searches to cross-reference sources
- Refine queries based on initial results

Cloud LLMs use native search (higher quality). Local LLMs use tsuku-provided search (DDG/Tavily/Brave).

### Verification Flow

GitHub verification queries the repository API to collect:
- **Existence**: 404 check confirms repository exists
- **Status**: Archived flag, fork status
- **Quality signals**: Stars, creation date, last push date
- **Ownership**: Owner login and type (User/Organization)
- **Fork parent**: If fork, the source repository for comparison

**Fork detection**: The GitHub API returns `fork: true` for repositories that are forks. When detected:

1. Fetch parent repo metadata via `parent.full_name` from the API response
2. Display warning: "This is a fork of {parent}. Consider the original instead."
3. Compare stars: if parent has 10x more stars, suggest parent instead
4. Never auto-select forks—always prompt for confirmation even with `--yes`

This prevents installing from abandoned forks when the original is actively maintained.

### Quality Threshold Logic

The threshold function implements the decision rules:

1. **Confidence gate (AND)**: If confidence below threshold, reject immediately
2. **Fork check**: Forks never auto-pass—always require explicit confirmation
3. **Quality signals (OR)**: Pass if ANY quality signal meets its threshold:
   - GitHub stars above minimum
   - Ecosystem downloads above minimum
4. **Default**: If no quality signal passes, reject for auto-selection (still available as user choice)

### Confirmation Display

```
Discovered via web search:

Repository: github.com/stripe/stripe-cli
  Stars: 8,234
  Created: 5 years ago (2020-01-15)
  Last commit: 2 weeks ago
  Owner: stripe (Organization)
  Description: Build, test, and manage your Stripe integration

Confidence: 95%
Evidence:
  - "Official Stripe CLI repository" from stripe.com/docs/stripe-cli
  - "Published releases with pre-built binaries" from GitHub releases page

This recipe will be tested in a sandbox before installation.

Install stripe-cli from this source? (y/N)
```

### Defense Layers

1. **Input Normalization** (existing)
   - Homoglyph detection, lowercase conversion, length limits
   - Prevents unicode tricks in tool names

2. **HTML Stripping** (new)
   - Remove script, style, noscript tags and HTML comments
   - Remove zero-width Unicode characters
   - Convert to plain text before LLM processing

3. **URL Validation** (new)
   - GitHub URLs must match `github.com/{owner}/{repo}` pattern
   - Owner/repo names restricted to safe character sets
   - Reject credentials, non-standard ports, path traversal

4. **GitHub API Verification** (new)
   - Repository exists (404 check)
   - Not archived
   - Owner matches extracted name

5. **User Confirmation** (new)
   - Rich metadata display (stars, age, owner, description)
   - Interactive y/N prompt
   - Non-interactive mode: error suggesting --yes
   - **`--yes` flag behavior**: Skips confirmation prompt but NOT verification. Layers 1-4 always run.

6. **Sandbox Validation** (existing—post-confirmation)

### Error Types

Discovery defines specific error categories:

| Error | Trigger | Retryable |
|-------|---------|-----------|
| Timeout | Discovery exceeds time budget | No |
| Budget | Daily LLM cost limit reached | No (until reset) |
| Verification | GitHub API check fails (404, archived) | Depends on cause |
| ConfirmationDenied | User rejects the source | No |
| Search | Web search fails (DDG down, rate limit) | Yes |

All errors implement a common interface with `IsRetryable()` for consistent handling.

### Error Handling

| Condition | Error Type | Behavior |
|-----------|------------|----------|
| No LLM provider configured | nil, nil | Chain continues to "not found" |
| 15-second timeout | TimeoutError | Return error, suggest --from |
| GitHub API 404 | VerificationError | Reject source, suggest --from |
| GitHub API rate limit | VerificationError (retryable) | Soft error, skip verification, higher confirmation bar |
| Low confidence (<70) | None | Present as option, don't auto-select |
| Archived repository | VerificationError | Reject, show warning |
| Fork detected | None | Warn, suggest parent if 10x stars, never auto-select |
| Budget exhausted | BudgetError | Return error (confirmable) |
| User denies confirmation | ConfirmationDeniedError | Abort with message |

### Data Flow

```
tsuku install stripe-cli
    |
    v
ChainResolver.Resolve("stripe-cli")
    |
    ├── RegistryLookup → miss
    ├── EcosystemProbe → miss (no npm/cargo/etc. package)
    └── LLMDiscovery.Resolve("stripe-cli")
            |
            ├── Create LLMDiscoverySession with tools: [web_search, extract_source]
            │
            ├── provider.Complete("Find official source for stripe-cli")
            │       │
            │       ├── LLM calls web_search("stripe-cli github official")
            │       │       └── Cloud: native search / Local: DDG handler
            │       │
            │       ├── LLM calls web_search("stripe cli documentation")
            │       │       └── (optional, LLM decides)
            │       │
            │       └── LLM calls extract_source({builder: "github", source: "stripe/stripe-cli", ...})
            │
            ├── Apply quality thresholds
            ├── verifyGitHub("stripe", "stripe-cli")
            ├── Display confirmation with metadata
            ├── User confirms
            └── Return DiscoveryResult{
                    Builder: "github",
                    Source: "stripe/stripe-cli",
                    Confidence: "llm",
                    Metadata: {Stars: 8234, ...}
                }
    |
    v
Create pipeline → sandbox → install
```

## Implementation Approach

### Prototype Status

A working prototype has been implemented that validates the core architecture. The following components are complete:

| Component | Status | Location |
|-----------|--------|----------|
| LLMDiscovery struct | Done | `internal/discover/llm_discovery.go` |
| Tool definitions (web_search, extract_source) | Done | `internal/discover/llm_discovery.go` |
| Conversation loop with bounded iterations | Done | `internal/discover/llm_discovery.go` |
| SearchProvider interface | Done | `internal/search/provider.go` |
| DDG HTML scraper | Done | `internal/search/ddg.go` |
| GitHub API verification (exists, archived) | Done | `internal/discover/llm_discovery.go` |
| Quality thresholds (confidence >= 70, stars >= 50) | Done | `internal/discover/llm_discovery.go` |
| ChainResolver integration (stage 3) | Done | `cmd/tsuku/create.go` |
| User confirmation flow | Done | `cmd/tsuku/create.go` |

**Remaining work** focuses on hardening, security layers, and optional features rather than core architecture.

### Roadmap Overview

The implementation starts with a tool-based architecture where `web_search` is exposed to the LLM. For Cloud LLMs, this uses native search. For local LLMs, we provide a DDG-based tool handler.

```
Phase 1-5: Core Discovery with Tool-Based Search
    │
    ├── Cloud LLMs: Use native web_search (Claude/Gemini handle internally)
    ├── Local LLMs: Use tsuku-provided web_search handler (DDG)
    │
    ├── Validates: LLM extraction, quality thresholds, verification, confirmation
    │
    └── Enables parallel track:
            │
            └── Track A: Embedded LLM Runtime (#1421)
                  └── Local models get full discovery via DDG tool handler
```

This approach:
1. Cloud LLMs automatically use native search (no extra work needed)
2. Local LLMs get search capability via DDG tool handler
3. The LLM controls search strategy (may search multiple times)
4. Unblocks local LLM work (#1421) immediately

### Phase 1: Tool Definitions and DDG Handler

Implement the tool-based search architecture. Cloud LLMs use native search; local LLMs use DDG handler.

**Prototype complete:**
- [x] Define `web_search` and `extract_source` tool schemas
- [x] Implement tool handler for local LLM web_search calls
- [x] Implement DDG scraper with HTML parsing
- [x] Implement SearchProvider interface for alternative backends

**Remaining:**
- [ ] Add retry logic for DDG rate limiting (202 responses)
- [ ] Write tests with recorded HTML responses

### Phase 2: Core Discovery Session

Implement the LLM session that analyzes search results.

**Prototype complete:**
- [x] Create LLMDiscoverySession with conversation management
- [x] Define extract_source tool schema (builder results only for now)
- [x] Write discovery system prompt
- [x] Implement conversation loop with bounded iterations
- [x] Wire into ChainResolver as stage 3
- [x] Add configurable timeout (60s default)

**Remaining:**
- [ ] Add `instructions` result type to extract_source schema (deferred to Phase 8)

### Phase 3: Verification and Sanitization

Add security layers: HTML stripping, URL validation, GitHub API verification.

**Prototype complete:**
- [x] Implement GitHub API verification (exists, not archived)

**Remaining:**
- [ ] Implement HTML stripping for search results
- [ ] Implement URL validation with domain allowlist
- [ ] Add fork detection (check if fork, compare to parent stars)
- [ ] Add rate limit handling with graceful degradation

### Phase 4: Quality Thresholds and Decision Algorithm

Implement the deterministic decision logic.

**Prototype complete:**
- [x] Implement threshold checking (confidence >= 70, stars >= 50)
- [x] Define initial threshold values

**Remaining:**
- [ ] Implement priority ranking for multiple candidates
- [ ] Handle edge cases (multiple matches, forks)
- [ ] Add tests with mock LLM responses

### Phase 5: Confirmation UX

Implement user-facing confirmation flow.

**Prototype complete:**
- [x] Create metadata display format (builder, source, stars, description, reason)
- [x] Handle interactive vs. non-interactive modes
- [x] Integrate with existing confirmation patterns (confirmWithUser)

**Remaining:**
- [ ] Add --yes handling (skip confirmation, not verification)
- [ ] Enhance display with more metadata (age, downloads if available)

### Phase 6: Alternative Search Providers (Optional)

Add Tavily and Brave as alternative search providers for local LLMs. Users with API keys may prefer these over DDG scraping.

- Implement Tavily searcher with JSON API
- Implement Brave searcher with JSON API
- Update provider selection to detect API keys
- Add `--search-provider` flag for explicit override

**Provider Selection for Local LLMs:**

| Condition | Search Provider |
|-----------|----------------|
| Default | DDG (free, no key) |
| `TAVILY_API_KEY` set | Tavily |
| `BRAVE_API_KEY` set | Brave |
| `--search-provider=X` | Explicit override |

Cloud LLMs always use native search (handled by the API, no configuration needed).

### Phase 7: Budget and Telemetry Integration

Wire into existing cost tracking and telemetry.

- Add cost tracking to discovery session
- Integrate with existing budget tracking infrastructure
- Emit discovery telemetry events (accuracy, latency, confirmation rate)
- Add discovery metrics to verbose output

### Phase 8: Non-Deterministic Builder (Requires Separate Design)

Handle `instructions` results from LLM discovery. This phase requires its own tactical design document before implementation.

**Blocked by:** DESIGN-non-deterministic-builder.md (to be created)

The non-deterministic builder:
- Receives instructions results from LLM discovery
- Uses LLM to parse and execute install instructions
- Runs in sandbox with verification
- Handles platform-specific branching

**LLM Discovery is feature-complete when this phase is implemented.**

## Security Considerations

### Download Verification

LLM Discovery does not download binaries directly, but **discovery misdirection is equivalent to download misdirection**: pointing to the wrong source is as dangerous as corrupting a download. An attacker who can influence the LLM's source selection achieves the same outcome as compromising the download itself.

This makes verification critical even though no download occurs during discovery. The mitigation strategy is multiple verification layers between LLM output and installation:
1. Structured output constrains what the LLM can suggest
2. GitHub API verification confirms the source exists and is legitimate
3. User confirmation shows metadata for human review
4. Sandbox validation tests the generated recipe before real installation

### Execution Isolation

LLM Discovery makes:
- HTTP requests to DuckDuckGo (search results)
- API calls to LLM provider (Claude, Gemini, or local)
- API calls to GitHub (verification)
- No file writes except logging
- No code execution during discovery

The discovered source goes through the existing create pipeline with sandbox validation before any binary execution.

### Supply Chain Risks

**Prompt injection via web content**: Malicious pages could contain hidden text attempting to steer the LLM toward bad sources.

Mitigations:
1. HTML stripping removes hidden elements, zero-width characters
2. URL validation rejects malformed patterns
3. GitHub API verification confirms repository exists
4. User confirmation shows metadata for manual review
5. Sandbox validation catches bad recipes post-discovery

**LLM hallucination**: Claude might invent repositories that don't exist.

Mitigations:
1. GitHub API verification fails for non-existent repos
2. Evidence array shows what search results the LLM used
3. Reasoning field explains the conclusion

**Typosquatting repos**: Attacker creates `stripe/stripe-cli-unofficial` to impersonate.

Mitigations:
1. Quality thresholds (low-star repos rejected)
2. Confirmation shows owner and star count
3. Evidence field shows how source was identified

### User Data Exposure

**Transmitted data:**
- Tool name sent to DuckDuckGo (search query—no API key, no account)
- Tool name and search results sent to LLM provider (covered by provider privacy policy)
- Owner/repo sent to GitHub API (public data)

**Not transmitted:**
- User identity
- System information beyond tool name
- Installed tools or previous searches

The existing telemetry opt-out (`TSUKU_TELEMETRY=0`) doesn't affect LLM discovery—it still queries the LLM provider because that's the core functionality. Users without API keys can still use local LLMs (#1421) for discovery.

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Prompt injection | HTML stripping, structured output | Novel visible-text injection |
| Hallucinated repos | GitHub API 404 check | None (API is authoritative) |
| Typosquatting | Quality thresholds, confirmation metadata | Sophisticated clone with stars |
| Fork misdirection | Fork detection, parent comparison, never auto-select | User ignores warning |
| Archived/abandoned repos | GitHub API archived flag | Recently archived (small window) |
| Budget exhaustion attack | Per-discovery timeout, daily budget | None (budget is protection) |
| Rate limit abuse | Circuit breaker, soft errors | Temporary unavailability |

## Consequences

### Positive

- Tools not in registry or ecosystems become discoverable
- Users don't need to know source details to install
- Quality metrics ensure only reputable sources are suggested
- Defense-in-depth protects against LLM misdirection
- Deterministic algorithm provides auditable decisions
- Reuses existing infrastructure (Factory, Provider, quality patterns)

### Negative

- Adds LLM dependency for long-tail tools
- $10/1K search cost (minimal at expected volume)
- 15-second latency worst-case for discovery
- Confirmation prompts interrupt automation (--yes mitigates)
- GitHub-only verification (other sources rely on ecosystem probe fallback)

### Mitigations

- LLM-free path still works (registry + ecosystem cover most tools)
- Daily budget limits unexpected costs
- Timeout ensures discovery doesn't hang indefinitely
- Rich confirmation helps users make informed decisions
- Ecosystem sources verified via existing probe infrastructure
