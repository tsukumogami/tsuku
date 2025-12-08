# Issue 278 Implementation Plan

## Summary

Create `internal/llm/` package with a Claude client that supports multi-turn tool use for recipe generation, using the official `github.com/anthropics/anthropic-sdk-go` package.

## Approach

The implementation follows the issue specification directly, using patterns established in the codebase:
- Dependency injection for HTTP client (pattern from `internal/builders/go.go`)
- Structured types matching SDK patterns
- Context-based cancellation and timeouts
- Unit tests with cost calculation tests and mock server integration tests

### Alternatives Considered

- **Direct HTTP calls**: Not chosen because the official Anthropic SDK handles auth, retries, and response parsing
- **Provider abstraction upfront**: Not chosen because Slice 1 focuses on proving the concept with Claude only; provider abstraction is Slice 3

## Files to Create

- `internal/llm/client.go` - Main Client struct with GenerateRecipe method
- `internal/llm/client_test.go` - Unit/integration tests for client
- `internal/llm/cost.go` - Usage struct with Cost() and String() methods
- `internal/llm/cost_test.go` - Unit tests for cost calculation
- `internal/llm/tools.go` - Tool schema definitions for fetch_file, inspect_archive, extract_pattern
- `internal/llm/tools_test.go` - Tests for tool schema generation (optional, if complex)

## Files to Modify

- `go.mod` - Add `github.com/anthropics/anthropic-sdk-go` dependency

## Implementation Steps

- [x] Add Anthropic SDK dependency to go.mod
- [x] Create `internal/llm/cost.go` with Usage struct and cost calculation
- [x] Create `internal/llm/cost_test.go` with unit tests
- [x] Create `internal/llm/tools.go` with tool schema definitions
- [ ] Create `internal/llm/client.go` with Client struct and GenerateRecipe method
- [ ] Create `internal/llm/client_test.go` with integration tests (skip if no API key)

Mark each step [x] after it is implemented and committed. This enables clear resume detection.

## Design Details

### Client Structure

```go
type Client struct {
    anthropic  *anthropic.Client
    model      string
    httpClient *http.Client  // For fetch_file tool execution
}

func NewClient() (*Client, error)
func NewClientWithHTTPClient(httpClient *http.Client) (*Client, error)
```

### GenerateRecipe Signature

```go
func (c *Client) GenerateRecipe(ctx context.Context, req *GenerateRequest) (*AssetPattern, *Usage, error)
```

### Request/Response Types

```go
type GenerateRequest struct {
    Repo        string
    Releases    []Release
    Description string
    README      string
}

type Release struct {
    Tag    string
    Assets []string
}

type AssetPattern struct {
    // Pattern for matching release assets to platforms
    // (exact fields TBD based on tool output schema)
}
```

### Tool Definitions

Three tools for the multi-turn loop:

1. **fetch_file** - Fetch a file from a URL
   - Input: `url` (string)
   - Output: file contents or error

2. **inspect_archive** - Inspect contents of a downloaded archive
   - Input: `url` (string)
   - Output: list of files in archive

3. **extract_pattern** - Signal completion with the discovered pattern
   - Input: structured asset mapping data
   - Output: None (ends conversation)

### Multi-Turn Loop

```
User: Initial context (releases, README, repo metadata)
  |
  v
Claude: May call fetch_file or inspect_archive
  |
  v
User: Tool results
  |
  v
Claude: May call more tools or extract_pattern
  |
  v
(loop until extract_pattern called or max 5 turns)
```

### Cost Calculation

Based on Claude Sonnet 4 pricing:
- Input: $3 per 1M tokens
- Output: $15 per 1M tokens

```go
func (u Usage) Cost() float64 {
    inputCost := float64(u.InputTokens) * 3.0 / 1_000_000
    outputCost := float64(u.OutputTokens) * 15.0 / 1_000_000
    return inputCost + outputCost
}
```

## Testing Strategy

- **Unit tests**: Cost calculation (deterministic, no external deps)
- **Integration tests**: Skip if `ANTHROPIC_API_KEY` not set
  - Test with mock server for tool use loop
  - Full integration test with real API (optional, for manual verification)
- **Manual verification**: Generate recipe for a known tool like `cli/cli` (gh)

## Risks and Mitigations

- **API key in tests**: Use `t.Skip()` when ANTHROPIC_API_KEY not available
- **SDK breaking changes**: Pin to specific SDK version in go.mod
- **Tool schema validation**: Validate tool outputs against expected schema

## Success Criteria

- [ ] `internal/llm/client.go` with multi-turn conversation loop
- [ ] `internal/llm/cost.go` with usage tracking
- [ ] `internal/llm/tools.go` with tool schemas (JSON schema definitions)
- [ ] Unit tests for cost calculation
- [ ] Integration test with mock server or skip if no API key
- [ ] All tests pass: `go test ./internal/llm/...`
- [ ] Build passes: `go build ./...`
- [ ] Lint passes: `go vet ./...`

## Open Questions

None - the issue specification is clear.
