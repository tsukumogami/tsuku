---
summary:
  constraints:
    - DDG returns HTTP 202 for rate limiting - must retry with backoff
    - Context cancellation must be respected during backoff waits
    - Tests must use recorded HTML fixtures, no live HTTP calls
    - Retry behavior must be logged at debug level for observability
    - Max retry count should be configurable (default 3 attempts)
  integration_points:
    - internal/search/ddg.go - DDGProvider.Search() needs retry logic
    - internal/search/ddg_test.go - Add fixture-based tests
    - internal/search/testdata/ - New directory for HTML fixtures
  risks:
    - DDG may change HTML structure breaking parser (mitigated by fixtures)
    - Jitter implementation must avoid thundering herd
    - Backoff timing needs to be testable without sleeping
  approach_notes: |
    Add exponential backoff with jitter to DDGProvider.Search(). When a 202 response
    is received, retry with delays of approximately 1s, 2s, 4s (base with jitter).
    Use a configurable MaxRetries (default 3). Respect context cancellation during
    waits. Log retry attempts at debug level. Create testdata/ with recorded DDG
    HTML responses to test parsing without network calls. Test cases should cover:
    success parse, 202 retry then success, max retries exceeded, context cancellation.
---

# Implementation Context: Issue #1610

**Source**: docs/designs/DESIGN-llm-discovery-implementation.md

## Key Design Points

This issue is part of Phase 1 of the LLM Discovery implementation (Tool Definitions and DDG Handler). The DDG scraper is the default web search provider for local LLMs - it enables discovery without requiring API keys (unlike Tavily/Brave).

**Why this matters**: DDG uses 202 responses for rate limiting. Without retry logic, searches fail silently or return incomplete results, making LLM discovery unreliable for local LLM users.

**Downstream dependency**: Issue #1617 (Tavily and Brave providers) depends on this issue establishing the test fixture pattern that alternative providers will follow.

## Acceptance Criteria Summary

1. DDGProvider retries on HTTP 202 with exponential backoff
2. Max retries configurable (default: 3)
3. Backoff uses jitter (1s, 2s, 4s base + random variance)
4. Context cancellation respected during waits
5. Debug-level logging for retry attempts
6. Unit tests use recorded HTML fixtures in testdata/
7. Test coverage: success, 202 retry success, max retries exceeded, context cancellation
