---
status: Accepted
problem: |
  The DDG scraper is the only search provider for local LLMs. Users with API keys
  for Tavily or Brave prefer API-based search for better reliability and structured
  JSON responses, but no implementation exists. The HTML scraping approach is fragile
  and subject to rate limiting.
decision: |
  Implement TavilyProvider and BraveProvider as additional search.Provider
  implementations. Auto-select based on API key presence with explicit flag override.
  Follow the existing DDGProvider patterns for options, retry logic, and testing.
rationale: |
  API-based providers offer structured JSON responses, documented rate limits, and
  stable contracts vs HTML scraping. Users who have API keys should benefit from them
  automatically. The existing Provider interface and test patterns from DDG make this
  a straightforward extension.
---

# DESIGN: Tavily and Brave Search Providers

## Status

Accepted

## Upstream Design Reference

This design implements Phase 6 (Alternative Search Providers) of [DESIGN-llm-discovery-implementation.md](DESIGN-llm-discovery-implementation.md).

**Relevant sections:**
- Decision 1: Web Search Integration (search as a tool)
- Phase 6: Alternative Search Providers

## Context and Problem Statement

The LLM Discovery feature uses web search to find tool sources. For local LLMs that lack native search, tsuku provides a DDG-based search handler. DDG scraping works but has limitations:

- HTML parsing is fragile (DDG can change their markup)
- Rate limiting requires retry logic with backoff
- No formal API contract or SLA

Users who have Tavily or Brave API keys would benefit from:
- Structured JSON responses (no HTML parsing)
- Documented rate limits and quotas
- Stable API contracts with versioning

The parent design specifies this as Phase 6 work, to be implemented after the core DDG provider is hardened (#1610).

## Decision Drivers

- Follow existing `search.Provider` interface pattern from DDG
- API key detection enables auto-selection without configuration
- Explicit `--search-provider` flag for users who want control
- Priority: explicit flag > Tavily (if key) > Brave (if key) > DDG
- Reuse retry/options pattern established in #1610

## Considered Options

### Decision 1: Provider Options Pattern

Each provider needs configuration (API key, HTTP client, retry settings). The question is how to structure this.

#### Chosen: Follow DDGOptions Pattern

Both Tavily and Brave will use the same options pattern as DDG:

```go
type TavilyOptions struct {
    APIKey     string       // Required for Tavily
    HTTPClient *http.Client // Optional, defaults to http.DefaultClient
    MaxRetries int          // Optional, defaults to DefaultMaxRetries
    Logger     log.Logger   // Optional, defaults to log.Default()
}

type BraveOptions struct {
    APIKey     string       // Required for Brave
    HTTPClient *http.Client
    MaxRetries int
    Logger     log.Logger
}
```

This mirrors DDGOptions and lets tests inject mock HTTP clients.

#### Alternatives Considered

**Single unified SearchOptions struct**: One struct with all provider options. Rejected because it mixes concerns and requires validation logic to check which fields apply.

**Functional options pattern**: `WithAPIKey()`, `WithHTTPClient()`, etc. Rejected as overkill for this simple case. The struct approach is clearer.

### Decision 2: API Key Detection

The question is how to detect which provider to use when the user doesn't explicitly specify.

#### Chosen: Environment Variable Check at Provider Selection Time

Check environment variables in priority order when building the search provider:

```go
func NewSearchProvider(explicit string) (Provider, error) {
    if explicit != "" {
        return newExplicitProvider(explicit)
    }
    if key := os.Getenv("TAVILY_API_KEY"); key != "" {
        return NewTavilyProvider(TavilyOptions{APIKey: key}), nil
    }
    if key := os.Getenv("BRAVE_API_KEY"); key != "" {
        return NewBraveProvider(BraveOptions{APIKey: key}), nil
    }
    return NewDDGProvider(), nil
}
```

This runs at LLMDiscovery construction time, not at search time.

#### Alternatives Considered

**Check at search time**: Detect API key on each search call. Rejected because it adds overhead and makes behavior less predictable. Provider selection should happen once.

**Config file provider preference**: Store preferred provider in config. Rejected as unnecessary complexity. Environment variables are the standard pattern for API keys.

### Decision 3: Error Handling for Missing Keys

When a user explicitly requests `--search-provider=tavily` but `TAVILY_API_KEY` isn't set, what happens?

#### Chosen: Fail Fast with Clear Error

Return an error immediately at provider construction time:

```
Error: --search-provider=tavily requires TAVILY_API_KEY environment variable
```

Don't fall back to DDG silently. If the user asked for Tavily, they want Tavily.

#### Alternatives Considered

**Fall back to DDG with warning**: Use DDG but warn the user. Rejected because it violates the principle of least surprise. Explicit flags should be honored or fail.

**Prompt for API key**: Ask the user to enter their key. Rejected as interrupting the flow. Users should set up their environment before running commands.

## Decision Outcome

**Chosen: Provider implementations following DDG patterns with environment-based auto-selection**

### Summary

Add `TavilyProvider` and `BraveProvider` to `internal/search/`. Each implements the existing `search.Provider` interface with `Name()` and `Search()` methods. Both use an options struct for configuration and the same retry pattern as DDG.

Provider selection happens in a new `NewSearchProvider()` factory function that checks:
1. Explicit `--search-provider` flag (highest priority)
2. `TAVILY_API_KEY` environment variable
3. `BRAVE_API_KEY` environment variable
4. Fall back to DDG (no key required)

Missing API keys when explicitly requested result in clear errors. Auto-selection silently falls through to the next option.

The `--search-provider` flag is added to `install` and `create` commands, accepting values `ddg`, `tavily`, or `brave`.

### Rationale

The existing Provider interface and DDGOptions pattern provide a clear extension point. Following the same patterns means less code to review and consistent behavior across providers. Environment-based auto-selection is the standard pattern for API key configuration in CLI tools.

## Solution Architecture

### API Specifications

**Tavily Search API:**
- Endpoint: `POST https://api.tavily.com/search`
- Auth: API key in request body (`api_key` field)
- Request body: `{"api_key": "...", "query": "...", "max_results": 10}`
- Response: `{"query": "...", "results": [{"title": "...", "url": "...", "content": "..."}]}`

**Brave Web Search API:**
- Endpoint: `GET https://api.search.brave.com/res/v1/web/search`
- Auth: `X-Subscription-Token` header
- Query params: `q=...`
- Response: `{"web": {"results": [{"title": "...", "url": "...", "description": "..."}]}}`

### New Files

| File | Purpose |
|------|---------|
| `internal/search/tavily.go` | TavilyProvider implementation |
| `internal/search/tavily_test.go` | Tests with mocked HTTP |
| `internal/search/brave.go` | BraveProvider implementation |
| `internal/search/brave_test.go` | Tests with mocked HTTP |
| `internal/search/factory.go` | NewSearchProvider factory |
| `internal/search/factory_test.go` | Provider selection tests |

### Modified Files

| File | Change |
|------|--------|
| `cmd/tsuku/create.go` | Add `--search-provider` flag |
| `cmd/tsuku/install.go` | Add `--search-provider` flag |
| `internal/discover/llm_discovery.go` | Use factory for provider selection |

### TavilyProvider Structure

```go
type TavilyProvider struct {
    apiKey     string
    client     *http.Client
    maxRetries int
    logger     log.Logger
}

func NewTavilyProvider(opts TavilyOptions) *TavilyProvider
func NewTavilyProviderWithOptions(opts TavilyOptions) *TavilyProvider

func (p *TavilyProvider) Name() string { return "tavily" }
func (p *TavilyProvider) Search(ctx context.Context, query string) (*Response, error)
```

### BraveProvider Structure

```go
type BraveProvider struct {
    apiKey     string
    client     *http.Client
    maxRetries int
    logger     log.Logger
}

func NewBraveProvider(opts BraveOptions) *BraveProvider
func NewBraveProviderWithOptions(opts BraveOptions) *BraveProvider

func (p *BraveProvider) Name() string { return "brave" }
func (p *BraveProvider) Search(ctx context.Context, query string) (*Response, error)
```

### Factory Function

```go
// NewSearchProvider creates a search provider based on configuration.
// Priority: explicit flag > TAVILY_API_KEY > BRAVE_API_KEY > DDG
func NewSearchProvider(explicit string) (Provider, error) {
    switch explicit {
    case "tavily":
        key := os.Getenv("TAVILY_API_KEY")
        if key == "" {
            return nil, fmt.Errorf("--search-provider=tavily requires TAVILY_API_KEY environment variable")
        }
        return NewTavilyProvider(TavilyOptions{APIKey: key}), nil
    case "brave":
        key := os.Getenv("BRAVE_API_KEY")
        if key == "" {
            return nil, fmt.Errorf("--search-provider=brave requires BRAVE_API_KEY environment variable")
        }
        return NewBraveProvider(BraveOptions{APIKey: key}), nil
    case "ddg", "":
        // Check for auto-selection
        if explicit == "" {
            if key := os.Getenv("TAVILY_API_KEY"); key != "" {
                return NewTavilyProvider(TavilyOptions{APIKey: key}), nil
            }
            if key := os.Getenv("BRAVE_API_KEY"); key != "" {
                return NewBraveProvider(BraveOptions{APIKey: key}), nil
            }
        }
        return NewDDGProvider(), nil
    default:
        return nil, fmt.Errorf("invalid search provider %q: must be ddg, tavily, or brave", explicit)
    }
}
```

### Test Strategy

Tests use httptest.Server with recorded JSON responses, same pattern as DDG:

```go
func TestTavilyProvider_Search_Success(t *testing.T) {
    content, _ := os.ReadFile("testdata/tavily_success.json")
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Write(content)
    }))
    // ... test implementation
}
```

Test fixtures:
- `testdata/tavily_success.json` - successful Tavily response
- `testdata/tavily_error.json` - Tavily error response
- `testdata/brave_success.json` - successful Brave response
- `testdata/brave_error.json` - Brave error response

## Implementation Approach

### Phase 1: Tavily Provider

1. Create `internal/search/tavily.go` with TavilyOptions and TavilyProvider
2. Implement Search() with JSON request/response handling
3. Add retry logic following DDG pattern
4. Create test fixtures and unit tests

### Phase 2: Brave Provider

1. Create `internal/search/brave.go` with BraveOptions and BraveProvider
2. Implement Search() with header-based auth and query params
3. Add retry logic following DDG pattern
4. Create test fixtures and unit tests

### Phase 3: Factory and CLI Integration

1. Create `internal/search/factory.go` with NewSearchProvider
2. Add `--search-provider` flag to install and create commands
3. Wire factory into LLMDiscovery
4. Add factory tests covering all selection paths

## Security Considerations

### Download Verification

Not applicable. Search providers don't download binaries; they return search results that are processed by the existing LLM discovery flow with its verification layers.

### Execution Isolation

Search providers make HTTP requests to external APIs:
- Tavily: `api.tavily.com`
- Brave: `api.search.brave.com`

No code execution, file writes, or system access beyond HTTP.

### Supply Chain Risks

**API key exposure**: Keys are read from environment variables, never logged or transmitted except to the respective API.

**Response tampering**: Both APIs use HTTPS. Malicious results are handled by the existing LLM discovery verification layers (GitHub API check, user confirmation).

**API availability**: If an API is down, Search() returns an error. The LLM discovery flow handles this gracefully.

### User Data Exposure

**Transmitted to Tavily/Brave:**
- Search query (tool name)
- API key (for authentication)

**Not transmitted:**
- User identity beyond API key
- System information
- Previous searches or installed tools

Users opt into this by setting their API keys. Without keys, the system uses DDG which requires no account.

## Consequences

### Positive

- Users with API keys get better search reliability
- Structured JSON responses are easier to parse than HTML
- API rate limits are documented and predictable
- Follows existing patterns, minimal new code

### Negative

- Requires API keys for Tavily/Brave (cost to users)
- Two more external dependencies to maintain
- API changes could break providers

### Mitigations

- DDG remains the default, no key required
- API versioning provides stability
- Tests with fixtures catch parsing changes
