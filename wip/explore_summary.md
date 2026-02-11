# Exploration Summary: Tavily and Brave Search Providers

## Problem (Phase 1)

The DDG scraper is the only search provider for local LLMs. Users with API keys for Tavily or Brave prefer API-based search for better reliability and structured responses, but no implementation exists.

## Decision Drivers (Phase 1)

- Follow existing `search.Provider` interface pattern from DDG
- API key detection for auto-selection
- Explicit `--search-provider` flag override
- Priority: explicit flag > Tavily (if key) > Brave (if key) > DDG
- Reuse retry/options pattern established in #1610

## Research (Phase 2)

### Tavily API
- Endpoint: `POST https://api.tavily.com/search`
- Auth: API key in request body (`api_key` field)
- Request: `{"api_key": "...", "query": "...", "max_results": 10}`
- Response: `{"query": "...", "results": [{"title": "...", "url": "...", "content": "..."}]}`

### Brave API
- Endpoint: `GET https://api.search.brave.com/res/v1/web/search`
- Auth: `X-Subscription-Token` header
- Query params: `q=...`
- Response: `{"web": {"results": [{"title": "...", "url": "...", "description": "..."}]}}`

## Design Document (Phase 4-7)

Created: `docs/designs/DESIGN-tavily-brave-providers.md`

Key decisions:
1. **Provider options**: Follow DDGOptions pattern with TavilyOptions/BraveOptions structs
2. **API key detection**: Environment variable check at provider selection time
3. **Missing key handling**: Fail fast with clear error for explicit selection

## Implementation Plan

### New files:
- `internal/search/tavily.go` - TavilyProvider
- `internal/search/tavily_test.go` - Tests with fixtures
- `internal/search/brave.go` - BraveProvider
- `internal/search/brave_test.go` - Tests with fixtures
- `internal/search/factory.go` - NewSearchProvider factory
- `internal/search/factory_test.go` - Selection tests
- `internal/search/testdata/tavily_success.json`
- `internal/search/testdata/brave_success.json`

### Modified files:
- `cmd/tsuku/create.go` - Add `--search-provider` flag
- `cmd/tsuku/install.go` - Add `--search-provider` flag
- `internal/discover/llm_discovery.go` - Use factory

## Current Status

**Phase:** Complete - Design ready for approval
**Last Updated:** 2026-02-11
**Design Doc:** `docs/designs/DESIGN-tavily-brave-providers.md` (status: Proposed)

## Next Steps

1. Get design approved (human review)
2. Decide: implement in current branch or split to separate PR
3. Implement per the design phases
